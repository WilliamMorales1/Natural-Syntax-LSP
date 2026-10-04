package inference

import (
	"math"
	"strings"
	"sync"
	"testing"

	"natural-syntax-ls/internal/postag"
	"natural-syntax-ls/internal/tokenizer"
)

// forEachModel runs f against every installed model, skipping those whose files are missing.
func forEachModel(t *testing.T, f func(t *testing.T, m Predictor)) {
	dir := testDataDir(t)
	for _, tm := range testModels {
		t.Run(tm.name, func(t *testing.T) {
			m, err := tm.load(dir)
			if err != nil {
				t.Skip(err)
			}
			defer m.Close()
			f(t, m)
		})
	}
}

// checkTokens fails unless every token maps to one of words by text and offsets, in order.
func checkTokens(t *testing.T, words []tokenizer.WordSpan, toks []postag.Token) {
	t.Helper()
	wi := 0
	for _, tok := range toks {
		for wi < len(words) && words[wi].Begin != tok.OffsetBegin {
			wi++
		}
		if wi == len(words) {
			t.Fatalf("token %+v matches no remaining word", tok)
		}
		if w := words[wi]; w.Text != tok.Word || w.End != tok.OffsetEnd {
			t.Fatalf("token %q %d-%d vs word %q %d-%d", tok.Word, tok.OffsetBegin, tok.OffsetEnd, w.Text, w.Begin, w.End)
		}
		if tokenizer.IsAllPunct(tok.Word) {
			t.Errorf("punctuation token %q emitted", tok.Word)
		}
		if tok.Score < 0 || tok.Score > 1 || math.IsNaN(tok.Score) {
			t.Errorf("token %q score %v", tok.Word, tok.Score)
		}
		wi++
	}
}

func TestModelEdgeInputs(t *testing.T) {
	long := strings.Repeat("antidisestablishmentarianism ", 150) // ~900 subwords, past maxSeqLen
	cases := map[string]struct {
		text    string
		minToks int
	}{
		"empty":       {"", 0},
		"single word": {"hello", 1},
		"punctuation": {"... !? ,", 0},
		"unicode":     {"Café naïve résumé, 日本語 and 🎉 emoji.", 4},
		"numbers":     {"In 1999 there were 42 cats.", 5},
		"long":        {long, 1},
	}
	forEachModel(t, func(t *testing.T, m Predictor) {
		for name, tc := range cases {
			t.Run(name, func(t *testing.T) {
				words := tokenizer.BasicTokenize(tc.text)
				toks, err := m.PredictChunk(words)
				if err != nil {
					t.Fatal(err)
				}
				if len(toks) < tc.minToks {
					t.Errorf("got %d tokens, want at least %d", len(toks), tc.minToks)
				}
				checkTokens(t, words, toks)
			})
		}
	})
}

func TestModelDependencyHeadsInChunk(t *testing.T) {
	forEachModel(t, func(t *testing.T, m Predictor) {
		if _, ok := m.(*DependencyModel); !ok {
			t.Skip("not a dependency model")
		}
		words := tokenizer.BasicTokenize("The quick fox jumps over the dog. It barks loudly!")
		toks, err := m.PredictChunk(words)
		if err != nil {
			t.Fatal(err)
		}
		begins := map[uint32]bool{}
		for _, w := range words {
			begins[w.Begin] = true
		}
		roots := 0
		for _, tok := range toks {
			if !tok.HasHead {
				roots++
				continue
			}
			if !begins[tok.HeadOffsetBegin] || tok.HeadOffsetBegin == tok.OffsetBegin {
				t.Errorf("%q has bad head offset %d", tok.Word, tok.HeadOffsetBegin)
			}
		}
		// Each sentence is parsed with its own root, so two sentences need at least two roots.
		if roots < 2 {
			t.Errorf("%d roots for two sentences", roots)
		}
	})
}

// TestModelConcurrentMatchesSequential runs chunks from many goroutines on one session and requires the same output as one-at-a-time calls.
func TestModelConcurrentMatchesSequential(t *testing.T) {
	texts := []string{
		"Please change the state of the system.",
		"The North Wind and the Sun were disputing which was the stronger.",
		"They agreed that the one who first succeeded should be considered stronger.",
		"Change of the state.",
		"Colorless green ideas sleep furiously.",
		"I saw her duck under the table.",
	}
	forEachModel(t, func(t *testing.T, m Predictor) {
		want := make([][]postag.Token, len(texts))
		for i, s := range texts {
			var err error
			if want[i], err = m.PredictChunk(tokenizer.BasicTokenize(s)); err != nil {
				t.Fatal(err)
			}
		}
		got := make([][]postag.Token, len(texts)*4)
		var wg sync.WaitGroup
		for i := range got {
			wg.Go(func() {
				toks, err := m.PredictChunk(tokenizer.BasicTokenize(texts[i%len(texts)]))
				if err != nil {
					t.Error(err)
				}
				got[i] = toks
			})
		}
		wg.Wait()
		for i, toks := range got {
			w := want[i%len(texts)]
			if len(toks) != len(w) {
				t.Fatalf("run %d: %d tokens, want %d", i, len(toks), len(w))
			}
			for j := range toks {
				a, b := toks[j], w[j]
				if a.Tag != b.Tag || a.Color != b.Color || a.Deprel != b.Deprel || a.HasHead != b.HasHead || a.HeadOffsetBegin != b.HeadOffsetBegin || math.Abs(a.Score-b.Score) > 1e-4 {
					t.Fatalf("run %d token %d: concurrent %+v, sequential %+v", i, j, a, b)
				}
			}
		}
	})
}
