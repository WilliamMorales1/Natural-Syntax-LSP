package luaload_test

import (
	"fmt"

	"natural-syntax-ls/internal/lang/luaload"
	"natural-syntax-ls/internal/postag"
	"natural-syntax-ls/internal/tokenizer"
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

// Every schema field in one script. Tokenizer flags become tokenizer options; rewrite gets the Penn tag and UD relation as strings and returns a headword or nil.
func ExampleParse_schema() {
	l, err := luaload.Parse("french.lua", `return {
	code = "fr",
	model_prefixes = { "xlmr_fr", "fr_sequoia" },
	tokenizer = {
		elisions = {
			clitics = { "l", "d", "qu" },
			words = { "aujourd'hui" },
		},
	},
	wiktionary = {
		code = "fr",
		sections = { "French" },
		wikitext = true,
		script = "Latin",
		rewrite = function(word, pos, deprel)
			if pos == "DT" and deprel == "det" and word == "l'" then return "le" end
		end,
		forms = function(word) return { (string.gsub(word, "s$", "")) } end,
	},
}`)
	if err != nil {
		panic(err)
	}
	w := l.Wiktionary
	fmt.Println(l.Code, l.ModelPrefixes, len(l.Tokenizer), w.Sections, w.Wikitext, w.Script != nil)
	fmt.Printf("%q %q %v\n", w.Rewrite("l'", postag.DT, postag.DepDet), w.Rewrite("ami", postag.NN, postag.DepNsubj), w.Forms("amis"))

	vocab := `{"model": {"type": "Unigram", "unk_id": 1, "vocab": [["<s>", 0], ["<unk>", 0], ["</s>", 0], ["▁l'", -4], ["▁ami", -5]]},
 "pre_tokenizer": {"type": "Metaspace"},
 "post_processor": {"type": "TemplateProcessing", "single": [{"SpecialToken": {"id": "<s>"}}, {"Sequence": {"id": "A"}}, {"SpecialToken": {"id": "</s>"}}]}}`
	tok, err := tokenizer.ParseUnigram([]byte(vocab), l.Tokenizer...)
	if err != nil {
		panic(err)
	}
	for _, word := range tok.Tokenize("l'ami") {
		fmt.Println(word.Text, word.Begin, word.End)
	}
	// Output:
	// fr [xlmr_fr fr_sequoia] 1 [French] true true
	// "le" "" [ami]
	// l' 0 2
	// ami 2 5
}
