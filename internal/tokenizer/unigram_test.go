package tokenizer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

const tinyUnigram = `{
 "model": {"type": "Unigram", "unk_id": 1, "vocab": [
  ["[CLS]", 0], ["[UNK]", 0], ["[SEP]", 0],
  ["今日", -3], ["今", -6], ["日", -6], ["は", -2], ["晴れ", -4], ["晴", -7], ["れ", -5], ["です", -3],
  ["コーヒー", -4], ["コ", -8], ["ー", -8], ["ヒ", -8],
  ["l'", -4], ["ami", -5], ["a", -9], ["m", -9], ["i", -9]
 ]},
 "pre_tokenizer": {"type": "Whitespace"},
 "post_processor": {"type": "TemplateProcessing", "single": [
  {"SpecialToken": {"id": "[CLS]", "type_id": 0}}, {"Sequence": {"id": "A", "type_id": 0}}, {"SpecialToken": {"id": "[SEP]", "type_id": 0}}]}
}`

func tinyTokenizer(t *testing.T) *UnigramTokenizer {
	t.Helper()
	tok, err := parseUnigram([]byte(tinyUnigram))
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestUnigramTokenizeSegmentsCJKByPieces(t *testing.T) {
	tok := tinyTokenizer(t)
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"今日は晴れです。", []string{"今日", "は", "晴れ", "です", "。"}},
		{"コーヒー and 今日", []string{"コーヒー", "and", "今日"}},
		{"Hello, 今日!", []string{"Hello", ",", "今日", "!"}},
		{"l'ami", []string{"l'", "ami"}},
		{"L’ami", []string{"L’", "ami"}},
		{"aujourd'hui", []string{"aujourd'hui"}},
		{"d'accord", []string{"d'", "accord"}},
	} {
		if got := texts(tok.Tokenize(tc.in)); !slices.Equal(got, tc.want) {
			t.Errorf("Tokenize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestUnigramTokenizeOffsets(t *testing.T) {
	tok := tinyTokenizer(t)
	in := "ＡＢ 今日は\n晴れです 🎉コーヒー。"
	runes := []rune(in)
	var prev uint32
	for _, w := range tok.Tokenize(in) {
		if w.Begin < prev || w.End <= w.Begin || int(w.End) > len(runes) {
			t.Fatalf("bad span %+v after %d", w, prev)
		}
		if got := string(runes[w.Begin:w.End]); got != w.Text {
			t.Errorf("span %d-%d is %q, Text %q", w.Begin, w.End, got, w.Text)
		}
		prev = w.End
	}
}

func TestUnigramWordIDs(t *testing.T) {
	tok := tinyTokenizer(t)
	if got := tok.WordIDs("今日は"); !slices.Equal(got, []int64{3, 6}) {
		t.Errorf("今日は = %v", got)
	}
	// Unknown characters become one [UNK] each, never dropped.
	if got := tok.WordIDs("今X"); !slices.Equal(got, []int64{4, 1}) {
		t.Errorf("今X = %v", got)
	}
	if got := tok.WordIDs("XYZ"); !slices.Equal(got, []int64{1}) {
		t.Errorf("XYZ = %v, want one fused [UNK]", got)
	}
	ids, mask, _, wordIdx, first := EncodeWords(tok, []WordSpan{{Text: "今日"}, {Text: "は"}})
	if !slices.Equal(ids, []int64{0, 3, 6, 2}) || !slices.Equal(mask, []int64{1, 1, 1, 1}) ||
		!slices.Equal(wordIdx, []int{-1, 0, 1, -1}) || !slices.Equal(first, []bool{false, true, true, false}) {
		t.Errorf("EncodeWords = %v %v %v %v", ids, mask, wordIdx, first)
	}
}

func TestOpenPrefersUnigramSidecar(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "m_tokenizer.json"), []byte(tinyUnigram), 0o644); err != nil {
		t.Fatal(err)
	}
	tok, err := Open(filepath.Join(dir, "m_vocab.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tok.(*UnigramTokenizer); !ok {
		t.Fatalf("Open returned %T", tok)
	}
}

// TestUnigramMatchesHF checks WordIDs against ids from the real HF tokenizer.json (see NSLS_UNIGRAM_REF).
func TestUnigramMatchesHF(t *testing.T) {
	dir := os.Getenv("NSLS_UNIGRAM_REF")
	if dir == "" {
		t.Skip("NSLS_UNIGRAM_REF unset")
	}
	data, err := os.ReadFile(filepath.Join(dir, "ref.json"))
	if err != nil {
		t.Skip(err)
	}
	var ref map[string]map[string][]int64
	if err := json.Unmarshal(data, &ref); err != nil {
		t.Fatal(err)
	}
	for name, words := range ref {
		tok, err := LoadUnigram(filepath.Join(dir, "..", "hf", name, "tokenizer.json"))
		if err != nil {
			t.Skip(err)
		}
		for w, want := range words {
			got := tok.WordIDs(w)
			if !slices.Equal(got, want) {
				t.Errorf("%s %q: got %v (%q), want %v (%q)", name, w, got, tok.Pieces(got), want, tok.Pieces(want))
			}
		}
	}
}
