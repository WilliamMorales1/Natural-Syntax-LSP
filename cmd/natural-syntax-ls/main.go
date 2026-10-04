// Command natural-syntax-ls is a language server that highlights prose by part of speech, dependency relation, or semantic embedding.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"natural-syntax-ls/internal/inference"
	"natural-syntax-ls/internal/lspserver"
)

func main() {
	// supported models: mpnet, minilm, mobilebert, bert_base, en_ewt.electra-base
	modelPath := flag.String("model", "", "Path to a .onnx model file")
	vocabPath := flag.String("vocab", "", "Path to a _vocab.txt file (the model's own vocab, whatever mode)")
	mode := flag.String("mode", "pos", "Highlighting mode: pos, semantic, or dependency")
	threads := flag.Int("threads", 0, "ORT threads per inference chunk (0 = auto from physical cores)")
	flag.Bool("stdio", false, "Use stdio transport (default; accepted for LSP client compatibility)")
	flag.Parse()

	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	userDir, _ := os.UserConfigDir()
	dataDir := filepath.Join(userDir, "natural-syntax-ls")

	if *modelPath == "" {
		*modelPath = findFile(
			filepath.Join(dataDir, "bert_base.onnx"),
			filepath.Join(dataDir, "mobilebert.onnx"),
			filepath.Join(exeDir, "bert_base.onnx"),
			filepath.Join(exeDir, "mobilebert.onnx"),
			filepath.Join(dataDir, "mpnet.onnx"),
			filepath.Join(dataDir, "minilm.onnx"),
			filepath.Join(exeDir, "mpnet.onnx"),
			filepath.Join(exeDir, "minilm.onnx"),
		)
	}

	embedHiddenSize := 768
	if strings.HasPrefix(filepath.Base(*modelPath), "minilm") {
		embedHiddenSize = 384
	}

	// The vocab must belong to the chosen model; any other vocab maps subwords to the wrong ids.
	if *vocabPath == "" && *modelPath != "" {
		*vocabPath = findFile(strings.TrimSuffix(*modelPath, ".onnx") + "_vocab.txt")
	}

	if *mode == "semantic" {
		inference.InitSemantic(embedHiddenSize)
	}

	if *modelPath == "" || *vocabPath == "" {
		fmt.Fprintln(os.Stderr, "natural-syntax-ls: cannot find embedding model or vocab file")
		fmt.Fprintln(os.Stderr, "Run scripts/export_model.py to export them.")
		os.Exit(1)
	}

	if ortLib := findFile(
		os.Getenv("ORT_LIB_PATH"),
		filepath.Join(dataDir, "onnxruntime.dll"),
		filepath.Join(dataDir, "libonnxruntime.so"),
		filepath.Join(dataDir, "libonnxruntime.dylib"),
		filepath.Join(exeDir, "onnxruntime.dll"),
		filepath.Join(exeDir, "libonnxruntime.so"),
		filepath.Join(exeDir, "libonnxruntime.dylib"),
	); ortLib != "" {
		inference.SetORTLibPath(ortLib)
	}

	inference.SetIntraOpThreads(*threads)

	if err := lspserver.Run(lspserver.Config{
		ModelPath:       *modelPath,
		VocabPath:       *vocabPath,
		Mode:            *mode,
		EmbedHiddenSize: embedHiddenSize,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "natural-syntax-ls: lsp: %v\n", err)
		os.Exit(1)
	}
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
