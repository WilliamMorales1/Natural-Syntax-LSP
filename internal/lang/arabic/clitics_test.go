package arabic

import (
	"slices"
	"testing"

	"natural-syntax-ls/internal/tokenizer"
)

var testVocab = map[string]float64{
	"الكتاب": -11, "كتاب": -12, "كتب": -12, "وزير": -11.7, "زير": -13.6, "فيها": -11.6,
	"يكون": -11.5, "سيكون": -13.3, "حكومة": -13.6, "قال": -11.1, "وقال": -11.9, "وصل": -13.0, "صل": -13.0,
	"عربي": -12, "عرب": -11,
}

func testScore(w string) (float64, bool) {
	s, ok := testVocab[w]
	return s, ok
}

func TestSplit(t *testing.T) {
	s := splitter{testScore}
	for _, tc := range []struct {
		word string
		want []string
	}{
		{"والكتاب", []string{"و", "الكتاب"}},
		{"وقال", []string{"و", "قال"}},        // the rest scores above the whole
		{"وزير", nil},                         // the whole scores above the rest
		{"وصل", nil},                          // a tie keeps the word
		{"للحكومة", []string{"ل", "الحكومة"}}, // the article's alef comes back
		{"بالكتاب", []string{"ب", "الكتاب"}},
		{"فيها", []string{"في", "ها"}},
		{"لهما", []string{"ل", "هما"}},
		{"كتابه", []string{"كتاب", "ه"}},
		{"حكومته", []string{"حكومت", "ه"}},
		{"وكتابها", []string{"و", "كتاب", "ها"}},
		{"سيكون", []string{"س", "يكون"}},
		{"الكتاب", nil},
		{"عربي", nil}, // a one-letter ي needs a listed host
		{"book", nil},
	} {
		var got []string
		for _, p := range s.split(tc.word) {
			got = append(got, p.text)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("split(%s) = %q, want %q", tc.word, got, tc.want)
		}
	}
}

func TestSplitKeepsMarksWithTheirLetter(t *testing.T) {
	words := []tokenizer.WordSpan{{Text: "وَكَتَبَ", Begin: 3, End: 11}}
	got := CliticMerger(testScore)(nil, words)
	want := []tokenizer.WordSpan{{Text: "وَ", Begin: 3, End: 5}, {Text: "كَتَبَ", Begin: 5, End: 11}}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
