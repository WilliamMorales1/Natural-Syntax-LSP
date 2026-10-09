package tokenizer

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func texts(words []WordSpan) []string {
	out := make([]string, len(words))
	for i, w := range words {
		out[i] = w.Text
	}
	return out
}

func TestBasicTokenize(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   \n\t ", nil},
		{"Hello, world!", []string{"Hello", ",", "world", "!"}},
		{"the dog's bone", []string{"the", "dog", "'s", "bone"}},
		{"don't stop", []string{"don't", "stop"}},
		{"JAMES'S", []string{"JAMES", "'S"}},
		{"'quoted'", []string{"'", "quoted", "'"}},
		{"a+b=c", []string{"a", "+", "b", "=", "c"}},
		{"café naïve", []string{"café", "naïve"}},
		{"end...", []string{"end", ".", ".", "."}},
		{"line1\r\nline2", []string{"line1", "line2"}},
	} {
		if got := texts(BasicTokenize(tc.in)); !slices.Equal(got, tc.want) {
			t.Errorf("BasicTokenize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestBasicTokenizeOffsets checks every span's rune offsets slice back to its text, in order and without overlap.
func TestBasicTokenizeOffsets(t *testing.T) {
	for _, in := range []string{
		"The North Wind and the Sun.\n\nThey agreed — mostly.",
		"emoji 🎉 then «guillemets» and 日本語テキスト",
		"tabs\tand  double  spaces, it's   fine",
		"trailing newline\n",
	} {
		runes := []rune(in)
		var prevEnd uint32
		for _, w := range BasicTokenize(in) {
			if w.Begin < prevEnd || w.End <= w.Begin || int(w.End) > len(runes) {
				t.Fatalf("%q: bad span %+v after end %d", in, w, prevEnd)
			}
			if got := string(runes[w.Begin:w.End]); got != w.Text {
				t.Errorf("%q: span %d-%d is %q, Text %q", in, w.Begin, w.End, got, w.Text)
			}
			prevEnd = w.End
		}
	}
}

// TestBasicTokenizeBlockStart checks which words open a markdown block; want lists the text of each BlockStart word.
func TestBasicTokenizeBlockStart(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want []string
	}{
		{"plain prose", "one line\nwrapped onto two.", nil},
		{"dash list", "- apples\n- pears\n- plums", []string{"-", "-", "-"}},
		{"star and plus list", "* a\n+ b", []string{"*", "+"}},
		{"ordered list", "1. first\n2) second\n10. tenth", []string{"1", "2", "10"}},
		{"indented sublist", "- outer\n  - inner", []string{"-", "-"}},
		{"list item continuation", "- a long item\n  that wraps", []string{"-"}},
		{"lazy continuation", "- a long item\nthat wraps", []string{"-"}},
		{"heading then text", "# Title\nBody text here.", []string{"#", "Body"}},
		{"text then heading", "Body text\n## Next", []string{"#"}},
		{"setext heading", "Title\n=====\nBody", []string{"=", "Body"}},
		{"thematic break", "above\n---\nbelow", []string{"-", "below"}},
		{"spaced thematic break", "above\n* * *\nbelow", []string{"*", "below"}},
		{"quoted list", "> - a\n> - b", []string{">", ">"}},
		{"quote continuation", "> one line\n> wrapped", nil},
		{"table rows", "| a | b |\n|---|---|\n| c | d |", []string{"|", "|", "|"}},
		{"fenced code", "intro\n```go\nx := 1\ny := 2\n```\noutro", []string{"`", "x", "y", "`", "outro"}},
		{"hashtag is not heading", "#hashtag\n#tag", nil},
		{"seven hashes is not heading", "####### no", nil},
		{"dash without space is not list", "-5 degrees\n-ish", nil},
		{"number without marker", "2024 was\n2025 is", nil},
		{"crlf list", "- a\r\n- b", []string{"-", "-"}},
		{"leading blank lines", "\n\n- a\n- b", []string{"-", "-"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, w := range BasicTokenize(tc.in) {
				if w.BlockStart {
					got = append(got, w.Text)
				}
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("BasicTokenize(%q) block starts = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsAllPunct(t *testing.T) {
	for in, want := range map[string]bool{
		"":      false,
		".":     true,
		"...":   true,
		"—":     true,
		"+":     true,
		"a.":    false,
		"'s":    false,
		"hello": false,
	} {
		if got := IsAllPunct(in); got != want {
			t.Errorf("IsAllPunct(%q) = %v, want %v", in, got, want)
		}
	}
}

func loadVocab(t *testing.T, entries ...string) *BERTTokenizer {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vocab.txt")
	if err := os.WriteFile(path, []byte(strings.Join(entries, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tok, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestWordPiece(t *testing.T) {
	// Filler keeps the vocab big enough for Load's 1%-uppercase cased/uncased heuristic to call it uncased.
	entries := []string{"[PAD]", "[UNK]", "[CLS]", "[SEP]", "play", "##ing", "##s", "run", "x"}
	for i := range 200 {
		entries = append(entries, fmt.Sprintf("filler%d", i))
	}
	tok := loadVocab(t, entries...)
	for in, want := range map[string][]string{
		"play":    {"play"},
		"playing": {"play", "##ing"},
		"plays":   {"play", "##s"},
		"PLAYING": {"play", "##ing"}, // uncased vocab lowercases first
		"zzz":     {"[UNK]"},
		"runzz":   {"[UNK]"}, // a word with any unmatched piece is wholly unknown
	} {
		if got := tok.WordPiece(in); !slices.Equal(got, want) {
			t.Errorf("WordPiece(%q) = %q, want %q", in, got, want)
		}
	}
	if tok.CLSID() != 2 || tok.VocabID("run") != 7 || tok.VocabID("nope") != 1 {
		t.Errorf("ids: CLS %d run %d nope %d", tok.CLSID(), tok.VocabID("run"), tok.VocabID("nope"))
	}
}

func TestWordPieceCasedVocab(t *testing.T) {
	// Over 1% uppercase entries marks the vocab cased, so input keeps its case.
	tok := loadVocab(t, "[UNK]", "[CLS]", "[SEP]", "Paris", "paris", "Rome", "London")
	if got := tok.WordPiece("Paris"); !slices.Equal(got, []string{"Paris"}) {
		t.Errorf("cased WordPiece(Paris) = %q", got)
	}
}

func TestTokenizeWords(t *testing.T) {
	tok := loadVocab(t, "[PAD]", "[UNK]", "[CLS]", "[SEP]", "play", "##ing", "go")
	ids, mask, tti, wordIdx, isFirst := tok.TokenizeWords(BasicTokenize("playing go"))
	if want := []int64{2, 4, 5, 6, 3}; !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
	if want := []int{-1, 0, 0, 1, -1}; !slices.Equal(wordIdx, want) {
		t.Errorf("wordIdx = %v, want %v", wordIdx, want)
	}
	if want := []bool{false, true, false, true, false}; !slices.Equal(isFirst, want) {
		t.Errorf("isFirst = %v, want %v", isFirst, want)
	}
	for i := range ids {
		if mask[i] != 1 || tti[i] != 0 {
			t.Fatalf("mask/tti at %d = %d/%d", i, mask[i], tti[i])
		}
	}
}
