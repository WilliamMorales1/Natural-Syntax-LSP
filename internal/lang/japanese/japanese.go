// Package japanese segments Japanese text into UniDic short-unit words with kagome, the word standard of UD Japanese-GSD, and looks up dictionary forms in Wiktionary.
// Build with -tags nojaseg to leave out the 37 MB embedded dictionary.
package japanese

import (
	"unicode"

	"natural-syntax-ls/internal/lang"
	"natural-syntax-ls/internal/tokenizer"
)

// Language returns the Japanese hooks.
func Language() lang.Language {
	l := lang.Language{
		Code: "ja",
		Wiktionary: &lang.Wiktionary{
			Code: "ja", Sections: []string{"Japanese"}, Wikitext: true, Script: unicode.Han,
		},
	}
	if hasDictionary {
		l.Tokenizer = []tokenizer.Option{tokenizer.WithSegmenter(segmenter{})}
		l.Wiktionary.Forms = lemmas
	}
	return l
}

// segmenter applies to text with kana, since Han runs alone cannot tell Japanese from Chinese.
type segmenter struct{}

func (segmenter) Applies(text []rune) bool {
	for _, r := range text {
		if unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) {
			return true
		}
	}
	return false
}

func (segmenter) Segment(run string) [][2]int { return segment(run) }
