// Package inference runs the ONNX models (POS, dependency, embedding) that back each highlighting mode.
package inference

import (
	"natural-syntax-ls/internal/postag"
	"natural-syntax-ls/internal/tokenizer"

	ort "github.com/yalue/onnxruntime_go"
)

// Predictor abstracts POS tagging, dependency parsing, and semantic embedding modes.
type Predictor interface {
	PredictChunk(words []tokenizer.WordSpan) ([]postag.Token, error)
	Close()
}

// SetORTLibPath sets the path to the onnxruntime shared library; must be called before any model is loaded.
func SetORTLibPath(path string) {
	ort.SetSharedLibraryPath(path)
}

// SplitChunks partitions words into inference chunks of at most ChunkSize words, breaking only at paragraph breaks and sentence ends so a word's syntactic context always shares its chunk.
func SplitChunks(text string, words []tokenizer.WordSpan) [][]tokenizer.WordSpan {
	runes := []rune(text)
	var chunks [][]tokenizer.WordSpan
	for _, para := range splitParagraphs(runes, words) {
		var cur []tokenizer.WordSpan
		for _, sent := range splitSentences(para) {
			if len(cur) > 0 && len(cur)+len(sent) > ChunkSize {
				chunks = append(chunks, cur)
				cur = nil
			}
			// A sentence longer than ChunkSize has no safe break, so hard-split it.
			for len(sent) > ChunkSize {
				chunks = append(chunks, sent[:ChunkSize])
				sent = sent[ChunkSize:]
			}
			cur = append(cur, sent...)
		}
		if len(cur) > 0 {
			chunks = append(chunks, cur)
		}
	}
	return chunks
}

// splitParagraphs groups words into paragraphs, breaking where the gap between two words holds a blank line.
func splitParagraphs(runes []rune, words []tokenizer.WordSpan) [][]tokenizer.WordSpan {
	var paras [][]tokenizer.WordSpan
	start := 0
	for i := 1; i < len(words); i++ {
		newlines := 0
		for _, r := range runes[words[i-1].End:words[i].Begin] {
			if r == '\n' {
				newlines++
			}
		}
		if newlines >= 2 {
			paras = append(paras, words[start:i])
			start = i
		}
	}
	if start < len(words) {
		paras = append(paras, words[start:])
	}
	return paras
}
