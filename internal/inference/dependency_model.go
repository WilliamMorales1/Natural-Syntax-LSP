package inference

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"natural-syntax-ls/internal/postag"
	"natural-syntax-ls/internal/tokenizer"

	ort "github.com/yalue/onnxruntime_go"
)

// DependencyModel runs a biaffine dependency-parsing ONNX model (BERT + BiLSTM + biaffine arc/relation scorers; see scripts/export_model.py).
type DependencyModel struct {
	session  *idleSession
	tok      *tokHolder
	rels     []postag.Deprel // index → decoded Deprel, loaded from _dependency_rels.json; index 0 is the unused "<bos>" placeholder
	wordPart int             // index of the "wp" relation of character-level models, else 0
}

// wordPartRel is the relation a character-level model uses to link a unit to the last unit of its word.
const wordPartRel = "wp"

// depRelsPathFor derives the rel-labels JSON path from the model path.
func depRelsPathFor(modelPath string) string {
	return strings.TrimSuffix(modelPath, ".onnx") + "_rels.json"
}

func loadDepRels(relsPath string) (rels []postag.Deprel, wordPart int, err error) {
	data, err := os.ReadFile(relsPath)
	if err != nil {
		return nil, 0, err
	}
	var raw []string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, 0, err
	}
	// unrecognized/placeholder ("<bos>") labels decode as generic "dep"
	return decodeEnumList(raw, postag.ParseDeprel, postag.DepDep), slices.Index(raw, wordPartRel), nil
}

// NewDependencyModel loads the dependency-parsing ONNX model.
func NewDependencyModel(modelPath, vocabPath string, opts ...tokenizer.Option) (*DependencyModel, error) {
	tok, err := initEnvAndTokenizer(vocabPath, opts...)
	if err != nil {
		return nil, err
	}

	rels, wordPart, err := loadDepRels(depRelsPathFor(modelPath))
	if err != nil {
		releaseEnv()
		return nil, fmt.Errorf("load dependency rel labels: %w", err)
	}

	session, err := newIdleSession(modelPath,
		[]string{"input_ids", "attention_mask", "pool_matrix"},
		[]string{"arc_logits", "rel_logits"},
	)
	if err != nil {
		releaseEnv()
		return nil, err
	}

	return &DependencyModel{session: session, tok: newTokHolder(tok, vocabPath), rels: rels, wordPart: max(wordPart, 0)}, nil
}

// Unload frees the model's weights until its next prediction.
func (m *DependencyModel) Unload() { m.session.unload() }

func (m *DependencyModel) loadedSession() bool { return m.session.loaded() }

// Close releases the ORT session and environment.
func (m *DependencyModel) Close() error {
	err := m.session.Destroy()
	releaseEnv()
	return err
}

// UsesDeprel reports that tokens carry dependency relations.
func (*DependencyModel) UsesDeprel() bool { return true }

// poolMatrixInput is the subword stream plus its [seqLen x nSub] mean-pooling matrix.
type poolMatrixInput struct {
	ids, mask []int64
	pool      []float32
	seqLen    int // words including the synthetic root
	nSub      int // subwords
}

// buildPoolMatrix wordpiece-tokenizes words (prepending a synthetic [CLS] root) and returns subword ids/mask plus a [seqLen x nSub] mean-pooling matrix.
func (m *DependencyModel) buildPoolMatrix(words []tokenizer.WordSpan) poolMatrixInput {
	type wordPieces struct {
		start, count int
	}
	pieceIDs := []int64{int64(m.tok.get().CLSID())}
	bounds := []wordPieces{{0, 1}} // root word = [CLS] alone

	for _, w := range words {
		ids := m.tok.get().WordIDs(w.Text)
		start := len(pieceIDs)
		pieceIDs = append(pieceIDs, ids...)
		bounds = append(bounds, wordPieces{start, len(ids)})
	}

	// Truncate to maxSeqLen subwords, dropping any word whose pieces don't fully fit.
	if len(pieceIDs) > maxSeqLen {
		pieceIDs = pieceIDs[:maxSeqLen]
		for len(bounds) > 0 && bounds[len(bounds)-1].start+bounds[len(bounds)-1].count > maxSeqLen {
			bounds = bounds[:len(bounds)-1]
		}
	}

	nSub := len(pieceIDs)
	seqLen := len(bounds)
	mask := slices.Repeat([]int64{1}, nSub)
	pool := make([]float32, seqLen*nSub)
	for wi, b := range bounds {
		if b.count == 0 {
			continue
		}
		weight := 1 / float32(b.count)
		row := wi * nSub
		for i := range b.count {
			pool[row+b.start+i] = weight
		}
	}
	return poolMatrixInput{ids: pieceIDs, mask: mask, pool: pool, seqLen: seqLen, nSub: nSub}
}

