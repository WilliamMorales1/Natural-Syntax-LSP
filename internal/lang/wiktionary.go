package lang

import (
	"unicode"

	"natural-syntax-ls/internal/postag"
)

// Wiktionary is one language's lookup behavior; a wiktionary.Client applies it when that language (or "auto") is selected.
type Wiktionary struct {
	// Code is the Wiktionary language code.
	Code string
	// Sections are the English Wiktionary L2 headings ("Spanish") whose entries belong to this code when read from wikitext.
	Sections []string
	// Rewrite returns the headword to query in place of word for a token, or "" to keep word.
	Rewrite func(word string, pos postag.PartOfSpeech, deprel postag.Deprel) string
	// Forms returns extra headwords to try after the word and its lowercase, such as dictionary or script variants.
	Forms func(word string) []string
	// Wikitext reads this language's sections from wikitext when the REST endpoint cannot parse them; only words containing Script are checked.
	Wikitext bool
	Script   *unicode.RangeTable
}
