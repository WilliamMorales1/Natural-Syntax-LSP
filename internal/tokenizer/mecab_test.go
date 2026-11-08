package tokenizer

import (
	"os"
	"path/filepath"
	"testing"
)

type fixedSegmenter struct{ ranges [][2]int }

func (fixedSegmenter) Applies([]rune) bool       { return true }
func (s fixedSegmenter) Segment(string) [][2]int { return s.ranges }

func TestMecabWordPieceUsesSegmenter(t *testing.T) {
	dir := t.TempDir()
	vocab := filepath.Join(dir, "m_vocab.txt")
	os.WriteFile(vocab, []byte("[UNK]\n[CLS]\n[SEP]\n私\nは\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "m_tokenizer.json"), []byte(`{"word_tokenizer_type":"mecab"}`), 0o644)

	seg, err := Open(vocab, WithSegmenter(fixedSegmenter{[][2]int{{0, 2}}}))
	if err != nil {
		t.Fatal(err)
	}
	if got := seg.Tokenize("私は"); len(got) != 1 || got[0].Text != "私は" || got[0].End != 2 {
		t.Errorf("segmented: %+v", got)
	}

	plain, err := Open(vocab)
	if err != nil {
		t.Fatal(err)
	}
	if got := plain.Tokenize("私は"); len(got) != 2 {
		t.Errorf("without a segmenter each character is a word: %+v", got)
	}
}
