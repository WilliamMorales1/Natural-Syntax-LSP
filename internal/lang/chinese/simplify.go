package chinese

import (
	_ "embed"
	"strings"
	"sync"
)

// trad2simp.tsv and simp2trad.tsv derive from OpenCC's TSCharacters.txt and STCharacters.txt (Apache-2.0, see NOTICE).
//
//go:embed trad2simp.tsv
var trad2simpData string

//go:embed simp2trad.tsv
var simp2tradData string

var (
	trad2simp = sync.OnceValue(func() map[rune]rune { return loadMap(trad2simpData) })
	simp2trad = sync.OnceValue(func() map[rune]rune { return loadMap(simp2tradData) })
)

func loadMap(data string) map[rune]rune {
	m := map[rune]rune{}
	for line := range strings.SplitSeq(data, "\n") {
		k, v, ok := strings.Cut(strings.TrimSuffix(line, "\r"), "\t")
		if ok {
			kr, vr := []rune(k), []rune(v)
			if len(kr) == 1 && len(vr) == 1 {
				m[kr[0]] = vr[0]
			}
		}
	}
	return m
}

// Simplified converts Traditional Chinese characters to Simplified ones; other text is unchanged.
func Simplified(s string) string { return convert(s, trad2simp()) }

// Traditional converts Simplified Chinese characters to their first-listed Traditional form; ambiguous ones (发, 后) can pick the wrong variant.
func Traditional(s string) string { return convert(s, simp2trad()) }

func convert(s string, m map[rune]rune) string {
	return strings.Map(func(r rune) rune {
		if v, ok := m[r]; ok {
			return v
		}
		return r
	}, s)
}
