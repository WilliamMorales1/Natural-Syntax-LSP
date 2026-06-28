package main

import (
	"fmt"
	"sort"

	ort "github.com/yalue/onnxruntime_go"
)

// embHiddenSize matches all-MiniLM-L6-v2's hidden dimension.
const embHiddenSize = 384

// EmbeddingModel runs all-MiniLM-L6-v2 via ONNX, extracts per-word contextual
// embeddings (mean-pooled over subwords), and hashes them to semantic color
// buckets via locality-sensitive hashing (embeddingBucket).
type EmbeddingModel struct {
	session    *ort.AdvancedSession
	tokenizer  *BERTTokenizer
	inputIDs   *ort.Tensor[int64]
	attMask    *ort.Tensor[int64]
	tokTypeIDs *ort.Tensor[int64]
	output     *ort.Tensor[float32] // shape [1, maxSeqLen, embHiddenSize]
}

func newEmbeddingModel(modelPath, vocabPath string) (*EmbeddingModel, error) {
	if err := ort.InitializeEnvironment(ort.WithLogLevelError()); err != nil {
		return nil, fmt.Errorf("init ort env: %w", err)
	}
	tok, err := loadBERTTokenizer(vocabPath)
	if err != nil {
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("load tokenizer: %w", err)
	}

	shape2 := ort.NewShape(1, maxSeqLen)
	shape3 := ort.NewShape(1, maxSeqLen, embHiddenSize)

	inputIDs, err := ort.NewTensor(shape2, make([]int64, maxSeqLen))
	if err != nil {
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("new inputIDs tensor: %w", err)
	}
	attMask, err := ort.NewTensor(shape2, make([]int64, maxSeqLen))
	if err != nil {
		inputIDs.Destroy()
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("new attMask tensor: %w", err)
	}
	tokTypeIDs, err := ort.NewTensor(shape2, make([]int64, maxSeqLen))
	if err != nil {
		inputIDs.Destroy()
		attMask.Destroy()
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("new tokTypeIDs tensor: %w", err)
	}
	output, err := ort.NewEmptyTensor[float32](shape3)
	if err != nil {
		inputIDs.Destroy()
		attMask.Destroy()
		tokTypeIDs.Destroy()
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("new output tensor: %w", err)
	}

	session, err := ort.NewAdvancedSession(modelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"},
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

	return &EmbeddingModel{
		session:    session,
		tokenizer:  tok,
		inputIDs:   inputIDs,
		attMask:    attMask,
		tokTypeIDs: tokTypeIDs,
		output:     output,
	}, nil
}

func (m *EmbeddingModel) Close() {
	m.session.Destroy()
	m.inputIDs.Destroy()
	m.attMask.Destroy()
	m.tokTypeIDs.Destroy()
	m.output.Destroy()
	ort.DestroyEnvironment()
}

func (m *EmbeddingModel) Predict(text string) ([]POSToken, error) {
	words := basicTokenize(text)
	if len(words) == 0 {
		return nil, nil
	}

	tokens := make([]POSToken, 0, len(words))
	for start := 0; start < len(words); start += chunkSize {
		end := min(start+chunkSize, len(words))
		chunk := words[start:end]
		embeds, err := m.embedChunk(chunk)
		if err != nil {
			return nil, err
		}
		for i, w := range chunk {
			if isAllPunct(w.text) {
				continue
			}
			v := l2Normalize(embeds[i])
			color := embeddingToColor(v)
			tokens = append(tokens, POSToken{
				Word:        w.text,
				Score:       1.0,
				Tag:         POS_O,
				Color:       color,
				OffsetBegin: w.begin,
				OffsetEnd:   w.end,
				Description: fmt.Sprintf("Semantic color %s", color),
			})
		}
	}

	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].OffsetBegin < tokens[j].OffsetBegin
	})
	return tokens, nil
}

func (m *EmbeddingModel) embedChunk(words []wordSpan) ([][]float32, error) {
	ids, mask, tti, swWordIdx, _ := m.tokenizer.tokenizeWords(words)

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

	hidden := m.output.GetData() // flat [1 * maxSeqLen * embHiddenSize]

	// Mean-pool subword vectors per word for a more faithful word embedding.
	sums := make([][]float32, len(words))
	counts := make([]int, len(words))
	for i := range sums {
		sums[i] = make([]float32, embHiddenSize)
	}
	for si := range seqLen {
		wi := swWordIdx[si]
		if wi < 0 {
			continue
		}
		base := si * embHiddenSize
		if base+embHiddenSize > len(hidden) {
			break
		}
		for d := range embHiddenSize {
			sums[wi][d] += hidden[base+d]
		}
		counts[wi]++
	}
	embeddings := make([][]float32, len(words))
	for i := range words {
		if counts[i] > 0 {
			vec := make([]float32, embHiddenSize)
			for d := range embHiddenSize {
				vec[d] = sums[i][d] / float32(counts[i])
			}
			embeddings[i] = vec
		} else {
			embeddings[i] = make([]float32, embHiddenSize)
		}
	}
	return embeddings, nil
}
