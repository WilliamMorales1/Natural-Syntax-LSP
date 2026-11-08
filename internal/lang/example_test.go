package lang_test

import (
	"fmt"
	"strings"
	"unicode"

	"natural-syntax-ls/internal/lang"
	"natural-syntax-ls/internal/postag"
	"natural-syntax-ls/internal/tokenizer"
)

// A native language sets the same hooks a Lua script selects: tokenizer options (tokenizer.WithWordMerger, tokenizer.WithSegmenter) and Wiktionary rules (Forms, Rewrite, Wikitext with Script).
func ExampleLanguage() {
	l := lang.Language{
		Code:          "xx",
		ModelPrefixes: []string{"xlmr_xx"},
		Tokenizer: []tokenizer.Option{tokenizer.WithWordMerger(func(_ []rune, words []tokenizer.WordSpan) []tokenizer.WordSpan {
			return words
		})},
		Wiktionary: &lang.Wiktionary{
			Code:     "xx",
			Sections: []string{"Example"},
			Forms:    func(word string) []string { return []string{strings.TrimSuffix(word, "s")} },
			Rewrite: func(word string, pos postag.PartOfSpeech, _ postag.Deprel) string {
				if pos == postag.POS {
					return "-" + word
				}
				return ""
			},
			Wikitext: true,
			Script:   unicode.Latin,
		},
	}
	langs := []lang.Language{l}
	fmt.Println(lang.ForModel(langs, "/models/xlmr_xx/xlmr_xx_int8.onnx"), lang.ForModel(langs, "bert_base.onnx") == "")
	fmt.Println(l.Wiktionary.Forms("cats"), l.Wiktionary.Rewrite("'s", postag.POS, postag.NoDeprel))
	// Output:
	// xx true
	// [cat] -'s
}
