package inference

import (
	"fmt"

	"natural-syntax-ls/internal/tokenizer"

	ort "github.com/yalue/onnxruntime_go"
)

const maxSeqLen = 512

// ChunkSize is the max words per inference chunk, conservative to stay under maxSeqLen after subword splitting.
const ChunkSize = 200

// baseModel holds the ONNX session shared by BERT-style token models (POSModel, EmbeddingModel); inputs are sized to each chunk, never padded.
type baseModel struct {
	session    *ort.DynamicAdvancedSession
	tokenizer  *tokenizer.BERTTokenizer
	outputName string
	lastDim    int64
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

// newSessionOptions returns ORT options tuned for a background editor process: few threads, no busy-wait spinning.
func newSessionOptions() (*ort.SessionOptions, error) {
	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, err
	}
	for _, set := range []func() error{
		func() error { threads, _ := Concurrency(); return opts.SetIntraOpNumThreads(threads) },
		func() error { return opts.SetInterOpNumThreads(1) },
		func() error { return opts.AddSessionConfigEntry("session.intra_op.allow_spinning", "0") },
		func() error { return opts.AddSessionConfigEntry("session.inter_op.allow_spinning", "0") },
	} {
		if err := set(); err != nil {
			opts.Destroy()
			return nil, err
		}
	}
	return opts, nil
}

// newDynamicSession creates a dynamic-shape ORT session with newSessionOptions.
func newDynamicSession(modelPath string, inputs, outputs []string) (*ort.DynamicAdvancedSession, error) {
	opts, err := newSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("session options: %w", err)
	}
	defer opts.Destroy()
	return ort.NewDynamicAdvancedSession(modelPath, inputs, outputs, opts)
}

// newBaseModel loads the tokenizer and creates the ORT session.
func newBaseModel(modelPath, vocabPath, outputName string, lastDim int64) (*baseModel, error) {
	tok, err := initEnvAndTokenizer(vocabPath)
	if err != nil {
		return nil, err
	}
	session, err := newDynamicSession(modelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{outputName},
	)
	if err != nil {
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create ort session: %w", err)
	}
	return &baseModel{session: session, tokenizer: tok, outputName: outputName, lastDim: lastDim}, nil
}

func (m *baseModel) Close() {
	m.session.Destroy()
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

// runWords tokenizes words, runs the session on exactly that many subwords, and returns the flat [seqLen x lastDim] output plus subword bookkeeping.
func (m *baseModel) runWords(words []tokenizer.WordSpan) (out []float32, seqLen int, swWordIdx []int, swIsFirst []bool, err error) {
	ids, mask, tti, swWordIdx, swIsFirst := m.tokenizer.TokenizeWords(words)
	seqLen = min(len(ids), maxSeqLen)
	shape := ort.NewShape(1, int64(seqLen))

	inputIDs, err := ort.NewTensor(shape, ids[:seqLen])
	if err != nil {
		return nil, 0, nil, nil, fmt.Errorf("new inputIDs tensor: %w", err)
	}
	defer inputIDs.Destroy()
	attMask, err := ort.NewTensor(shape, mask[:seqLen])
	if err != nil {
		return nil, 0, nil, nil, fmt.Errorf("new attMask tensor: %w", err)
	}
	defer attMask.Destroy()
	tokTypeIDs, err := ort.NewTensor(shape, tti[:seqLen])
	if err != nil {
		return nil, 0, nil, nil, fmt.Errorf("new tokTypeIDs tensor: %w", err)
	}
	defer tokTypeIDs.Destroy()
	output, err := ort.NewEmptyTensor[float32](ort.NewShape(1, int64(seqLen), m.lastDim))
	if err != nil {
		return nil, 0, nil, nil, fmt.Errorf("new output tensor: %w", err)
	}
	defer output.Destroy()

	if err := m.session.Run(
		[]ort.Value{inputIDs, attMask, tokTypeIDs},
		[]ort.Value{output},
	); err != nil {
		return nil, 0, nil, nil, fmt.Errorf("ort run: %w", err)
	}
	return append([]float32(nil), output.GetData()...), seqLen, swWordIdx, swIsFirst, nil
}
