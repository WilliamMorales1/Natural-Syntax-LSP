package wiktionary

import (
	"context"
	"errors"
	"strings"
	"testing"

	nlang "natural-syntax-ls/internal/lang"
	"natural-syntax-ls/internal/postag"
)

var errNoPage = errors.New("no page")

func pageFetcher(pages map[string]map[string][]entry) func(context.Context, string) (map[string][]entry, error) {
	return func(_ context.Context, word string) (map[string][]entry, error) {
		if p, ok := pages[word]; ok {
			return p, nil
		}
		return nil, errNoPage
	}
}

func TestLangFormsAreTriedAfterWord(t *testing.T) {
	t.Parallel()
	zh := nlang.Wiktionary{Code: "zh", Forms: func(w string) []string { return []string{strings.ToUpper(w)} }}
	c := NewClient(zh)
	c.fetch = pageFetcher(map[string]map[string][]entry{
		"ABC": {"zh": {{PartOfSpeech: "Noun", Language: "Chinese", Definitions: []sense{{"alphabet"}}}}},
	})
	def, _, ok, _ := c.lookup(t.Context(), "abc", "zh", "", 0, 0)
	if !ok || def != "*(Chinese)* alphabet" {
		t.Errorf("got %q, %v", def, ok)
	}
}

func TestLangFormsSkippedForOtherLanguage(t *testing.T) {
	t.Parallel()
	zh := nlang.Wiktionary{Code: "zh", Forms: func(w string) []string { return []string{strings.ToUpper(w)} }}
	c := NewClient(zh)
	c.fetch = pageFetcher(map[string]map[string][]entry{
		"ABC": {"en": {{PartOfSpeech: "Noun", Language: "English", Definitions: []sense{{"alphabet"}}}}},
	})
	if _, _, ok, _ := c.lookup(t.Context(), "abc", "en", "", 0, 0); ok {
		t.Error("zh forms used for an English lookup")
	}
}

func TestLangRewriteReplacesWord(t *testing.T) {
	t.Parallel()
	en := nlang.Wiktionary{Code: "en", Rewrite: func(w string, pos postag.PartOfSpeech, _ postag.Deprel) string {
		if pos == postag.POS {
			return "-'s"
		}
		return ""
	}}
	c := NewClient(en)
	c.fetch = pageFetcher(map[string]map[string][]entry{
		"-'s": {"en": {{PartOfSpeech: "Suffix", Language: "English", Definitions: []sense{{"possessive"}}}}},
	})
	if _, _, ok, _ := c.lookup(t.Context(), "'s", "en", "", postag.POS, 0); !ok {
		t.Error("possessive not rewritten to -'s")
	}
}
