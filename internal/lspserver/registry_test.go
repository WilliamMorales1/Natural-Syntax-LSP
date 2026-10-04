package lspserver

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"natural-syntax-ls/internal/inference"
	"natural-syntax-ls/internal/postag"
	"natural-syntax-ls/internal/tokenizer"
)

// contextModel tags every word from a hash of its whole chunk, so any stale chunk reuse shows up as a tag mismatch.
type contextModel struct{}

func (contextModel) PredictChunk(words []tokenizer.WordSpan) ([]postag.Token, error) {
	h := fnv.New32a()
	for _, w := range words {
		h.Write([]byte(w.Text + "\x00"))
	}
	seed := h.Sum32()
	toks := make([]postag.Token, 0, len(words))
	for i, w := range words {
		toks = append(toks, postag.Token{
			Word:        w.Text,
			Score:       1,
			Tag:         postag.PartOfSpeech((seed + uint32(i)) % postag.NumPartsOfSpeech),
			OffsetBegin: w.Begin,
			OffsetEnd:   w.End,
		})
	}
	return toks, nil
}

func (contextModel) Close() {}

const testURI = "file:///t.txt"

// syncTokens feeds text as the next version and blocks until its semantic tokens are ready.
func syncTokens(reg *documentRegistry, text string, version int32) []uint32 {
	return syncTokensURI(reg, testURI, text, version)
}

func hoverAt(t *testing.T, reg *documentRegistry, line, char uint32) *hoverQueryResult {
	t.Helper()
	reply := make(chan *hoverQueryResult, 1)
	reg.hover(testURI, line, char, reply)
	res := <-reply
	if res == nil || res.tok == nil {
		t.Fatalf("no token at %d:%d", line, char)
	}
	return res
}

func TestIncrementalMatchesFresh(t *testing.T) {
	vocab := strings.Fields("change the state of a system we run fast and slow it")
	enders := []string{".", "!", "?", "\n\n", " ", " ", " ", " "}
	rng := rand.New(rand.NewPCG(1, 2))
	var words []string
	for range 600 {
		words = append(words, vocab[rng.IntN(len(vocab))], enders[rng.IntN(len(enders))])
	}

	inc := newDocumentRegistry(contextModel{})
	inc.workers = 3
	for v := range int32(200) {
		i := rng.IntN(len(words)/2) * 2
		switch rng.IntN(3) {
		case 0:
			words = slices.Insert(words, i, vocab[rng.IntN(len(vocab))], " ")
		case 1:
			words = slices.Delete(words, i, i+2)
		default:
			words[i+1] = enders[rng.IntN(len(enders))]
		}
		text := strings.Join(words, "")
		got := syncTokens(inc, text, v)
		want := syncTokens(newDocumentRegistry(contextModel{}), text, 0)
		if !slices.Equal(got, want) {
			t.Fatalf("version %d: incremental tokens differ from fresh run", v)
		}
	}
}

// dependentWords lists the words of a hover result's dependents.
func dependentWords(r *hoverQueryResult) []string {
	var out []string
	for _, d := range r.dependents {
		out = append(out, d.Word)
	}
	return out
}

