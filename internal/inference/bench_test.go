package inference

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"natural-syntax-ls/internal/tokenizer"
)

// testModels maps each mode to its model slug and loader; loaders return nil when files are missing.
var testModels = []struct {
	name string
	load func(dir string) (Predictor, error)
}{
	{"pos", func(dir string) (Predictor, error) {
		return NewPOSModel(filepath.Join(dir, "bert_base.onnx"), filepath.Join(dir, "bert_base_vocab.txt"))
	}},
	{"minilm", func(dir string) (Predictor, error) {
		InitSemantic(384)
		return NewEmbeddingModel(filepath.Join(dir, "minilm.onnx"), filepath.Join(dir, "minilm_vocab.txt"), 384)
	}},
	{"mpnet", func(dir string) (Predictor, error) {
		InitSemantic(768)
		return NewEmbeddingModel(filepath.Join(dir, "mpnet.onnx"), filepath.Join(dir, "mpnet_vocab.txt"), 768)
	}},
	{"dependency", func(dir string) (Predictor, error) {
		return NewDependencyModel(filepath.Join(dir, "en_ewt_electra_base_dependency.onnx"), filepath.Join(dir, "en_ewt_electra_base_dependency_vocab.txt"))
	}},
}

func testDataDir(t testing.TB) string {
	t.Helper()
	cfg, err := os.UserConfigDir()
	if err != nil {
		t.Skip("no user config dir")
	}
	dir := filepath.Join(cfg, "natural-syntax-ls")
	lib := filepath.Join(dir, "libonnxruntime.so")
	if _, err := os.Stat(lib); err != nil {
		t.Skipf("missing %s", lib)
	}
	SetORTLibPath(lib)
	return dir
}

func cpuTime() time.Duration {
	var ru syscall.Rusage
	syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}

// TestModelCost prints wall/CPU time per mode for a full-document run and a one-sentence run; NLS_TAG_DUMP=dir also writes each mode's outputs for before/after diffing.
func TestModelCost(t *testing.T) {
	dir := testDataDir(t)
	raw, err := os.ReadFile("../../docs/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	words := tokenizer.BasicTokenize(strings.Repeat(string(raw)+"\n\n", 10))
	sentence := tokenizer.BasicTokenize("Please change the state of the system.")

	for _, tm := range testModels {
		t.Run(tm.name, func(t *testing.T) {
			m, err := tm.load(dir)
			if err != nil {
				t.Skip(err)
			}
			defer m.Close()
			m.PredictChunk(sentence) // warm-up

			measure := func(name string, runs int, f func()) {
				w0, c0 := time.Now(), cpuTime()
				for range runs {
					f()
				}
				wall := time.Since(w0) / time.Duration(runs)
				cpu := (cpuTime() - c0) / time.Duration(runs)
				t.Logf("%-10s wall %8v  cpu %8v", name, wall.Round(time.Microsecond), cpu.Round(time.Microsecond))
			}
			var sb strings.Builder
			measure("full-doc", 2, func() {
				sb.Reset()
				for i := 0; i < len(words); i += ChunkSize {
					toks, err := m.PredictChunk(words[i:min(i+ChunkSize, len(words))])
					if err != nil {
						t.Fatal(err)
					}
					for _, tok := range toks {
						fmt.Fprintf(&sb, "%d %s %v %s %v %v %d %.4f\n", tok.OffsetBegin, tok.Word, tok.Tag, tok.Color, tok.Deprel, tok.HasHead, tok.HeadOffsetBegin, tok.Score)
					}
				}
			})
			measure("sentence", 10, func() {
				if _, err := m.PredictChunk(sentence); err != nil {
					t.Fatal(err)
				}
			})
			if out := os.Getenv("NLS_TAG_DUMP"); out != "" {
				os.WriteFile(filepath.Join(out, tm.name+".txt"), []byte(sb.String()), 0o644)
			}
		})
	}
}
