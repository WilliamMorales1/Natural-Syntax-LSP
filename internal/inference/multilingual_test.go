package inference

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"natural-syntax-ls/internal/postag"
	"natural-syntax-ls/internal/tokenizer"
)

// multilingualDir holds exported multilingual models; tests skip without NSLS_ML_DIR.
func multilingualDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("NSLS_ML_DIR")
	if dir == "" {
		t.Skip("NSLS_ML_DIR unset")
	}
	testDataDir(t) // sets the ORT library path
	return dir
}

const udposPrefix = "wietsedv_xlm_roberta_base_ft_udpos28_"

// TestMultilingualPOS tags sample sentences with the XLM-R UPOS models and checks anchor words, mapped onto Penn tags.
func TestMultilingualPOS(t *testing.T) {
	dir := multilingualDir(t)
	type anchor map[string]postag.PartOfSpeech
	for _, tc := range []struct {
		lang, text string
		want       anchor
	}{
		{"es", "El niño comió manzanas rojas en la cocina.", anchor{"El": postag.DT, "niño": postag.NN, "comió": postag.VB, "rojas": postag.JJ, "en": postag.IN}},
		{"es", "¿Cómo estás? María vive en Madrid.", anchor{"María": postag.NNP, "Madrid": postag.NNP, "vive": postag.VB}},
		{"fr", "Le chat noir mange la souris dans le jardin.", anchor{"Le": postag.DT, "chat": postag.NN, "noir": postag.JJ, "mange": postag.VB, "dans": postag.IN}},
		{"fr", "L'ami d'aujourd'hui qu'il a vu est très gentil.", anchor{"ami": postag.NN, "très": postag.RB, "gentil": postag.JJ}},
		{"ja", "私は東京大学で自然言語処理を勉強しています。", anchor{"東京": postag.NNP, "大学": postag.NN, "処理": postag.NN, "を": postag.IN}},
		{"ja", "今日は晴れです。明日は雨かもしれません。", anchor{"明日": postag.NN, "雨": postag.NN, "は": postag.IN, "です": postag.MD}},
		{"zh", "我喜欢在北京大学学习自然语言处理。", anchor{"我": postag.PRP, "喜欢": postag.VB, "大学": postag.NN, "学习": postag.VB}},
		{"zh", "中华人民共和国成立于1949年。", anchor{"中华人民共和国": postag.NNP, "1949": postag.CD, "年": postag.NN}},
	} {
		t.Run(tc.lang+"/"+tc.text, func(t *testing.T) {
			base := filepath.Join(dir, udposPrefix+tc.lang)
			m, err := NewPOSModel(base+".onnx", base+"_tokenizer.json")
			if err != nil {
				t.Skip(err)
			}
			defer m.Close()
			words := m.Tokenize(tc.text)
			toks, err := m.PredictChunk(words)
			if err != nil {
				t.Fatal(err)
			}
			checkTokens(t, words, toks)
			var line []string
			got := map[string]postag.PartOfSpeech{}
			for _, tok := range toks {
				line = append(line, fmt.Sprintf("%s/%s", tok.Word, tok.Tag))
				got[tok.Word] = tok.Tag
			}
			t.Log(strings.Join(line, " "))
			for w, want := range tc.want {
				if got[w] != want {
					t.Errorf("%q tagged %v, want %v", w, got[w], want)
				}
			}
		})
	}
}

// TestMultilingualSemantic checks that word embeddings of one concept sit closer across languages than embeddings of different concepts.
func TestMultilingualSemantic(t *testing.T) {
	dir := multilingualDir(t)
	base := filepath.Join(dir, "sentence_transformers_paraphrase_multilingual_MiniLM_L12_v2")
	InitSemantic(384)
	m, err := NewEmbeddingModel(base+".onnx", base+"_tokenizer.json", 384)
	if err != nil {
		t.Skip(err)
	}
	defer m.Close()

	// Each concept lists, per language, a sentence and the substring of the word that carries the concept.
	type form struct{ text, word string }
	concepts := map[string]map[string]form{
		"cat": {
			"en": {"The cat sleeps on the sofa.", "cat"}, "es": {"El gato duerme en el sofá.", "gato"},
			"fr": {"Le chat dort sur le canapé.", "chat"}, "ja": {"猫はソファで寝ています。", "猫"}, "zh": {"猫在沙发上睡觉。", "猫"},
		},
		"car": {
			"en": {"The car stops on the street.", "car"}, "es": {"El coche para en la calle.", "coche"},
			"fr": {"La voiture s'arrête dans la rue.", "voiture"}, "ja": {"車は道路で止まります。", "車"}, "zh": {"汽车在街上停下。", "汽车"},
		},
	}
	embed := func(f form) []float32 {
		words := m.Tokenize(f.text)
		_, units, err := m.EmbedChunk(words)
		if err != nil {
			t.Fatal(err)
		}
		i := 0
		for _, w := range words {
			if tokenizer.IsAllPunct(w.Text) {
				continue
			}
			if strings.Contains(w.Text, f.word) {
				return units[i]
			}
			i++
		}
		t.Fatalf("no word containing %q in %q", f.word, f.text)
		return nil
	}
	cos := func(a, b []float32) float32 {
		var s float32
		for i := range a {
			s += a[i] * b[i]
		}
		return s
	}
	langs := []string{"en", "es", "fr", "ja", "zh"}
	for _, l1 := range langs {
		for _, l2 := range langs {
			if l1 == l2 {
				continue
			}
			same := cos(embed(concepts["cat"][l1]), embed(concepts["cat"][l2]))
			diff := cos(embed(concepts["cat"][l1]), embed(concepts["car"][l2]))
			t.Logf("cat %s-%s same %.3f diff %.3f", l1, l2, same, diff)
			if same <= diff {
				t.Errorf("cat %s-%s: same-concept %.3f not above cross-concept %.3f", l1, l2, same, diff)
			}
		}
	}
}

