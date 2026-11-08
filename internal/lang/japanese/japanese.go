// Package japanese segments Japanese text into UniDic short-unit words with kagome, the word standard of UD Japanese-GSD, and looks up dictionary forms in Wiktionary.
package japanese

import (
	"unicode"

	"natural-syntax-ls/internal/lang"
	"natural-syntax-ls/internal/tokenizer"
)

// Language returns the Japanese hooks.
func Language() lang.Language {
	return lang.Language{
		Code:          "ja",
		ModelPrefixes: []string{"xlmr_ja", "ja_gsd"},
		Tokenizer:     []tokenizer.Option{tokenizer.WithSegmenter(segmenter{})},
		Wiktionary: &lang.Wiktionary{
			Code: "ja", Sections: []string{"Japanese"}, Wikitext: true, Script: unicode.Han, Forms: lemmas,
		},
	}
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
