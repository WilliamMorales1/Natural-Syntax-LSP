package russian

import (
	"slices"
	"testing"

	"natural-syntax-ls/internal/lang"
)

func TestForModel(t *testing.T) {
	for in, want := range map[string]string{"xlmr_ru_pos.onnx": "ru", "/m/XLMR_RU_int8.onnx": "ru", "xlmr_es.onnx": ""} {
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
		{"мо́ре", "море"},
		{"елка", "ёлка"},
		{"все", "всё"},
	} {
		if got := Forms(c.in); !slices.Contains(got, c.want) {
			t.Errorf("Forms(%q) = %q, want it to contain %q", c.in, got, c.want)
		}
	}
	if got := Forms("дом"); len(got) != 0 {
		t.Errorf("Forms(дом) = %q, want none", got)
	}
}
