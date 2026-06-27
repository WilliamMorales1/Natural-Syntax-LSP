package main

import "encoding/json"

// TokenBits holds the encoded LSP semantic token type and modifiers.
type TokenBits struct {
	TokenType           uint32
	TokenModifierBitset uint32
}

// TokenMap maps PartOfSpeech → optional TokenBits (nil = disabled).
type TokenMap [N_PART_OF_SPEECH]*TokenBits

func defaultTokenMap() TokenMap {
	var m TokenMap
	for i := range N_PART_OF_SPEECH {
		bits := pos2TokenBits(PartOfSpeech(i))
		m[i] = &bits
	}
	return m
}

func (m *TokenMap) extend(update map[PartOfSpeech]*TokenTypeNModifiers) {
	for pos, tnm := range update {
		if tnm == nil {
			m[pos] = nil
		} else {
			bits := TokenBits{
				TokenType:           uint32(tnm.Type),
				TokenModifierBitset: modifiersToBitmap(tnm.Modifiers),
			}
			m[pos] = &bits
		}
	}
}

func (m *TokenMap) get(pos PartOfSpeech) *TokenBits {
	if pos < 0 || int(pos) >= N_PART_OF_SPEECH {
		return nil
	}
	return m[pos]
}

// TokenTypeNModifiers is the JSON-deserializable override shape.
type TokenTypeNModifiers struct {
	Type      TokenType       `json:"type"`
	Modifiers []TokenModifier `json:"modifiers"`
}

// TokenType enum (indices match LSP SemanticTokensLegend positions).
type TokenType uint32

const (
	TT_Namespace     TokenType = 0
	TT_Type          TokenType = 1
	TT_Class         TokenType = 2
	TT_Enum          TokenType = 3
	TT_Interface     TokenType = 4
	TT_Struct        TokenType = 5
	TT_TypeParameter TokenType = 6
	TT_Parameter     TokenType = 7
	TT_Variable      TokenType = 8
	TT_Property      TokenType = 9
	TT_EnumMember    TokenType = 10
	TT_Event         TokenType = 11
	TT_Function      TokenType = 12
	TT_Method        TokenType = 13
	TT_Macro         TokenType = 14
	TT_Keyword       TokenType = 15
	TT_Modifier      TokenType = 16
	TT_Comment       TokenType = 17
	TT_String        TokenType = 18
	TT_Number        TokenType = 19
	TT_Regexp        TokenType = 20
	TT_Operator      TokenType = 21
	TT_Decorator     TokenType = 22

	N_TOKEN_TYPES = 23
)

var tokenTypeNames = [N_TOKEN_TYPES]string{
	"namespace", "type", "class", "enum", "interface", "struct",
	"typeParameter", "parameter", "variable", "property", "enumMember",
	"event", "function", "method", "macro", "keyword", "modifier",
	"comment", "string", "number", "regexp", "operator", "decorator",
}

var tokenTypeByName map[string]TokenType

func init() {
	tokenTypeByName = make(map[string]TokenType, N_TOKEN_TYPES)
	for i, name := range tokenTypeNames {
		tokenTypeByName[name] = TokenType(i)
	}
}

func (t *TokenType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v, ok := tokenTypeByName[s]
	if !ok {
		return nil // ignore unknown, keep default
	}
	*t = v
	return nil
}

// TokenModifier enum (bit positions).
type TokenModifier uint32

const (
	TM_Declaration    TokenModifier = 0
	TM_Definition     TokenModifier = 1
	TM_Readonly       TokenModifier = 2
	TM_Static         TokenModifier = 3
	TM_Deprecated     TokenModifier = 4
	TM_Abstract       TokenModifier = 5
	TM_Async          TokenModifier = 6
	TM_Modification   TokenModifier = 7
	TM_Documentation  TokenModifier = 8
	TM_DefaultLibrary TokenModifier = 9

	N_TOKEN_MODIFIERS = 10
)

var tokenModifierNames = [N_TOKEN_MODIFIERS]string{
	"declaration", "definition", "readonly", "static", "deprecated",
	"abstract", "async", "modification", "documentation", "defaultLibrary",
}

var tokenModifierByName map[string]TokenModifier

func init() {
	tokenModifierByName = make(map[string]TokenModifier, N_TOKEN_MODIFIERS)
	for i, name := range tokenModifierNames {
		tokenModifierByName[name] = TokenModifier(i)
	}
}

func (m *TokenModifier) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v, ok := tokenModifierByName[s]
	if !ok {
		return nil
	}
	*m = v
	return nil
}

func modifiersToBitmap(mods []TokenModifier) uint32 {
	var bits uint32
	for _, m := range mods {
		bits |= 1 << m
	}
	return bits
}

