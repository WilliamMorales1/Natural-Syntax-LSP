//go:build !nojaseg

package japanese

import (
	"slices"
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

// lemmas returns headword candidates for a word or model piece: the first content token's dictionary form, then its surface when the piece carries trailing particles ("私は" gives 私).
func lemmas(word string) []string {
	tk := kagome()
	if tk == nil {
		return nil
	}
	mu.Lock()
	toks := tk.Tokenize(word)
	mu.Unlock()
	for _, t := range toks {
		if pos := t.POS(); len(pos) > 0 && functionPOS[pos[0]] && len(toks) > 1 {
			continue
		}
		var out []string
		if b, ok := t.BaseForm(); ok && b != "*" && b != word {
			out = append(out, b)
		}
		if t.Surface != word && !slices.Contains(out, t.Surface) {
			out = append(out, t.Surface)
		}
		return out
	}
	return nil
}

// functionPOS are UniDic top-level classes that never head a dictionary lookup.
var functionPOS = map[string]bool{"助詞": true, "助動詞": true, "補助記号": true, "接頭辞": true}

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
