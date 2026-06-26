package main

import (
	"encoding/json"
	"fmt"
)

type PartOfSpeech int

const (
	POS_CC   PartOfSpeech = 0
	POS_CD   PartOfSpeech = 1
	POS_DT   PartOfSpeech = 2
	POS_EX   PartOfSpeech = 3
	POS_FW   PartOfSpeech = 4
	POS_IN   PartOfSpeech = 5
	POS_JJ   PartOfSpeech = 6
	POS_JJR  PartOfSpeech = 7
	POS_JJS  PartOfSpeech = 8
	POS_MD   PartOfSpeech = 9
	POS_NN   PartOfSpeech = 10
	POS_NNP  PartOfSpeech = 11
	POS_NNPS PartOfSpeech = 12
	POS_NNS  PartOfSpeech = 13
	POS_O    PartOfSpeech = 14
	POS_PDT  PartOfSpeech = 15
	POS_POS  PartOfSpeech = 16
	POS_PRP  PartOfSpeech = 17
	POS_RB   PartOfSpeech = 18
	POS_RBR  PartOfSpeech = 19
	POS_RBS  PartOfSpeech = 20
	POS_RP   PartOfSpeech = 21
	POS_SYM  PartOfSpeech = 22
	POS_TO   PartOfSpeech = 23
	POS_UH   PartOfSpeech = 24
	POS_VB   PartOfSpeech = 25
	POS_VBD  PartOfSpeech = 26
	POS_VBG  PartOfSpeech = 27
	POS_VBN  PartOfSpeech = 28
	POS_VBP  PartOfSpeech = 29
	POS_VBZ  PartOfSpeech = 30
	POS_WDT  PartOfSpeech = 31
	POS_WP   PartOfSpeech = 32
	POS_WRB  PartOfSpeech = 33

	N_PART_OF_SPEECH = 34
)

var posFromString = map[string]PartOfSpeech{
	"CC": POS_CC, "CD": POS_CD, "DT": POS_DT, "EX": POS_EX,
	"FW": POS_FW, "IN": POS_IN, "JJ": POS_JJ, "JJR": POS_JJR,
	"JJS": POS_JJS, "MD": POS_MD, "NN": POS_NN, "NNP": POS_NNP,
	"NNPS": POS_NNPS, "NNS": POS_NNS, "O": POS_O, "PDT": POS_PDT,
	"POS": POS_POS, "PRP": POS_PRP, "RB": POS_RB, "RBR": POS_RBR,
	"RBS": POS_RBS, "RP": POS_RP, "SYM": POS_SYM, "TO": POS_TO,
	"UH": POS_UH, "VB": POS_VB, "VBD": POS_VBD, "VBG": POS_VBG,
	"VBN": POS_VBN, "VBP": POS_VBP, "VBZ": POS_VBZ, "WDT": POS_WDT,
	"WP": POS_WP, "WRB": POS_WRB,
}

var posToString = [N_PART_OF_SPEECH]string{
	"CC", "CD", "DT", "EX", "FW", "IN", "JJ", "JJR",
	"JJS", "MD", "NN", "NNP", "NNPS", "NNS", "O", "PDT",
	"POS", "PRP", "RB", "RBR", "RBS", "RP", "SYM", "TO",
	"UH", "VB", "VBD", "VBG", "VBN", "VBP", "VBZ", "WDT",
	"WP", "WRB",
}

func (p PartOfSpeech) String() string {
	if p >= 0 && int(p) < N_PART_OF_SPEECH {
		return posToString[p]
	}
	return fmt.Sprintf("POS(%d)", int(p))
}

func (p PartOfSpeech) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.String())
}

func (p *PartOfSpeech) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v, ok := posFromString[s]
	if !ok {
		return fmt.Errorf("unknown POS tag: %s", s)
	}
	*p = v
	return nil
}

func posDescription(pos PartOfSpeech) string {
	switch pos {
	case POS_CC:
		return "Coordinating conjunction"
	case POS_CD:
		return "Cardinal number"
	case POS_DT:
		return "Determiner"
	case POS_EX:
		return "Existential"
	case POS_FW:
		return "Foreign word"
	case POS_IN:
		return "Preposition or subordinating conjunction"
	case POS_JJ:
		return "Adjective"
	case POS_JJR:
		return "Adjective, comparative"
	case POS_JJS:
		return "Adjective, superlative"
	case POS_MD:
		return "Modal"
	case POS_NN:
		return "Noun, singular or mass"
	case POS_NNP:
		return "Proper noun, singular"
	case POS_NNPS:
		return "Proper noun, plural"
	case POS_NNS:
		return "Noun, plural"
	case POS_O:
		return "Other"
	case POS_PDT:
		return "Predeterminer"
	case POS_POS:
		return "Possessive ending"
	case POS_PRP:
		return "Personal pronoun"
	case POS_RB:
		return "Adverb"
	case POS_RBR:
		return "Adverb, comparative"
	case POS_RBS:
		return "Adverb, superlative"
	case POS_RP:
		return "Particle"
	case POS_SYM:
		return "Symbol"
	case POS_TO:
		return "to"
	case POS_UH:
		return "Interjection"
	case POS_VB:
		return "Verb, base form"
	case POS_VBD:
		return "Verb, past tense"
	case POS_VBG:
		return "Verb, gerund or present participle"
	case POS_VBN:
		return "Verb, past participle"
	case POS_VBP:
		return "Verb, non-3rd person singular present"
	case POS_VBZ:
		return "Verb, 3rd person singular present"
	case POS_WDT:
		return "Wh-determiner"
	case POS_WP:
		return "Wh-pronoun"
	case POS_WRB:
		return "Wh-adverb"
	default:
		return "Unknown"
	}
}

type POSToken struct {
	Word        string
	Score       float64
	Tag         PartOfSpeech
	OffsetBegin uint32
	OffsetEnd   uint32
}

func filterToken(t POSToken, threshold float64) bool {
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
