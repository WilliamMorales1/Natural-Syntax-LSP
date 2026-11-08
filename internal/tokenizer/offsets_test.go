package tokenizer

import (
	"slices"
	"testing"
)

func TestCurlyApostropheMatchesStraight(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"it’s the dog’s bone", []string{"it", "’s", "the", "dog", "’s", "bone"}},
		{"don’t stop", []string{"don’t", "stop"}},
		{"‘quoted’ and “double”", []string{"‘", "quoted", "’", "and", "“", "double", "”"}},
	} {
		if got := texts(BasicTokenize(tc.in)); !slices.Equal(got, tc.want) {
			t.Errorf("BasicTokenize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSpanishFrenchPunctuation(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"¿Qué? ¡Hola!", []string{"¿", "Qué", "?", "¡", "Hola", "!"}},
		{"peut-être «oui»", []string{"peut", "-", "être", "«", "oui", "»"}},
	} {
		for name, got := range map[string][]string{"basic": texts(BasicTokenize(tc.in)), "unigram": texts(tinyTokenizer(t).Tokenize(tc.in))} {
			if !slices.Equal(got, tc.want) {
				t.Errorf("%s(%q) = %q, want %q", name, tc.in, got, tc.want)
			}
		}
	}
}

// TestOffsetsSurviveMidDocumentEdit checks that inserting text shifts later spans by the inserted rune count and leaves earlier ones alone.
func TestOffsetsSurviveMidDocumentEdit(t *testing.T) {
	tok := tinyTokenizer(t)
	before := "école é ñandú。（好）\nsegunda línea ”fin”"
	insert := "日本語 "
	at := []rune(before)
	cut := 6
	after := string(at[:cut]) + insert + string(at[cut:])
	n := uint32(len([]rune(insert)))
	for name, tk := range map[string]func(string) []WordSpan{"basic": BasicTokenize, "unigram": tok.Tokenize} {
		for _, w := range tk(before) {
			runes := []rune(before)
			if string(runes[w.Begin:w.End]) != w.Text {
				t.Errorf("%s before: span %+v does not slice back", name, w)
			}
		}
		for _, w := range tk(after) {
			runes := []rune(after)
			if string(runes[w.Begin:w.End]) != w.Text {
				t.Errorf("%s after: span %+v does not slice back", name, w)
			}
		}
		last := func(ws []WordSpan) WordSpan { return ws[len(ws)-1] }
		b, a := last(tk(before)), last(tk(after))
		if a.Text != b.Text || a.Begin != b.Begin+n {
			t.Errorf("%s: last span %+v, want %q at %d", name, a, b.Text, b.Begin+n)
		}
	}
}

func TestFullWidthPunctuationUnigram(t *testing.T) {
	want := []string{"你好", "，", "世界", "！", "（", "好", "）"}
	if got := texts(tinyTokenizer(t).Tokenize("你好，世界！（好）")); !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
