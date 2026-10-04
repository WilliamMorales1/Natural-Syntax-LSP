package postag

import (
	"encoding/json"
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
	if PartOfSpeech(-1).String() != "POS(-1)" {
		t.Errorf("out-of-range String: %s", PartOfSpeech(-1))
	}
}

func TestDeprel(t *testing.T) {
	for d := range Deprel(NumDeprels) {
		b, _ := json.Marshal(d)
		var back Deprel
		if err := json.Unmarshal(b, &back); err != nil || back != d {
			t.Errorf("%v: round trip gave %v, %v", d, back, err)
		}
	}
	for in, want := range map[string]Deprel{"nsubj": DepNsubj, "nsubj:pass": DepNsubj, "OBJ": DepObj, "acl:relcl": DepAcl} {
		if got, ok := ParseDeprel(in); !ok || got != want {
			t.Errorf("ParseDeprel(%q) = %v, %v; want %v", in, got, ok, want)
		}
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
