//go:build !nojaseg

package japanese

import (
	"sync"

	"github.com/ikawaha/kagome-dict/uni"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

var mu sync.Mutex

// The dictionary decodes to about 370 MB of heap, so it is loaded only when Japanese text first appears.
var kagome = sync.OnceValue(func() *tokenizer.Tokenizer {
	t, err := tokenizer.New(uni.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		return nil
	}
	return t
})

const hasDictionary = true

// lemmas returns candidate dictionary forms for an inflected Japanese word, nil if there are none.
func lemmas(word string) []string {
	tk := kagome()
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

// segment returns the rune ranges of the words in run, which must be a Han/kana run; nil if the dictionary cannot be loaded.
func segment(run string) [][2]int {
	tk := kagome()
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
