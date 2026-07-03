package tokenmap

import "natural-syntax-ls/internal/postag"

func posBits(pos postag.PartOfSpeech) Bits {
	type tb = Bits
	mod := func(ms ...Modifier) uint32 { return modifiersToBitmap(ms) }
	switch pos {
	case postag.POS_CC: // and, but, or
		return tb{uint32(TT_Keyword), mod()}
	case postag.POS_CD: // cardinal number
		return tb{uint32(TT_Number), mod()}
	case postag.POS_DT: // the, a, an
		return tb{uint32(TT_Macro), mod()}
	case postag.POS_EX: // existential there
		return tb{uint32(TT_Keyword), mod(TM_Abstract)}
	case postag.POS_FW: // foreign word
		return tb{uint32(TT_String), mod()}
	case postag.POS_IN: // preposition
		return tb{uint32(TT_Operator), mod()}
	case postag.POS_JJ: // adjective
		return tb{uint32(TT_Type), mod()}
	case postag.POS_JJR: // adjective comparative
		return tb{uint32(TT_Struct), mod()}
	case postag.POS_JJS: // adjective superlative
		return tb{uint32(TT_Interface), mod()}
	case postag.POS_LS: // list item marker
		return tb{uint32(TT_Decorator), mod()}
	case postag.POS_MD: // modal: could, will
		return tb{uint32(TT_Modifier), mod()}
	case postag.POS_NN: // noun
		return tb{uint32(TT_Variable), mod()}
	case postag.POS_NNP: // proper noun
		return tb{uint32(TT_Namespace), mod()}
	case postag.POS_NNPS: // proper noun plural
		return tb{uint32(TT_TypeParameter), mod()}
	case postag.POS_NNS: // noun plural
		return tb{uint32(TT_Variable), mod(TM_Modification)}
	case postag.POS_O: // other/punctuation
		return tb{uint32(TT_Comment), mod(TM_Deprecated)}
	case postag.POS_PDT: // predeterminer: all, both
		return tb{uint32(TT_Macro), mod(TM_Definition)}
	case postag.POS_POS: // possessive 's
		return tb{uint32(TT_Operator), mod(TM_Definition)}
	case postag.POS_PRP: // personal pronoun: I, he
		return tb{uint32(TT_Keyword), mod(TM_Declaration)}
	case postag.POS_PRPS: // possessive pronoun: my, his
		return tb{uint32(TT_Property), mod()}
	case postag.POS_RB: // adverb
		return tb{uint32(TT_EnumMember), mod()}
	case postag.POS_RBR: // adverb comparative
		return tb{uint32(TT_EnumMember), mod(TM_Async)}
	case postag.POS_RBS: // adverb superlative
		return tb{uint32(TT_EnumMember), mod(TM_DefaultLibrary)}
	case postag.POS_RP: // particle
		return tb{uint32(TT_Operator), mod(TM_Modification)}
	case postag.POS_SYM: // symbol [filtered out]
		return tb{uint32(TT_Operator), mod(TM_Documentation)}
	case postag.POS_TO: // to
		return tb{uint32(TT_Keyword), mod(TM_Static)}
	case postag.POS_UH: // interjection: oh, wow
		return tb{uint32(TT_String), mod(TM_Declaration)}
	case postag.POS_VB: // verb base
		return tb{uint32(TT_Function), mod()}
	case postag.POS_VBD: // verb past tense
		return tb{uint32(TT_Function), mod(TM_Modification)}
	case postag.POS_VBG: // verb gerund
		return tb{uint32(TT_Function), mod(TM_Async)}
	case postag.POS_VBN: // verb past participle
		return tb{uint32(TT_Method), mod(TM_DefaultLibrary)}
	case postag.POS_VBP: // verb non-3rd present
		return tb{uint32(TT_Function), mod(TM_Readonly)}
	case postag.POS_VBZ: // verb 3rd person
		return tb{uint32(TT_Method), mod(TM_Static)}
	case postag.POS_WDT: // wh-determiner: which, that
		return tb{uint32(TT_Macro), mod(TM_Modification)}
	case postag.POS_WP: // wh-pronoun: who, what
		return tb{uint32(TT_Regexp), mod()}
	case postag.POS_WPS: // possessive wh-pronoun: whose
		return tb{uint32(TT_Property), mod(TM_Declaration)}
	case postag.POS_WRB: // wh-adverb: where, when
		return tb{uint32(TT_EnumMember), mod(TM_Modification)}
	default:
		return tb{uint32(TT_Comment), mod(TM_Deprecated)}
	}
}
