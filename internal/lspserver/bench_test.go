package lspserver

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"natural-syntax-ls/internal/tokenizer"
	"natural-syntax-ls/internal/tokenmap"
)

// benchText is docs/test.txt repeated into ~20 paragraphs, so a document spans many chunks.
func benchText(b *testing.B) string {
	b.Helper()
	raw, err := os.ReadFile("../../docs/test.txt")
	if err != nil {
		b.Fatal(err)
	}
	return strings.Repeat(string(raw)+"\n\n", 20)
}

func BenchmarkRegistryOpen(b *testing.B) {
	text := benchText(b)
	// One registry for every iteration, as in the server; a registry per iteration would skew GC pacing.
	reg := newDocumentRegistry(contextModel{})
	i := 0
	for b.Loop() {
		uri := fmt.Sprintf("file:///open%d.txt", i)
		syncTokensURI(reg, uri, text, 1)
		reg.discard(uri)
		i++
	}
}

func BenchmarkRegistryEdit(b *testing.B) {
	text := benchText(b)
	edited := text[:len(text)/2] + " zebra " + text[len(text)/2:]
	reg := newDocumentRegistry(contextModel{})
	syncTokens(reg, text, 0)
	v := int32(1)
	for b.Loop() {
		t := text
		if v%2 == 1 {
			t = edited
		}
		syncTokens(reg, t, v)
		v++
	}
}

func BenchmarkHover(b *testing.B) {
	text := benchText(b)
	lines := strings.Count(text, "\n")
	reg := newDocumentRegistry(contextModel{})
	syncTokens(reg, text, 1)
	reply := make(chan *hoverQueryResult, 1)
	i := 0
	for b.Loop() {
		reg.hover(testURI, uint32(i*37%lines), 3, reply)
		<-reply
		i++
	}
}

func BenchmarkEncodeSemanticTokens(b *testing.B) {
	text := benchText(b)
	toks, _ := contextModel{}.PredictChunk(tokenizer.BasicTokenize(text))
	doc := &document{text: text, tokens: toks}
	tm := tokenmap.NewDefault()
	for b.Loop() {
		encodeSemanticTokens(doc, &tm, false)
	}
}
