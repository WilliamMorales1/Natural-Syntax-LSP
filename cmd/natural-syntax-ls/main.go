// Command natural-syntax-ls is a language server that highlights prose by part of speech, dependency relation, or semantic embedding.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"natural-syntax-ls/internal/inference"
	"natural-syntax-ls/internal/lang"
	"natural-syntax-ls/internal/lang/builtin"
	"natural-syntax-ls/internal/lspserver"
	"natural-syntax-ls/internal/models"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "natural-syntax-ls: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	modelPath := flag.String("model", "", "Path to a .onnx model file")
	vocabPath := flag.String("vocab", "", "Path to a _vocab.txt (or Unigram _tokenizer.json) file (the model's own tokenizer, whatever mode)")
	modeFlag := flag.String("mode", "pos", "Highlighting mode: pos, semantic, or dependency")
	forced := flag.String("lang", "", "Language code (en, fr, es, ...) that replaces detection and picks that language's model when -model is not given; empty detects per line")
	threads := flag.Int("threads", 0, "ORT threads per inference chunk (0 = auto from physical cores)")
	langDir := flag.String("lang-dir", "", "Extra directory of language *.lua scripts, loaded last, after <exe dir>/lang, <model dir>/lang and <config dir>/natural-syntax-ls/lang")
	_ = flag.Bool("stdio", false, "Use stdio transport (default; accepted for LSP client compatibility)")
	flag.Parse()

	mode, err := lspserver.ParseMode(*modeFlag)
	if err != nil {
		return err
	}

	exe, _ := os.Executable()
	// Package managers (Mason, Homebrew) launch through a symlink; bundled libs sit next to the real binary.
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	exeDir := filepath.Dir(exe)
	cfgDir, err := configDir()
	if err != nil {
		return fmt.Errorf("find config dir: %w", err)
	}
	dataDir := filepath.Join(cfgDir, "natural-syntax-ls")
	// Source checkout: bin/natural-syntax-ls sits beside data/.
	repoData := filepath.Join(exeDir, "..", "data")
	dirs := []string{dataDir, exeDir}

	catalog, err := models.Load(repoData, exeDir, dataDir)
	if err != nil {
		return fmt.Errorf("read model catalog: %w", err)
	}
	if *modelPath == "" {
		var candidates []string
		for _, stem := range catalog.StemsFor(string(mode), *forced) {
			for _, dir := range dirs {
				candidates = append(candidates, filepath.Join(dir, "models", stem, stem+".onnx"), filepath.Join(dir, stem+".onnx"))
			}
		}
		*modelPath = findFile(candidates...)
		if *forced != "" && *modelPath != "" && !matchesLang(catalog, *forced, *modelPath) {
			fmt.Fprintf(os.Stderr, "natural-syntax-ls: no installed %s model for language %q; using %s\n", mode, *forced, *modelPath)
		}
	}
	if *modelPath != "" {
		fmt.Fprintf(os.Stderr, "natural-syntax-ls: model %s\n", *modelPath)
	}

	// The vocab must belong to the chosen model; any other vocab maps subwords to the wrong ids.
	if *vocabPath == "" && *modelPath != "" {
		base := strings.TrimSuffix(*modelPath, ".onnx")
		// Unigram models ship a tokenizer.json instead of a vocab.txt.
		*vocabPath = findFile(base+"_vocab.txt", base+"_tokenizer.json")
	}

	if *modelPath == "" || *vocabPath == "" {
		return errors.New("cannot find embedding model or vocab file; run scripts/export_model.py to export them")
	}

	ortCandidates := []string{os.Getenv("ORT_LIB_PATH")}
	for _, dir := range dirs {
		for _, lib := range []string{"onnxruntime.dll", "libonnxruntime.so", "libonnxruntime.dylib"} {
			ortCandidates = append(ortCandidates, filepath.Join(dir, "lib", lib), filepath.Join(dir, lib))
		}
	}
	ortCandidates = append(ortCandidates, "/usr/lib/libonnxruntime.so", "/usr/local/lib/libonnxruntime.so")
	if ortLib := findFile(ortCandidates...); ortLib != "" {
		inference.SetORTLibPath(ortLib)
	}

	inference.SetIntraOpThreads(*threads)
	languages := builtin.Languages(filepath.Join(repoData, "lang"), filepath.Join(exeDir, "lang"), filepath.Join(filepath.Dir(*modelPath), "lang"), filepath.Join(dataDir, "lang"), *langDir)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := lspserver.Run(ctx, lspserver.Config{
		ModelPath: *modelPath,
		VocabPath: *vocabPath,
		Mode:      mode,
		Language:  modelLanguage(languages, *modelPath, *forced),
		Forced:    *forced,
		Languages: languages,
		Switch:    switchFor(catalog, mode, *modelPath, *forced, dirs),
	}); err != nil {
		return fmt.Errorf("lsp: %w", err)
	}
	return nil
}

// configDir returns where setup.sh and the VS Code extension put models: XDG on macOS too, not ~/Library.
func configDir() (string, error) {
	if runtime.GOOS == "windows" || runtime.GOOS == "plan9" {
		return os.UserConfigDir()
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return xdg, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config"), nil
}

// findFile returns the first existing path among candidates, or "" if none exist.
func findFile(candidates ...string) string {
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// modelLanguage is the forced language when set, else the language the model's file name marks.
func modelLanguage(languages []lang.Language, modelPath, forced string) string {
	if forced != "" {
		return forced
	}
	return lang.ForModel(languages, modelPath)
}

// matchesLang reports whether the catalog marks modelPath as made for the language code.
func matchesLang(c models.Catalog, code, modelPath string) bool {
	base := strings.TrimSuffix(filepath.Base(modelPath), ".onnx")
	for _, m := range c.Models {
		if m.Stem() == base && (m.Language == code || m.Language == "") {
			return true
		}
	}
	return false
}

// switchFor returns the per-language model lookup, or nil when the configured model is not an English one: a model picked for another language is the user's choice and stays.
func switchFor(c models.Catalog, mode lspserver.Mode, modelPath, forced string, dirs []string) func(string) (string, string) {
	if forced != "" || !isEnglishModel(c, modelPath) {
		return nil
	}
	return func(code string) (string, string) {
		for _, m := range c.Models {
			if m.Mode != string(mode) || m.Language != code {
				continue
			}
			stem := m.Stem()
			var candidates []string
			for _, dir := range dirs {
				candidates = append(candidates, filepath.Join(dir, "models", stem, stem+".onnx"), filepath.Join(dir, stem+".onnx"))
			}
			path := findFile(candidates...)
			if path == "" {
				continue
			}
			base := strings.TrimSuffix(path, ".onnx")
			if vocab := findFile(base+"_vocab.txt", base+"_tokenizer.json"); vocab != "" {
				return path, vocab
			}
		}
		return "", ""
	}
}

// isEnglishModel reports whether the catalog marks modelPath as an English model.
func isEnglishModel(c models.Catalog, modelPath string) bool {
	base := strings.TrimSuffix(filepath.Base(modelPath), ".onnx")
	for _, m := range c.Models {
		if m.Stem() == base {
			return m.Language == "en"
		}
	}
	return false
}
