package inference

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"

	"natural-syntax-ls/internal/postag"
	"natural-syntax-ls/internal/tokenizer"
)

// POSModel tags each word with a Penn Treebank POS from a BERT token-classification model.
type POSModel struct {
	*baseModel
	labels []postag.PartOfSpeech // index → PartOfSpeech, loaded from _labels.json
}

// labelsPathFor derives the labels JSON path from the model path.
func labelsPathFor(modelPath string) string {
	base, ok := strings.CutSuffix(modelPath, "_pos.onnx")
	if !ok {
		base = strings.TrimSuffix(modelPath, ".onnx")
	}
	return base + "_labels.json"
}

func loadLabels(labelsPath string) ([]postag.PartOfSpeech, error) {
	data, err := os.ReadFile(labelsPath)
	if err != nil {
		return nil, err
	}
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	n := len(raw)
	tags := make([]string, n)
	for idxStr, tag := range raw {
		idx, err := strconv.Atoi(idxStr)
		if err != nil || idx < 0 || idx >= n {
			return nil, fmt.Errorf("bad label index %q", idxStr)
		}
		tags[idx] = tag
	}
	// unknown tags treated as O
	return decodeEnumList(tags, postag.ParsePartOfSpeech, postag.O), nil
}

// NewPOSModel loads a POS model and its labels sidecar, falling back to the default label order.
func NewPOSModel(modelPath, vocabPath string) (*POSModel, error) {
	labels, err := loadLabels(labelsPathFor(modelPath))
	if err != nil {
		// Fall back to default mobilebert label order if no sidecar found.
		labels = make([]postag.PartOfSpeech, postag.NumPartsOfSpeech)
		for i := range labels {
			labels[i] = postag.PartOfSpeech(i)
		}
	}

	base, err := newBaseModel(modelPath, vocabPath, "logits", int64(len(labels)))
	if err != nil {
		return nil, err
	}

	return &POSModel{baseModel: base, labels: labels}, nil
}

// PredictChunk tags each non-punctuation word by its first subword's argmax label.
func (m *POSModel) PredictChunk(words []tokenizer.WordSpan) ([]postag.Token, error) {
	logits, seqLen, swWordIdx, swIsFirst, err := m.runWords(words)
	if err != nil {
		return nil, err
	}

	numLabels := len(m.labels)

	type best struct {
		label int
		score float64
		ok    bool
	}
	wordBest := make([]best, len(words))

	for si := range seqLen {
		wi := swWordIdx[si]
		if wi < 0 || !swIsFirst[si] {
			continue
		}
		base := si * numLabels
		if base+numLabels > len(logits) {
			break
		}
		scores := softmax(logits[base : base+numLabels])
		label := argmax(scores)
		wordBest[wi] = best{label: label, score: float64(scores[label]), ok: true}
	}

	tokens := make([]postag.Token, 0, len(words))
	for wi, w := range words {
		b := wordBest[wi]
		if !b.ok || tokenizer.IsAllPunct(w.Text) {
			continue
		}
		pos := m.labels[b.label]
		tokens = append(tokens, postag.Token{
			Word:        w.Text,
			Score:       b.score,
			Tag:         pos,
			OffsetBegin: w.Begin,
			OffsetEnd:   w.End,
		})
	}
	return tokens, nil
}

func softmax(logits []float32) []float32 {
	peak := slices.Max(logits)
	var sum float32
	out := make([]float32, len(logits))
	for i, v := range logits {
		out[i] = float32(math.Exp(float64(v - peak)))
		sum += out[i]
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

func argmax(scores []float32) int {
	best := 0
	for i, s := range scores {
		if s > scores[best] {
			best = i
		}
	}
	return best
}
