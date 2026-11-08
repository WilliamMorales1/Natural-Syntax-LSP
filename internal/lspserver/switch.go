package lspserver

import (
	"log/slog"
	"sync"

	"natural-syntax-ls/internal/inference"
	"natural-syntax-ls/internal/lang"
	"natural-syntax-ls/internal/langid"
)

// maxSwitched caps extra models held in memory beside the configured one.
const maxSwitched = 2

// switcher loads a model per detected language on first use and hands documents the one that fits.
type switcher struct {
	cfg     Config
	mu      sync.Mutex
	loaded  map[string]predictor
	pending map[string]bool // loading now
	skip    map[string]bool // no installed model, or it failed to load
	onReady func()          // called after a model finishes loading
}

func newSwitcher(cfg Config) *switcher {
	return &switcher{cfg: cfg, loaded: map[string]predictor{}, pending: map[string]bool{}, skip: map[string]bool{}}
}

// pick returns the model for text: the loaded one for its language, else base. A text too short to tell keeps cur.
func (sw *switcher) pick(text string, cur, base predictor) predictor {
	code := langid.Detect(text[:min(len(text), docSample)])
	if code == "" {
		return cur
	}
	if code == sw.cfg.Language || code == "en" && sw.cfg.Language == "" {
		return base
	}
	sw.mu.Lock()
	defer sw.mu.Unlock()
	if m, ok := sw.loaded[code]; ok {
		return m
	}
	if !sw.pending[code] && !sw.skip[code] && len(sw.loaded)+len(sw.pending) < maxSwitched {
		sw.pending[code] = true
		go sw.load(code)
	}
	return base
}

func (sw *switcher) load(code string) {
	path, vocab := sw.cfg.Switch(code)
	if path == "" || vocab == "" {
		sw.finish(code, nil)
		return
	}
	opts := lang.TokenizerOptions(sw.cfg.Languages, "")
	var m predictor
	var err error
	switch sw.cfg.Mode {
	case ModeSemantic:
		m, err = inference.NewEmbeddingModel(path, vocab, opts...)
	case ModeDependency:
		m, err = inference.NewDependencyModel(path, vocab, opts...)
	default:
		m, err = inference.NewPOSModel(path, vocab, opts...)
	}
	if err != nil {
		slog.Warn("load model for language", "lang", code, "err", err)
		sw.finish(code, nil)
		return
	}
	sw.finish(code, m)
	if sw.onReady != nil {
		sw.onReady()
	}
}

func (sw *switcher) finish(code string, m predictor) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	delete(sw.pending, code)
	if m == nil {
		sw.skip[code] = true
		return
	}
	sw.loaded[code] = m
}

func (sw *switcher) close() {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	for _, m := range sw.loaded {
		if err := m.Close(); err != nil {
			slog.Warn("close model", "err", err)
		}
	}
}
