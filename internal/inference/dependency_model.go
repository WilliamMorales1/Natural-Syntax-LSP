package inference

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"

	"natural-syntax-ls/internal/postag"
	"natural-syntax-ls/internal/tokenizer"

	ort "github.com/yalue/onnxruntime_go"
)

// DependencyModel runs a biaffine dependency-parsing ONNX model (BERT + BiLSTM + biaffine arc/relation scorers; see scripts/export_model.py).
type DependencyModel struct {
	session   *ort.DynamicAdvancedSession
	tokenizer *tokenizer.BERTTokenizer
	rels      []postag.Deprel // index → decoded Deprel, loaded from _dependency_rels.json; index 0 is the unused "<bos>" placeholder
}

// depRelsPathFor derives the rel-labels JSON path from the model path.
func depRelsPathFor(modelPath string) string {
	return strings.TrimSuffix(modelPath, ".onnx") + "_rels.json"
}

func loadDepRels(relsPath string) ([]postag.Deprel, error) {
	data, err := os.ReadFile(relsPath)
	if err != nil {
		return nil, err
	}
	var raw []string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	// unrecognized/placeholder ("<bos>") labels decode as generic "dep"
	return decodeEnumList(raw, postag.ParseDeprel, postag.DepDep), nil
}

// NewDependencyModel loads the dependency-parsing ONNX model.
func NewDependencyModel(modelPath, vocabPath string) (*DependencyModel, error) {
	tok, err := initEnvAndTokenizer(vocabPath)
	if err != nil {
		return nil, err
	}

	rels, err := loadDepRels(depRelsPathFor(modelPath))
	if err != nil {
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("load dependency rel labels: %w", err)
	}

	session, err := newDynamicSession(modelPath,
		[]string{"input_ids", "attention_mask", "pool_matrix"},
		[]string{"arc_logits", "rel_logits"},
	)
	if err != nil {
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create ort session: %w", err)
	}

	return &DependencyModel{session: session, tokenizer: tok, rels: rels}, nil
}

// Close releases the ORT session and environment.
func (m *DependencyModel) Close() {
	m.session.Destroy()
	ort.DestroyEnvironment()
}

// buildPoolMatrix wordpiece-tokenizes words (prepending a synthetic [CLS] root) and returns subword ids/mask plus a [seqLen x nSub] mean-pooling matrix.
func (m *DependencyModel) buildPoolMatrix(words []tokenizer.WordSpan) (ids, mask []int64, pool []float32, seqLen, nSub int) {
	type wordPieces struct {
		start, count int
	}
	pieceIDs := []int64{int64(m.tokenizer.CLSID())}
	bounds := []wordPieces{{0, 1}} // root word = [CLS] alone

	for _, w := range words {
		pieces := m.tokenizer.WordPiece(w.Text)
		start := len(pieceIDs)
		for _, piece := range pieces {
			pieceIDs = append(pieceIDs, int64(m.tokenizer.VocabID(piece)))
		}
		bounds = append(bounds, wordPieces{start, len(pieces)})
	}

	// Truncate to maxSeqLen subwords, dropping any word whose pieces don't fully fit.
	if len(pieceIDs) > maxSeqLen {
		pieceIDs = pieceIDs[:maxSeqLen]
		for len(bounds) > 0 && bounds[len(bounds)-1].start+bounds[len(bounds)-1].count > maxSeqLen {
			bounds = bounds[:len(bounds)-1]
		}
	}

	nSub = len(pieceIDs)
	seqLen = len(bounds)
	mask = slices.Repeat([]int64{1}, nSub)
	pool = make([]float32, seqLen*nSub)
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
	return pieceIDs, mask, pool, seqLen, nSub
}

// sentenceEnders are word texts that end a sentence for chunking purposes (the biaffine model was trained one sentence per synthetic root).
var sentenceEnders = map[string]bool{".": true, "!": true, "?": true}

// splitSentences groups words into sentences: markdown blocks, each further split right after any sentenceEnders token.
func splitSentences(words []tokenizer.WordSpan) [][]tokenizer.WordSpan {
	var sentences [][]tokenizer.WordSpan
	for _, block := range splitBlocks(words) {
		start := 0
		for i, w := range block {
			if sentenceEnders[w.Text] && !isOrderedListMarker(block[start:i+1]) {
				sentences = append(sentences, block[start:i+1])
				start = i + 1
			}
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
	ids, mask, pool, seqLen, nSub := m.buildPoolMatrix(words)

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

	tokens := make([]postag.Token, 0, len(words))
	for wi := range numRealWords {
		w := words[wi]
		if tokenizer.IsAllPunct(w.Text) {
			continue
		}
		row := wi + 1
		arcRow := arcData[row*seqLen : (row+1)*seqLen]
		head := argmaxFloat32ExceptSelf(arcRow, row)
		score := float64(softmaxAt(arcRow, head))

		hasHead := head != 0 // row/col 0 is the root sentinel
		var headOffset uint32
		if hasHead {
			headWordIdx := head - 1
			if headWordIdx >= 0 && headWordIdx < len(words) {
				headOffset = words[headWordIdx].Begin
			} else {
				hasHead = false
			}
		}

		relBase := (row*seqLen + head) * numRels
		relRow := relData[relBase : relBase+numRels]
		relClass := argmax(relRow[1:]) + 1 // skip index 0, the unused "<bos>" placeholder
		var rel postag.Deprel
		if relClass < len(m.rels) { // a rels file holding only "<bos>" leaves no real class
			rel = m.rels[relClass]
		}

		tokens = append(tokens, postag.Token{
			Word:            w.Text,
			Score:           score,
			Tag:             postag.O,
			HasHead:         hasHead,
			HeadOffsetBegin: headOffset,
			Deprel:          rel,
			OffsetBegin:     w.Begin,
			OffsetEnd:       w.End,
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
