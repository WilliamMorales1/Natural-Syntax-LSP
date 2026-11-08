package inference

import (
	"fmt"
	"slices"
	"sync"
	"sync/atomic"

	"natural-syntax-ls/internal/tokenizer"

	ort "github.com/yalue/onnxruntime_go"
)

const maxSeqLen = 512

// ChunkSize is the max words per inference chunk, conservative to stay under maxSeqLen after subword splitting.
const ChunkSize = 200

// baseModel holds the ONNX session shared by BERT-style token models (POSModel, EmbeddingModel); inputs are sized to each chunk, never padded.
type baseModel struct {
	session    *idleSession
	tok        *tokHolder
	outputName string
	lastDim    int64
	// contiguous encodes unspaced neighbors without a word-start mark (see tokenizer.EncodeWordsContiguous).
	contiguous bool
}

// Tokenize splits text using the model's own tokenizer rules.
func (m *baseModel) Tokenize(text string) []tokenizer.WordSpan { return m.tok.get().Tokenize(text) }

var (
	envMu   sync.Mutex
	envRefs int
)

// acquireEnv initializes the process-global ORT environment on first use and counts later users.
func acquireEnv() error {
	envMu.Lock()
	defer envMu.Unlock()
	if envRefs == 0 {
		if err := ort.InitializeEnvironment(ort.WithLogLevelError()); err != nil {
			return fmt.Errorf("init ort env: %w", err)
		}
	}
	envRefs++
	return nil
}

// releaseEnv drops one user and destroys the ORT environment after the last.
func releaseEnv() {
	envMu.Lock()
	defer envMu.Unlock()
	if envRefs--; envRefs == 0 {
		ort.DestroyEnvironment()
	}
}

// initEnvAndTokenizer takes an ORT env reference and loads the tokenizer; on success the caller owns the reference and must releaseEnv() on later errors and on Close().
func initEnvAndTokenizer(vocabPath string, opts ...tokenizer.Option) (tokenizer.Tokenizer, error) {
	if err := acquireEnv(); err != nil {
		return nil, err
	}
	tok, err := tokenizer.Open(vocabPath, opts...)
	if err != nil {
		releaseEnv()
		return nil, fmt.Errorf("load tokenizer: %w", err)
	}
	return tok, nil
}

// outputLastDim reads the static last dimension of the named output from the ONNX file.
func outputLastDim(modelPath, name string) (int, error) {
	if err := acquireEnv(); err != nil {
		return 0, err
	}
	defer releaseEnv()
	_, outputs, err := ort.GetInputOutputInfo(modelPath)
	if err != nil {
		return 0, fmt.Errorf("read model info: %w", err)
	}
	for _, o := range outputs {
		if o.Name == name && len(o.Dimensions) > 0 {
			if d := o.Dimensions[len(o.Dimensions)-1]; d > 0 {
				return int(d), nil
			}
		}
	}
	return 0, fmt.Errorf("model %s: output %q has no fixed last dimension", modelPath, name)
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
func newBaseModel(modelPath, vocabPath, outputName string, lastDim int64, opts ...tokenizer.Option) (*baseModel, error) {
	tok, err := initEnvAndTokenizer(vocabPath, opts...)
	if err != nil {
		return nil, err
	}
	session, err := newIdleSession(modelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{outputName},
	)
	if err != nil {
		releaseEnv()
		return nil, err
	}
	return &baseModel{session: session, tok: newTokHolder(tok, vocabPath), outputName: outputName, lastDim: lastDim}, nil
}

// ReloadTokenizer rebuilds the tokenizer from its vocab with new options; the session stays loaded.
func (m *baseModel) ReloadTokenizer(opts ...tokenizer.Option) error { return m.tok.reload(opts...) }

// tokHolder lets a running model swap its tokenizer, e.g. after a language script changed.
type tokHolder struct {
	p     atomic.Pointer[tokenizer.Tokenizer]
	vocab string
}

func newTokHolder(t tokenizer.Tokenizer, vocab string) *tokHolder {
	h := &tokHolder{vocab: vocab}
	h.p.Store(&t)
	return h
}

func (h *tokHolder) get() tokenizer.Tokenizer { return *h.p.Load() }

func (h *tokHolder) reload(opts ...tokenizer.Option) error {
	t, err := tokenizer.Open(h.vocab, opts...)
	if err != nil {
		return fmt.Errorf("reload tokenizer: %w", err)
	}
	h.p.Store(&t)
	return nil
}

// Close releases the ORT session and environment.
func (m *baseModel) Close() error {
	err := m.session.Destroy()
	releaseEnv()
	return err
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

// wordsRun is one model run over a block of words.
type wordsRun struct {
	out       []float32 // flat [seqLen x lastDim] output
	seqLen    int
	swWordIdx []int  // subword → word index, negative for special tokens
	swIsFirst []bool // subword is its word's first piece
}

// runWords tokenizes words and runs the session on exactly that many subwords.
func (m *baseModel) runWords(words []tokenizer.WordSpan) (wordsRun, error) {
	encode := tokenizer.EncodeWords
	if m.contiguous {
		encode = tokenizer.EncodeWordsContiguous
	}
	enc := encode(m.tok.get(), words)
	ids, mask, tti := enc.InputIDs, enc.AttMask, enc.TokTypeIDs
	seqLen := min(len(ids), maxSeqLen)
	shape := ort.NewShape(1, int64(seqLen))

	var inputs []ort.Value
	defer func() {
		for _, v := range inputs {
			v.Destroy()
		}
	}()
	for _, in := range []struct {
		name string
		data []int64
	}{{"inputIDs", ids}, {"attMask", mask}, {"tokTypeIDs", tti}} {
		t, err := ort.NewTensor(shape, in.data[:seqLen])
		if err != nil {
			return wordsRun{}, fmt.Errorf("new %s tensor: %w", in.name, err)
		}
		inputs = append(inputs, t)
	}
	output, err := ort.NewEmptyTensor[float32](ort.NewShape(1, int64(seqLen), m.lastDim))
	if err != nil {
		return wordsRun{}, fmt.Errorf("new output tensor: %w", err)
	}
	defer output.Destroy()

	if err := m.session.Run(inputs, []ort.Value{output}); err != nil {
		return wordsRun{}, fmt.Errorf("ort run: %w", err)
	}
	return wordsRun{slices.Clone(output.GetData()), seqLen, enc.SwWordIdx, enc.SwIsFirst}, nil
}
