package lspserver

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"natural-syntax-ls/internal/lang"
	"natural-syntax-ls/internal/tokenizer"
	"natural-syntax-ls/internal/wiktionary"
)

// scriptPoll is how often language script files are checked for changes.
const scriptPoll = 2 * time.Second

// tokenizerReloader is a predictor whose tokenizer can be rebuilt without reloading the network.
type tokenizerReloader interface {
	ReloadTokenizer(opts ...tokenizer.Option) error
}

// scriptSignature fingerprints the *.lua files under dirs by name, size and modification time.
func scriptSignature(dirs []string) string {
	var b strings.Builder
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".lua" {
				continue
			}
			if info, err := e.Info(); err == nil {
				fmt.Fprintf(&b, "%s/%s:%d:%d;", d, e.Name(), info.Size(), info.ModTime().UnixNano())
			}
		}
	}
	return b.String()
}

// watchScripts reloads languages whenever a script file changes, until shutdown.
func (s *lspServer) watchScripts() {
	last := scriptSignature(s.cfg.ScriptDirs)
	t := time.NewTicker(scriptPoll)
	defer t.Stop()
	for {
		select {
		case <-s.context().Done():
			return
		case <-t.C:
		}
		if s.shutdown.Load() {
			return
		}
		if sig := scriptSignature(s.cfg.ScriptDirs); sig != last {
			last = sig
			s.reloadLanguages()
		}
	}
}

// reloadLanguages rereads the language scripts and applies them to the tokenizers, Wiktionary lookups and every open document.
func (s *lspServer) reloadLanguages() {
	langs := s.cfg.LoadLanguages()
	s.langs.Store(&langs)
	s.wikt.Store(wiktionary.NewClient(lang.WiktionaryLangs(langs)...))
	reg := s.registry.Load()
	if reg == nil {
		return
	}
	reload := func(p predictor, forced string) {
		if r, ok := p.(tokenizerReloader); ok {
			if err := r.ReloadTokenizer(lang.TokenizerOptions(langs, forced)...); err != nil {
				slog.Warn("reload tokenizer", "err", err)
			}
		}
	}
	reload(s.predictor, s.cfg.Forced)
	if s.switcher != nil {
		s.switcher.each(func(m predictor) { reload(m, "") })
	}
	reg.reset()
	s.sendRequest("workspace/semanticTokens/refresh")
	slog.Info("language scripts reloaded", "languages", len(langs))
}