func pos2TokenBits(pos PartOfSpeech) TokenBits {
	type tb = TokenBits
	mod := func(ms ...TokenModifier) uint32 { return modifiersToBitmap(ms) }
	switch pos { // colors listed below are what would be displayed when using the One Dark Pro VS Code theme
	case POS_CC: // and, but, or → purple
		return tb{uint32(TT_Keyword), mod()}
	case POS_CD: // cardinal number → orange
		return tb{uint32(TT_Number), mod()}
	case POS_DT: // the, a, an → orange
		return tb{uint32(TT_Macro), mod()}
	case POS_EX: // existential there → purple
		return tb{uint32(TT_Keyword), mod(TM_Abstract)}
	case POS_FW: // foreign word → green
		return tb{uint32(TT_String), mod()}
	case POS_IN: // preposition → cyan
		return tb{uint32(TT_Operator), mod()}
	case POS_JJ: // adjective → yellow
		return tb{uint32(TT_Type), mod()}
	case POS_JJR: // adjective comparative → yellow
		return tb{uint32(TT_Struct), mod()}
	case POS_JJS: // adjective superlative → yellow
		return tb{uint32(TT_Interface), mod()}
	case POS_LS: // list item marker → ???
		return tb{uint32(TT_Decorator), mod()}
	case POS_MD: // modal: could, will → purple
		return tb{uint32(TT_Modifier), mod()}
	case POS_NN: // noun → red
		return tb{uint32(TT_Variable), mod()}
	case POS_NNP: // proper noun → yellow
		return tb{uint32(TT_Class), mod()}
	case POS_NNPS: // proper noun plural → yellow
		return tb{uint32(TT_Enum), mod()}
	case POS_NNS: // noun plural → red
		return tb{uint32(TT_Variable), mod(TM_Modification)}
	case POS_O: // other/punctuation → gray
		return tb{uint32(TT_Comment), mod(TM_Deprecated)}
	case POS_PDT: // predeterminer: all, both → orange
		return tb{uint32(TT_Macro), mod(TM_Definition)}
	case POS_POS: // possessive 's → cyan
		return tb{uint32(TT_Operator), mod(TM_Definition)}
	case POS_PRP: // personal pronoun: I, he → purple
		return tb{uint32(TT_Keyword), mod(TM_Declaration)}
	case POS_PRPS: // possessive pronoun: my, his → red
		return tb{uint32(TT_Property), mod()}
	case POS_RB: // adverb → enumMember
		return tb{uint32(TT_EnumMember), mod()}
	case POS_RBR: // adverb comparative → enumMember
		return tb{uint32(TT_EnumMember), mod(TM_Async)}
	case POS_RBS: // adverb superlative → enumMember
		return tb{uint32(TT_EnumMember), mod(TM_DefaultLibrary)}
	case POS_RP: // particle → cyan
		return tb{uint32(TT_Operator), mod(TM_Modification)}
	case POS_SYM: // symbol → cyan
		return tb{uint32(TT_Operator), mod(TM_Documentation)}
	case POS_TO: // to → purple
		return tb{uint32(TT_Keyword), mod(TM_Static)}
	case POS_UH: // interjection: oh, wow → green
		return tb{uint32(TT_String), mod(TM_Declaration)}
	case POS_VB: // verb base → blue
		return tb{uint32(TT_Function), mod()}
	case POS_VBD: // verb past tense → blue
		return tb{uint32(TT_Function), mod(TM_Modification)}
	case POS_VBG: // verb gerund → blue
		return tb{uint32(TT_Function), mod(TM_Async)}
	case POS_VBN: // verb past participle → blue
		return tb{uint32(TT_Method), mod(TM_DefaultLibrary)}
	case POS_VBP: // verb non-3rd present → blue
		return tb{uint32(TT_Function), mod(TM_Readonly)}
	case POS_VBZ: // verb 3rd person → blue
		return tb{uint32(TT_Method), mod(TM_Static)}
	case POS_WDT: // wh-determiner: which, that → orange
		return tb{uint32(TT_Macro), mod(TM_Modification)}
	case POS_WP: // wh-pronoun: who, what → red
		return tb{uint32(TT_Regexp), mod()}
	case POS_WPS: // possessive wh-pronoun: whose → red
		return tb{uint32(TT_Property), mod(TM_Declaration)}
	case POS_WRB: // wh-adverb: where, when → enumMember
		return tb{uint32(TT_EnumMember), mod(TM_Modification)}
	default:
		return tb{uint32(TT_Comment), mod(TM_Deprecated)}
	}
}
