package lang

import "testing"

func TestForModel(t *testing.T) {
	langs := []Language{{Code: "es", ModelPrefixes: []string{"xlmr_es"}}, {Code: "fr", ModelPrefixes: []string{"fr_sequoia"}}, {Code: "en"}}
	for in, want := range map[string]string{
		"/x/XLMR_es_pos.onnx": "es", "fr_sequoia.camembert.onnx": "fr", "bert_base.onnx": "", "multilingual_minilm.onnx": "",
	} {
		if got := ForModel(langs, in); got != want {
			t.Errorf("ForModel(%q) = %q, want %q", in, got, want)
		}
	}
}
