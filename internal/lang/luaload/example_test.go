package luaload_test

import (
	"fmt"

	"natural-syntax-ls/internal/lang/luaload"
)

// A script returns one table; Parse turns it into the lang.Language the server uses. Hooks stay Lua functions called per lookup.
func ExampleParse() {
	l, err := luaload.Parse("spanish.lua", `return {
	code = "es",
	model_prefixes = { "xlmr_es", "es_ancora" },
	wiktionary = {
		code = "es",
		sections = { "Spanish" },
		forms = function(word) return { (string.gsub(word, "mente$", "")) } end,
	},
}`)
	if err != nil {
		panic(err)
	}
	fmt.Println(l.Code, l.ModelPrefixes, len(l.Tokenizer))
	fmt.Println(l.Wiktionary.Code, l.Wiktionary.Sections, l.Wiktionary.Forms("rápidamente"), l.Wiktionary.Rewrite == nil)

	_, err = luaload.Parse("bad.lua", `return {code = "es", modle_prefixes = {}}`)
	fmt.Println(err)
	// Output:
	// es [xlmr_es es_ancora] 0
	// es [Spanish] [rápida] true
	// bad.lua: unknown field modle_prefixes (allowed: code, model_prefixes, tokenizer, wiktionary)
}
