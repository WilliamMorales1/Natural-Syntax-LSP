// Package inference runs the ONNX models (POS, dependency, embedding) that back each highlighting mode.
package inference

import (
	"natural-syntax-ls/internal/postag"
	"natural-syntax-ls/internal/tokenizer"

	ort "github.com/yalue/onnxruntime_go"
)

// Predictor abstracts POS tagging, dependency parsing, and semantic embedding modes.
type Predictor interface {
	PredictChunk(words []tokenizer.WordSpan) ([]postag.POSToken, error)
	Close()
}

// SetORTLibPath sets the path to the onnxruntime shared library; must be called before any model is loaded.
func SetORTLibPath(path string) {
	ort.SetSharedLibraryPath(path)
}
