package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	modelPath := flag.String("model", "", "Path to a _pos.onnx model file")
	vocabPath := flag.String("vocab", "", "Path to a _vocab.txt file")
	flag.Bool("stdio", false, "Use stdio transport (default; accepted for LSP client compatibility)")
	flag.Parse()

	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	dataDir := filepath.Join(userConfigDir(), "natural-syntax-ls")

	if *modelPath == "" {
		*modelPath = findFile([]string{
			os.Getenv("NATURAL_SYNTAX_LS_MODEL"),
			filepath.Join(dataDir, "bert_base_pos.onnx"),
			filepath.Join(dataDir, "mobilebert_pos.onnx"),
			filepath.Join(exeDir, "bert_base_pos.onnx"),
			filepath.Join(exeDir, "mobilebert_pos.onnx"),
		})
	}
	if *vocabPath == "" {
		*vocabPath = findFile([]string{
			os.Getenv("NATURAL_SYNTAX_LS_VOCAB"),
			filepath.Join(dataDir, "bert_base_vocab.txt"),
			filepath.Join(dataDir, "mobilebert_vocab.txt"),
			filepath.Join(exeDir, "bert_base_vocab.txt"),
			filepath.Join(exeDir, "mobilebert_vocab.txt"),
		})
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
		setORTLibPath(ortLib)
	}

	if *modelPath == "" || *vocabPath == "" {
		fmt.Fprintln(os.Stderr, "natural-syntax-ls: cannot find model or vocab file")
		fmt.Fprintln(os.Stderr, "Run scripts/setup.sh to export them to the data directory.")
		fmt.Fprintln(os.Stderr, "Or set NATURAL_SYNTAX_LS_MODEL and NATURAL_SYNTAX_LS_VOCAB env vars.")
		os.Exit(1)
	}

	if err := runLSP(*modelPath, *vocabPath); err != nil {
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

func userConfigDir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return d
}
