package inference

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"natural-syntax-ls/internal/postag"
	"natural-syntax-ls/internal/tokenizer"
)

// POSModel holds the ONNX session and pre-allocated tensors.
type POSModel struct {
	*baseFixedModel
	labels []postag.PartOfSpeech // index → PartOfSpeech, loaded from _labels.json
}

// labelsPathFor derives the labels JSON path from the model path.
func labelsPathFor(modelPath string) string {
	base := modelPath
	if i := strings.LastIndex(base, "_pos.onnx"); i >= 0 {
		base = base[:i]
	} else {
		base = strings.TrimSuffix(base, ".onnx")
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
	return decodeEnumList(tags, func(s string) (postag.PartOfSpeech, bool) { pos, ok := postag.FromString[s]; return pos, ok }, postag.POS_O), nil
}

func NewPOSModel(modelPath, vocabPath string) (*POSModel, error) {
	labels, err := loadLabels(labelsPathFor(modelPath))
	if err != nil {
		// Fall back to default mobilebert label order if no sidecar found.
		labels = make([]postag.PartOfSpeech, postag.N_PART_OF_SPEECH)
		for i := range labels {
			labels[i] = postag.PartOfSpeech(i)
		}
	}

	base, err := newBaseFixedModel(modelPath, vocabPath, "logits", int64(len(labels)))
	if err != nil {
		return nil, err
	}

	return &POSModel{
		baseFixedModel: base,
		labels:         labels,
	}, nil
}

func (m *POSModel) PredictChunk(words []tokenizer.WordSpan) ([]postag.POSToken, error) {
	seqLen, swWordIdx, swIsFirst, err := m.runWords(words)
	if err != nil {
		return nil, err
	}

	logits := m.output.GetData()
	numLabels := len(m.labels)

	type best struct {
		label int
		score float64
	}
	wordBest := make([]*best, len(words))

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
		wordBest[wi] = &best{label: label, score: float64(scores[label])}
	}

	tokens := make([]postag.POSToken, 0, len(words))
	for wi, w := range words {
		b := wordBest[wi]
		if b == nil {
			continue
		}
		if tokenizer.IsAllPunct(w.Text) {
			continue
		}
		pos := postag.POS_O
		if b.label >= 0 && b.label < numLabels {
			pos = m.labels[b.label]
		}
		tokens = append(tokens, postag.POSToken{
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
	max := logits[0]
	for _, v := range logits[1:] {
		if v > max {
			max = v
		}
	}
	sum := float32(0)
	out := make([]float32, len(logits))
	for i, v := range logits {
		out[i] = float32(math.Exp(float64(v - max)))
		sum += out[i]
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

func argmax(scores []float32) int {
	best := 0
	for i := 1; i < len(scores); i++ {
		if scores[i] > scores[best] {
			best = i
		}
	}
	return best
}
