package wiktionary

import (
	"testing"

	"natural-syntax-ls/internal/postag"
)

func TestStripHTML(t *testing.T) {
	for in, want := range map[string]string{
		`<span class="x">a <b>bold</b> word</span>`:      "a bold word",
		`<style>.x{color:red}</style>kept`:               "kept",
		"fish &amp; chips &mdash; &lt;tasty&gt; &bogus;": "fish & chips — <tasty> &bogus;",
		"  padded  ": "padded",
	} {
		if got := stripHTML(in); got != want {
			t.Errorf("stripHTML(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsNumeralGlyph(t *testing.T) {
	for in, want := range map[string]bool{"": false, "42": true, "4a": false, "1.5": false} {
		if got := isNumeralGlyph(in); got != want {
			t.Errorf("isNumeralGlyph(%q) = %v", in, got)
		}
	}
}

func TestCategories(t *testing.T) {
	for p, want := range map[postag.PartOfSpeech]string{
		postag.POS_NNS: "Noun", postag.POS_VBZ: "Verb", postag.POS_MD: "Verb", postag.POS_JJR: "Adjective",
		postag.POS_TO: "Preposition", postag.POS_PRP: "Pronoun", postag.POS_SYM: "",
	} {
		if got := posToWiktCategory(p); got != want {
			t.Errorf("posToWiktCategory(%v) = %q, want %q", p, got, want)
		}
	}
	for d, want := range map[postag.Deprel]string{
		postag.DEP_NSUBJ: "Noun", postag.DEP_AMOD: "Adjective", postag.DEP_CASE: "Preposition", postag.DEP_PUNCT: "",
	} {
		if got := deprelToWiktCategory(d); got != want {
			t.Errorf("deprelToWiktCategory(%v) = %q, want %q", d, got, want)
		}
	}
}
