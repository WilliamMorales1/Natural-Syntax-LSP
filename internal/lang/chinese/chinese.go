// Package chinese maps between Simplified and Traditional characters so Wiktionary lookups find whichever spelling has the page.
package chinese

import (
	"unicode"

	"natural-syntax-ls/internal/lang"
)

// Language returns the Chinese hooks.
func Language() lang.Language {
	return lang.Language{Code: "zh", ModelPrefixes: []string{"chinese_roberta", "zh_ud"}, Wiktionary: &lang.Wiktionary{
		Code: "zh", Sections: []string{"Chinese"}, Wikitext: true, Script: unicode.Han, Forms: forms,
	}}
}

// forms lists the other script's spellings, since Wiktionary keeps full entries under the Traditional one and the Simplified page may be missing or a stub.
// Model pieces are unsegmented, so "学生们" has no page but its prefix does; two trims bound the extra lookups.
func forms(word string) []string {
	out := variants(word)
	runes := []rune(word)
	for n := len(runes) - 1; n >= 2 && n >= len(runes)-2; n-- {
		prefix := string(runes[:n])
		out = append(out, prefix)
		out = append(out, variants(prefix)...)
	}
	return out
}

func variants(word string) []string {
	var out []string
	if trad := Traditional(word); trad != word {
		out = append(out, trad)
	}
	if simp := Simplified(word); simp != word {
		out = append(out, simp)
	}
	return out
}
