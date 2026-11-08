// Package langid guesses the language of a text with a linear softmax model over hashed character n-grams and words, embedded in the binary.
// A detection hashes each n-gram once, so it costs microseconds per line.
package langid

// minLetters is the least text worth classifying; shorter input is mostly noise.
const minLetters = 4

// Languages returns the codes the model can tell apart.
func Languages() []string {
	m, err := loadLinear()
	if err != nil {
		return nil
	}
	return append([]string(nil), m.codes...)
}

// Detect returns the Wiktionary code of text's language, or "" when text is too short or too ambiguous to say.
func Detect(text string) string {
	m, err := loadLinear()
	if err != nil {
		return ""
	}
	code, _ := m.detect(text)
	return code
}
