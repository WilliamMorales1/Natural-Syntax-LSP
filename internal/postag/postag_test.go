package postag

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestPartOfSpeechJSONRoundTrip(t *testing.T) {
	for p := range PartOfSpeech(NumPartsOfSpeech) {
		b, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		var back PartOfSpeech
		if err := json.Unmarshal(b, &back); err != nil || back != p {
			t.Errorf("%v: round trip gave %v, %v", p, back, err)
		}
		if p.Description() == "" {
			t.Errorf("%v has no description", p)
		}
	}
	var p PartOfSpeech
	if json.Unmarshal([]byte(`"NOPE"`), &p) == nil {
		t.Error("unknown tag accepted")
	}
	if bad := PartOfSpeech(NumPartsOfSpeech); bad.String() != fmt.Sprintf("POS(%d)", NumPartsOfSpeech) {
		t.Errorf("out-of-range String: %s", bad)
	}
}

func TestDeprel(t *testing.T) {
	for d := range Deprel(NumDeprels) {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()
			b, _ := json.Marshal(d)
			var back Deprel
			if err := json.Unmarshal(b, &back); err != nil || back != d {
				t.Errorf("%v: round trip gave %v, %v", d, back, err)
			}
		})
	}
	for in, want := range map[string]Deprel{"nsubj": DepNsubj, "nsubj:pass": DepNsubj, "OBJ": DepObj, "acl:relcl": DepAcl} {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			if got, ok := ParseDeprel(in); !ok || got != want {
				t.Errorf("ParseDeprel(%q) = %v, %v; want %v", in, got, ok, want)
			}
		})
	}
	if _, ok := ParseDeprel("bogus"); ok {
		t.Error("ParseDeprel accepted bogus")
	}
}

func TestFilterToken(t *testing.T) {
	for _, tc := range []struct {
		tok  Token
		keep bool
	}{
		{Token{Word: "dog", Score: 0.9}, true},
		{Token{Word: "dog", Score: 0.3}, false},
		{Token{Word: "dog", Score: 1.0 / 3}, false}, // threshold is exclusive
		{Token{Word: "...", Score: 0.9}, false},
		{Token{Word: "a.", Score: 0.9}, true},
		{Token{Word: "—", Score: 0.9}, true}, // non-ASCII punctuation is kept
	} {
		if got := tc.tok.Keep(1.0 / 3); got != tc.keep {
			t.Errorf("Keep(%q, score %.2f) = %v, want %v", tc.tok.Word, tc.tok.Score, got, tc.keep)
		}
	}
}

func TestParseLabel(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want PartOfSpeech
		ok   bool
	}{
		{"NN", NN, true},
		{"PRP$", PRPS, true},
		{"NOUN", NN, true},
		{"PROPN", NNP, true},
		{"AUX", MD, true},
		{"B-VERB", VB, true},
		{"I-NOUN+ADP", NN, true},
		{"VERB+AUX", VB, true},
		{"NMS", 0, false},
		{"", 0, false},
	} {
		got, ok := ParseLabel(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("ParseLabel(%q) = %v, %v; want %v, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
	if !IsContinuation("I-NOUN") || IsContinuation("B-NOUN") || IsContinuation("NOUN") {
		t.Error("IsContinuation misclassifies chunk prefixes")
	}
}

func TestLabelDescription(t *testing.T) {
	for label, want := range map[string]string{"NOUN": "Noun", "B-VERB": "Verb", "I-PROPN": "Proper noun", "VERB+AUX": "Verb", "NN": "", "VBZ": ""} {
		if got := LabelDescription(label); got != want {
			t.Errorf("LabelDescription(%q) = %q, want %q", label, got, want)
		}
	}
}

func TestDescriptionIsLanguageNeutral(t *testing.T) {
	for _, tc := range []struct {
		pos  PartOfSpeech
		want string
	}{{NN, "Noun"}, {NNS, "Noun"}, {VBZ, "Verb"}, {VBD, "Verb"}, {JJR, "Adjective"}, {NNPS, "Proper noun"}} {
		if got := tc.pos.Description(); got != tc.want {
			t.Errorf("%v.Description() = %q, want %q", tc.pos, got, tc.want)
		}
	}
	for label, want := range map[string]PartOfSpeech{"NOUN": NN, "NNS": NNS} {
		got, _ := ParseLabel(label)
		if got.Description() != "Noun" || got != want {
			t.Errorf("ParseLabel(%q) = %v", label, got)
		}
	}
}

func TestParseLabelEAGLESAndCKIP(t *testing.T) {
	for label, want := range map[string]PartOfSpeech{
		"NCMS000": NN, "NP00000": NNP, "VMIP3S0": VB, "VAIP3S0": MD, "AQ0CS0": JJ, "RG": RB,
		"DA0MS0": DT, "PP3CNA00": PRP, "CC": CC, "CS": IN, "SP": IN, "Z": CD, "Fc": O, "Zm": CD,
		"Na": NN, "Nb": NNP, "Nh": PRP, "Neu": CD, "VC": VB, "VH": VB, "DE": RP, "D": RB, "P": IN,
		"COMMACATEGORY": O, "Cbb": IN, "FW": FW, "B-Na": NN,
	} {
		got, ok := ParseLabel(label)
		if !ok || got != want {
			t.Errorf("ParseLabel(%q) = %v, %v; want %v", label, got, ok, want)
		}
	}
	if _, ok := ParseLabel("???"); ok {
		t.Error("unknown label accepted")
	}
	if d := LabelDescription("NCMS000"); d != "Noun" {
		t.Errorf("LabelDescription = %q", d)
	}
}
