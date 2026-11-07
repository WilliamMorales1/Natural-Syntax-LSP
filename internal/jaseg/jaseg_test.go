//go:build !nojaseg && !english

package jaseg

import (
	"slices"
	"testing"
)

func TestSegmentUniDicWords(t *testing.T) {
	run := "キツネは古いコンピューターの近くでパンを見つけて"
	want := []string{"キツネ", "は", "古い", "コンピューター", "の", "近く", "で", "パン", "を", "見つけ", "て"}
	runes := []rune(run)
	var got []string
	pos := 0
	for _, r := range Segment(run) {
		if r[0] != pos {
			t.Fatalf("gap before %v", r)
		}
		got = append(got, string(runes[r[0]:r[1]]))
		pos = r[1]
	}
	if pos != len(runes) || !slices.Equal(got, want) {
		t.Errorf("Segment = %q, want %q", got, want)
	}
}

func TestLemmas(t *testing.T) {
	for in, want := range map[string]string{"いただい": "いただく", "持ち込ん": "持ち込む", "素晴らしく": "素晴らしい"} {
		if got := Lemmas(in); len(got) == 0 || got[0] != want {
			t.Errorf("Lemmas(%q) = %q, want %q", in, got, want)
		}
	}
}
