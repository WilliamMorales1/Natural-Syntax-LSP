package inference

import (
	"natural-syntax-ls/internal/postag"
	"natural-syntax-ls/internal/tokenizer"
)

// EmbeddingModel runs a sentence-transformer ONNX model, pools per-word embeddings from subwords, and maps them to semantic colors via OKLCH projection.
type EmbeddingModel struct {
	*baseModel
	hiddenSize int
}

// NewEmbeddingModel loads a sentence-transformer model whose last_hidden_state has hiddenSize dims.
func NewEmbeddingModel(modelPath, vocabPath string, hiddenSize int) (*EmbeddingModel, error) {
	base, err := newBaseModel(modelPath, vocabPath, "last_hidden_state", int64(hiddenSize))
	if err != nil {
		return nil, err
	}

	return &EmbeddingModel{baseModel: base, hiddenSize: hiddenSize}, nil
}

// PredictChunk colors each non-punctuation word by its mean-pooled subword embedding.
func (m *EmbeddingModel) PredictChunk(words []tokenizer.WordSpan) ([]postag.Token, error) {
	embeds, err := m.embedChunk(words)
	if err != nil {
		return nil, err
	}
	var tokens []postag.Token
	for i, w := range words {
		if tokenizer.IsAllPunct(w.Text) {
			continue
		}
		color := EmbeddingToColor(l2Normalize(embeds[i]))
		tokens = append(tokens, postag.Token{
			Word:        w.Text,
			Score:       1,
			Tag:         postag.O,
			Color:       color,
			OffsetBegin: w.Begin,
			OffsetEnd:   w.End,
			Description: "Semantic color " + color,
		})
	}
	return tokens, nil
}

func (m *EmbeddingModel) embedChunk(words []tokenizer.WordSpan) ([][]float32, error) {
	hidden, seqLen, swWordIdx, _, err := m.runWords(words)
	if err != nil {
		return nil, err
	}
	hs := m.hiddenSize

	// embeddings start as per-word subword sums and are divided into means below.
	embeddings := make([][]float32, len(words))
	counts := make([]float32, len(words))
	for i := range embeddings {
		embeddings[i] = make([]float32, hs)
	}
	for si := range seqLen {
		wi := swWordIdx[si]
		if wi < 0 {
			continue
		}
		base := si * hs
		if base+hs > len(hidden) {
			break
		}
		for d, h := range hidden[base : base+hs] {
			embeddings[wi][d] += h
		}
		counts[wi]++
	}
	for i, vec := range embeddings {
		if counts[i] > 0 {
			for d := range vec {
				vec[d] /= counts[i]
			}
		}
	}
	return embeddings, nil
}
