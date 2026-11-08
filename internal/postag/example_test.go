package postag_test

import (
	"fmt"

	"natural-syntax-ls/internal/postag"
)

// Labels from every supported tagset decode to one Penn tag per Universal class; a label no inventory knows decodes as O with ok false, so a model's label file should be checked once.
func ExampleParseLabel() {
	for _, label := range []string{"NNS", "NOUN", "B-VERB", "I-NOUN", "VERB+AUX", "NCMS000", "PERIODCATEGORY", "Na", "LABEL_3"} {
		pos, ok := postag.ParseLabel(label)
		fmt.Printf("%-8s %-3s %-5v %s\n", label, pos, ok, pos.Description())
	}
	// Output:
	// NNS      NNS true  Noun
	// NOUN     NN  true  Noun
	// B-VERB   VB  true  Verb
	// I-NOUN   NN  true  Noun
	// VERB+AUX VB  true  Verb
	// NCMS000  NN  true  Noun
	// PERIODCATEGORY O   true  Other
	// Na       NN  true  Noun
	// LABEL_3  O   false Other
}
