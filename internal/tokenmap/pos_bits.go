package tokenmap

import "natural-syntax-ls/internal/postag"

// posBits is the built-in POS coloring behind NewDefault.
func posBits(pos postag.PartOfSpeech) Bits {
	switch pos {
	case postag.CC: // and, but, or
		return bits(TypeKeyword)
	case postag.CD: // cardinal number
		return bits(TypeNumber)
	case postag.DT: // the, a, an
		return bits(TypeMacro)
	case postag.EX: // existential there
		return bits(TypeKeyword, ModifierAbstract)
	case postag.FW: // foreign word
		return bits(TypeString)
	case postag.IN: // preposition
		return bits(TypeOperator)
	case postag.JJ: // adjective
		return bits(TypeType)
	case postag.JJR: // adjective comparative
		return bits(TypeStruct)
	case postag.JJS: // adjective superlative
		return bits(TypeInterface)
	case postag.LS: // list item marker
		return bits(TypeDecorator)
	case postag.MD: // modal: could, will
		return bits(TypeModifier)
	case postag.NN: // noun
		return bits(TypeVariable)
	case postag.NNP: // proper noun
		return bits(TypeNamespace)
	case postag.NNPS: // proper noun plural
		return bits(TypeTypeParameter)
	case postag.NNS: // noun plural
		return bits(TypeVariable, ModifierModification)
	case postag.O: // other/punctuation
		return bits(TypeComment, ModifierDeprecated)
	case postag.PDT: // predeterminer: all, both
		return bits(TypeMacro, ModifierDefinition)
	case postag.POS: // possessive 's
		return bits(TypeOperator, ModifierDefinition)
	case postag.PRP: // personal pronoun: I, he
		return bits(TypeKeyword, ModifierDeclaration)
	case postag.PRPS: // possessive pronoun: my, his
		return bits(TypeProperty)
	case postag.RB: // adverb
		return bits(TypeEnumMember)
	case postag.RBR: // adverb comparative
		return bits(TypeEnumMember, ModifierAsync)
	case postag.RBS: // adverb superlative
		return bits(TypeEnumMember, ModifierDefaultLibrary)
	case postag.RP: // particle
		return bits(TypeOperator, ModifierModification)
	case postag.SYM: // symbol [filtered out]
		return bits(TypeOperator, ModifierDocumentation)
	case postag.TO: // to
		return bits(TypeKeyword, ModifierStatic)
	case postag.UH: // interjection: oh, wow
		return bits(TypeString, ModifierDeclaration)
	case postag.VB: // verb base
		return bits(TypeFunction)
	case postag.VBD: // verb past tense
		return bits(TypeFunction, ModifierModification)
	case postag.VBG: // verb gerund
		return bits(TypeFunction, ModifierAsync)
	case postag.VBN: // verb past participle
		return bits(TypeMethod, ModifierDefaultLibrary)
	case postag.VBP: // verb non-3rd present
		return bits(TypeFunction, ModifierReadonly)
	case postag.VBZ: // verb 3rd person
		return bits(TypeMethod, ModifierStatic)
	case postag.WDT: // wh-determiner: which, that
		return bits(TypeMacro, ModifierModification)
	case postag.WP: // wh-pronoun: who, what
		return bits(TypeRegexp)
	case postag.WPS: // possessive wh-pronoun: whose
		return bits(TypeProperty, ModifierDeclaration)
	case postag.WRB: // wh-adverb: where, when
		return bits(TypeEnumMember, ModifierModification)
	default:
		return bits(TypeComment, ModifierDeprecated)
	}
}
