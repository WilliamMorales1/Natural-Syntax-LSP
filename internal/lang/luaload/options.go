package luaload

import (
	"fmt"
	"strings"
	"unicode"

	lua "github.com/yuin/gopher-lua"

	"natural-syntax-ls/internal/lang/arabic"
	"natural-syntax-ls/internal/tokenizer"
)

// tokenizerOptions maps the closed set of named tokenizer flags a script may select; the work stays in Go.
func tokenizerOptions(t *lua.LTable) ([]tokenizer.Option, error) {
	if err := checkKeys(t, "tokenizer.", "elisions", "arabic_clitics"); err != nil {
		return nil, err
	}
	var opts []tokenizer.Option
	switch v := t.RawGetString("arabic_clitics").(type) {
	case lua.LBool:
		if v {
			opts = append(opts, tokenizer.WithVocabWordMerger(arabic.CliticMerger))
		}
	case *lua.LNilType:
	default:
		return nil, fmt.Errorf("field tokenizer.arabic_clitics: must be a boolean")
	}
	switch v := t.RawGetString("elisions").(type) {
	case *lua.LTable:
		if err := checkKeys(v, "tokenizer.elisions.", "clitics", "words"); err != nil {
			return nil, err
		}
		clitics, err := strList(v, "clitics")
		if err != nil {
			return nil, fmt.Errorf("tokenizer.elisions: %w", err)
		}
		words, err := strList(v, "words")
		if err != nil {
			return nil, fmt.Errorf("tokenizer.elisions: %w", err)
		}
		opts = append(opts, tokenizer.WithWordMerger(elisionMerger(clitics, words)))
	case *lua.LNilType:
	default:
		return nil, fmt.Errorf("field tokenizer.elisions: must be a table")
	}
	return opts, nil
}

// elisionMerger glues a listed clitic to its apostrophe ("l ' ami" becomes "l'", "ami") and rejoins listed apostrophe words ("aujourd'hui").
func elisionMerger(clitics, words []string) tokenizer.WordMerger {
	cl := make(map[string]bool, len(clitics))
	for _, c := range clitics {
		cl[strings.ToLower(c)] = true
	}
	type pair struct{ head, tail string }
	joins := make(map[pair]bool, len(words))
	for _, w := range words {
		for _, ap := range []string{"'", "’"} {
			if head, tail, ok := strings.Cut(strings.ToLower(w), ap); ok {
				joins[pair{head, tail}] = true
			}
		}
	}
	return func(runes []rune, ws []tokenizer.WordSpan) []tokenizer.WordSpan {
		var out []tokenizer.WordSpan
		for i := 0; i < len(ws); i++ {
			w := ws[i]
			if i+2 < len(ws) {
				ap, next := ws[i+1], ws[i+2]
				joined := (ap.Text == "'" || ap.Text == "’") && ap.Begin == w.End && next.Begin == ap.End
				switch {
				case joined && joins[pair{strings.ToLower(w.Text), strings.ToLower(next.Text)}]:
					w.Text, w.End = string(runes[w.Begin:next.End]), next.End
					i += 2
				case joined && cl[strings.ToLower(w.Text)] && unicode.IsLetter(runes[next.Begin]):
					w.Text, w.End = w.Text+ap.Text, ap.End
					i++
				}
			}
			out = append(out, w)
		}
		return out
	}
}
