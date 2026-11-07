package wiktionary

import (
	"context"
	"net/http"
	"testing"
	"time"

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
	t.Parallel()
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
	c := NewClient()
	c.fetch = func(_ context.Context, word string) (map[string][]entry, error) {
		if p, ok := pages[word]; ok {
			return p, nil
		}
		return nil, ErrNotFound
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
		got, url, ok, _ := c.lookup(t.Context(), tc.word, "en", "", tc.pos, postag.DepRoot)
		if !ok || got != tc.wantDef || url != tc.wantURL {
			t.Errorf("lookup(t.Context(), %q, %v) = %q, %q, %v; want %q, %q", tc.word, tc.pos, got, url, ok, tc.wantDef, tc.wantURL)
		}
	}
}

func TestLanguageSelection(t *testing.T) {
	t.Parallel()
	ent := func(lang, pos, text string) entry {
		return entry{PartOfSpeech: pos, Language: lang, Definitions: []sense{{text}}}
	}
	pages := map[string]map[string][]entry{
		"a": {
			"en":    {ent("English", "Letter", "The first letter.")},
			"es":    {ent("Spanish", "Preposition", "to, at")},
			"other": {ent("Zulu", "Noun", "zulu noun")},
		},
		"7": {"en": {ent("Translingual", "Symbol", "The digit seven.")}},
		"私": {"ja": {ent("Japanese", "Pronoun", "I")}},
	}
	c := NewClient()
	c.fetch = func(_ context.Context, word string) (map[string][]entry, error) {
		if p, ok := pages[word]; ok {
			return p, nil
		}
		return nil, ErrNotFound
	}

	for _, tc := range []struct {
		name, word, lang, model string
		pos                     postag.PartOfSpeech
		want, wantURL           string
		ok                      bool
	}{
		{"en default", "a", "en", "", postag.IN, "The first letter.", "https://en.wiktionary.org/wiki/a", true},
		{"es fixed", "a", "es", "", postag.IN, "*(Spanish)* to, at", "https://en.wiktionary.org/wiki/a#Spanish", true},
		{"missing bucket", "a", "fr", "", postag.IN, "", "", false},
		{"auto model first", "a", Auto, "es", postag.IN, "*(Spanish)* to, at", "https://en.wiktionary.org/wiki/a#Spanish", true},
		{"auto en first", "a", Auto, "", postag.NN, "The first letter.", "https://en.wiktionary.org/wiki/a", true},
		{"auto skips other", "a", Auto, "", postag.O, "The first letter.", "https://en.wiktionary.org/wiki/a", true},
		{"translingual under es", "7", "es", "", postag.CD, "The digit seven.", "https://en.wiktionary.org/wiki/7", true},
		{"non-latin", "私", "ja", "", postag.PRP, "*(Japanese)* I", "https://en.wiktionary.org/wiki/%E7%A7%81#Japanese", true},
	} {
		got, url, ok, _ := c.lookup(t.Context(), tc.word, tc.lang, tc.model, tc.pos, postag.DepRoot)
		if ok != tc.ok || got != tc.want || url != tc.wantURL {
			t.Errorf("%s: got %q, %q, %v; want %q, %q, %v", tc.name, got, url, ok, tc.want, tc.wantURL, tc.ok)
		}
	}
}

func TestCacheSeparatesLanguages(t *testing.T) {
	t.Parallel()
	pages := map[string][]entry{
		"en": {{PartOfSpeech: "Noun", Language: "English", Definitions: []sense{{"english sense"}}}},
		"es": {{PartOfSpeech: "Noun", Language: "Spanish", Definitions: []sense{{"sentido"}}}},
	}
	c := NewClient()
	c.fetch = func(context.Context, string) (map[string][]entry, error) { return pages, nil }
	en, _, _ := c.FetchDef(t.Context(), "cachetestword", "en", "", postag.NN, postag.DepRoot)
	es, _, _ := c.FetchDef(t.Context(), "cachetestword", "es", "", postag.NN, postag.DepRoot)
	if en == es || en != "english sense" {
		t.Errorf("cache mixed languages: en=%q es=%q", en, es)
	}
}

func TestParticipleMatchesVerbAndAdjective(t *testing.T) {
	entries := []entry{
		{PartOfSpeech: "Noun", Language: "Spanish", Definitions: []sense{{"a noun"}}},
		{PartOfSpeech: "Participle", Language: "Spanish", Definitions: []sense{{"past participle of recibir"}}},
	}
	for _, target := range []string{"Verb", "Adjective"} {
		def, _, matched := pickDef(entries, target, false)
		if !matched || def != "past participle of recibir" {
			t.Errorf("%s: got %q, %v", target, def, matched)
		}
	}
}

func TestNegativeCache(t *testing.T) {
	t.Parallel()
	now := time.Unix(1000, 0)
	c := NewClient()
	c.now = func() time.Time { return now }
	var calls int
	err := error(ErrNotFound)
	c.fetch = func(context.Context, string) (map[string][]entry, error) {
		calls++
		return nil, err
	}
	look := func() bool {
		_, _, ok := c.FetchDef(t.Context(), "nowordhere", "en", "", postag.NN, postag.DepRoot)
		return ok
	}

	look()
	look()
	if calls != 1 {
		t.Fatalf("404 not cached: %d fetches", calls)
	}
	now = now.Add(missTTL + time.Second)
	look()
	if calls != 2 {
		t.Errorf("miss did not expire: %d fetches", calls)
	}

	err = &StatusError{Word: "x", StatusCode: http.StatusTooManyRequests, Status: "429 Too Many Requests"}
	c.misses = map[string]time.Time{}
	calls = 0
	look()
	look()
	if calls != 2 {
		t.Errorf("transient failure cached: %d fetches", calls)
	}
}
