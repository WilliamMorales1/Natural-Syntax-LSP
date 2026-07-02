package lspserver

import "natural-syntax-ls/internal/tokenmap"

// encodeSemanticTokens encodes filtered tokens as a flat []uint32 of LSP semantic-token 5-tuples: deltaLine, deltaStart, length, tokenType, modifiers.
func encodeSemanticTokens(doc *document, tm *tokenmap.Map, useDeprel bool) []uint32 {
	lineStarts := buildLineStarts([]rune(doc.text))

	result := make([]uint32, 0, len(doc.tokens)*5)

	prevLine := 0
	prevStart := 0

	for _, tok := range doc.tokens {
		var bits *tokenmap.Bits
		if useDeprel {
			b := tokenmap.DeprelBits(tok.Deprel)
			bits = &b
		} else {
			bits = tm.Get(tok.Tag)
		}
		if bits == nil {
			continue
		}
		charIdx := int(tok.OffsetBegin)
		line := charOffsetToLine(lineStarts, charIdx)
		col := charIdx - lineStarts[line]
		length := int(tok.OffsetEnd - tok.OffsetBegin)

		deltaLine := uint32(line - prevLine)
		var deltaStart uint32
		if deltaLine == 0 {
			deltaStart = uint32(col - prevStart)
		} else {
			deltaStart = uint32(col)
		}
		prevLine = line
		prevStart = col

		result = append(result,
			deltaLine,
			deltaStart,
			uint32(length),
			bits.TokenType,
			bits.TokenModifierBitset,
		)
	}
	return result
}

// buildLineStarts returns the char offset (within runes) of the start of each line.
func buildLineStarts(runes []rune) []int {
	n := len(runes)
	lineStarts := []int{0}
	for i, r := range runes {
		if r == '\n' && i+1 < n {
			lineStarts = append(lineStarts, i+1)
		}
	}
	return lineStarts
}

func charOffsetToLine(lineStarts []int, offset int) int {
	lo, hi := 0, len(lineStarts)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if lineStarts[mid] <= offset {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}
