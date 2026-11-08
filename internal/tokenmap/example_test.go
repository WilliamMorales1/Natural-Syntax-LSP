package tokenmap_test

import (
	"fmt"

	"natural-syntax-ls/internal/postag"
	"natural-syntax-ls/internal/tokenmap"
)

// Every tag of a Universal class gets the class's token type, whether it comes from a Penn or a UD model.
func ExampleNewDefault() {
	m := tokenmap.NewDefault()
	for _, label := range []string{"NN", "NNS", "NOUN", "VBD", "SCONJ", "PRP$", "PUNCT"} {
		pos, _ := postag.ParseLabel(label)
		fmt.Printf("%-5s %-4s %-12s %s\n", label, pos, pos.Description(), m.Get(pos))
	}
	// Output:
	// NN    NN   Noun         variable
	// NNS   NNS  Noun         variable
	// NOUN  NN   Noun         variable
	// VBD   VBD  Verb         function
	// SCONJ IN   Adposition   operator
	// PRP$  PRP$ Pronoun      keyword (declaration)
	// PUNCT O    Other        comment (deprecated)
}

// Close relatives share a token type and differ by modifier.
func ExampleDeprelBits() {
	for _, name := range []string{"nsubj", "csubj", "obj", "iobj", "nsubj:pass", "root"} {
		rel, _ := postag.ParseDeprel(name)
		fmt.Printf("%-10s %s\n", name, tokenmap.DeprelBits(rel))
	}
	// Output:
	// nsubj      variable (declaration)
	// csubj      variable (definition)
	// obj        variable
	// iobj       variable (readonly)
	// nsubj:pass variable (declaration)
	// root       class (declaration)
}
