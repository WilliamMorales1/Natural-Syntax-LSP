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
	flag.Bool("stdio", false, "Use stdio transport (default; accepted for LSP client compatibility)")
	flag.Parse()

	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	userDir, err := os.UserConfigDir()
	if err != nil {
		userDir = ""
	}
	dataDir := filepath.Join(userDir, "natural-syntax-ls")

	embedHiddenSize := 768
	if *modelPath == "minilm.onnx" {
		embedHiddenSize = 384
	}

	if *modelPath == "" {
		*modelPath = findFile([]string{
			filepath.Join(dataDir, "bert_base.onnx"),
			filepath.Join(dataDir, "mobilebert.onnx"),
			filepath.Join(exeDir, "bert_base.onnx"),
			filepath.Join(exeDir, "mobilebert.onnx"),
			filepath.Join(dataDir, "mpnet.onnx"),
			filepath.Join(dataDir, "minilm.onnx"),
			filepath.Join(exeDir, "mpnet.onnx"),
			filepath.Join(exeDir, "minilm.onnx"),
		})
	}

	if *vocabPath == "" {
		*vocabPath = findFile([]string{
			filepath.Join(dataDir, "mpnet_vocab.txt"),
			filepath.Join(dataDir, "minilm_vocab.txt"),
			filepath.Join(exeDir, "mpnet_vocab.txt"),
			filepath.Join(exeDir, "minilm_vocab.txt"),
			filepath.Join(dataDir, "bert_base_vocab.txt"),
			filepath.Join(dataDir, "mobilebert_vocab.txt"),
			filepath.Join(exeDir, "bert_base_vocab.txt"),
			filepath.Join(exeDir, "mobilebert_vocab.txt"),
		})
	}

	if *mode == "semantic" {
		inference.InitSemantic(embedHiddenSize)
	}

	if *mode == "dependency" && *vocabPath == "" && *modelPath != "" {
		*vocabPath = findFile([]string{
			strings.TrimSuffix(*modelPath, ".onnx") + "_vocab.txt",
		})
	}

	if *modelPath == "" || *vocabPath == "" {
		fmt.Fprintln(os.Stderr, "natural-syntax-ls: cannot find embedding model or vocab file")
		fmt.Fprintf(os.Stderr, "Run scripts/export_models.py to export them.\n")
		os.Exit(1)
	}

	ortLib := findFile([]string{
		os.Getenv("ORT_LIB_PATH"),
		filepath.Join(dataDir, "onnxruntime.dll"),
		filepath.Join(dataDir, "libonnxruntime.so"),
		filepath.Join(dataDir, "libonnxruntime.dylib"),
		filepath.Join(exeDir, "onnxruntime.dll"),
		filepath.Join(exeDir, "libonnxruntime.so"),
		filepath.Join(exeDir, "libonnxruntime.dylib"),
	})
	if ortLib != "" {
		inference.SetORTLibPath(ortLib)
	}

	cfg := lspserver.Config{
		ModelPath:       *modelPath,
		VocabPath:       *vocabPath,
		Mode:            *mode,
		EmbedHiddenSize: embedHiddenSize,
	}
	if err := lspserver.Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "natural-syntax-ls: lsp: %v\n", err)
		os.Exit(1)
	}
}

func findFile(candidates []string) string {
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
