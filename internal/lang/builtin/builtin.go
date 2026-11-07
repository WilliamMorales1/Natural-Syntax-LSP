//go:build !english

// Package builtin lists the languages compiled into the server.
package builtin

import (
	"natural-syntax-ls/internal/lang"
	"natural-syntax-ls/internal/lang/arabic"
	"natural-syntax-ls/internal/lang/chinese"
	"natural-syntax-ls/internal/lang/english"
	"natural-syntax-ls/internal/lang/french"
	"natural-syntax-ls/internal/lang/japanese"
	"natural-syntax-ls/internal/lang/russian"
	"natural-syntax-ls/internal/lang/spanish"
)

// Languages returns every language in this build.
func Languages() []lang.Language {
	return []lang.Language{english.Language(), french.Language(), spanish.Language(), russian.Language(), arabic.Language(), japanese.Language(), chinese.Language()}
}
