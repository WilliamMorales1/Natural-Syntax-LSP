package inference

import (
	"strings"
	"testing"

	"natural-syntax-ls/internal/tokenizer"
)

func wordsOf(s string) []tokenizer.WordSpan {
	var out []tokenizer.WordSpan
	for i, f := range strings.Fields(s) {
		out = append(out, tokenizer.WordSpan{Text: f, Begin: uint32(i), End: uint32(i + 1)})
	}
	return out
}

func TestSplitSentencesScripts(t *testing.T) {
	for _, c := range []struct {
		name, in string
		want     []string
	}{
		{"english", "Go home . Stay . ok", []string{"Go home .", "Stay .", "ok"}},
		{"arabic question", "هل أنت هنا ؟ نعم", []string{"هل أنت هنا ؟", "نعم"}},
		{"arabic full stop", "ذهب الولد ۔ عاد", []string{"ذهب الولد ۔", "عاد"}},
		{"russian guillemets", "Он сказал « да . » Потом ушёл", []string{"Он сказал « да . »", "Потом ушёл"}},
		{"cjk closer", "今日 は 。 」 明日", []string{"今日 は 。 」", "明日"}},
		{"cjk enders", "あ ！ い ？ う", []string{"あ ！", "い ？", "う"}},
		{"arabic comma", "نعم ، لا", []string{"نعم ، لا"}},
	} {
		var got []string
		for _, s := range splitSentences(wordsOf(c.in)) {
			var ws []string
			for _, w := range s {
				ws = append(ws, w.Text)
			}
			got = append(got, strings.Join(ws, " "))
		}
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
