//go:build !english

package builtin

import (
	"natural-syntax-ls/internal/lang"
	"natural-syntax-ls/internal/lang/chinese"
	"natural-syntax-ls/internal/lang/japanese"
)

// native lists the languages whose hooks need Go: kagome for Japanese, the conversion tables for Chinese.
func native() []lang.Language {
	return []lang.Language{japanese.Language(), chinese.Language()}
}
