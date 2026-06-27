package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	ort "github.com/yalue/onnxruntime_go"
)

func setORTLibPath(path string) {
	ort.SetSharedLibraryPath(path)
}

const maxSeqLen = 512

// POSModel holds the ONNX session and pre-allocated tensors.
type POSModel struct {
	session    *ort.AdvancedSession
	tokenizer  *BERTTokenizer
	inputIDs   *ort.Tensor[int64]
	attMask    *ort.Tensor[int64]
	tokTypeIDs *ort.Tensor[int64]
	output     *ort.Tensor[float32]
	labels     []PartOfSpeech // index → PartOfSpeech, loaded from _labels.json
}

// labelsPathFor derives the labels JSON path from the model path.
// e.g. mobilebert_pos.onnx → mobilebert_labels.json
func labelsPathFor(modelPath string) string {
	base := modelPath
	if i := strings.LastIndex(base, "_pos.onnx"); i >= 0 {
		base = base[:i]
	} else {
		base = strings.TrimSuffix(base, ".onnx")
	}
	return base + "_labels.json"
}

func loadLabels(labelsPath string) ([]PartOfSpeech, error) {
	data, err := os.ReadFile(labelsPath)
	if err != nil {
		return nil, err
	}
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	n := len(raw)
	labels := make([]PartOfSpeech, n)
	for idxStr, tag := range raw {
		idx, err := strconv.Atoi(idxStr)
		if err != nil || idx < 0 || idx >= n {
			return nil, fmt.Errorf("bad label index %q", idxStr)
		}
		pos, ok := posFromString[tag]
		if !ok {
			pos = POS_O // unknown tags treated as O
		}
		labels[idx] = pos
	}
	return labels, nil
}

func newPOSModel(modelPath, vocabPath string) (*POSModel, error) {
	if err := ort.InitializeEnvironment(ort.WithLogLevelError()); err != nil {
		return nil, fmt.Errorf("init ort env: %w", err)
	}

	tok, err := loadBERTTokenizer(vocabPath)
	if err != nil {
		return nil, fmt.Errorf("load tokenizer: %w", err)
	}

	labels, err := loadLabels(labelsPathFor(modelPath))
	if err != nil {
		// Fall back to default mobilebert label order if no sidecar found.
		labels = make([]PartOfSpeech, N_PART_OF_SPEECH)
		for i := range labels {
			labels[i] = PartOfSpeech(i)
		}
	}
	numLabels := int64(len(labels))

	shape2 := ort.NewShape(1, maxSeqLen)
	shape3 := ort.NewShape(1, maxSeqLen, numLabels)

	inputIDs, err := ort.NewTensor(shape2, make([]int64, maxSeqLen))
	if err != nil {
		return nil, fmt.Errorf("new inputIDs tensor: %w", err)
	}
	attMask, err := ort.NewTensor(shape2, make([]int64, maxSeqLen))
	if err != nil {
		return nil, fmt.Errorf("new attMask tensor: %w", err)
	}
	tokTypeIDs, err := ort.NewTensor(shape2, make([]int64, maxSeqLen))
	if err != nil {
		return nil, fmt.Errorf("new tokTypeIDs tensor: %w", err)
	}
	output, err := ort.NewEmptyTensor[float32](shape3)
	if err != nil {
		return nil, fmt.Errorf("new output tensor: %w", err)
	}

	session, err := ort.NewAdvancedSession(modelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"logits"},
		[]ort.ArbitraryTensor{inputIDs, attMask, tokTypeIDs},
		[]ort.ArbitraryTensor{output},
		nil,
	)
	if err != nil {
		inputIDs.Destroy()
		attMask.Destroy()
		tokTypeIDs.Destroy()
		output.Destroy()
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create ort session: %w", err)
	}

	return &POSModel{
		session:    session,
		tokenizer:  tok,
		inputIDs:   inputIDs,
		attMask:    attMask,
		tokTypeIDs: tokTypeIDs,
		output:     output,
		labels:     labels,
	}, nil
}

func (m *POSModel) Close() {
	m.session.Destroy()
	m.inputIDs.Destroy()
	m.attMask.Destroy()
	m.tokTypeIDs.Destroy()
	m.output.Destroy()
	ort.DestroyEnvironment()
}

// chunkSize is the max words per inference chunk. Conservative to stay under
// maxSeqLen even with aggressive subword splitting (~2 subwords/word average).
const chunkSize = 200

func (m *POSModel) Predict(text string) ([]POSToken, error) {
	words := basicTokenize(text)
	if len(words) == 0 {
		return nil, nil
	}

	var tokens []POSToken
	for start := 0; start < len(words); start += chunkSize {
		end := min(start+chunkSize, len(words))
		chunk, err := m.predictChunk(words[start:end])
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, chunk...)
	}

	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].OffsetBegin < tokens[j].OffsetBegin
	})
	return tokens, nil
}

func (m *POSModel) predictChunk(words []wordSpan) ([]POSToken, error) {
	ids, mask, tti, swWordIdx, swIsFirst := m.tokenizer.tokenizeWords(words)

	seqLen := min(len(ids), maxSeqLen)

	idBuf := m.inputIDs.GetData()
	maskBuf := m.attMask.GetData()
	ttiBuf := m.tokTypeIDs.GetData()
	for i := range idBuf {
		idBuf[i] = 0
		maskBuf[i] = 0
		ttiBuf[i] = 0
	}
	copy(idBuf, ids[:seqLen])
	copy(maskBuf, mask[:seqLen])
	copy(ttiBuf, tti[:seqLen])

	if err := m.session.Run(); err != nil {
		return nil, fmt.Errorf("ort run: %w", err)
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

	tokens := make([]POSToken, 0, len(words))
	for wi, w := range words {
		b := wordBest[wi]
		if b == nil {
			continue
		}
		pos := POS_O
		if b.label >= 0 && b.label < numLabels {
			pos = m.labels[b.label]
		}
		tokens = append(tokens, POSToken{
			Word:        w.text,
			Score:       b.score,
			Tag:         pos,
			OffsetBegin: w.begin,
			OffsetEnd:   w.end,
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
