// Package chinese maps between Simplified and Traditional characters so Wiktionary lookups find whichever spelling has the page.
// Build with -tags english to leave out the conversion tables.
package chinese

import (
	"unicode"

	"natural-syntax-ls/internal/lang"
)

// Language returns the Chinese hooks.
func Language() lang.Language {
	return lang.Language{Code: "zh", Wiktionary: &lang.Wiktionary{
		Code: "zh", Sections: []string{"Chinese"}, Wikitext: true, Script: unicode.Han, Forms: forms,
	}}
}

// forms lists the other script's spellings, since Wiktionary keeps full entries under the Traditional one and the Simplified page may be missing or a stub.
func forms(word string) []string {
	var out []string
	if trad := Traditional(word); trad != word {
		out = append(out, trad)
	}
	if simp := Simplified(word); simp != word {
		out = append(out, simp)
	}
	return out
}
