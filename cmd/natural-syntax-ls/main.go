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
	_ "natural-syntax-ls/internal/jaseg"
	"natural-syntax-ls/internal/lspserver"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "natural-syntax-ls: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// supported models: multilingual_minilm, mpnet, minilm, mobilebert, bert_base, xlmr_es, xlmr_fr, en_ewt.electra-base
	modelPath := flag.String("model", "", "Path to a .onnx model file")
	vocabPath := flag.String("vocab", "", "Path to a _vocab.txt (or Unigram _tokenizer.json) file (the model's own tokenizer, whatever mode)")
	modeFlag := flag.String("mode", "pos", "Highlighting mode: pos, semantic, or dependency")
	threads := flag.Int("threads", 0, "ORT threads per inference chunk (0 = auto from physical cores)")
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
	dirs := []string{dataDir, exeDir}

	if *modelPath == "" {
		// English POS models first, then the embedding models.
		var candidates []string
		for _, names := range [][]string{{"bert_base", "mobilebert"}, {"multilingual_minilm", "mpnet", "minilm"}} {
			for _, dir := range dirs {
				for _, name := range names {
					candidates = append(candidates, filepath.Join(dir, name+".onnx"))
				}
			}
		}
		*modelPath = findFile(candidates...)
	}

	embedHiddenSize := 768
	if strings.Contains(strings.ToLower(filepath.Base(*modelPath)), "minilm") {
		embedHiddenSize = 384
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
			ortCandidates = append(ortCandidates, filepath.Join(dir, lib))
		}
	}
	ortCandidates = append(ortCandidates, "/usr/lib/libonnxruntime.so", "/usr/local/lib/libonnxruntime.so")
	if ortLib := findFile(ortCandidates...); ortLib != "" {
		inference.SetORTLibPath(ortLib)
	}

	inference.SetIntraOpThreads(*threads)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := lspserver.Run(ctx, lspserver.Config{
		ModelPath:       *modelPath,
		VocabPath:       *vocabPath,
		Mode:            mode,
		EmbedHiddenSize: embedHiddenSize,
		Language:        modelLanguage(*modelPath),
	}); err != nil {
		return fmt.Errorf("lsp: %w", err)
	}
	return nil
}

// modelLangPrefixes maps model file-name prefixes to Wiktionary language codes.
var modelLangPrefixes = []struct{ prefix, lang string }{
	{"xlmr_es", "es"}, {"xlmr_fr", "fr"}, {"es_ancora", "es"}, {"fr_sequoia", "fr"},
}

// modelLanguage guesses a model's Wiktionary language code from its file name; "" when unknown or multilingual.
func modelLanguage(modelPath string) string {
	base := strings.ToLower(filepath.Base(modelPath))
	for _, m := range modelLangPrefixes {
		if strings.HasPrefix(base, m.prefix) {
			return m.lang
		}
	}
	return ""
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
