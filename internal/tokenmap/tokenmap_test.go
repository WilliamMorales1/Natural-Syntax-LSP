package tokenmap

import (
	"encoding/json"
	"testing"

	"natural-syntax-ls/internal/postag"
)

func TestDefaultMapCoversEveryTag(t *testing.T) {
	m := NewDefault()
	for p := range postag.PartOfSpeech(postag.NumPartsOfSpeech) {
		b := m.Get(p)
		if b == nil {
			t.Errorf("%v unmapped", p)
			continue
		}
		if b.TokenType >= NumTypes || b.TokenModifierBitset >= 1<<NumModifiers {
			t.Errorf("%v: bits %+v out of legend range", p, *b)
		}
	}
	for d := range postag.Deprel(postag.NumDeprels) {
		if b := DeprelBits(d); b.TokenType >= NumTypes {
			t.Errorf("%v: type %d out of legend range", d, b.TokenType)
		}
	}
	if m.Get(postag.NumPartsOfSpeech) != nil {
		t.Error("out-of-range Get returned bits")
	}
}

func TestExtendFromJSON(t *testing.T) {
	var o Override
	if err := json.Unmarshal([]byte(`{"type":"function","modifiers":["abstract","readonly"]}`), &o); err != nil {
		t.Fatal(err)
	}
	m := NewDefault()
	m.Extend(map[postag.PartOfSpeech]*Override{postag.NN: &o, postag.DT: nil})
	if b := m.Get(postag.NN); b == nil || b.TokenType != uint32(TypeFunction) || b.TokenModifierBitset&(1<<ModifierAbstract) == 0 {
		t.Errorf("override not applied: %+v", b)
	}
	if m.Get(postag.DT) != nil {
		t.Error("nil override did not disable DT")
	}
	// Unknown names are ignored by design, leaving the previous value in place.
	keep := Override{Type: TypeKeyword}
	if err := json.Unmarshal([]byte(`{"type":"nonsense"}`), &keep); err != nil || keep.Type != TypeKeyword {
		t.Errorf("unknown type: %v, type %d", err, keep.Type)
	}
}

func TestUnknownModifierIgnored(t *testing.T) {
	var o Override
	if err := json.Unmarshal([]byte(`{"type":"function","modifiers":["readonyl","static"]}`), &o); err != nil {
		t.Fatal(err)
	}
	if got, want := bits(o.Type, o.Modifiers...).TokenModifierBitset, uint32(1<<ModifierStatic); got != want {
		t.Errorf("bitmap %b, want %b (typo must not set declaration)", got, want)
	}
}

// TestClassesColorAlike checks Penn variants of one Universal class share a color, so English matches UPOS languages.
func TestClassesColorAlike(t *testing.T) {
	m := NewDefault()
	for _, group := range [][]postag.PartOfSpeech{
		{postag.NN, postag.NNS}, {postag.NNP, postag.NNPS},
		{postag.VB, postag.VBD, postag.VBG, postag.VBN, postag.VBP, postag.VBZ},
		{postag.JJ, postag.JJR, postag.JJS}, {postag.RB, postag.RBR, postag.RBS, postag.WRB},
		{postag.DT, postag.PDT, postag.WDT}, {postag.PRP, postag.PRPS, postag.WP, postag.WPS},
		{postag.RP, postag.TO, postag.POS},
	} {
		for _, p := range group[1:] {
			if *m.Get(p) != *m.Get(group[0]) {
				t.Errorf("%v colors differently from %v", p, group[0])
			}
		}
	}
}
