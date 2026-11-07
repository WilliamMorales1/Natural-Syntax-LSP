//go:build english

package chinese

// Chinese script conversion is left out of English-only builds.
func Simplified(s string) string { return s }

func Traditional(s string) string { return s }