// TestMultilingualLongCJK runs a long unspaced Japanese document through chunking and tagging, checking offsets stay exact.
func TestMultilingualLongCJK(t *testing.T) {
	dir := multilingualDir(t)
	base := filepath.Join(dir, udposPrefix+"ja")
	m, err := NewPOSModel(base+".onnx", base+"_tokenizer.json")
	if err != nil {
		t.Skip(err)
	}
	defer m.Close()
	text := strings.Repeat("今日は晴れです。明日は雨かもしれません。\n\n", 120)
	words := m.Tokenize(text)
	chunks := SplitChunks(text, words)
	if len(chunks) < 2 {
		t.Fatalf("got %d chunks for %d words", len(chunks), len(words))
	}
	n := 0
	for _, c := range chunks {
		if len(c) > ChunkSize+1 {
			t.Fatalf("chunk of %d words exceeds ChunkSize", len(c))
		}
		toks, err := m.PredictChunk(c)
		if err != nil {
			t.Fatal(err)
		}
		checkTokens(t, c, toks)
		n += len(toks)
	}
	if n == 0 {
		t.Fatal("no tokens")
	}
}

// TestMultilingualDependency parses sample sentences and checks anchor (head, relation) pairs.
func TestMultilingualDependency(t *testing.T) {
	dir := multilingualDir(t)
	for _, tc := range []struct {
		slug, text string
		want       map[string][2]string // word -> {head word, relation}
	}{
		{"es_ancora_mbert", "El gato negro come pescado fresco.", map[string][2]string{
			"El": {"gato", "det"}, "gato": {"come", "nsubj"}, "negro": {"gato", "amod"}, "pescado": {"come", "obj"}, "fresco": {"pescado", "amod"},
		}},
		{"fr_sequoia_camembert_large", "Le petit chat mange la souris dans le jardin.", map[string][2]string{
			"Le": {"chat", "det"}, "petit": {"chat", "amod"}, "chat": {"mange", "nsubj"}, "souris": {"mange", "obj"}, "jardin": {"mange", "obl"},
		}},
		{"fr_sequoia_camembert_large", "L'ami d'aujourd'hui lit un livre.", map[string][2]string{
			"ami": {"lit", "nsubj"}, "livre": {"lit", "obj"},
		}},
	} {
		t.Run(tc.slug+"/"+tc.text, func(t *testing.T) {
			base := filepath.Join(dir, tc.slug+"_dependency")
			vocab := base + "_vocab.txt"
			if _, err := os.Stat(vocab); err != nil {
				vocab = base + "_tokenizer.json"
			}
			m, err := NewDependencyModel(base+".onnx", vocab)
			if err != nil {
				t.Skip(err)
			}
			defer m.Close()
			words := m.Tokenize(tc.text)
			toks, err := m.PredictChunk(words)
			if err != nil {
				t.Fatal(err)
			}
			checkTokens(t, words, toks)
			byOffset := map[uint32]string{}
			for _, w := range words {
				byOffset[w.Begin] = w.Text
			}
			var line []string
			got := map[string][2]string{}
			for _, tok := range toks {
				head := "ROOT"
				if tok.HasHead {
					head = byOffset[tok.HeadOffsetBegin]
				}
				got[tok.Word] = [2]string{head, tok.Deprel.String()}
				line = append(line, fmt.Sprintf("%s<-%s(%s)", tok.Word, head, tok.Deprel))
			}
			t.Log(strings.Join(line, " "))
			for w, want := range tc.want {
				if got[w] != want {
					t.Errorf("%q: got %v, want %v", w, got[w], want)
				}
			}
		})
	}
}
