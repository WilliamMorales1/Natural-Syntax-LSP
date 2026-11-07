// Package arabic holds Arabic headword rules for Wiktionary lookups.
package arabic

import (
	"strings"

	"natural-syntax-ls/internal/lang"
)

// Language returns the Arabic hooks.
func Language() lang.Language {
	return lang.Language{
		Code:          "ar",
		ModelPrefixes: []string{"xlmr_ar", "ar_padt"},
		Wiktionary:    &lang.Wiktionary{Code: "ar", Sections: []string{"Arabic"}, Forms: Forms},
	}
}

// Forms returns the word without tashkeel and tatweel, which headwords omit.
func Forms(word string) []string {
	plain := strings.Map(func(r rune) rune {
		if r == 'ـ' || (r >= 'ً' && r <= 'ٟ') || r == 'ٰ' {
			return -1
		}
		return r
	}, word)
	if plain == word {
		return nil
	}
	return []string{plain}
}
