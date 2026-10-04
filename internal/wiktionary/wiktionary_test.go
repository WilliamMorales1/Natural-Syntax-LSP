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
		postag.NNS: "Noun", postag.VBZ: "Verb", postag.MD: "Verb", postag.JJR: "Adjective",
		postag.TO: "Preposition", postag.PRP: "Pronoun", postag.SYM: "",
	} {
		if got := posCategory(p); got != want {
			t.Errorf("posCategory(%v) = %q, want %q", p, got, want)
		}
	}
	for d, want := range map[postag.Deprel]string{
		postag.DepNsubj: "Noun", postag.DepAmod: "Adjective", postag.DepCase: "Preposition", postag.DepPunct: "",
	} {
		if got := deprelCategory(d); got != want {
			t.Errorf("deprelCategory(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestFetchDefSentenceInitialVerb(t *testing.T) {
	def := func(pos, text string) entry {
		return entry{PartOfSpeech: pos, Language: "English", Definitions: []sense{{text}}}
	}
	pages := map[string]map[string][]entry{
		"Said": {"en": {def("Proper noun", "A male given name from Arabic."), def("Noun", "plural of Sa")}},
		"said": {"en": {def("Verb", "simple past and past participle of say"), def("Adjective", "mentioned")}},
		"make": {"en": {def("Verb", "To create.")}},
		"sure": {"en": {def("Adjective", "Certain.")}},
		"to":   {"en": {def("Preposition", "Toward.")}},
	}
	orig := fetchPayload
	t.Cleanup(func() { fetchPayload = orig })
	fetchPayload = func(word string) (map[string][]entry, error) {
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
		{"Said", postag.VBD, "simple past and past participle of say", "https://en.wiktionary.org/wiki/said"},
		{"to", postag.TO, "Toward.", "https://en.wiktionary.org/wiki/to"},
		{"make", postag.VB, "To create.", "https://en.wiktionary.org/wiki/make"},
		{"sure", postag.JJ, "Certain.", "https://en.wiktionary.org/wiki/sure"},
		{"Said", postag.NNP, "A male given name from Arabic.", "https://en.wiktionary.org/wiki/Said"},
	} {
		got, url, ok := fetchDefUncached(tc.word, tc.pos, postag.DepRoot)
		if !ok || got != tc.wantDef || url != tc.wantURL {
			t.Errorf("fetchDefUncached(%q, %v) = %q, %q, %v; want %q, %q", tc.word, tc.pos, got, url, ok, tc.wantDef, tc.wantURL)
		}
	}
}
