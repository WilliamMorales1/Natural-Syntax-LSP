package arabic

import (
	"slices"
	"testing"

	"natural-syntax-ls/internal/lang"
)

func TestForModel(t *testing.T) {
	for in, want := range map[string]string{"xlmr_ar_pos.onnx": "ar", "ar_padt.onnx": "ar", "xlmr_fr.onnx": ""} {
		if got := lang.ForModel([]lang.Language{Language()}, in); got != want {
			t.Errorf("ForModel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestForms(t *testing.T) {
	for _, c := range []struct {
		in   string
		want string
	}{
		{"وَالْكِتَاب", "والكتاب"},
		{"كـتـاب", "كتاب"},
		{"مُحَمَّد", "محمد"},
	} {
		if got := Forms(c.in); !slices.Contains(got, c.want) {
			t.Errorf("Forms(%q) = %q, want it to contain %q", c.in, got, c.want)
		}
	}
	if got := Forms("كتاب"); len(got) != 0 {
		t.Errorf("Forms(كتاب) = %q, want none", got)
	}
}
