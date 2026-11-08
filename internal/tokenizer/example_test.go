package tokenizer_test

import (
	"fmt"
	"strings"

	"natural-syntax-ls/internal/lang/luaload"
	"natural-syntax-ls/internal/tokenizer"
)

// A vocabulary of whole-word pieces, as XLM-R marks them with ▁.
const vocab = `{
 "model": {"type": "Unigram", "unk_id": 1, "vocab": [
  ["<s>", 0], ["<unk>", 0], ["</s>", 0],
  ["▁l'", -4], ["▁ami", -5], ["▁aujourd'hui", -6], ["▁الكتاب", -11], ["▁كتاب", -12], ["▁قال", -11.1], ["▁وقال", -11.9], ["▁وزير", -11.7], ["▁زير", -13.6]
 ]},
 "pre_tokenizer": {"type": "Metaspace"},
 "post_processor": {"type": "TemplateProcessing", "single": [
  {"SpecialToken": {"id": "<s>", "type_id": 0}}, {"Sequence": {"id": "A", "type_id": 0}}, {"SpecialToken": {"id": "</s>", "type_id": 0}}]}
}`

// Language scripts choose tokenizer rules by name: French elisions rejoin l' and keep aujourd'hui whole; Arabic clitics split where the vocabulary backs the split.
func ExampleParseUnigram() {
	for _, tc := range []struct{ script, text string }{
		{`return {code = "fr", tokenizer = {elisions = {clitics = {"l"}, words = {"aujourd'hui"}}}}`, "l'ami aujourd'hui"},
		{`return {code = "ar", tokenizer = {arabic_clitics = true}}`, "وبالكتاب وقال وزير"},
	} {
		l, err := luaload.Parse("example.lua", tc.script)
		if err != nil {
			panic(err)
		}
		tok, err := tokenizer.ParseUnigram([]byte(vocab), l.Tokenizer...)
		if err != nil {
			panic(err)
		}
		var words []string
		for _, w := range tok.Tokenize(tc.text) {
			words = append(words, fmt.Sprintf("%s %d-%d", w.Text, w.Begin, w.End))
		}
		fmt.Println(strings.Join(words, " | "))
	}
	// Output:
	// l' 0-2 | ami 2-5 | aujourd'hui 6-17
	// و 0-1 | ب 1-2 | الكتاب 2-8 | و 9-10 | قال 10-13 | وزير 14-18
}
