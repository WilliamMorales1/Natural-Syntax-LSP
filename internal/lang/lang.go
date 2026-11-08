// Package lang holds everything specific to one natural language: word segmentation and merging rules for the tokenizer, and headword rules for Wiktionary lookups.
// The generic packages (tokenizer, wiktionary, inference, lspserver) know nothing about any language; a Language bundles its hooks and the caller passes the set it wants.
package lang

import (
	"path/filepath"
	"slices"
	"strings"

	"natural-syntax-ls/internal/langid"
	"natural-syntax-ls/internal/tokenizer"
)

// Language is one language's hooks into the generic code. Every field but Code is optional, so a language with no special rules is just a name and its model prefixes.
//
// To add a language: write a Lua script (see docs/adding-a-language.md), or for native code a package exporting Language() listed in internal/lang/builtin/native.go. Nothing in the tokenizer, wiktionary, inference or lspserver packages changes.
type Language struct {
	// Code is the Wiktionary language code ("en", "ja").
	Code string
	// ModelPrefixes are lowercase model file-name prefixes that mark a model as this language's ("xlmr_fr").
	ModelPrefixes []string
	// Tokenizer options apply to Unigram model tokenizers.
	Tokenizer []tokenizer.Option
	// Wiktionary is the language's lookup behavior; nil when it needs none.
	Wiktionary *Wiktionary
}

// ForModel returns the code of the language whose ModelPrefixes match modelPath's file name, or "" for unknown or multilingual models.
func ForModel(langs []Language, modelPath string) string {
	base := strings.ToLower(filepath.Base(modelPath))
	for _, l := range langs {
		for _, p := range l.ModelPrefixes {
			if strings.HasPrefix(base, p) {
				return l.Code
			}
		}
	}
	return ""
}

// TokenizerOptions gathers the tokenizer options of langs. A language the detector knows gets its options only on text detected as that language; others apply everywhere.
// A non-empty forced code replaces detection: every text counts as that language.
func TokenizerOptions(langs []Language, forced string) []tokenizer.Option {
	known := langid.Languages()
	detect := langid.Detect
	if forced != "" {
		known = append(known, forced)
		detect = func(string) string { return forced }
	}
	opts := []tokenizer.Option{tokenizer.WithDetector(detect)}
	for _, l := range langs {
		if len(l.Tokenizer) == 0 {
			continue
		}
		if slices.Contains(known, l.Code) {
			opts = append(opts, tokenizer.WithLanguage(l.Code, l.Tokenizer...))
		} else {
			opts = append(opts, l.Tokenizer...)
		}
	}
	return opts
}

// WiktionaryLangs gathers the Wiktionary behavior of langs.
func WiktionaryLangs(langs []Language) []Wiktionary {
	var out []Wiktionary
	for _, l := range langs {
		if l.Wiktionary != nil {
			out = append(out, *l.Wiktionary)
		}
	}
	return out
}
