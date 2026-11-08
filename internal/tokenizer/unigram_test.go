package tokenizer

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

func tinyTokenizer(t *testing.T, opts ...Option) *UnigramTokenizer {
	t.Helper()
	tok, err := parseUnigram([]byte(tinyUnigram), opts...)
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
		{"l'ami", []string{"l", "'", "ami"}},
	} {
		if got := texts(tok.Tokenize(tc.in)); !slices.Equal(got, tc.want) {
			t.Errorf("Tokenize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// testSegmenter applies to text containing the marker rune 'は' and segments via seg.
type testSegmenter struct {
	seg    func(run string) [][2]int
	called *bool
}

func (s testSegmenter) Applies(text []rune) bool { return slices.Contains(text, 'は') }

func (s testSegmenter) Segment(run string) [][2]int {
	if s.called != nil {
		*s.called = true
	}
	return s.seg(run)
}

func TestSegmenterReplacesPiecesOnlyWhereItApplies(t *testing.T) {
	tok := tinyTokenizer(t, WithSegmenter(testSegmenter{seg: func(run string) [][2]int {
		if run == "コーヒー" {
			return [][2]int{{0, 4}}
		}
		return nil
	}}))
	if got := texts(tok.Tokenize("コーヒーは")); !slices.Equal(got, []string{"コーヒー", "は"}) {
		t.Errorf("applicable text = %q", got)
	}
	called := false
	tok = tinyTokenizer(t, WithSegmenter(testSegmenter{seg: func(string) [][2]int { return nil }, called: &called}))
	tok.Tokenize("今日")
	if called {
		t.Error("segmenter called for text it does not apply to")
	}
	// A segmentation that leaves a gap is ignored in favor of Unigram pieces.
	tok = tinyTokenizer(t, WithSegmenter(testSegmenter{seg: func(string) [][2]int { return [][2]int{{0, 1}} }}))
	if got := texts(tok.Tokenize("今日は晴れです")); !slices.Equal(got, []string{"今日", "は", "晴れ", "です"}) {
		t.Errorf("bad segmentation not rejected: %q", got)
	}
}

func TestWordMergersRunInOrder(t *testing.T) {
	join := func(runes []rune, words []WordSpan) []WordSpan {
		if len(words) >= 3 && words[1].Text == "'" {
			words[0].Text, words[0].End = words[0].Text+"'", words[1].End
			return append([]WordSpan{words[0]}, words[2:]...)
		}
		return words
	}
	tok := tinyTokenizer(t, WithWordMerger(join))
	if got := texts(tok.Tokenize("l'ami")); !slices.Equal(got, []string{"l'", "ami"}) {
		t.Errorf("merged = %q", got)
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
	enc := EncodeWords(tok, []WordSpan{{Text: "今日"}, {Text: "は"}})
	ids, mask, wordIdx, first := enc.InputIDs, enc.AttMask, enc.SwWordIdx, enc.SwIsFirst
	if !slices.Equal(ids, []int64{0, 3, 6, 2}) || !slices.Equal(mask, []int64{1, 1, 1, 1}) ||
		!slices.Equal(wordIdx, []int{-1, 0, 1, -1}) || !slices.Equal(first, []bool{false, true, true, false}) {
		t.Errorf("EncodeWords = %v %v %v %v", ids, mask, wordIdx, first)
	}
}

const tinyMetaspace = `{
 "model": {"type": "Unigram", "unk_id": 1, "vocab": [
  ["<s>", 0], ["<unk>", 0], ["</s>", 0],
  ["▁キ", -5], ["ツ", -5], ["ネ", -5], ["▁ツ", -5], ["▁ネ", -5], ["▁", -3]
 ]},
 "pre_tokenizer": {"type": "Metaspace"},
 "post_processor": {"type": "TemplateProcessing", "single": [
  {"SpecialToken": {"id": "<s>", "type_id": 0}}, {"Sequence": {"id": "A", "type_id": 0}}, {"SpecialToken": {"id": "</s>", "type_id": 0}}]}
}`

func TestEncodeWordsContiguousDropsMarkInsideUnspacedText(t *testing.T) {
	tok, err := parseUnigram([]byte(tinyMetaspace))
	if err != nil {
		t.Fatal(err)
	}
	glued := []WordSpan{{Text: "キ", Begin: 0, End: 1}, {Text: "ツ", Begin: 1, End: 2}, {Text: "ネ", Begin: 2, End: 3}}
	spaced := []WordSpan{{Text: "キ", Begin: 0, End: 1}, {Text: "ツ", Begin: 2, End: 3}, {Text: "ネ", Begin: 4, End: 5}}
	for _, tc := range []struct {
		name  string
		words []WordSpan
		want  []int64
	}{
		{"glued", glued, []int64{0, 3, 4, 5, 2}},
		{"spaced", spaced, []int64{0, 3, 6, 7, 2}},
	} {
		enc := EncodeWordsContiguous(tok, tc.words)
		ids, idx := enc.InputIDs, enc.SwWordIdx
		if !slices.Equal(ids, tc.want) || !slices.Equal(idx, []int{-1, 0, 1, 2, -1}) {
			t.Errorf("%s: ids %v idx %v, want %v", tc.name, ids, idx, tc.want)
		}
	}
	if ids := EncodeWords(tok, glued).InputIDs; !slices.Equal(ids, []int64{0, 3, 6, 7, 2}) {
		t.Errorf("EncodeWords must keep a mark on every word, got %v", ids)
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

func TestLanguageScopedMergerFollowsDetectedLine(t *testing.T) {
	join := func(runes []rune, words []WordSpan) []WordSpan {
		var out []WordSpan
		for k := 0; k < len(words); k++ {
			w := words[k]
			if k+1 < len(words) && words[k+1].Text == "'" {
				w.Text, w.End = w.Text+"'", words[k+1].End
				k++
			}
			out = append(out, w)
		}
		return out
	}
	detect := func(s string) string {
		switch {
		case strings.Contains(s, "FR"):
			return "fr"
		case strings.Contains(s, "EN"):
			return "en"
		}
		return ""
	}
	tok := tinyTokenizer(t, WithDetector(detect), WithLanguage("fr", WithWordMerger(join)))
	if got := texts(tok.Tokenize("FR l'ami\nEN l'ami")); !slices.Equal(got, []string{"FR", "l'", "ami", "EN", "l", "'", "ami"}) {
		t.Errorf("mixed doc = %q", got)
	}
	// A line too short to tell takes its predecessor's language, not the whole text's (fr here).
	if got := texts(tok.Tokenize("FR l'ami\nEN x\nl'ami")); !slices.Equal(got[len(got)-3:], []string{"l", "'", "ami"}) {
		t.Errorf("undetected line after EN = %q", got)
	}
	// An undetectable text keeps the old behavior of running every merger.
	if got := texts(tok.Tokenize("l'ami")); !slices.Equal(got, []string{"l'", "ami"}) {
		t.Errorf("undetected = %q", got)
	}
}

func TestVocabWordMergerScoresWholeWordPieces(t *testing.T) {
	type got struct {
		score float64
		ok    bool
	}
	var seen map[string]got
	build := func(score VocabScore) WordMerger {
		seen = map[string]got{}
		for _, w := range []string{"キ", "キツ"} {
			s, ok := score(w)
			seen[w] = got{s, ok}
		}
		return func(_ []rune, words []WordSpan) []WordSpan { return words[:1] }
	}
	tok, err := parseUnigram([]byte(tinyMetaspace), WithVocabWordMerger(build))
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]got{"キ": {-5, true}, "キツ": {0, false}}; !maps.Equal(seen, want) {
		t.Errorf("scores = %v, want %v", seen, want)
	}
	if words := tok.Tokenize("ab cd"); len(words) != 1 {
		t.Errorf("merger not applied: %v", words)
	}
}
