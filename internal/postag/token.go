package postag

type POSToken struct {
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

// FilterToken reports whether t should be kept, given a score threshold: below-threshold or all-punctuation tokens are dropped.
func FilterToken(t POSToken, threshold float64) bool {
	if t.Score <= threshold {
		return false
	}
	for _, ch := range t.Word {
		if !isASCIIPunct(ch) {
			return true
		}
	}
	return false
}

func isASCIIPunct(r rune) bool {
	return r >= '!' && r <= '/' ||
		r >= ':' && r <= '@' ||
		r >= '[' && r <= '`' ||
		r >= '{' && r <= '~'
}
