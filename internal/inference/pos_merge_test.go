package inference

import (
	"testing"

	"natural-syntax-ls/internal/postag"
)

func TestMergeContinuations(t *testing.T) {
	toks := []postag.Token{
		{Word: "北", Tag: postag.NNP, Score: 0.9, OffsetBegin: 0, OffsetEnd: 1},
		{Word: "京", Tag: postag.NNP, Score: 0.7, OffsetBegin: 1, OffsetEnd: 2},
		{Word: "大", Tag: postag.NN, Score: 0.8, OffsetBegin: 2, OffsetEnd: 3},
		{Word: "学", Tag: postag.NN, Score: 0.95, OffsetBegin: 4, OffsetEnd: 5}, // gap: never merged
		{Word: "习", Tag: postag.NN, Score: 0.9, OffsetBegin: 5, OffsetEnd: 6},
	}
	got := mergeContinuations(toks, []bool{false, true, true, true, true})
	if len(got) != 2 {
		t.Fatalf("got %d tokens: %+v", len(got), got)
	}
	if got[0].Word != "北京大" || got[0].OffsetEnd != 3 || got[0].Score != 0.7 || got[0].Tag != postag.NNP {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].Word != "学习" || got[1].OffsetBegin != 4 || got[1].OffsetEnd != 6 {
		t.Errorf("second = %+v", got[1])
	}
}
