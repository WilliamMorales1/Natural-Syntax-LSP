package inference

import (
	"fmt"

	"natural-syntax-ls/internal/tokenizer"

	ort "github.com/yalue/onnxruntime_go"
)

const maxSeqLen = 512

// ChunkSize is the max words per inference chunk, conservative to stay under maxSeqLen after subword splitting.
const ChunkSize = 200

// baseFixedModel holds the ONNX session/tensor plumbing shared by fixed-size [1, maxSeqLen] models (POSModel, EmbeddingModel).
type baseFixedModel struct {
	session    *ort.AdvancedSession
	tokenizer  *tokenizer.BERTTokenizer
	inputIDs   *ort.Tensor[int64]
	attMask    *ort.Tensor[int64]
	tokTypeIDs *ort.Tensor[int64]
	output     *ort.Tensor[float32]
}

// initEnvAndTokenizer inits the ORT env and loads the BERT tokenizer; callers must call ort.DestroyEnvironment() on later errors and on Close().
func initEnvAndTokenizer(vocabPath string) (*tokenizer.BERTTokenizer, error) {
	if err := ort.InitializeEnvironment(ort.WithLogLevelError()); err != nil {
		return nil, fmt.Errorf("init ort env: %w", err)
	}

	tok, err := tokenizer.Load(vocabPath)
	if err != nil {
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("load tokenizer: %w", err)
	}
	return tok, nil
}

// newBaseFixedModel loads the tokenizer, allocates fixed-shape input/output tensors, and creates the ORT session.
func newBaseFixedModel(modelPath, vocabPath, outputName string, lastDim int64) (*baseFixedModel, error) {
	tok, err := initEnvAndTokenizer(vocabPath)
	if err != nil {
		return nil, err
	}

	shape2 := ort.NewShape(1, maxSeqLen)
	shape3 := ort.NewShape(1, maxSeqLen, lastDim)

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
		[]string{outputName},
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

	return &baseFixedModel{
		session:    session,
		tokenizer:  tok,
		inputIDs:   inputIDs,
		attMask:    attMask,
		tokTypeIDs: tokTypeIDs,
		output:     output,
	}, nil
}

func (m *baseFixedModel) Close() {
	m.session.Destroy()
	m.inputIDs.Destroy()
	m.attMask.Destroy()
	m.tokTypeIDs.Destroy()
	m.output.Destroy()
	ort.DestroyEnvironment()
}

// decodeEnumList decodes each string in raw via decode, substituting fallback for entries decode rejects.
func decodeEnumList[T any](raw []string, decode func(string) (T, bool), fallback T) []T {
	out := make([]T, len(raw))
	for i, s := range raw {
		v, ok := decode(s)
		if !ok {
			v = fallback
		}
		out[i] = v
	}
	return out
}

// runWords tokenizes words, fills the fixed-size input tensors, runs the session, and returns subword id/first-piece bookkeeping plus seqLen.
func (m *baseFixedModel) runWords(words []tokenizer.WordSpan) (seqLen int, swWordIdx []int, swIsFirst []bool, err error) {
	ids, mask, tti, swWordIdx, swIsFirst := m.tokenizer.TokenizeWords(words)

	seqLen = min(len(ids), maxSeqLen)

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
		return 0, nil, nil, fmt.Errorf("ort run: %w", err)
	}

	return seqLen, swWordIdx, swIsFirst, nil
}
