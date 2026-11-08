// Package builtin lists the languages compiled into the server: Lua scripts embedded from internal/lang/scripts, plus the Go packages for languages that need native code.
package builtin

import (
	"log/slog"
	"slices"

	"natural-syntax-ls/internal/lang"
	"natural-syntax-ls/internal/lang/luaload"
	"natural-syntax-ls/internal/lang/scripts"
)

// order fixes the "auto" Wiktionary lookup order; other codes follow in load order.
var order = []string{"en", "fr", "es", "ru", "ar", "ja", "zh"}

// Languages returns every language in this build, then the *.lua files found in dirs; a later definition replaces an earlier one with the same code.
func Languages(dirs ...string) []lang.Language {
	scripted, errs := luaload.ParseFS(scripts.FS, ".")
	for _, err := range errs {
		slog.Error("embedded language script", "err", err)
	}
	all := luaload.Merge(scripted, native())
	slices.SortStableFunc(all, func(a, b lang.Language) int { return rank(a.Code) - rank(b.Code) })
	for _, d := range dirs {
		all = luaload.Merge(all, luaload.LoadDir(d))
	}
	return all
}

func rank(code string) int {
	if i := slices.Index(order, code); i >= 0 {
		return i
	}
	return len(order)
}
