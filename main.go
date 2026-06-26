package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	modelPath := flag.String("model", "", "Path to mobilebert_pos.onnx")
	vocabPath := flag.String("vocab", "", "Path to vocab.txt")
	flag.Parse()

	// Search default locations if not specified.
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)

	if *modelPath == "" {
		*modelPath = findFile([]string{
			os.Getenv("NATURAL_SYNTAX_LS_MODEL"),
			filepath.Join(exeDir, "mobilebert_pos.onnx"),
			filepath.Join(userConfigDir(), "natural-syntax-ls", "mobilebert_pos.onnx"),
		})
	}
	if *vocabPath == "" {
		*vocabPath = findFile([]string{
			os.Getenv("NATURAL_SYNTAX_LS_VOCAB"),
			filepath.Join(exeDir, "vocab.txt"),
			filepath.Join(userConfigDir(), "natural-syntax-ls", "vocab.txt"),
		})
	}

	// onnxruntime_go needs the shared library path on Windows.
	ortLib := findFile([]string{
		os.Getenv("ORT_LIB_PATH"),
		filepath.Join(exeDir, "onnxruntime.dll"),
		filepath.Join(exeDir, "libonnxruntime.so"),
		filepath.Join(exeDir, "libonnxruntime.dylib"),
	})
	if ortLib != "" {
		setORTLibPath(ortLib)
	}

	if *modelPath == "" || *vocabPath == "" {
		fmt.Fprintln(os.Stderr, "natural-syntax-ls: cannot find mobilebert_pos.onnx or vocab.txt")
		fmt.Fprintln(os.Stderr, "Run export_model.py to generate them, then place next to this binary.")
		fmt.Fprintln(os.Stderr, "Or set NATURAL_SYNTAX_LS_MODEL and NATURAL_SYNTAX_LS_VOCAB env vars.")
		os.Exit(1)
	}

	model, err := newPOSModel(*modelPath, *vocabPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "natural-syntax-ls: load model: %v\n", err)
		os.Exit(1)
	}
	defer model.Close()

	if err := runLSP(model); err != nil {
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
