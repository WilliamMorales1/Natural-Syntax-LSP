package french

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"natural-syntax-ls/internal/tokenizer"
)

// split mimics the generic split that cuts every apostrophe out of a word.
func split(text string, cuts ...int) []tokenizer.WordSpan {
	runes := []rune(text)
	var out []tokenizer.WordSpan
	prev := 0
	for _, c := range append(cuts, len(runes)) {
		if c > prev {
			out = append(out, tokenizer.WordSpan{Text: string(runes[prev:c]), Begin: uint32(prev), End: uint32(c)})
		}
		prev = c
	}
	return out
}

func TestMergeElisions(t *testing.T) {
	for _, tc := range []struct {
		in   string
		cuts []int
		want []string
	}{
		{"l'ami", []int{1, 2}, []string{"l'", "ami"}},
		{"L’ami", []int{1, 2}, []string{"L’", "ami"}},
		{"aujourd'hui", []int{7, 8}, []string{"aujourd'hui"}},
		{"d'accord", []int{1, 2}, []string{"d'", "accord"}},
		{"rock'n", []int{4, 5}, []string{"rock", "'", "n"}},
	} {
		runes := []rune(tc.in)
		var got []string
		for _, w := range MergeElisions(runes, split(tc.in, tc.cuts...)) {
			got = append(got, w.Text)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%q = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTokenizerKeepsElisions(t *testing.T) {
	const tiny = `{"model": {"type": "Unigram", "unk_id": 1, "vocab": [
  ["[CLS]", 0], ["[UNK]", 0], ["[SEP]", 0], ["l'", -4], ["ami", -5], ["a", -9], ["m", -9], ["i", -9]]},
 "pre_tokenizer": {"type": "Whitespace"},
 "post_processor": {"single": [{"SpecialToken": {"id": "[CLS]"}}, {"Sequence": {"id": "A"}}, {"SpecialToken": {"id": "[SEP]"}}]}}`
	path := filepath.Join(t.TempDir(), "m_tokenizer.json")
	if err := os.WriteFile(path, []byte(tiny), 0o600); err != nil {
		t.Fatal(err)
	}
	tok, err := tokenizer.Open(path, Language().Tokenizer...)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, w := range tok.Tokenize("l'ami d'accord aujourd'hui") {
		got = append(got, w.Text)
	}
	if want := []string{"l'", "ami", "d'", "accord", "aujourd'hui"}; !slices.Equal(got, want) {
		t.Errorf("Tokenize = %q, want %q", got, want)
	}
}
