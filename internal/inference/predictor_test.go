package inference

import (
	"natural-syntax-ls/internal/postag"
	"natural-syntax-ls/internal/tokenizer"
)

// Predictor is the surface the model tests exercise.
type Predictor interface {
	PredictChunk(words []tokenizer.WordSpan) ([]postag.Token, error)
	Close() error
}
