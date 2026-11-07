// Package french holds French word-splitting rules.
package french

import (
	"strings"
	"unicode"

	"natural-syntax-ls/internal/lang"
	"natural-syntax-ls/internal/tokenizer"
)

// Language returns the French hooks.
func Language() lang.Language {
	return lang.Language{Code: "fr", ModelPrefixes: []string{"xlmr_fr", "fr_sequoia"}, Tokenizer: []tokenizer.Option{tokenizer.WithWordMerger(MergeElisions)}, Wiktionary: &lang.Wiktionary{Code: "fr", Sections: []string{"French"}}}
}

// elisions are clitics written with a trailing apostrophe that UD treats as their own word.
var elisions = map[string]bool{
	"l": true, "d": true, "j": true, "m": true, "t": true, "s": true, "n": true, "c": true,
	"qu": true, "jusqu": true, "lorsqu": true, "puisqu": true, "quoiqu": true,
}

// MergeElisions glues an elided clitic to its apostrophe, so "l ' ami" becomes "l'" and "ami", and rejoins "aujourd'hui".
func MergeElisions(runes []rune, words []tokenizer.WordSpan) []tokenizer.WordSpan {
	var out []tokenizer.WordSpan
	for i := 0; i < len(words); i++ {
		w := words[i]
		if i+2 < len(words) {
			ap, next := words[i+1], words[i+2]
			joined := (ap.Text == "'" || ap.Text == "’") && ap.Begin == w.End && next.Begin == ap.End
			switch {
			case joined && strings.EqualFold(w.Text, "aujourd") && strings.EqualFold(next.Text, "hui"):
				w.Text, w.End = string(runes[w.Begin:next.End]), next.End
				i += 2
			case joined && elisions[strings.ToLower(w.Text)] && unicode.IsLetter(runes[next.Begin]):
				w.Text, w.End = w.Text+ap.Text, ap.End
				i++
			}
		}
		out = append(out, w)
	}
	return out
}
