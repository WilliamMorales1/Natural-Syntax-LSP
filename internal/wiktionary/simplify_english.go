//go:build english

package wiktionary

// Chinese script conversion is left out of English-only builds.
func simplified(s string) string { return s }

func traditional(s string) string { return s }
