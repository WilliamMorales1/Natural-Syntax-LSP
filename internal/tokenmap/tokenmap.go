// Package tokenmap maps linguistic tags (POS, deprel) to LSP semantic token types/modifiers.
package tokenmap

import (
	"encoding/json"

	"natural-syntax-ls/internal/postag"
)

// Bits holds the encoded LSP semantic token type and modifiers.
type Bits struct {
	TokenType           uint32
	TokenModifierBitset uint32
}

// Map maps PartOfSpeech → optional Bits (nil = disabled).
type Map [postag.N_PART_OF_SPEECH]*Bits

// NewDefault builds a Map from the built-in POS → token-type coloring.
func NewDefault() Map {
	var m Map
	for i := range postag.N_PART_OF_SPEECH {
		bits := posBits(postag.PartOfSpeech(i))
		m[i] = &bits
	}
	return m
}

func (m *Map) Extend(update map[postag.PartOfSpeech]*Override) {
	for pos, o := range update {
		if o == nil {
			m[pos] = nil
		} else {
			bits := Bits{
				TokenType:           uint32(o.Type),
				TokenModifierBitset: modifiersToBitmap(o.Modifiers),
			}
			m[pos] = &bits
		}
	}
}

func (m *Map) Get(pos postag.PartOfSpeech) *Bits {
	if pos < 0 || int(pos) >= postag.N_PART_OF_SPEECH {
		return nil
	}
	return m[pos]
}

// Override is the JSON-deserializable per-POS token type/modifier override shape.
type Override struct {
	Type      Type       `json:"type"`
	Modifiers []Modifier `json:"modifiers"`
}

// Type enum (indices match LSP SemanticTokensLegend positions).
type Type uint32

const (
	TT_Namespace     Type = 0
	TT_Type          Type = 1
	TT_Class         Type = 2
	TT_Enum          Type = 3
	TT_Interface     Type = 4
	TT_Struct        Type = 5
	TT_TypeParameter Type = 6
	TT_Parameter     Type = 7
	TT_Variable      Type = 8
	TT_Property      Type = 9
	TT_EnumMember    Type = 10
	TT_Event         Type = 11
	TT_Function      Type = 12
	TT_Method        Type = 13
	TT_Macro         Type = 14
	TT_Keyword       Type = 15
	TT_Modifier      Type = 16
	TT_Comment       Type = 17
	TT_String        Type = 18
	TT_Number        Type = 19
	TT_Regexp        Type = 20
	TT_Operator      Type = 21
	TT_Decorator     Type = 22

	N_TOKEN_TYPES = 23
)

// TypeNames is the LSP-legend-ordered list of semantic token type names.
var TypeNames = [N_TOKEN_TYPES]string{
	"namespace", "type", "class", "enum", "interface", "struct",
	"typeParameter", "parameter", "variable", "property", "enumMember",
	"event", "function", "method", "macro", "keyword", "modifier",
	"comment", "string", "number", "regexp", "operator", "decorator",
}

var typeByName map[string]Type

func init() {
	typeByName = make(map[string]Type, N_TOKEN_TYPES)
	for i, name := range TypeNames {
		typeByName[name] = Type(i)
	}
}

func (t *Type) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v, ok := typeByName[s]
	if !ok {
		return nil // ignore unknown, keep default
	}
	*t = v
	return nil
}

// Modifier enum (bit positions).
type Modifier uint32

const (
	TM_Declaration    Modifier = 0
	TM_Definition     Modifier = 1
	TM_Readonly       Modifier = 2
	TM_Static         Modifier = 3
	TM_Deprecated     Modifier = 4
	TM_Abstract       Modifier = 5
	TM_Async          Modifier = 6
	TM_Modification   Modifier = 7
	TM_Documentation  Modifier = 8
	TM_DefaultLibrary Modifier = 9

	N_TOKEN_MODIFIERS = 10
)

// ModifierNames is the LSP-legend-ordered list of semantic token modifier names.
var ModifierNames = [N_TOKEN_MODIFIERS]string{
	"declaration", "definition", "readonly", "static", "deprecated",
	"abstract", "async", "modification", "documentation", "defaultLibrary",
}

var modifierByName map[string]Modifier

func init() {
	modifierByName = make(map[string]Modifier, N_TOKEN_MODIFIERS)
	for i, name := range ModifierNames {
		modifierByName[name] = Modifier(i)
	}
}

func (m *Modifier) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v, ok := modifierByName[s]
	if !ok {
		// A list element can't be dropped from inside its own unmarshaler, so mark it for modifiersToBitmap to skip.
		v = N_TOKEN_MODIFIERS
	}
	*m = v
	return nil
}

func modifiersToBitmap(mods []Modifier) uint32 {
	var bits uint32
	for _, m := range mods {
		if m < N_TOKEN_MODIFIERS {
			bits |= 1 << m
		}
	}
	return bits
}
