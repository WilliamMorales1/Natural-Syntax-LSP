// Package postag holds the linguistic tag types (POS, UD deprel) shared across inference, token-mapping, and LSP layers.
package postag

import "fmt"

// PartOfSpeech is a Penn Treebank part-of-speech tag.
type PartOfSpeech int

const (
	CC PartOfSpeech = iota
	CD
	DT
	EX
	FW
	IN
	JJ
	JJR
	JJS
	LS
	MD
	NN
	NNP
	NNPS
	NNS
	O
	PDT
	POS
	PRP
	PRPS
	RB
	RBR
	RBS
	RP
	SYM
	TO
	UH
	VB
	VBD
	VBG
	VBN
	VBP
	VBZ
	WDT
	WP
	WPS
	WRB

	NumPartsOfSpeech = iota
)

var posInfo = [NumPartsOfSpeech]struct{ tag, description string }{
	CC:   {"CC", "Coordinating conjunction"},
	CD:   {"CD", "Cardinal number"},
	DT:   {"DT", "Determiner"},
	EX:   {"EX", "Existential"},
	FW:   {"FW", "Foreign word"},
	IN:   {"IN", "Preposition or subordinating conjunction"},
	JJ:   {"JJ", "Adjective"},
	JJR:  {"JJR", "Adjective, comparative"},
	JJS:  {"JJS", "Adjective, superlative"},
	LS:   {"LS", "List item marker"},
	MD:   {"MD", "Modal"},
	NN:   {"NN", "Noun, singular or mass"},
	NNP:  {"NNP", "Proper noun, singular"},
	NNPS: {"NNPS", "Proper noun, plural"},
	NNS:  {"NNS", "Noun, plural"},
	O:    {"O", "Other"},
	PDT:  {"PDT", "Predeterminer"},
	POS:  {"POS", "Possessive ending"},
	PRP:  {"PRP", "Personal pronoun"},
	PRPS: {"PRP$", "Possessive pronoun"},
	RB:   {"RB", "Adverb"},
	RBR:  {"RBR", "Adverb, comparative"},
	RBS:  {"RBS", "Adverb, superlative"},
	RP:   {"RP", "Particle"},
	SYM:  {"SYM", "Symbol"},
	TO:   {"TO", "to"},
	UH:   {"UH", "Interjection"},
	VB:   {"VB", "Verb, base form"},
	VBD:  {"VBD", "Verb, past tense"},
	VBG:  {"VBG", "Verb, gerund or present participle"},
	VBN:  {"VBN", "Verb, past participle"},
	VBP:  {"VBP", "Verb, non-3rd person singular present"},
	VBZ:  {"VBZ", "Verb, 3rd person singular present"},
	WDT:  {"WDT", "Wh-determiner"},
	WP:   {"WP", "Wh-pronoun"},
	WPS:  {"WP$", "Possessive wh-pronoun"},
	WRB:  {"WRB", "Wh-adverb"},
}

var posByTag = func() map[string]PartOfSpeech {
	m := make(map[string]PartOfSpeech, NumPartsOfSpeech)
	for i, info := range posInfo {
		m[info.tag] = PartOfSpeech(i)
	}
	return m
}()

// ParsePartOfSpeech maps a Penn Treebank tag such as "NN" or "PRP$" to its PartOfSpeech.
func ParsePartOfSpeech(tag string) (PartOfSpeech, bool) {
	p, ok := posByTag[tag]
	return p, ok
}

func (p PartOfSpeech) valid() bool { return p >= 0 && p < NumPartsOfSpeech }

func (p PartOfSpeech) String() string {
	if p.valid() {
		return posInfo[p].tag
	}
	return fmt.Sprintf("POS(%d)", int(p))
}

// Description returns a human-readable label for p, used in hover text.
func (p PartOfSpeech) Description() string {
	if p.valid() {
		return posInfo[p].description
	}
	return "Unknown"
}

func (p PartOfSpeech) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

func (p *PartOfSpeech) UnmarshalText(text []byte) error {
	v, ok := ParsePartOfSpeech(string(text))
	if !ok {
		return fmt.Errorf("unknown POS tag: %s", text)
	}
	*p = v
	return nil
}
