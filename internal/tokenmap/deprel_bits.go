package tokenmap

import "natural-syntax-ls/internal/postag"

// DeprelBits colors dependency-mode tokens by UD relation category, grouping ~37 base deprels into a handful of visually distinct roles.
func DeprelBits(rel postag.Deprel) Bits {
	type tb = Bits
	mod := func(ms ...Modifier) uint32 { return modifiersToBitmap(ms) }
	switch rel {
	case postag.DEP_NSUBJ, postag.DEP_CSUBJ: // subjects
		return tb{uint32(TT_Variable), mod(TM_Declaration)}
	case postag.DEP_OBJ, postag.DEP_IOBJ: // objects
		return tb{uint32(TT_Variable), mod()}
	case postag.DEP_CCOMP, postag.DEP_XCOMP: // clausal complements
		return tb{uint32(TT_Function), mod()}
	case postag.DEP_ADVCL, postag.DEP_ACL: // adverbial/adnominal clauses
		return tb{uint32(TT_Function), mod(TM_Modification)}
	case postag.DEP_AMOD: // adjectival modifier
		return tb{uint32(TT_Type), mod()}
	case postag.DEP_ADVMOD: // adverbial modifier
		return tb{uint32(TT_Type), mod(TM_Modification)}
	case postag.DEP_NMOD, postag.DEP_APPOS, postag.DEP_NUMMOD: // nominal dependents
		return tb{uint32(TT_Property), mod()}
	case postag.DEP_AUX, postag.DEP_COP: // auxiliaries/copula
		return tb{uint32(TT_Keyword), mod()}
	case postag.DEP_MARK, postag.DEP_CASE: // subordinators/adpositions
		return tb{uint32(TT_Operator), mod()}
	case postag.DEP_DET: // determiners
		return tb{uint32(TT_Macro), mod()}
	case postag.DEP_CC, postag.DEP_CONJ: // coordination
		return tb{uint32(TT_Operator), mod(TM_Static)}
	case postag.DEP_COMPOUND, postag.DEP_FIXED, postag.DEP_FLAT, postag.DEP_GOESWITH: // multiword units
		return tb{uint32(TT_Namespace), mod()}
	case postag.DEP_DISCOURSE, postag.DEP_VOCATIVE, postag.DEP_EXPL: // discourse elements
		return tb{uint32(TT_String), mod()}
	case postag.DEP_ROOT: // sentence root
		return tb{uint32(TT_Class), mod(TM_Declaration)}
	case postag.DEP_PUNCT: // punctuation
		return tb{uint32(TT_Comment), mod(TM_Deprecated)}
	default: // dep, clf, list, orphan, parataxis, reparandum, dislocated
		return tb{uint32(TT_Modifier), mod()}
	}
}
