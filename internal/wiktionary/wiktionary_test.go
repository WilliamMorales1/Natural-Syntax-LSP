package wiktionary

import (
	"errors"
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

func TestFetchDefSentenceInitialVerb(t *testing.T) {
	def := func(pos, text string) wiktDef {
		d := wiktDef{PartOfSpeech: pos, Language: "English"}
		d.Definitions = append(d.Definitions, struct {
			Definition string `json:"definition"`
		}{text})
		return d
	}
	pages := map[string]map[string][]wiktDef{
		"Said": {"en": {def("Proper noun", "A male given name from Arabic."), def("Noun", "plural of Sa")}},
		"said": {"en": {def("Verb", "simple past and past participle of say"), def("Adjective", "mentioned")}},
		"make": {"en": {def("Verb", "To create.")}},
		"sure": {"en": {def("Adjective", "Certain.")}},
		"to":   {"en": {def("Preposition", "Toward.")}},
	}
	orig := fetchPayload
	t.Cleanup(func() { fetchPayload = orig })
	fetchPayload = func(word string) (map[string][]wiktDef, error) {
		if p, ok := pages[word]; ok {
			return p, nil
		}
		return nil, errors.New("not found")
	}

	// "Said to make sure": sentence-initial "Said" tagged VBD must get the verb sense of "said".
	for _, tc := range []struct {
		word    string
		pos     postag.PartOfSpeech
		wantDef string
		wantURL string
	}{
		{"Said", postag.POS_VBD, "simple past and past participle of say", "https://en.wiktionary.org/wiki/said"},
		{"to", postag.POS_TO, "Toward.", "https://en.wiktionary.org/wiki/to"},
		{"make", postag.POS_VB, "To create.", "https://en.wiktionary.org/wiki/make"},
		{"sure", postag.POS_JJ, "Certain.", "https://en.wiktionary.org/wiki/sure"},
		{"Said", postag.POS_NNP, "A male given name from Arabic.", "https://en.wiktionary.org/wiki/Said"},
	} {
		got, url, ok := fetchDefUncached(tc.word, tc.pos, postag.DEP_ROOT)
		if !ok || got != tc.wantDef || url != tc.wantURL {
			t.Errorf("fetchDefUncached(%q, %v) = %q, %q, %v; want %q, %q", tc.word, tc.pos, got, url, ok, tc.wantDef, tc.wantURL)
		}
	}
}
