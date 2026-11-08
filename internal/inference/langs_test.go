package inference

import (
	"natural-syntax-ls/internal/lang"
	"natural-syntax-ls/internal/lang/builtin"
	"natural-syntax-ls/internal/tokenizer"
)

// testTokOpts is what the server passes models: every built-in language's tokenizer rules.
func testTokOpts() []tokenizer.Option { return lang.TokenizerOptions(builtin.Languages("../../data/lang"), "") }
