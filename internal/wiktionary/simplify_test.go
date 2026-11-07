//go:build !english

package wiktionary

import (
	"context"
	"errors"
	"testing"
)

func TestSimplified(t *testing.T) {
	for in, want := range map[string]string{"該國": "该国", "經": "经", "桌": "桌", "hello": "hello"} {
		if got := simplified(in); got != want {
			t.Errorf("simplified(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTraditional(t *testing.T) {
	for in, want := range map[string]string{"国家": "國家", "学习": "學習", "hello": "hello"} {
		if got := traditional(in); got != want {
			t.Errorf("traditional(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSimplifiedFallsBackToTraditionalPage(t *testing.T) {
	orig := fetchPayload
	t.Cleanup(func() { fetchPayload = orig })
	fetchPayload = func(_ context.Context, word string) (map[string][]entry, error) {
		if word == "電腦" {
			return map[string][]entry{"zh": {{PartOfSpeech: "Noun", Language: "Chinese", Definitions: []sense{{"computer"}}}}}, nil
		}
		return nil, errNoPage
	}
	def, _, ok := fetchDefUncached(t.Context(), "电脑", "zh", "", 0, 0)
	if !ok || def != "*(Chinese)* computer" {
		t.Errorf("got %q, %v", def, ok)
	}
}

func TestTraditionalFallsBackToSimplifiedPage(t *testing.T) {
	orig := fetchPayload
	t.Cleanup(func() { fetchPayload = orig })
	fetchPayload = func(_ context.Context, word string) (map[string][]entry, error) {
		if word == "电脑" {
			return map[string][]entry{"zh": {{PartOfSpeech: "Noun", Language: "Chinese", Definitions: []sense{{"computer"}}}}}, nil
		}
		return nil, errNoPage
	}
	def, _, ok := fetchDefUncached(t.Context(), "電腦", "zh", "", 0, 0)
	if !ok || def != "*(Chinese)* computer" {
		t.Errorf("got %q, %v", def, ok)
	}
}

var errNoPage = errors.New("no page")
