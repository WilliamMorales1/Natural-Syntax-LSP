// Package english holds English-specific Wiktionary rules.
package english

import (
	"strings"

	"natural-syntax-ls/internal/lang"
	"natural-syntax-ls/internal/postag"
)

// Language returns the English hooks.
func Language() lang.Language {
	return lang.Language{Code: "en", Wiktionary: &lang.Wiktionary{Code: "en", Sections: []string{"English", "Translingual"}, Rewrite: rewrite}}
}

// rewrite sends the possessive clitic to the "-'s" suffix entry, the only page that defines it.
func rewrite(word string, pos postag.PartOfSpeech, deprel postag.Deprel) string {
	if pos == postag.POS || (deprel == postag.DepCase && strings.ToLower(word) == "'s") {
		return "-'s"
	}
	return ""
}
