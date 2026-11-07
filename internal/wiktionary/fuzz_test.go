package wiktionary

import "testing"

func FuzzStripHTML(f *testing.F) {
	f.Add("<b>bold</b> &amp; <style>x</style>&#65;")
	f.Fuzz(func(t *testing.T, s string) { _ = stripHTML(s) })
}
