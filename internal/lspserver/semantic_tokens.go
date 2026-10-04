package lspserver

import (
	"slices"

	"natural-syntax-ls/internal/tokenmap"
)

// encodeSemanticTokens encodes filtered tokens as a flat []uint32 of LSP semantic-token 5-tuples: deltaLine, deltaStart, length, tokenType, modifiers.
func encodeSemanticTokens(doc *document, tm *tokenmap.Map, useDeprel bool) []uint32 {
	lines := lineCursor{starts: buildLineStarts([]rune(doc.text))}
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
			tok.OffsetEnd-tok.OffsetBegin,
			bits.TokenType,
			bits.TokenModifierBitset,
		)
		prevLine, prevCol = line, col
	}
	return result
}

// buildLineStarts returns the char offset (within runes) of the start of each line.
func buildLineStarts(runes []rune) []int {
	lineStarts := []int{0}
	for i, r := range runes {
		if r == '\n' && i+1 < len(runes) {
			lineStarts = append(lineStarts, i+1)
		}
	}
	return lineStarts
}

// charOffsetToLine returns the line containing offset, given lineStarts from buildLineStarts.
func charOffsetToLine(lineStarts []int, offset int) int {
	i, found := slices.BinarySearch(lineStarts, offset)
	if found {
		return i
	}
	return i - 1
}

// lineCursor converts rune offsets to line/column, scanning forward from the previous lookup since tokens arrive in order.
type lineCursor struct {
	starts []int
	line   int
}

func (c *lineCursor) position(offset int) (line, col int) {
	if offset < c.starts[c.line] {
		c.line = charOffsetToLine(c.starts, offset)
	}
	for c.line+1 < len(c.starts) && c.starts[c.line+1] <= offset {
		c.line++
	}
	return c.line, offset - c.starts[c.line]
}
