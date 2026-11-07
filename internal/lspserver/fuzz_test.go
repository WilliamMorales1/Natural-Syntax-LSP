package lspserver

import "testing"

func FuzzApplyContentChange(f *testing.F) {
	f.Add("one two\nthree", uint32(0), uint32(3), uint32(1), uint32(2), "x")
	f.Add("é😀z", uint32(0), uint32(1), uint32(0), uint32(3), "")
	f.Fuzz(func(t *testing.T, text string, sl, sc, el, ec uint32, repl string) {
		_, _ = applyContentChange(text, contentChangeRange{
			Start: documentPosition{Line: sl, Character: sc},
			End:   documentPosition{Line: el, Character: ec},
		}, repl)
	})
}