// isSentenceEnd reports whether w is a lone Unicode sentence terminal (. ! ? 。 ？ ؟ ۔ ...); the biaffine model was trained one sentence per synthetic root.
func isSentenceEnd(w string) bool {
	r, n := utf8.DecodeRuneInString(w)
	return n == len(w) && unicode.Is(unicode.Sentence_Terminal, r)
}

// isCloser reports whether w is a lone non-ASCII closing bracket or quote that stays with the sentence it closes, as in 「行きます。」 or «да.».
func isCloser(w string) bool {
	r, n := utf8.DecodeRuneInString(w)
	return n == len(w) && r > unicode.MaxASCII && (unicode.Is(unicode.Pe, r) || unicode.Is(unicode.Pf, r))
}

// splitSentences groups words into sentences: markdown blocks, each further split right after any sentence-end token.
func splitSentences(words []tokenizer.WordSpan) [][]tokenizer.WordSpan {
	var sentences [][]tokenizer.WordSpan
	for _, block := range splitBlocks(words) {
		start := 0
		for i, w := range block {
			if i < start || !isSentenceEnd(w.Text) || isOrderedListMarker(block[start:i+1]) {
				continue
			}
			end := i + 1
			for end < len(block) && isCloser(block[end].Text) {
				end++
			}
			sentences = append(sentences, block[start:end])
			start = end
		}
		if start < len(block) {
			sentences = append(sentences, block[start:])
		}
	}
	return sentences
}

// isOrderedListMarker reports whether sent is just the "1." that opens an ordered list item, whose period doesn't end a sentence.
func isOrderedListMarker(sent []tokenizer.WordSpan) bool {
	return len(sent) == 2 && sent[0].BlockStart && sent[1].Text == "." &&
		!strings.ContainsFunc(sent[0].Text, func(r rune) bool { return r < '0' || r > '9' })
}

// PredictChunk parses each sentence in words separately, each under its own synthetic root.
func (m *DependencyModel) PredictChunk(words []tokenizer.WordSpan) ([]postag.Token, error) {
	if len(words) == 0 {
		return nil, nil
	}
	var tokens []postag.Token
	for _, sentence := range splitSentences(words) {
		sentTokens, err := m.predictSentence(sentence)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, sentTokens...)
	}
	return tokens, nil
}

