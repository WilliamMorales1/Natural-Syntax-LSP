//go:build nojaseg || english

package japanese

// Builds with -tags nojaseg or english leave out the kagome dictionary.
const hasDictionary = false

func lemmas(string) []string { return nil }

func segment(string) [][2]int { return nil }
