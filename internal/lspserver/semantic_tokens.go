package lspserver

import (
	"natural-syntax-ls/internal/tokenmap"
)

// encodeSemanticTokens encodes filtered tokens as a flat []uint32 of LSP semantic-token 5-tuples: deltaLine, deltaStart, length, tokenType, modifiers.
func encodeSemanticTokens(doc *document, tm *tokenmap.Map, useDeprel bool) []uint32 {
	lines := doc.lineIndex()
	result := make([]uint32, 0, len(doc.tokens)*5)
	prevLine, prevCol := 0, 0
	for _, tok := range doc.tokens {
		var bits tokenmap.Bits
		if useDeprel {
			bits = tokenmap.DeprelBits(tok.Deprel)
		} else if b := tm.Get(tok.Tag); b != nil {
			bits = *b
		} else {
			continue
		}
		line, col := lines.position(int(tok.OffsetBegin))
		deltaStart := col
		if line == prevLine {
			deltaStart -= prevCol
		}
		result = append(result,
			uint32(line-prevLine),
			uint32(deltaStart),
			uint32(lines.utf16Len(int(tok.OffsetBegin), int(tok.OffsetEnd))),
			bits.TokenType,
			bits.TokenModifierBitset,
		)
		prevLine, prevCol = line, col
	}
	return result
}
