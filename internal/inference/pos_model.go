package inference

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
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
	cont   []bool                // index → label is an I- chunk label that extends the previous word
	descs  []string              // index → neutral hover wording for Universal labels, empty for Penn
}

// labelsPathFor derives the labels JSON path from the model path.
func labelsPathFor(modelPath string) string {
	base, ok := strings.CutSuffix(modelPath, "_pos.onnx")
	if !ok {
		base = strings.TrimSuffix(modelPath, ".onnx")
	}
	return base + "_labels.json"
}

// loadLabels reads the index→tag sidecar, returning decoded tags and continuation flags.
func loadLabels(labelsPath string) ([]postag.PartOfSpeech, []bool, []string, error) {
	data, err := os.ReadFile(labelsPath)
	if err != nil {
		return nil, nil, nil, err
	}
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, nil, err
	}
	n := len(raw)
	tags := make([]string, n)
	for idxStr, tag := range raw {
		idx, err := strconv.Atoi(idxStr)
		if err != nil || idx < 0 || idx >= n {
			return nil, nil, nil, fmt.Errorf("bad label index %q", idxStr)
		}
		tags[idx] = tag
	}
	cont := make([]bool, n)
	descs := make([]string, n)
	for i, tag := range tags {
		cont[i] = postag.IsContinuation(tag)
		descs[i] = postag.LabelDescription(tag)
	}
	// Unknown labels decode as O, so name them rather than lose the words silently.
	for _, tag := range tags {
		if _, ok := postag.ParseLabel(tag); !ok {
			slog.Warn("unknown POS label decodes as O", "label", tag, "labels", labelsPath)
		}
	}
	return decodeEnumList(tags, postag.ParseLabel, postag.O), cont, descs, nil
}

// NewPOSModel loads a POS model and its labels sidecar, falling back to the default label order.
func NewPOSModel(modelPath, vocabPath string, opts ...tokenizer.Option) (*POSModel, error) {
	labels, cont, descs, err := loadLabels(labelsPathFor(modelPath))
	if err != nil {
		// The default English order is only safe for the bundled English WordPiece models, which export no sidecar tokenizer.
		if _, statErr := os.Stat(tokenizer.SidecarPath(vocabPath)); statErr == nil || !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("load pos labels: %w", err)
		}
		labels = make([]postag.PartOfSpeech, postag.NumPartsOfSpeech)
		for i := range labels {
			labels[i] = postag.PartOfSpeech(i)
		}
	}

	base, err := newBaseModel(modelPath, vocabPath, "logits", int64(len(labels)), opts...)
	if err != nil {
		return nil, err
	}

	return &POSModel{baseModel: base, labels: labels, cont: cont, descs: descs}, nil
}

// PredictChunk tags each non-punctuation word by its first subword's argmax label, running each markdown block as its own sequence.
func (m *POSModel) PredictChunk(words []tokenizer.WordSpan) ([]postag.Token, error) {
	var tokens []postag.Token
	for _, block := range splitBlocks(words) {
		blockTokens, err := m.predictBlock(block)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, blockTokens...)
	}
	return tokens, nil
}

// predictBlock tags one block in a single model run.
func (m *POSModel) predictBlock(words []tokenizer.WordSpan) ([]postag.Token, error) {
	run, err := m.runWords(words)
	if err != nil {
		return nil, err
	}
	logits, seqLen, swWordIdx, swIsFirst := run.out, run.seqLen, run.swWordIdx, run.swIsFirst

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
		label := slices.Index(scores, slices.Max(scores))
		wordBest[wi] = best{label: label, score: float64(scores[label]), ok: true}
	}

	tokens := make([]postag.Token, 0, len(words))
	var conts []bool
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
			Description: m.describe(b.label),
			OffsetBegin: w.Begin,
			OffsetEnd:   w.End,
		})
		conts = append(conts, m.cont != nil && m.cont[b.label])
	}
	return mergeContinuations(tokens, conts), nil
}

// describe returns the neutral wording for a label index, or empty.
func (m *POSModel) describe(label int) string {
	if label < len(m.descs) {
		return m.descs[label]
	}
	return ""
}

// mergeContinuations folds each token flagged as an I- continuation into the touching token before it, so character-level B-/I- taggers yield whole words; the merged token keeps the first tag and the lowest score.
func mergeContinuations(tokens []postag.Token, conts []bool) []postag.Token {
	out := tokens[:0:0]
	for i, t := range tokens {
		if n := len(out); conts[i] && n > 0 && out[n-1].OffsetEnd == t.OffsetBegin {
			prev := &out[n-1]
			prev.Word += t.Word
			prev.OffsetEnd = t.OffsetEnd
			prev.Score = min(prev.Score, t.Score)
			continue
		}
		out = append(out, t)
	}
	return out
}

// softmax normalizes logits in place and returns them.
func softmax(logits []float32) []float32 {
	peak := slices.Max(logits)
	var sum float32
	for i, v := range logits {
		logits[i] = float32(math.Exp(float64(v - peak)))
		sum += logits[i]
	}
	for i := range logits {
		logits[i] /= sum
	}
	return logits
}
