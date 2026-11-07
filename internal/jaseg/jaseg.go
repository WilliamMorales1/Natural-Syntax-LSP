//go:build !nojaseg && !english

// Package jaseg segments Japanese text into UniDic short-unit words with kagome, the word standard of UD Japanese-GSD.
// Importing it registers the segmenter with the tokenizer package; build with -tags nojaseg to leave out the 37 MB embedded dictionary.
package jaseg

import (
	"sync"

	"github.com/ikawaha/kagome-dict/uni"
	"github.com/ikawaha/kagome/v2/tokenizer"

	nstok "natural-syntax-ls/internal/tokenizer"
)

var (
	once sync.Once
	mu   sync.Mutex
	tk   *tokenizer.Tokenizer
)

func init() {
	nstok.SetJapaneseSegmenter(Segment)
	nstok.SetJapaneseLemmatizer(Lemmas)
}

// Lemmas returns candidate dictionary forms for an inflected Japanese word, nil if there are none.
func Lemmas(word string) []string {
	segmentInit()
	if tk == nil {
		return nil
	}
	mu.Lock()
	toks := tk.Tokenize(word)
	mu.Unlock()
	var out []string
	for _, t := range toks {
		if b, ok := t.BaseForm(); ok && b != "*" && b != t.Surface {
			out = append(out, b)
		}
		if len(out) > 0 {
			break
		}
	}
	return out
}

func segmentInit() {
	once.Do(func() {
		// The dictionary decodes to about 370 MB of heap, so it is loaded only when Japanese text first appears.
		if t, err := tokenizer.New(uni.Dict(), tokenizer.OmitBosEos()); err == nil {
			tk = t
		}
	})
}

// Segment returns the rune ranges of the words in run, which must be a Han/kana run; nil if the dictionary cannot be loaded.
func Segment(run string) [][2]int {
	segmentInit()
	if tk == nil {
		return nil
	}
	mu.Lock()
	toks := tk.Tokenize(run)
	mu.Unlock()
	out := make([][2]int, 0, len(toks))
	for _, t := range toks {
		if t.End > t.Start {
			out = append(out, [2]int{t.Start, t.End})
		}
	}
	return out
}