// TestEditRetagsAcrossOldChunkBoundary puts "change" at the last slot of an old fixed 200-word window, then inserts "of" after it, in every mode whose model is installed.
func TestEditRetagsAcrossOldChunkBoundary(t *testing.T) {
	cfg, _ := os.UserConfigDir()
	dir := filepath.Join(cfg, "natural-syntax-ls")
	lib := filepath.Join(dir, "libonnxruntime.so")
	if _, err := os.Stat(lib); err != nil {
		t.Skipf("missing %s", lib)
	}
	inference.SetORTLibPath(lib)

	// 49 four-token sentences fill words 0-195, so "Consider this:" puts "change" at word 199.
	filler := strings.Repeat("The dog runs. ", 49) + "Consider this: "
	before := filler + "change the state."
	after := filler + "change of the state."
	col := uint32(len([]rune(filler)))
	if w := tokenizer.BasicTokenize(before); w[inference.ChunkSize-1].Text != "change" {
		t.Fatalf("setup: word %d is %q, want change", inference.ChunkSize-1, w[inference.ChunkSize-1].Text)
	}

	modes := []struct {
		name  string
		load  func() (inference.Predictor, error)
		check func(t *testing.T, before, after *hoverQueryResult)
	}{
		{"pos", func() (inference.Predictor, error) {
			return inference.NewPOSModel(filepath.Join(dir, "bert_base.onnx"), filepath.Join(dir, "bert_base_vocab.txt"))
		}, func(t *testing.T, b, a *hoverQueryResult) {
			if b.tok.Tag == a.tok.Tag {
				t.Errorf("change kept tag %v", b.tok.Tag)
			}
			t.Logf("change: %v -> %v", b.tok.Tag, a.tok.Tag)
		}},
		{"semantic", func() (inference.Predictor, error) {
			inference.InitSemantic(768)
			return inference.NewEmbeddingModel(filepath.Join(dir, "mpnet.onnx"), filepath.Join(dir, "mpnet_vocab.txt"), 768)
		}, func(t *testing.T, b, a *hoverQueryResult) {
			// Re-embedding is checked on the stored embedding: a sub-visible shift can snap to the same color.
			t.Logf("change: %s -> %s", b.tok.Color, a.tok.Color)
		}},
		{"dependency", func() (inference.Predictor, error) {
			return inference.NewDependencyModel(filepath.Join(dir, "en_ewt_electra_base_dependency.onnx"), filepath.Join(dir, "en_ewt_electra_base_dependency_vocab.txt"))
		}, func(t *testing.T, b, a *hoverQueryResult) {
			if !slices.Contains(dependentWords(a), "state") {
				t.Errorf("state not a dependent of change: %v", dependentWords(a))
			}
			t.Logf("change: %v deps %v -> %v deps %v", b.tok.Deprel, dependentWords(b), a.tok.Deprel, dependentWords(a))
		}},
	}
	for _, md := range modes {
		t.Run(md.name, func(t *testing.T) {
			m, err := md.load()
			if err != nil {
				t.Skip(err)
			}
			defer m.Close()

			inc := newDocumentRegistry(m)
			syncTokens(inc, before, 1)
			hb := hoverAt(t, inc, 0, col)
			eb := embeddingAt(inc, col)
			got := syncTokens(inc, after, 2)
			ha := hoverAt(t, inc, 0, col)
			if md.name == "semantic" && slices.Equal(eb, embeddingAt(inc, col)) {
				t.Error("change kept its embedding")
			}

			fresh := newDocumentRegistry(m)
			want := syncTokens(fresh, after, 1)
			if !slices.Equal(got, want) {
				t.Errorf("incremental tokens differ from fresh run")
			}
			hf := hoverAt(t, fresh, 0, col)
			// Semantic colors depend on edit history (the plane is refit from its previous fit), so only the other fields must match a fresh run.
			if ha.tok.Tag != hf.tok.Tag || (md.name != "semantic" && ha.tok.Color != hf.tok.Color) || ha.tok.Deprel != hf.tok.Deprel || !slices.Equal(dependentWords(ha), dependentWords(hf)) {
				t.Errorf("change: incremental %+v, fresh %+v", *ha.tok, *hf.tok)
			}
			md.check(t, hb, ha)
		})
	}
}

// countingModel is contextModel plus a count of PredictChunk calls, to prove unchanged chunks are reused.
type countingModel struct {
	contextModel
	calls atomic.Int64
}

func (m *countingModel) PredictChunk(words []tokenizer.WordSpan) ([]postag.Token, error) {
	m.calls.Add(1)
	return m.contextModel.PredictChunk(words)
}

func syncTokensURI(reg *documentRegistry, uri, text string, version int32) []uint32 {
	reg.update(&textItem{uri: uri, text: text, version: version})
	reply := make(chan []uint32, 1)
	reg.semanticTokens(uri, reply)
	return <-reply
}

