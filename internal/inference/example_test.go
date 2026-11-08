package inference

import (
	"fmt"
	"strings"

	"natural-syntax-ls/internal/tokenizer"
)

// Dependency mode parses one sentence at a time. Any lone Unicode Sentence_Terminal word ends one, and non-ASCII closing brackets and quotes after it stay with it; the period of an ordered-list marker does not.
func Example_sentences() {
	var words []tokenizer.WordSpan
	for i, w := range strings.Fields("1 . Hi . 「 はい 。 」 ok ؟ « да . » next") {
		words = append(words, tokenizer.WordSpan{Text: w, Begin: uint32(i), End: uint32(i + 1), BlockStart: i == 0})
	}
	for _, s := range splitSentences(words) {
		var texts []string
		for _, w := range s {
			texts = append(texts, w.Text)
		}
		fmt.Println(strings.Join(texts, " "))
	}
	// Output:
	// 1 . Hi .
	// 「 はい 。 」
	// ok ؟
	// « да . »
	// next
}
