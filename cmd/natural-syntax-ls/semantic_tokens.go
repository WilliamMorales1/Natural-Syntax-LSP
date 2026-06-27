package main

// encodeSemanticTokens encodes filtered tokens as LSP semantic token deltas.
// Output is a flat []uint32 of 5-tuples: deltaLine, deltaStart, length, tokenType, modifiers.
func encodeSemanticTokens(doc *document, tm *TokenMap) []uint32 {
	runes := []rune(doc.text)
	n := len(runes)

	// Build line start table (char offsets).
	lineStarts := []int{0}
	for i, r := range runes {
		if r == '\n' && i+1 < n {
			lineStarts = append(lineStarts, i+1)
		}
	}

	result := make([]uint32, 0, len(doc.tokens)*5)

	prevLine := 0
	prevStart := 0

	for _, tok := range doc.tokens {
		bits := tm.get(tok.Tag)
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
