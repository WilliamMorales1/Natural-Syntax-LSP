package tokenmap

import "natural-syntax-ls/internal/postag"

// posBits is the built-in POS coloring behind NewDefault.
func posBits(pos postag.PartOfSpeech) Bits {
	switch pos.Canonical() {
	case postag.CC: // and, but, or
		return bits(TypeKeyword)
	case postag.CD: // cardinal number
		return bits(TypeNumber)
	case postag.DT: // the, a, an
		return bits(TypeMacro)
	case postag.FW: // foreign word
		return bits(TypeString)
	case postag.IN: // preposition
		return bits(TypeOperator)
	case postag.JJ: // adjective
		return bits(TypeType)
	case postag.MD: // modal: could, will
		return bits(TypeModifier)
	case postag.NN: // noun
		return bits(TypeVariable)
	case postag.NNP: // proper noun
		return bits(TypeNamespace)
	case postag.O: // other/punctuation
		return bits(TypeComment, ModifierDeprecated)
	case postag.PRP: // personal pronoun: I, he
		return bits(TypeKeyword, ModifierDeclaration)
	case postag.RB: // adverb
		return bits(TypeEnumMember)
	case postag.RP: // particle
		return bits(TypeOperator, ModifierModification)
	case postag.SYM: // symbol [filtered out]
		return bits(TypeOperator, ModifierDocumentation)
	case postag.UH: // interjection: oh, wow
		return bits(TypeString, ModifierDeclaration)
	case postag.VB: // verb base
		return bits(TypeFunction)
	default:
		return bits(TypeComment, ModifierDeprecated)
	}
}
