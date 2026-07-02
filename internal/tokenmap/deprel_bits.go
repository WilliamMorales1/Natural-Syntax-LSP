package tokenmap

import "natural-syntax-ls/internal/postag"

// DeprelBits colors dependency-mode tokens by UD relation, one case per relation like posBits, using modifiers to distinguish close relatives within the same token type.
func DeprelBits(rel postag.Deprel) Bits {
	type tb = Bits
	mod := func(ms ...Modifier) uint32 { return modifiersToBitmap(ms) }
	switch rel {
	case postag.DEP_NSUBJ: // nominal subject
		return tb{uint32(TT_Variable), mod(TM_Declaration)}
	case postag.DEP_CSUBJ: // clausal subject
		return tb{uint32(TT_Variable), mod(TM_Definition)}
	case postag.DEP_OBJ: // direct object
		return tb{uint32(TT_Variable), mod()}
	case postag.DEP_IOBJ: // indirect object
		return tb{uint32(TT_Variable), mod(TM_Readonly)}
	case postag.DEP_CCOMP: // clausal complement
		return tb{uint32(TT_Function), mod()}
	case postag.DEP_XCOMP: // open clausal complement
		return tb{uint32(TT_Function), mod(TM_Async)}
	case postag.DEP_ADVCL: // adverbial clause modifier
		return tb{uint32(TT_Function), mod(TM_Modification)}
	case postag.DEP_ACL: // adnominal clause
		return tb{uint32(TT_Method), mod(TM_Modification)}
	case postag.DEP_AMOD: // adjectival modifier
		return tb{uint32(TT_Type), mod()}
	case postag.DEP_ADVMOD: // adverbial modifier
		return tb{uint32(TT_EnumMember), mod()}
	case postag.DEP_NMOD: // nominal modifier
		return tb{uint32(TT_Property), mod()}
	case postag.DEP_APPOS: // appositional modifier
		return tb{uint32(TT_Property), mod(TM_Definition)}
	case postag.DEP_NUMMOD: // numeric modifier
		return tb{uint32(TT_Number), mod()}
	case postag.DEP_OBL: // oblique nominal
		return tb{uint32(TT_Parameter), mod()}
	case postag.DEP_AUX: // auxiliary
		return tb{uint32(TT_Keyword), mod()}
	case postag.DEP_COP: // copula
		return tb{uint32(TT_Keyword), mod(TM_Static)}
	case postag.DEP_EXPL: // expletive: there, it
		return tb{uint32(TT_Keyword), mod(TM_Abstract)}
	case postag.DEP_MARK: // subordinating conjunction
		return tb{uint32(TT_Operator), mod()}
	case postag.DEP_CASE: // adposition
		return tb{uint32(TT_Operator), mod(TM_Modification)}
	case postag.DEP_CONJ: // coordinated conjunct
		return tb{uint32(TT_Operator), mod(TM_Static)}
	case postag.DEP_DET: // determiner
		return tb{uint32(TT_Macro), mod()}
	case postag.DEP_CC: // coordinating conjunction
		return tb{uint32(TT_Keyword), mod(TM_Modification)}
	case postag.DEP_COMPOUND: // compound
		return tb{uint32(TT_Namespace), mod()}
	case postag.DEP_FIXED: // fixed multiword expression
		return tb{uint32(TT_Namespace), mod(TM_Readonly)}
	case postag.DEP_FLAT: // flat multiword expression
		return tb{uint32(TT_Namespace), mod(TM_Definition)}
	case postag.DEP_GOESWITH: // goes-with (typo split across tokens)
		return tb{uint32(TT_Namespace), mod(TM_Deprecated)}
	case postag.DEP_DISCOURSE: // discourse element
		return tb{uint32(TT_String), mod()}
	case postag.DEP_VOCATIVE: // vocative
		return tb{uint32(TT_String), mod(TM_Declaration)}
	case postag.DEP_ROOT: // sentence root
		return tb{uint32(TT_Class), mod(TM_Declaration)}
	case postag.DEP_CLF: // classifier
		return tb{uint32(TT_Decorator), mod()}
	case postag.DEP_LIST: // list item
		return tb{uint32(TT_Decorator), mod(TM_Modification)}
	case postag.DEP_PUNCT: // punctuation [filtered out]
		return tb{uint32(TT_Comment), mod(TM_Deprecated)}
	case postag.DEP_DEP: // unspecified dependency
		return tb{uint32(TT_Modifier), mod()}
	case postag.DEP_ORPHAN: // orphan
		return tb{uint32(TT_Modifier), mod(TM_Deprecated)}
	case postag.DEP_PARATAXIS: // parataxis
		return tb{uint32(TT_Modifier), mod(TM_Modification)}
	case postag.DEP_DISLOCATED: // dislocated element
		return tb{uint32(TT_Modifier), mod(TM_Definition)}
	case postag.DEP_REPARANDUM: // overridden disfluency
		return tb{uint32(TT_Modifier), mod(TM_Deprecated, TM_Definition)}
	default:
		return tb{uint32(TT_Comment), mod(TM_Deprecated)}
	}
}
