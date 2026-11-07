// Package spanish names Spanish models and its Wiktionary section; Spanish needs no tokenizer or headword rules.
package spanish

import "natural-syntax-ls/internal/lang"

// Language returns the Spanish hooks.
func Language() lang.Language {
	return lang.Language{
		Code:          "es",
		ModelPrefixes: []string{"xlmr_es", "es_ancora"},
		Wiktionary:    &lang.Wiktionary{Code: "es", Sections: []string{"Spanish"}},
	}
}