// predictSentence runs the biaffine model on a single sentence (one synthetic root).
func (m *DependencyModel) predictSentence(words []tokenizer.WordSpan) ([]postag.Token, error) {
	if len(words) == 0 {
		return nil, nil
	}
	pm := m.buildPoolMatrix(words)
	ids, mask, pool, seqLen, nSub := pm.ids, pm.mask, pm.pool, pm.seqLen, pm.nSub

	inputIDs, err := ort.NewTensor(ort.NewShape(1, int64(nSub)), ids)
	if err != nil {
		return nil, fmt.Errorf("new inputIDs tensor: %w", err)
	}
	defer inputIDs.Destroy()
	attMask, err := ort.NewTensor(ort.NewShape(1, int64(nSub)), mask)
	if err != nil {
		return nil, fmt.Errorf("new attMask tensor: %w", err)
	}
	defer attMask.Destroy()
	poolMatrix, err := ort.NewTensor(ort.NewShape(1, int64(seqLen), int64(nSub)), pool)
	if err != nil {
		return nil, fmt.Errorf("new poolMatrix tensor: %w", err)
	}
	defer poolMatrix.Destroy()

	arcLogits, err := ort.NewEmptyTensor[float32](ort.NewShape(1, int64(seqLen), int64(seqLen)))
	if err != nil {
		return nil, fmt.Errorf("new arcLogits tensor: %w", err)
	}
	defer arcLogits.Destroy()
	numRels := len(m.rels)
	relLogits, err := ort.NewEmptyTensor[float32](ort.NewShape(1, int64(seqLen), int64(seqLen), int64(numRels)))
	if err != nil {
		return nil, fmt.Errorf("new relLogits tensor: %w", err)
	}
	defer relLogits.Destroy()

	if err := m.session.Run(
		[]ort.Value{inputIDs, attMask, poolMatrix},
		[]ort.Value{arcLogits, relLogits},
	); err != nil {
		return nil, fmt.Errorf("ort run: %w", err)
	}

	arcData := arcLogits.GetData()
	relData := relLogits.GetData()

	// words[wi] corresponds to seq row wi+1 (row 0 is the synthetic root).
	numRealWords := seqLen - 1

	// Character-level models mark a multi-unit word by pointing each unit but the last at the last one with the word-part relation.
	groups := make([][2]int, 0, numRealWords) // inclusive unit index ranges
	for wi := 0; wi < numRealWords; {
		end := wi
		if m.wordPart > 0 {
			row := wi + 1
			head := argmaxFloat32ExceptSelf(arcData[row*seqLen:(row+1)*seqLen], row)
			relRow := relData[(row*seqLen+head)*numRels : (row*seqLen+head+1)*numRels]
			if head > row && slices.Index(relRow[1:], slices.Max(relRow[1:]))+1 == m.wordPart {
				end = head - 1
				for k := wi; k < end; k++ { // a gap in the source cuts the word
					if words[k].End != words[k+1].Begin {
						end = k
						break
					}
				}
			}
		}
		groups = append(groups, [2]int{wi, end})
		wi = end + 1
	}
	groupOf := make([]int, numRealWords)
	for gi, g := range groups {
		for k := g[0]; k <= g[1]; k++ {
			groupOf[k] = gi
		}
	}

	tokens := make([]postag.Token, 0, len(groups))
	for _, g := range groups {
		first, last := words[g[0]], words[g[1]]
		var sb strings.Builder
		sb.WriteString(first.Text)
		for k := g[0] + 1; k <= g[1]; k++ {
			sb.WriteString(words[k].Text)
		}
		text := sb.String()
		if tokenizer.IsAllPunct(text) {
			continue
		}
		row := g[1] + 1 // the last unit carries the word's head and relation
		arcRow := slices.Clone(arcData[row*seqLen : (row+1)*seqLen])
		for k := g[0] + 1; k <= row; k++ { // a word cannot hang off its own units
			arcRow[k] = float32(math.Inf(-1))
		}
		head := argmaxFloat32ExceptSelf(arcRow, row)
		score := float64(softmaxAt(arcRow, head))

		hasHead := head != 0 // row/col 0 is the root sentinel
		var headOffset uint32
		if hasHead {
			if hi := head - 1; hi >= 0 && hi < numRealWords {
				headOffset = words[groups[groupOf[hi]][0]].Begin
			} else {
				hasHead = false
			}
		}

		relBase := (row*seqLen + head) * numRels
		relRow := slices.Clone(relData[relBase : relBase+numRels])
		if m.wordPart > 0 {
			relRow[m.wordPart] = float32(math.Inf(-1))
		}
		relClass := slices.Index(relRow[1:], slices.Max(relRow[1:])) + 1 // skip index 0, the unused "<bos>" placeholder
		var rel postag.Deprel
		if relClass < len(m.rels) { // a rels file holding only "<bos>" leaves no real class
			rel = m.rels[relClass]
		}

		tokens = append(tokens, postag.Token{
			Word:            text,
			Score:           score,
			Tag:             postag.O,
			HasHead:         hasHead,
			HeadOffsetBegin: headOffset,
			Deprel:          rel,
			OffsetBegin:     first.Begin,
			OffsetEnd:       last.End,
			Description:     rel.String(),
		})
	}
	return tokens, nil
}

// argmaxFloat32ExceptSelf is argmax over v, excluding index `self` (a word can't be its own head).
func argmaxFloat32ExceptSelf(v []float32, self int) int {
	best := -1
	for i, x := range v {
		if i == self {
			continue
		}
		if best == -1 || x > v[best] {
			best = i
		}
	}
	if best == -1 {
		return 0
	}
	return best
}

// softmaxAt returns the softmax probability of v[idx] within v, handling -Inf entries (masked self-loops) safely.
func softmaxAt(v []float32, idx int) float32 {
	peak := slices.Max(v)
	var sum float64
	for _, x := range v {
		sum += math.Exp(float64(x - peak))
	}
	if sum == 0 {
		return 0
	}
	return float32(math.Exp(float64(v[idx]-peak)) / sum)
}

// Tokenize splits text using the model's own tokenizer rules.
func (m *DependencyModel) Tokenize(text string) []tokenizer.WordSpan {
	return m.tok.get().Tokenize(text)
}

// ReloadTokenizer rebuilds the tokenizer from its vocab with new options; the session stays loaded.
func (m *DependencyModel) ReloadTokenizer(opts ...tokenizer.Option) error {
	return m.tok.reload(opts...)
}
