package inference

import (
	"fmt"

	"natural-syntax-ls/internal/postag"
	"natural-syntax-ls/internal/tokenizer"
)

// EmbeddingModel runs a sentence-transformer ONNX model, pools per-word embeddings from subwords, and maps them to semantic colors via OKLCH projection.
type EmbeddingModel struct {
	*baseFixedModel
	hiddenSize int
}

func NewEmbeddingModel(modelPath, vocabPath string, hiddenSize int) (*EmbeddingModel, error) {
	base, err := newBaseFixedModel(modelPath, vocabPath, "last_hidden_state", int64(hiddenSize))
	if err != nil {
		return nil, err
	}

	return &EmbeddingModel{
		baseFixedModel: base,
		hiddenSize:     hiddenSize,
	}, nil
}

func (m *EmbeddingModel) PredictChunk(words []tokenizer.WordSpan) ([]postag.POSToken, error) {
	embeds, err := m.embedChunk(words)
	if err != nil {
		return nil, err
	}
	var tokens []postag.POSToken
	for i, w := range words {
		if tokenizer.IsAllPunct(w.Text) {
			continue
		}
		v := l2Normalize(embeds[i])
		color := EmbeddingToColor(v)
		tokens = append(tokens, postag.POSToken{
			Word:        w.Text,
			Score:       1.0,
			Tag:         postag.POS_O,
			Color:       color,
			OffsetBegin: w.Begin,
			OffsetEnd:   w.End,
			Description: fmt.Sprintf("Semantic color %s", color),
		})
	}
	return tokens, nil
}

func (m *EmbeddingModel) embedChunk(words []tokenizer.WordSpan) ([][]float32, error) {
	seqLen, swWordIdx, _, err := m.runWords(words)
	if err != nil {
		return nil, err
	}

	hidden := m.output.GetData() // flat [1 * maxSeqLen * hiddenSize]
	hs := m.hiddenSize
	maskBuf := m.attMask.GetData()

	// Attention-mask-weighted pool: subwords with mask=0 (padding) contribute nothing.
	sums := make([][]float32, len(words))
	weights := make([]float32, len(words))
	for i := range sums {
		sums[i] = make([]float32, hs)
	}
	for si := range seqLen {
		wi := swWordIdx[si]
		if wi < 0 {
			continue
		}
		w := float32(maskBuf[si])
		if w == 0 {
			continue
		}
		base := si * hs
		if base+hs > len(hidden) {
			break
		}
		for d := range hs {
			sums[wi][d] += w * hidden[base+d]
		}
		weights[wi] += w
	}
	embeddings := make([][]float32, len(words))
	for i := range words {
		if weights[i] > 0 {
			vec := make([]float32, hs)
			for d := range hs {
				vec[d] = sums[i][d] / weights[i]
			}
			embeddings[i] = vec
		} else {
			embeddings[i] = make([]float32, hs)
		}
	}
	return embeddings, nil
}
