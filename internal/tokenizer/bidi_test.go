package tokenizer

import "testing"

func TestBasicTokenizeBidiControls(t *testing.T) {
	got := BasicTokenize("قرأت \u200fكتاب\u200e جديد \u202bمرحبا\u202c")
	want := []WordSpan{{Text: "قرأت", Begin: 0, End: 4}, {Text: "كتاب", Begin: 6, End: 10}, {Text: "جديد", Begin: 12, End: 16}, {Text: "مرحبا", Begin: 18, End: 23}}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		g := got[i]
		if g.Text != want[i].Text || g.Begin != want[i].Begin || g.End != want[i].End {
			t.Errorf("word %d: got %+v, want %+v", i, g, want[i])
		}
	}
}
