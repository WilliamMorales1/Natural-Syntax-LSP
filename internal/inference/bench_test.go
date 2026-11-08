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
type testModel struct {
	name string
	load func(dir string) (Predictor, error)
}

var testModels = []testModel{
	{"pos", func(dir string) (Predictor, error) {
		return NewPOSModel(testModelFiles(dir, "bert_base"))
	}},
	{"semantic", func(dir string) (Predictor, error) {
		InitSemantic(384)
		return NewEmbeddingModel(testModelFiles(dir, "multilingual_minilm"))
	}},
	{"dependency", func(dir string) (Predictor, error) {
		return NewDependencyModel(testModelFiles(dir, "en_ewt_electra_base_dependency"))
	}},
}

// testModelFiles returns slug's model and tokenizer file (_tokenizer.json, else _vocab.txt), from the setup.sh layout (models/<slug>/) if present, else flat in dir.
func testModelFiles(dir, slug string) (string, string) {
	if sub := filepath.Join(dir, "models", slug); dirExists(sub) {
		dir = sub
	}
	vocab := filepath.Join(dir, slug+"_tokenizer.json")
	if _, err := os.Stat(vocab); err != nil {
		vocab = filepath.Join(dir, slug+"_vocab.txt")
	}
	return filepath.Join(dir, slug+".onnx"), vocab
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func testDataDir(t testing.TB) string {
	t.Helper()
	if testing.Short() {
		t.Skip("model tests skipped with -short")
	}
	cfg, err := os.UserConfigDir()
	if err != nil {
		t.Skip("no user config dir")
	}
	dir := filepath.Join(cfg, "natural-syntax-ls")
	lib := filepath.Join(dir, "lib", "libonnxruntime.so")
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

// BenchmarkPredictChunk times one chunk of docs/examples/test.txt through each installed model.
func BenchmarkPredictChunk(b *testing.B) {
	dir := testDataDir(b)
	raw, err := os.ReadFile("../../docs/examples/test.txt")
	if err != nil {
		b.Fatal(err)
	}
	words := tokenizer.BasicTokenize(string(raw))
	for _, tm := range testModels {
		b.Run(tm.name, func(b *testing.B) {
			m, err := sharedModel(dir, tm.name, tm.load)
			if err != nil {
				b.Skip(err)
			}
			for b.Loop() {
				if _, err := m.PredictChunk(words); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// TestModelCost prints wall/CPU time per mode for a full-document run and a one-sentence run; NLS_TAG_DUMP=dir also writes each mode's outputs for before/after diffing. Off unless NSLS_MODEL_COST=1 or NLS_TAG_DUMP is set, since it checks nothing.
func TestModelCost(t *testing.T) {
	if os.Getenv("NSLS_MODEL_COST") == "" && os.Getenv("NLS_TAG_DUMP") == "" {
		t.Skip("set NSLS_MODEL_COST=1")
	}
	dir := testDataDir(t)
	raw, err := os.ReadFile("../../docs/examples/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	words := tokenizer.BasicTokenize(strings.Repeat(string(raw)+"\n\n", 10))
	sentence := tokenizer.BasicTokenize("Please change the state of the system.")

	for _, tm := range testModels {
		t.Run(tm.name, func(t *testing.T) {
			m, err := sharedModel(dir, tm.name, tm.load)
			if err != nil {
				t.Skip(err)
			}
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
