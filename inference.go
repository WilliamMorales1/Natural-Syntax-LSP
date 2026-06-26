package main

import (
	"fmt"
	"math"
	"sort"

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
}

func newPOSModel(modelPath, vocabPath string) (*POSModel, error) {
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("init ort env: %w", err)
	}

	tok, err := loadBERTTokenizer(vocabPath)
	if err != nil {
		return nil, fmt.Errorf("load tokenizer: %w", err)
	}

	shape2 := ort.NewShape(1, maxSeqLen)
	shape3 := ort.NewShape(1, maxSeqLen, N_PART_OF_SPEECH)

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

func (m *POSModel) Predict(text string) ([]POSToken, error) {
	ids, mask, tti, words, swWordIdx, swIsFirst := m.tokenizer.Tokenize(text)

	seqLen := len(ids)
	if seqLen > maxSeqLen {
		seqLen = maxSeqLen
		ids = ids[:seqLen]
		mask = mask[:seqLen]
		tti = tti[:seqLen]
	}

	// Zero then fill pre-allocated tensor buffers.
	idBuf := m.inputIDs.GetData()
	maskBuf := m.attMask.GetData()
	ttiBuf := m.tokTypeIDs.GetData()
	for i := range idBuf {
		idBuf[i] = 0
		maskBuf[i] = 0
		ttiBuf[i] = 0
	}
	copy(idBuf, ids)
	copy(maskBuf, mask)
	copy(ttiBuf, tti)

	if err := m.session.Run(); err != nil {
		return nil, fmt.Errorf("ort run: %w", err)
	}

	logits := m.output.GetData()

	// Gather best label per original word from its first subword.
	type best struct {
		label int
		score float64
		valid bool
	}
	wordBest := make([]best, len(words))

	for si := 0; si < seqLen; si++ {
		wi := swWordIdx[si]
		if wi < 0 || !swIsFirst[si] {
			continue
		}
		base := si * N_PART_OF_SPEECH
		if base+N_PART_OF_SPEECH > len(logits) {
			break
		}
		scores := softmax(logits[base : base+N_PART_OF_SPEECH])
		label := argmax(scores)
		wordBest[wi] = best{label: label, score: float64(scores[label]), valid: true}
	}

	tokens := make([]POSToken, 0, len(words))
	for wi, w := range words {
		b := wordBest[wi]
		if !b.valid {
			continue
		}
		tokens = append(tokens, POSToken{
			Word:        w.text,
			Score:       b.score,
			Tag:         PartOfSpeech(b.label),
			OffsetBegin: w.begin,
			OffsetEnd:   w.end,
		})
	}

	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].OffsetBegin < tokens[j].OffsetBegin
	})
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
