// Package tokenmap maps linguistic tags (POS, deprel) to LSP semantic token types/modifiers.
package tokenmap

import (
	"strings"

	"natural-syntax-ls/internal/postag"
)

// Bits holds the encoded LSP semantic token type and modifiers.
type Bits struct {
	TokenType           uint32
	TokenModifierBitset uint32
}

// Map maps PartOfSpeech → optional Bits (nil = disabled).
type Map [postag.NumPartsOfSpeech]*Bits

// NewDefault builds a Map from the built-in POS → token-type coloring.
func NewDefault() Map {
	var m Map
	for i := range postag.NumPartsOfSpeech {
		b := posBits(postag.PartOfSpeech(i))
		m[i] = &b
	}
	return m
}

// Extend applies per-POS overrides; a nil override disables that POS.
func (m *Map) Extend(update map[postag.PartOfSpeech]*Override) {
	for pos, o := range update {
		if o == nil {
			m[pos] = nil
			continue
		}
		b := bits(o.Type, o.Modifiers...)
		m[pos] = &b
	}
}

// Get returns the bits for pos, or nil if pos is disabled or out of range.
func (m *Map) Get(pos postag.PartOfSpeech) *Bits {
	if pos >= postag.NumPartsOfSpeech {
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

// Semantic token types.
const (
	TypeNamespace Type = iota
	TypeType
	TypeClass
	TypeEnum
	TypeInterface
	TypeStruct
	TypeTypeParameter
	TypeParameter
	TypeVariable
	TypeProperty
	TypeEnumMember
	TypeEvent
	TypeFunction
	TypeMethod
	TypeMacro
	TypeKeyword
	TypeModifier
	TypeComment
	TypeString
	TypeNumber
	TypeRegexp
	TypeOperator
	TypeDecorator

	NumTypes = iota
)

// TypeNames is the LSP-legend-ordered list of semantic token type names.
var TypeNames = [NumTypes]string{
	"namespace", "type", "class", "enum", "interface", "struct",
	"typeParameter", "parameter", "variable", "property", "enumMember",
	"event", "function", "method", "macro", "keyword", "modifier",
	"comment", "string", "number", "regexp", "operator", "decorator",
}

var typeByName = indexByName[Type](TypeNames[:])

// UnmarshalText decodes a token type name.
func (t *Type) UnmarshalText(text []byte) error {
	if v, ok := typeByName[string(text)]; ok {
		*t = v
	}
	return nil // unknown names keep the previous value
}

// Modifier enum (bit positions).
type Modifier uint32

// Semantic token modifiers.
const (
	ModifierDeclaration Modifier = iota
	ModifierDefinition
	ModifierReadonly
	ModifierStatic
	ModifierDeprecated
	ModifierAbstract
	ModifierAsync
	ModifierModification
	ModifierDocumentation
	ModifierDefaultLibrary

	NumModifiers = iota
)

// ModifierNames is the LSP-legend-ordered list of semantic token modifier names.
var ModifierNames = [NumModifiers]string{
	"declaration", "definition", "readonly", "static", "deprecated",
	"abstract", "async", "modification", "documentation", "defaultLibrary",
}

var modifierByName = indexByName[Modifier](ModifierNames[:])

func indexByName[T ~uint32](names []string) map[string]T {
	m := make(map[string]T, len(names))
	for i, name := range names {
		m[name] = T(i)
	}
	return m
}

// UnmarshalText decodes a token modifier name.
func (m *Modifier) UnmarshalText(text []byte) error {
	v, ok := modifierByName[string(text)]
	if !ok {
		// A list element can't be dropped from inside its own unmarshaler, so mark it for bits to skip.
		v = NumModifiers
	}
	*m = v
	return nil
}

// bits encodes t and mods, skipping out-of-range modifiers.
func bits(t Type, mods ...Modifier) Bits {
	b := Bits{TokenType: uint32(t)}
	for _, m := range mods {
		if m < NumModifiers {
			b.TokenModifierBitset |= 1 << m
		}
	}
	return b
}

// String formats b as docs/tags.md does: the type name, then its modifier names in parentheses.
func (b Bits) String() string {
	s := "unknown"
	if b.TokenType < NumTypes {
		s = TypeNames[b.TokenType]
	}
	var mods []string
	for i, name := range ModifierNames {
		if b.TokenModifierBitset&(1<<i) != 0 {
			mods = append(mods, name)
		}
	}
	if len(mods) > 0 {
		s += " (" + strings.Join(mods, ", ") + ")"
	}
	return s
}
