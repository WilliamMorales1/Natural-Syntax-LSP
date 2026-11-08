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
	// languages returns the current language scripts.
	languages func() []lang.Language
}

func newSwitcher(cfg Config) *switcher {
	return &switcher{cfg: cfg, loaded: map[string]predictor{}, pending: map[string]bool{}, skip: map[string]bool{}}
}

// pick returns the model for text: the loaded one for its language, nil while that model loads, else base. A text too short to tell keeps cur.
func (sw *switcher) pick(text string, cur, base predictor) predictor {
	code := sw.detect(text)
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
	if sw.pending[code] || sw.startLocked(code) {
		return nil
	}
	return base
}

// prefetch starts loading text's model before the configured one is up, so the two loads overlap.
func (sw *switcher) prefetch(text string) {
	code := sw.detect(text)
	if code == "" || code == sw.cfg.Language || code == "en" && sw.cfg.Language == "" {
		return
	}
	sw.mu.Lock()
	defer sw.mu.Unlock()
	if _, ok := sw.loaded[code]; !ok && !sw.pending[code] {
		sw.startLocked(code)
	}
}

func (sw *switcher) detect(text string) string {
	return langid.Detect(text[:min(len(text), docSample)])
}

// startLocked begins loading code's model unless it has none or the cap is reached.
func (sw *switcher) startLocked(code string) bool {
	if sw.skip[code] || len(sw.loaded)+len(sw.pending) >= maxSwitched {
		return false
	}
	sw.pending[code] = true
	go sw.load(code)
	return true
}

// load runs onReady whether or not the model loads, so documents waiting on it get analysed either way.
func (sw *switcher) load(code string) {
	m := sw.open(code)
	sw.finish(code, m)
	if sw.onReady != nil {
		sw.onReady()
	}
}

func (sw *switcher) open(code string) predictor {
	path, vocab := sw.cfg.Switch(code)
	if path == "" || vocab == "" {
		return nil
	}
	opts := lang.TokenizerOptions(sw.languages(), "")
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
		return nil
	}
	return m
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

// each calls f on every loaded model.
func (sw *switcher) each(f func(predictor)) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	for _, m := range sw.loaded {
		f(m)
	}
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
