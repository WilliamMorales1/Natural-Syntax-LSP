package postag

import "strings"

// Token is one tagged word from a model, positioned by rune offsets in the source text.
type Token struct {
	Word        string
	Score       float64
	Tag         PartOfSpeech
	OffsetBegin uint32
	OffsetEnd   uint32
	// Description overrides the POS label in hover text.
	Description string
	// Color is a "#RRGGBB" hex color set by semantic mode; empty in POS mode.
	Color string
	// HasHead is true when this token has a syntactic head (dependency mode only).
	HasHead bool
	// HeadOffsetBegin is the head token's OffsetBegin (absolute, so it survives chunking/re-sorting); valid only if HasHead.
	HeadOffsetBegin uint32
	// Deprel is the Universal Dependencies relation to the head; meaningless unless HasHead.
	Deprel Deprel
}

// Keep reports whether t scores above threshold and has at least one non-ASCII-punctuation rune.
func (t Token) Keep(threshold float64) bool {
	return t.Score > threshold && strings.ContainsFunc(t.Word, func(r rune) bool { return !isASCIIPunct(r) })
}

func isASCIIPunct(r rune) bool {
	return r >= '!' && r <= '/' ||
		r >= ':' && r <= '@' ||
		r >= '[' && r <= '`' ||
		r >= '{' && r <= '~'
}
