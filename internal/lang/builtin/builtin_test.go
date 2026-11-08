package builtin

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"natural-syntax-ls/internal/lang"
	"natural-syntax-ls/internal/postag"
	"natural-syntax-ls/internal/tokenizer"
)

// repoLang is the shipped script directory, which setup scripts copy into the config dir.
const repoLang = "../../../data/lang"

func byCode(t *testing.T, code string, dirs ...string) lang.Language {
	t.Helper()
	for _, l := range Languages(append([]string{repoLang}, dirs...)...) {
		if l.Code == code {
			return l
		}
	}
	t.Fatalf("language %q not loaded", code)
	return lang.Language{}
}

func TestScriptedLanguagesLoad(t *testing.T) {
	var codes []string
	for _, l := range Languages(repoLang) {
		codes = append(codes, l.Code)
	}
	for _, want := range []string{"en", "fr", "es", "ru", "ar", "de", "it", "pt"} {
		if !slices.Contains(codes, want) {
			t.Errorf("missing %q in %q", want, codes)
		}
	}
	if !slices.Equal(codes[:4], []string{"en", "fr", "es", "ru"}) {
		t.Errorf("order = %q", codes)
	}
}

func TestForModel(t *testing.T) {
	langs := Languages(repoLang)
	for in, want := range map[string]string{
		"xlmr_ru_pos.onnx": "ru", "/m/XLMR_RU_int8.onnx": "ru", "ru_syntagrus.onnx": "ru",
		"xlmr_es.onnx": "es", "es_ancora.mbert.onnx": "es",
		"xlmr_fr.onnx": "fr", "fr_sequoia.camembert.onnx": "fr", "xlmr_de.onnx": "de", "xlmr_it_int8.onnx": "it", "xlmr_pt.onnx": "pt", "xlmr_ar.onnx": "ar", "bert_base.onnx": "", "multilingual_minilm.onnx": "",
	} {
		if got := lang.ForModel(langs, in); got != want {
			t.Errorf("ForModel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRussianForms(t *testing.T) {
	forms := byCode(t, "ru").Wiktionary.Forms
	for in, want := range map[string]string{"мо́ре": "море", "елка": "ёлка", "все": "всё"} {
		if got := forms(in); !slices.Contains(got, want) {
			t.Errorf("Forms(%q) = %q, want it to contain %q", in, got, want)
		}
	}
	if got := forms("дом"); len(got) != 0 {
		t.Errorf("Forms(дом) = %q, want none", got)
	}
}

func TestArabicForms(t *testing.T) {
	forms := byCode(t, "ar").Wiktionary.Forms
	for in, want := range map[string]string{
		"المدرسة": "مدرسة", "كَلْب": "كلب", "للعدل": "عدل", "بالمدرسة": "مدرسة", "والكلب": "كلب", "الكَلْب": "كلب",
	} {
		if got := forms(in); !slices.Contains(got, want) {
			t.Errorf("Forms(%q) = %q, want it to contain %q", in, got, want)
		}
	}
	if got := forms("كلب"); len(got) != 0 {
		t.Errorf("Forms(كلب) = %q, want none", got)
	}
	if got := forms("ال"); len(got) != 0 {
		t.Errorf("Forms(ال) = %q, want none", got)
	}
}

func TestEnglishRewrite(t *testing.T) {
	rewrite := byCode(t, "en").Wiktionary.Rewrite
	for _, c := range []struct {
		word string
		pos  postag.PartOfSpeech
		dep  postag.Deprel
		want string
	}{
		{"'s", postag.POS, postag.DepRoot, "-'s"},
		{"'S", postag.NN, postag.DepCase, "-'s"},
		{"dog", postag.NN, postag.DepRoot, ""},
		{"of", postag.IN, postag.DepCase, ""},
	} {
		if got := rewrite(c.word, c.pos, c.dep); got != c.want {
			t.Errorf("Rewrite(%q, %v, %v) = %q, want %q", c.word, c.pos, c.dep, got, c.want)
		}
	}
}

func split(text string, cuts ...int) []tokenizer.WordSpan {
	runes := []rune(text)
	var out []tokenizer.WordSpan
	prev := 0
	for _, c := range append(cuts, len(runes)) {
		if c > prev {
			out = append(out, tokenizer.WordSpan{Text: string(runes[prev:c]), Begin: uint32(prev), End: uint32(c)})
		}
		prev = c
	}
	return out
}

func TestTokenizerKeepsElisions(t *testing.T) {
	const tiny = `{"model": {"type": "Unigram", "unk_id": 1, "vocab": [
  ["[CLS]", 0], ["[UNK]", 0], ["[SEP]", 0], ["l'", -4], ["ami", -5], ["a", -9], ["m", -9], ["i", -9]]},
 "pre_tokenizer": {"type": "Whitespace"},
 "post_processor": {"single": [{"SpecialToken": {"id": "[CLS]"}}, {"Sequence": {"id": "A"}}, {"SpecialToken": {"id": "[SEP]"}}]}}`
	path := filepath.Join(t.TempDir(), "m_tokenizer.json")
	if err := os.WriteFile(path, []byte(tiny), 0o600); err != nil {
		t.Fatal(err)
	}
	tok, err := tokenizer.Open(path, byCode(t, "fr").Tokenizer...)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, w := range tok.Tokenize("l'ami L’ami d'accord aujourd'hui rock'n") {
		got = append(got, w.Text)
	}
	if want := []string{"l'", "ami", "L’", "ami", "d'", "accord", "aujourd'hui", "rock", "'", "n"}; !slices.Equal(got, want) {
		t.Errorf("Tokenize = %q, want %q", got, want)
	}
}

func TestDirOverridesByCode(t *testing.T) {
	dir := t.TempDir()
	script := `return { code = "es", model_prefixes = {"custom_es"}, wiktionary = { code = "es", sections = {"Spanish"} } }`
	if err := os.WriteFile(filepath.Join(dir, "es.lua"), []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	// A broken script must not take other scripts down.
	if err := os.WriteFile(filepath.Join(dir, "bad.lua"), []byte(`return {code = 1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	langs := Languages(repoLang, dir)
	if got := lang.ForModel(langs, "custom_es.onnx"); got != "es" {
		t.Errorf("override not applied: %q", got)
	}
	if got := lang.ForModel(langs, "xlmr_es.onnx"); got != "" {
		t.Errorf("old prefixes survived override: %q", got)
	}
	if got := lang.ForModel(langs, "xlmr_fr.onnx"); got != "fr" {
		t.Errorf("fr lost: %q", got)
	}
}

func TestElisionsOnlyInDetectedFrench(t *testing.T) {
	const tiny = `{"model": {"type": "Unigram", "unk_id": 1, "vocab": [
  ["[CLS]", 0], ["[UNK]", 0], ["[SEP]", 0], ["l'", -4], ["ami", -5], ["a", -9], ["m", -9], ["i", -9]]},
 "pre_tokenizer": {"type": "Whitespace"},
 "post_processor": {"single": [{"SpecialToken": {"id": "[CLS]"}}, {"Sequence": {"id": "A"}}, {"SpecialToken": {"id": "[SEP]"}}]}}`
	path := filepath.Join(t.TempDir(), "m_tokenizer.json")
	if err := os.WriteFile(path, []byte(tiny), 0o600); err != nil {
		t.Fatal(err)
	}
	tok, err := tokenizer.Open(path, lang.TokenizerOptions(Languages(repoLang), "")...)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, w := range tok.Tokenize("Le renard saute par-dessus le chien, et l'ami regarde.\nThe quick brown fox jumps over the lazy dog, and l'ami watches.") {
		got = append(got, w.Text)
	}
	joined := " " + strings.Join(got, " ") + " "
	if !strings.Contains(joined, " l' ami regarde") || !strings.Contains(joined, " l ' ami watches") {
		t.Errorf("tokens = %q", got)
	}
}

// BenchmarkLanguages measures loading every shipped script; startup budget is 5 ms per language.
func BenchmarkLanguages(b *testing.B) {
	for b.Loop() {
		Languages(repoLang)
	}
}