func TestRegistryRepredictsOnlyChangedChunks(t *testing.T) {
	paras := []string{"First para here.", "Second one now.", "Third block of text.", "Fourth and so on.", "Fifth and last."}
	doc := func(ps []string) string { return strings.Join(ps, "\n\n") }
	m := &countingModel{}
	reg := newDocumentRegistry(m)
	reg.workers = 2

	steps := []struct {
		name  string
		paras []string
		calls int64
	}{
		{"open", paras, 5},
		{"edit one paragraph", []string{paras[0], paras[1], "Third block of edited text.", paras[3], paras[4]}, 1},
		{"insert paragraph at top", []string{"Brand new intro.", paras[0], paras[1], "Third block of edited text.", paras[3], paras[4]}, 1},
		{"delete a paragraph", []string{"Brand new intro.", paras[0], "Third block of edited text.", paras[3], paras[4]}, 0},
		{"move a paragraph", []string{paras[4], "Brand new intro.", paras[0], "Third block of edited text.", paras[3]}, 0},
		{"merge two paragraphs", []string{paras[4], "Brand new intro.\n" + paras[0], "Third block of edited text.", paras[3]}, 1},
		{"duplicate a paragraph", []string{paras[4], paras[4], paras[4]}, 0},
	}
	for v, st := range steps {
		before := m.calls.Load()
		got := syncTokens(reg, doc(st.paras), int32(v))
		if calls := m.calls.Load() - before; calls != st.calls {
			t.Errorf("%s: %d chunk predictions, want %d", st.name, calls, st.calls)
		}
		if want := syncTokens(newDocumentRegistry(contextModel{}), doc(st.paras), 0); !slices.Equal(got, want) {
			t.Errorf("%s: tokens differ from fresh run", st.name)
		}
	}
}

func TestRegistryIgnoresStaleVersion(t *testing.T) {
	reg := newDocumentRegistry(contextModel{})
	want := syncTokens(reg, "new text here.", 5)
	if got := syncTokens(reg, "old text from before the edit.", 3); !slices.Equal(got, want) {
		t.Error("older version replaced newer document")
	}
}

func TestRegistryDiscard(t *testing.T) {
	reg := newDocumentRegistry(contextModel{})
	syncTokens(reg, "some words.", 1)
	reg.discard(testURI)
	reply := make(chan *hoverQueryResult, 1)
	reg.hover(testURI, 0, 0, reply)
	if res := <-reply; res != nil {
		t.Errorf("hover after close: %+v", res.tok)
	}
	// Reopening at version 1 must work again after close.
	if got := syncTokens(reg, "some words.", 1); len(got) == 0 {
		t.Error("reopened document has no tokens")
	}
}

func TestRegistryHoverMisses(t *testing.T) {
	reg := newDocumentRegistry(contextModel{})
	syncTokens(reg, "one two\n\nthree", 1)
	for _, pos := range [][2]uint32{{0, 3}, {1, 0}, {9, 0}, {0, 99}} {
		reply := make(chan *hoverQueryResult, 1)
		reg.hover(testURI, pos[0], pos[1], reply)
		if res := <-reply; res != nil {
			t.Errorf("hover at %v hit %q", pos, res.tok.Word)
		}
	}
	reply := make(chan *hoverQueryResult, 1)
	reg.hover("file:///never-opened", 0, 0, reply)
	if <-reply != nil {
		t.Error("hover on unknown document returned a token")
	}
}

// TestRegistryManyDocuments edits several documents at once with parallel chunk workers; each must match a fresh run.
func TestRegistryManyDocuments(t *testing.T) {
	reg := newDocumentRegistry(contextModel{})
	reg.workers = 4
	words := strings.Fields("the a dog cat runs sat . ! \n\n")
	var wg sync.WaitGroup
	for d := range 6 {
		wg.Go(func() {
			uri := fmt.Sprintf("file:///doc%d.txt", d)
			rng := rand.New(rand.NewPCG(9, uint64(d)))
			var text string
			for v := range int32(20) {
				var sb strings.Builder
				for range 50 + d*40 {
					sb.WriteString(words[rng.IntN(len(words))])
					sb.WriteByte(' ')
				}
				text = sb.String()
				syncTokensURI(reg, uri, text, v)
			}
			got := syncTokensURI(reg, uri, text, 99)
			if want := syncTokens(newDocumentRegistry(contextModel{}), text, 0); !slices.Equal(got, want) {
				t.Errorf("%s differs from fresh run", uri)
			}
		})
	}
	wg.Wait()
}

