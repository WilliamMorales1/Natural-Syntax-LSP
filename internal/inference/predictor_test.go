package inference

import (
	"math"
	"math/rand/v2"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"natural-syntax-ls/internal/tokenizer"
)

func chunkTexts(chunks [][]tokenizer.WordSpan) [][]string {
	out := make([][]string, len(chunks))
	for i, c := range chunks {
		for _, w := range c {
			out[i] = append(out[i], w.Text)
		}
	}
	return out
}

func TestSplitChunks(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want [][]string
	}{
		{"empty", "", [][]string{}},
		{"one sentence", "the dog runs.", [][]string{{"the", "dog", "runs", "."}}},
		{"sentences share a paragraph chunk", "a b. c d!", [][]string{{"a", "b", ".", "c", "d", "!"}}},
		{"single newline stays together", "a b\nc d.", [][]string{{"a", "b", "c", "d", "."}}},
		{"blank line splits", "a b.\n\nc d.", [][]string{{"a", "b", "."}, {"c", "d", "."}}},
		{"crlf blank line splits", "a b.\r\n\r\nc d.", [][]string{{"a", "b", "."}, {"c", "d", "."}}},
		{"whitespace-only line splits", "a.\n  \t\nb.", [][]string{{"a", "."}, {"b", "."}}},
		{"no trailing ender", "a b\n\nc", [][]string{{"a", "b"}, {"c"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := chunkTexts(SplitChunks(tc.in, tokenizer.BasicTokenize(tc.in)))
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !slices.EqualFunc(got, tc.want, slices.Equal) {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSplitChunksPacksSentencesUnderLimit(t *testing.T) {
	// 60 four-word sentences in one paragraph: 240 words must split at a sentence end, never mid-sentence.
	in := strings.Repeat("one two three. ", 60)
	chunks := SplitChunks(in, tokenizer.BasicTokenize(in))
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2", len(chunks))
	}
	for i, c := range chunks {
		if len(c) > ChunkSize || c[len(c)-1].Text != "." {
			t.Errorf("chunk %d: %d words ending %q", i, len(c), c[len(c)-1].Text)
		}
	}
}

func TestSplitChunksHardSplitsLongSentence(t *testing.T) {
	in := strings.Repeat("word ", 2*ChunkSize+7) + "."
	chunks := SplitChunks(in, tokenizer.BasicTokenize(in))
	var sizes []int
	for _, c := range chunks {
		sizes = append(sizes, len(c))
	}
	if want := []int{ChunkSize, ChunkSize, 8}; !slices.Equal(sizes, want) {
		t.Errorf("chunk sizes %v, want %v", sizes, want)
	}
}

// TestSplitChunksProperties checks on random documents that chunks cover every word once, in order, within ChunkSize, and break only at sentence or paragraph ends unless a sentence overflows.
func TestSplitChunksProperties(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 7))
	pieces := []string{"alpha", "beta", "gamma", "it's", "x", ".", "!", "?", ",", " ", " ", " ", "\n", "\n\n", "\r\n\r\n", "\n \n"}
	for trial := range 300 {
		var sb strings.Builder
		for range rng.IntN(800) {
			p := pieces[rng.IntN(len(pieces))]
			sb.WriteString(p)
			if p != " " && !strings.ContainsAny(p, "\n") {
				sb.WriteByte(' ')
			}
		}
		in := sb.String()
		words := tokenizer.BasicTokenize(in)
		chunks := SplitChunks(in, words)

		flat := slices.Concat(chunks...)
		if !slices.Equal(flat, words) {
			t.Fatalf("trial %d: chunks don't cover words in order", trial)
		}
		runes := []rune(in)
		for i, c := range chunks {
			if len(c) == 0 || len(c) > ChunkSize {
				t.Fatalf("trial %d chunk %d: size %d", trial, i, len(c))
			}
			if i == len(chunks)-1 || len(c) == ChunkSize {
				continue
			}
			last, next := c[len(c)-1], chunks[i+1][0]
			gap := string(runes[last.End:next.Begin])
			if !sentenceEnders[last.Text] && strings.Count(gap, "\n") < 2 {
				t.Fatalf("trial %d chunk %d: breaks mid-sentence after %q (gap %q)", trial, i, last.Text, gap)
			}
		}
	}
}

func TestSplitSentences(t *testing.T) {
	words := tokenizer.BasicTokenize("Hi. How are you? Fine! trailing")
	got := chunkTexts(splitSentences(words))
	want := [][]string{{"Hi", "."}, {"How", "are", "you", "?"}, {"Fine", "!"}, {"trailing"}}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseCPUList(t *testing.T) {
	for in, want := range map[string][]int{
		"0":         {0},
		"0-3":       {0, 1, 2, 3},
		"0,4":       {0, 4},
		"0-1,6,8-9": {0, 1, 6, 8, 9},
		"x":         nil,
		"1-":        nil,
		"":          nil,
	} {
		if got := parseCPUList(in); !slices.Equal(got, want) {
			t.Errorf("parseCPUList(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestConcurrency(t *testing.T) {
	defer SetIntraOpThreads(0)
	phys := physicalCores()
	if phys < 1 || phys > runtime.NumCPU() {
		t.Fatalf("physicalCores = %d with NumCPU %d", phys, runtime.NumCPU())
	}

	threads, workers := Concurrency()
	if threads != min(phys, maxIntraOpThreads) || workers != max(1, phys/threads) {
		t.Errorf("auto: threads=%d workers=%d for %d physical cores", threads, workers, phys)
	}

	for _, n := range []int{1, 2, 3, 16} {
		SetIntraOpThreads(n)
		threads, workers := Concurrency()
		if threads != n || workers != max(1, phys/n) {
			t.Errorf("override %d: threads=%d workers=%d for %d physical cores", n, threads, workers, phys)
		}
	}
}

var hexColor = regexp.MustCompile(`^#[0-9A-F]{6}$`)

func TestEmbeddingToColor(t *testing.T) {
	InitSemantic(8)
	rng := rand.New(rand.NewPCG(3, 4))
	seen := map[string]bool{}
	for range 50 {
		v := make([]float32, 8)
		for i := range v {
			v[i] = rng.Float32()*2 - 1
		}
		v = l2Normalize(v)
		c := EmbeddingToColor(v)
		if !hexColor.MatchString(c) {
			t.Fatalf("bad color %q", c)
		}
		if EmbeddingToColor(v) != c {
			t.Fatalf("color not deterministic")
		}
		seen[c] = true
	}
	if len(seen) < 10 {
		t.Errorf("only %d distinct colors from 50 random embeddings", len(seen))
	}
}

func TestL2Normalize(t *testing.T) {
	v := l2Normalize([]float32{3, 4})
	if math.Abs(float64(v[0])-0.6) > 1e-6 || math.Abs(float64(v[1])-0.8) > 1e-6 {
		t.Errorf("l2Normalize(3,4) = %v", v)
	}
	zero := []float32{0, 0}
	if got := l2Normalize(zero); !slices.Equal(got, zero) {
		t.Errorf("zero vector changed: %v", got)
	}
}

func TestOKLCHGrayAndClamp(t *testing.T) {
	// Zero chroma is gray regardless of hue; ±1 allows for matrix rounding and truncation to uint8.
	near := func(a, b uint8) bool { return a-b <= 1 || b-a <= 1 }
	for _, h := range []float64{0, 1, -2, math.Pi} {
		r, g, b := oklchToSRGB(0.7, 0, h)
		if !near(r, g) || !near(g, b) {
			t.Errorf("hue %v: chroma 0 gave %d,%d,%d", h, r, g, b)
		}
	}
	if linearToU8(-1) != 0 || linearToU8(2) != 255 || linearToU8(0.5) == 0 {
		t.Errorf("linearToU8 clamp: %d %d %d", linearToU8(-1), linearToU8(2), linearToU8(0.5))
	}
}
