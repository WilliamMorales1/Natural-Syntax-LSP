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
	for i := 0; i < N_PART_OF_SPEECH; i++ {
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
	Type      TokenType      `json:"type"`
	Modifiers []TokenModifier `json:"modifiers"`
}

// tokenMapUpdate is the JSON shape for initializationOptions.token_map_update.
// Values can be null (disable) or a TokenTypeNModifiers object.
type tokenMapUpdateEntry struct {
	valid bool
	tnm   *TokenTypeNModifiers
}

func (e *tokenMapUpdateEntry) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		e.valid = true
		e.tnm = nil
		return nil
	}
	var v TokenTypeNModifiers
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	e.valid = true
	e.tnm = &v
	return nil
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
	TM_Declaration  TokenModifier = 0
	TM_Definition   TokenModifier = 1
	TM_Readonly     TokenModifier = 2
	TM_Static       TokenModifier = 3
	TM_Deprecated   TokenModifier = 4
	TM_Abstract     TokenModifier = 5
	TM_Async        TokenModifier = 6
	TM_Modification TokenModifier = 7
	TM_Documentation TokenModifier = 8
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
	switch pos {
	case POS_CC:
		return tb{uint32(TT_Keyword), mod()}
	case POS_CD:
		return tb{uint32(TT_Number), mod()}
	case POS_DT:
		return tb{uint32(TT_String), mod(TM_Documentation)}
	case POS_EX:
		return tb{uint32(TT_Keyword), mod(TM_Definition)}
	case POS_FW:
		return tb{uint32(TT_String), mod()}
	case POS_IN:
		return tb{uint32(TT_Comment), mod(TM_Async)}
	case POS_JJ:
		return tb{uint32(TT_Type), mod()}
	case POS_JJR:
		return tb{uint32(TT_Struct), mod(TM_Modification)}
	case POS_JJS:
		return tb{uint32(TT_Interface), mod(TM_DefaultLibrary)}
	case POS_MD:
		return tb{uint32(TT_Keyword), mod(TM_Readonly)}
	case POS_NN:
		return tb{uint32(TT_Parameter), mod()}
	case POS_NNP:
		return tb{uint32(TT_Parameter), mod(TM_Declaration)}
	case POS_NNPS:
		return tb{uint32(TT_Parameter), mod(TM_Declaration, TM_Modification)}
	case POS_NNS:
		return tb{uint32(TT_Parameter), mod(TM_Modification)}
	case POS_O:
		return tb{uint32(TT_Comment), mod(TM_Deprecated)}
	case POS_PDT:
		return tb{uint32(TT_String), mod(TM_Abstract)}
	case POS_POS:
		return tb{uint32(TT_Property), mod(TM_Declaration)}
	case POS_PRP:
		return tb{uint32(TT_Property), mod()}
	case POS_RB:
		return tb{uint32(TT_EnumMember), mod()}
	case POS_RBR:
		return tb{uint32(TT_EnumMember), mod(TM_Async)}
	case POS_RBS:
		return tb{uint32(TT_EnumMember), mod(TM_DefaultLibrary)}
	case POS_RP:
		return tb{uint32(TT_Operator), mod()}
	case POS_SYM:
		return tb{uint32(TT_Operator), mod(TM_Documentation)}
	case POS_TO:
		return tb{uint32(TT_Keyword), mod(TM_Static)}
	case POS_UH:
		return tb{uint32(TT_Keyword), mod(TM_Modification)}
	case POS_VB:
		return tb{uint32(TT_Function), mod()}
	case POS_VBD:
		return tb{uint32(TT_Function), mod(TM_Modification)}
	case POS_VBG:
		return tb{uint32(TT_Function), mod(TM_Async)}
	case POS_VBN:
		return tb{uint32(TT_Method), mod(TM_DefaultLibrary)}
	case POS_VBP:
		return tb{uint32(TT_Function), mod(TM_Readonly)}
	case POS_VBZ:
		return tb{uint32(TT_Method), mod(TM_Static)}
	case POS_WDT:
		return tb{uint32(TT_Keyword), mod(TM_Documentation)}
	case POS_WP:
		return tb{uint32(TT_Keyword), mod(TM_DefaultLibrary)}
	case POS_WRB:
		return tb{uint32(TT_Keyword), mod(TM_Async)}
	default:
		return tb{uint32(TT_Comment), mod()}
	}
}
