package lspserver

import (
	"natural-syntax-ls/internal/postag"
	"natural-syntax-ls/internal/tokenizer"
)

// predictor tags the words of one chunk; the registry never owns it.
type predictor interface {
	PredictChunk(words []tokenizer.WordSpan) ([]postag.Token, error)
	Close() error
}

// textTokenizer is a predictor that dictates its own word segmentation.
type textTokenizer interface {
	Tokenize(text string) []tokenizer.WordSpan
}

// embedder is a predictor whose tokens come with unit embeddings, so they can be recolored per document.
type embedder interface {
	predictor
	EmbedChunk(words []tokenizer.WordSpan) ([]postag.Token, [][]float32, error)
}

// deprelModel marks predictors whose tokens carry dependency relations.
type deprelModel interface {
	UsesDeprel() bool
}
