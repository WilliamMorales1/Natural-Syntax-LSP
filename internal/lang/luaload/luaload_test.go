package luaload

import (
	"natural-syntax-ls/internal/lang"
	"strings"
	"testing"
	"time"
)

func TestSandboxForbidsGlobals(t *testing.T) {
	for _, g := range []string{"os", "io", "package", "debug", "require", "dofile", "loadfile", "load", "loadstring", "print", "collectgarbage", "setfenv", "coroutine"} {
		src := `if ` + g + ` ~= nil then error("` + g + ` visible") end return {code = "x"}`
		if _, err := Parse("t.lua", src); err != nil {
			t.Errorf("%s: %v", g, err)
		}
	}
	if _, err := Parse("t.lua", `return {code = string.dump ~= nil and "x" or "y"}`); err != nil {
		t.Error(err)
	}
}

func TestParseErrorsNameScriptAndField(t *testing.T) {
	for src, want := range map[string]string{
		`return {code = 1}`:                                               "t.lua: field code",
		`return {code = "x", modle_prefixes = {}}`:                        "unknown field modle_prefixes",
		`return {code = "x", wiktionary = {code = "x", forms = 1}}`:       "wiktionary.forms",
		`return {code = "x", wiktionary = {code = "x", script = "Nope"}}`: "unknown Unicode script",
		`return 5`:       "must return a table",
		`return {code =`: "t.lua",
		`error("boom")`:  "boom",
		`return {code="x", tokenizer = {elisions = {clitics = 1}}}`: "clitics",
		`return {code="x", tokenizer = {arabic_clitics = "yes"}}`:   "tokenizer.arabic_clitics",
	} {
		_, err := Parse("t.lua", src)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Parse(%q) error = %v, want substring %q", src, err, want)
		}
	}
}

func TestArabicCliticsFlag(t *testing.T) {
	for src, want := range map[string]int{
		`return {code = "ar", tokenizer = {arabic_clitics = true}}`:  1,
		`return {code = "ar", tokenizer = {arabic_clitics = false}}`: 0,
	} {
		l, err := Parse("ar.lua", src)
		if err != nil {
			t.Fatal(err)
		}
		if len(l.Tokenizer) != want {
			t.Errorf("%s: %d options, want %d", src, len(l.Tokenizer), want)
		}
	}
}

func TestLoadTimeout(t *testing.T) {
	start := time.Now()
	_, err := Parse("loop.lua", `while true do end`)
	if err == nil {
		t.Fatal("infinite loop at load must fail")
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %v", d)
	}
}

func TestHookFailureIsContained(t *testing.T) {
	l, err := Parse("h.lua", `return {code = "x", wiktionary = {code = "x", sections = {"X"},
  forms = function(w) while true do end end,
  rewrite = function(w) error("boom") end}}`)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if got := l.Wiktionary.Forms("a"); got != nil {
		t.Errorf("looping hook returned %q", got)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("hook not interrupted")
	}
	if got := l.Wiktionary.Rewrite("a", 0, 0); got != "" {
		t.Errorf("erroring hook returned %q", got)
	}
	// The state still works after both failures.
	l2, err := Parse("h.lua", `return {code = "x", wiktionary = {code = "x", forms = function(w) return {w .. "!"} end}}`)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if got := l2.Wiktionary.Forms("a"); len(got) != 1 || got[0] != "a!" {
			t.Errorf("Forms = %q", got)
		}
	}
}

func TestStringRepCapped(t *testing.T) {
	l, err := Parse("r.lua", `return {code = "x", wiktionary = {code = "x", forms = function(w) return {string.rep("x", 1e9)} end}}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := l.Wiktionary.Forms("a"); got != nil {
		t.Errorf("huge rep returned %d bytes", len(got[0]))
	}
}

func TestMergeByCode(t *testing.T) {
	a, _ := Parse("a.lua", `return {code = "x", model_prefixes = {"one"}}`)
	b, _ := Parse("b.lua", `return {code = "x", model_prefixes = {"two"}}`)
	got := Merge([]lang.Language{a}, []lang.Language{b})
	if len(got) != 1 || got[0].ModelPrefixes[0] != "two" {
		t.Errorf("Merge = %+v", got)
	}
}
