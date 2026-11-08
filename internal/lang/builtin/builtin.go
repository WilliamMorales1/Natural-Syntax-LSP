// Package builtin lists the languages the server starts with: the Go packages for languages that need native code, then every Lua script found in the given directories.
package builtin

import (
	"slices"

	"natural-syntax-ls/internal/lang"
	"natural-syntax-ls/internal/lang/luaload"
)

// order fixes the "auto" Wiktionary lookup order; other codes follow in load order.
var order = []string{"en", "fr", "es", "ru", "ar", "ja", "zh"}

// Languages returns the native languages, then the *.lua files found in dirs; a later definition replaces an earlier one with the same code.
func Languages(dirs ...string) []lang.Language {
	all := native()
	for _, d := range dirs {
		all = luaload.Merge(all, luaload.LoadDir(d))
	}
	slices.SortStableFunc(all, func(a, b lang.Language) int { return rank(a.Code) - rank(b.Code) })
	return all
}

func rank(code string) int {
	if i := slices.Index(order, code); i >= 0 {
		return i
	}
	return len(order)
}