const clusterDim = 16

// clusterModel embeds each word near one of four cluster centers spanning e0/e1, so a document has a clear principal plane.
type clusterModel struct{ contextModel }

func (m clusterModel) EmbedChunk(words []tokenizer.WordSpan) ([]postag.Token, [][]float32, error) {
	toks, _ := m.PredictChunk(words)
	embeds := make([][]float32, len(toks))
	for i, t := range toks {
		h := fnv.New64a()
		h.Write([]byte(t.Word))
		rng := rand.New(rand.NewPCG(h.Sum64(), 0))
		v := make([]float32, clusterDim)
		for k := range v {
			v[k] = 0.2 * float32(rng.NormFloat64())
		}
		v[int(h.Sum64()%2)] += []float32{-3, 3}[h.Sum64()/2%2]
		var n float32
		for _, x := range v {
			n += x * x
		}
		for k := range v {
			v[k] /= float32(math.Sqrt(float64(n)))
		}
		toks[i].Color = inference.EmbeddingToColor(v)
		embeds[i] = v
	}
	return toks, embeds, nil
}

func storeTokens(reg *documentRegistry, uri string) []postag.Token {
	reply := make(chan []postag.Token, 1)
	reg.do(func() { reply <- slices.Clone(reg.stores[uri].doc.tokens) })
	return <-reply
}

func hexDist(a, b string) int {
	d := 0
	for i := 1; i < 7; i += 2 {
		x, _ := strconv.ParseUint(a[i:i+2], 16, 8)
		y, _ := strconv.ParseUint(b[i:i+2], 16, 8)
		d = max(d, int(x)-int(y), int(y)-int(x))
	}
	return d
}

func TestRegistrySemanticColorsStableAcrossEdits(t *testing.T) {
	inference.InitSemantic(clusterDim)
	vocab := strings.Fields("dog cat bird fish tree rock river cloud king queen run jump red blue seven twelve")
	rng := rand.New(rand.NewPCG(7, 8))
	var sb strings.Builder
	for i := range 300 {
		sb.WriteString(vocab[rng.IntN(len(vocab))])
		if i%12 == 11 {
			sb.WriteString(". ")
		} else {
			sb.WriteByte(' ')
		}
	}
	text := sb.String()

	reg := newDocumentRegistry(clusterModel{})
	syncTokens(reg, text, 1)
	before := storeTokens(reg, testURI)
	for _, tok := range before {
		if tok.Color == "" || tok.Description != "Semantic color "+tok.Color {
			t.Fatalf("token %q: color %q, description %q", tok.Word, tok.Color, tok.Description)
		}
	}
	if fixed := (clusterModel{}).fixedColors(text); slices.Equal(colorsOf(before), fixed) {
		t.Error("colors still come from the fixed plane")
	}

	syncTokens(reg, text+"river cloud king jump red seven fish dog.", 2)
	after := storeTokens(reg, testURI)
	moved := 0
	for i, tok := range before {
		if hexDist(tok.Color, after[i].Color) > 8 {
			moved++
		}
	}
	if moved > len(before)/10 {
		t.Errorf("%d of %d existing words changed color after appending a sentence", moved, len(before))
	}
}

func (m clusterModel) fixedColors(text string) []string {
	toks, _, _ := m.EmbedChunk(tokenizer.BasicTokenize(text))
	return colorsOf(toks)
}

func colorsOf(toks []postag.Token) []string {
	out := make([]string, len(toks))
	for i, t := range toks {
		out[i] = t.Color
	}
	return out
}

// embeddingAt returns a copy of the stored embedding of the token starting at offset, or nil.
func embeddingAt(reg *documentRegistry, offset uint32) []float32 {
	reply := make(chan []float32, 1)
	reg.do(func() {
		for _, cr := range reg.stores[testURI].chunkResults {
			for i, tok := range cr.tokens {
				if tok.OffsetBegin == offset && i < len(cr.embeds) {
					reply <- slices.Clone(cr.embeds[i])
					return
				}
			}
		}
		reply <- nil
	})
	return <-reply
}
