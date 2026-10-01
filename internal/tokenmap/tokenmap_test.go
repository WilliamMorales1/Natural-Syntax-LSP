package tokenmap

import (
	"encoding/json"
	"testing"

	"natural-syntax-ls/internal/postag"
)

func TestDefaultMapCoversEveryTag(t *testing.T) {
	m := NewDefault()
	for p := range postag.PartOfSpeech(postag.N_PART_OF_SPEECH) {
		b := m.Get(p)
		if b == nil {
			t.Errorf("%v unmapped", p)
			continue
		}
		if b.TokenType >= N_TOKEN_TYPES || b.TokenModifierBitset >= 1<<N_TOKEN_MODIFIERS {
			t.Errorf("%v: bits %+v out of legend range", p, *b)
		}
	}
	for d := range postag.Deprel(postag.N_DEPREL) {
		if b := DeprelBits(d); b.TokenType >= N_TOKEN_TYPES {
			t.Errorf("%v: type %d out of legend range", d, b.TokenType)
		}
	}
	if m.Get(-1) != nil || m.Get(postag.N_PART_OF_SPEECH) != nil {
		t.Error("out-of-range Get returned bits")
	}
}

func TestExtendFromJSON(t *testing.T) {
	var o Override
	if err := json.Unmarshal([]byte(`{"type":"function","modifiers":["abstract","readonly"]}`), &o); err != nil {
		t.Fatal(err)
	}
	m := NewDefault()
	m.Extend(map[postag.PartOfSpeech]*Override{postag.POS_NN: &o, postag.POS_DT: nil})
	if b := m.Get(postag.POS_NN); b == nil || b.TokenType != uint32(TT_Function) || b.TokenModifierBitset&(1<<TM_Abstract) == 0 {
		t.Errorf("override not applied: %+v", b)
	}
	if m.Get(postag.POS_DT) != nil {
		t.Error("nil override did not disable DT")
	}
	// Unknown names are ignored by design, leaving the previous value in place.
	keep := Override{Type: TT_Keyword}
	if err := json.Unmarshal([]byte(`{"type":"nonsense"}`), &keep); err != nil || keep.Type != TT_Keyword {
		t.Errorf("unknown type: %v, type %d", err, keep.Type)
	}
}

func TestUnknownModifierIgnored(t *testing.T) {
	var o Override
	if err := json.Unmarshal([]byte(`{"type":"function","modifiers":["readonyl","static"]}`), &o); err != nil {
		t.Fatal(err)
	}
	if got, want := modifiersToBitmap(o.Modifiers), uint32(1<<TM_Static); got != want {
		t.Errorf("bitmap %b, want %b (typo must not set declaration)", got, want)
	}
}
