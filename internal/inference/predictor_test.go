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
	pieces := []string{"alpha", "beta", "gamma", "it's", "x", ".", "!", "?", ",", " ", " ", " ", "\n", "\n\n", "\r\n\r\n", "\n \n", "\n- ", "\n1. ", "\n# ", "\n```\n"}
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
			if !sentenceEnders[last.Text] && !next.BlockStart && strings.Count(gap, "\n") < 2 {
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

// sentenceTexts joins each sentence's words with spaces, for compact comparison.
func sentenceTexts(sents [][]tokenizer.WordSpan) []string {
	out := make([]string, len(sents))
	for i, words := range chunkTexts(sents) {
		out[i] = strings.Join(words, " ")
	}
	return out
}

// TestSplitSentencesMarkdown checks that markdown blocks split into separate sentences even without terminal punctuation, while wrapped prose stays whole.
func TestSplitSentencesMarkdown(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want []string
	}{
		{"dash list items", "- buy milk\n- walk the dog\n- call mom", []string{"- buy milk", "- walk the dog", "- call mom"}},
		{"list items with enders", "- First item.\n- Second item!", []string{"- First item .", "- Second item !"}},
		{"multi-sentence item", "- One. Two\n- Three", []string{"- One .", "Two", "- Three"}},
		{"ordered list keeps its period", "1. preheat the oven\n2. mix the flour\n3) bake", []string{"1 . preheat the oven", "2 . mix the flour", "3 ) bake"}},
		{"ordered list item with ender", "1. Done.\n2. Next", []string{"1 . Done .", "2 . Next"}},
		{"version number mid-line still splits", "use v1. then", []string{"use v1 .", "then"}},
		{"intro then list", "You will need:\n- eggs\n- flour", []string{"You will need :", "- eggs", "- flour"}},
		{"wrapped item stays whole", "- a long item that\n  wraps onto two lines\n- short", []string{"- a long item that wraps onto two lines", "- short"}},
		{"lazy continuation stays whole", "- a long item that\nwraps lazily\n- short", []string{"- a long item that wraps lazily", "- short"}},
		{"nested list", "- fruit\n  - apple\n  - pear\n- veg", []string{"- fruit", "- apple", "- pear", "- veg"}},
		{"heading then paragraph", "# Getting Started\nInstall the tool first", []string{"# Getting Started", "Install the tool first"}},
		{"paragraph then heading", "Some text here\n## Usage", []string{"Some text here", "# # Usage"}},
		{"setext heading", "Overview\n========\nThe tool colors words", []string{"Overview", "= = = = = = = =", "The tool colors words"}},
		{"thematic break", "end of part one\n***\nstart of part two", []string{"end of part one", "* * *", "start of part two"}},
		{"table rows", "| name | role |\n| Ann | dev |", []string{"| name | role |", "| Ann | dev |"}},
		{"quoted list", "> - quoted one\n> - quoted two", []string{"> - quoted one", "> - quoted two"}},
		{"quote wraps", "> a quote that\n> wraps", []string{"> a quote that > wraps"}},
		{"code lines", "```\nfoo bar\nbaz qux\n```\nafter code", []string{"` ` `", "foo bar", "baz qux", "` ` `", "after code"}},
		{"plain wrapped prose", "the wind blew hard\nand the sun shone.", []string{"the wind blew hard and the sun shone ."}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sentenceTexts(splitSentences(tokenizer.BasicTokenize(tc.in))); !slices.Equal(got, tc.want) {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSplitBlocks(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want []string
	}{
		{"prose is one block", "One sentence. Another one\nwrapped.", []string{"One sentence . Another one wrapped ."}},
		{"list items", "Steps:\n- a b\n- c. d", []string{"Steps :", "- a b", "- c . d"}},
		{"heading and body", "# Title\nBody. More body.", []string{"# Title", "Body . More body ."}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sentenceTexts(splitBlocks(tokenizer.BasicTokenize(tc.in))); !slices.Equal(got, tc.want) {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSplitChunksMarkdownDoc checks a realistic README-style document: paragraphs become chunks, and every sentence inside stays within its block.
func TestSplitChunksMarkdownDoc(t *testing.T) {
	in := `# Natural Syntax

Highlights words by part of speech.
It runs locally.

## Features

- POS mode colors by tag
- Semantic mode colors by meaning
  across the document
- Dependency mode

1. Install the extension
2. Export the models
`
	words := tokenizer.BasicTokenize(in)
	chunks := SplitChunks(in, words)
	var got [][]string
	for _, c := range chunks {
		got = append(got, sentenceTexts(splitSentences(c)))
	}
	want := [][]string{
		{"# Natural Syntax"},
		{"Highlights words by part of speech .", "It runs locally ."},
		{"# # Features"},
		{"- POS mode colors by tag", "- Semantic mode colors by meaning across the document", "- Dependency mode"},
		{"1 . Install the extension", "2 . Export the models"},
	}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestSplitChunksPacksListItems checks that a list too long for one chunk breaks between items, never inside one.
func TestSplitChunksPacksListItems(t *testing.T) {
	in := strings.Repeat("- one two three four\n", 60)
	chunks := SplitChunks(in, tokenizer.BasicTokenize(in))
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2", len(chunks))
	}
	for i, c := range chunks {
		if len(c) > ChunkSize || !c[0].BlockStart || c[len(c)-1].Text != "four" {
			t.Errorf("chunk %d: %d words from %q to %q", i, len(c), c[0].Text, c[len(c)-1].Text)
		}
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
