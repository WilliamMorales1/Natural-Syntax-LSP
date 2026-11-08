// Package russian holds Russian headword rules for Wiktionary lookups.
package russian

import (
	"slices"
	"strings"

	"natural-syntax-ls/internal/lang"
)

// Language returns the Russian hooks.
func Language() lang.Language {
	return lang.Language{
		Code:          "ru",
		ModelPrefixes: []string{"xlmr_ru", "ru_syntagrus"},
		Wiktionary:    &lang.Wiktionary{Code: "ru", Sections: []string{"Russian"}, Forms: Forms},
	}
}

const stressMark = '́'

// maxYoCandidates caps the extra requests a word with many е letters can cause.
const maxYoCandidates = 3

// Forms returns the word without stress marks, then spellings with one е turned into ё, since Wiktionary files ё words under ё and texts often write е.
func Forms(word string) []string {
	var out []string
	if strings.ContainsRune(word, stressMark) {
		word = strings.ReplaceAll(word, string(stressMark), "")
		out = append(out, word)
	}
	if strings.ContainsRune(word, 'ё') {
		return out
	}
	runes := []rune(word)
	for i, r := range runes {
		if r != 'е' && r != 'Е' {
			continue
		}
		if len(out) >= maxYoCandidates {
			break
		}
		yo := 'ё'
		if r == 'Е' {
			yo = 'Ё'
		}
		cand := slices.Clone(runes)
		cand[i] = yo
		out = append(out, string(cand))
	}
	return out
}
