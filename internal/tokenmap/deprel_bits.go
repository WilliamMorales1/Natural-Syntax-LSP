package tokenmap

import "natural-syntax-ls/internal/postag"

// DeprelBits colors dependency-mode tokens by UD relation, one case per relation like posBits, using modifiers to distinguish close relatives within the same token type.
func DeprelBits(rel postag.Deprel) Bits {
	switch rel {
	case postag.DepNsubj: // nominal subject
		return bits(TypeVariable, ModifierDeclaration)
	case postag.DepCsubj: // clausal subject
		return bits(TypeVariable, ModifierDefinition)
	case postag.DepObj: // direct object
		return bits(TypeVariable)
	case postag.DepIobj: // indirect object
		return bits(TypeVariable, ModifierReadonly)
	case postag.DepCcomp: // clausal complement
		return bits(TypeFunction)
	case postag.DepXcomp: // open clausal complement
		return bits(TypeFunction, ModifierAsync)
	case postag.DepAdvcl: // adverbial clause modifier
		return bits(TypeFunction, ModifierModification)
	case postag.DepAcl: // adnominal clause
		return bits(TypeMethod, ModifierModification)
	case postag.DepAmod: // adjectival modifier
		return bits(TypeType)
	case postag.DepAdvmod: // adverbial modifier
		return bits(TypeEnumMember)
	case postag.DepNmod: // nominal modifier
		return bits(TypeProperty)
	case postag.DepAppos: // appositional modifier
		return bits(TypeProperty, ModifierDefinition)
	case postag.DepNummod: // numeric modifier
		return bits(TypeNumber)
	case postag.DepObl: // oblique nominal
		return bits(TypeParameter)
	case postag.DepAux: // auxiliary
		return bits(TypeKeyword)
	case postag.DepCop: // copula
		return bits(TypeKeyword, ModifierStatic)
	case postag.DepExpl: // expletive: there, it
		return bits(TypeKeyword, ModifierAbstract)
	case postag.DepMark: // subordinating conjunction
		return bits(TypeOperator)
	case postag.DepCase: // adposition
		return bits(TypeOperator, ModifierModification)
	case postag.DepConj: // coordinated conjunct
		return bits(TypeOperator, ModifierStatic)
	case postag.DepDet: // determiner
		return bits(TypeMacro)
	case postag.DepCc: // coordinating conjunction
		return bits(TypeKeyword, ModifierModification)
	case postag.DepCompound: // compound
		return bits(TypeNamespace)
	case postag.DepFixed: // fixed multiword expression
		return bits(TypeNamespace, ModifierReadonly)
	case postag.DepFlat: // flat multiword expression
		return bits(TypeNamespace, ModifierDefinition)
	case postag.DepGoeswith: // goes-with (typo split across tokens)
		return bits(TypeNamespace, ModifierDeprecated)
	case postag.DepDiscourse: // discourse element
		return bits(TypeString)
	case postag.DepVocative: // vocative
		return bits(TypeString, ModifierDeclaration)
	case postag.DepRoot: // sentence root
		return bits(TypeClass, ModifierDeclaration)
	case postag.DepClf: // classifier
		return bits(TypeDecorator)
	case postag.DepList: // list item
		return bits(TypeDecorator, ModifierModification)
	case postag.DepPunct: // punctuation [filtered out]
		return bits(TypeComment, ModifierDeprecated)
	case postag.DepDep: // unspecified dependency
		return bits(TypeModifier)
	case postag.DepOrphan: // orphan
		return bits(TypeModifier, ModifierDeprecated)
	case postag.DepParataxis: // parataxis
		return bits(TypeModifier, ModifierModification)
	case postag.DepDislocated: // dislocated element
		return bits(TypeModifier, ModifierDefinition)
	case postag.DepReparandum: // overridden disfluency
		return bits(TypeModifier, ModifierDeprecated, ModifierDefinition)
	default:
		return bits(TypeComment, ModifierDeprecated)
	}
}
