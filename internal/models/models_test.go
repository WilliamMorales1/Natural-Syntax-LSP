package models

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "models.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestShippedCatalog(t *testing.T) {
	c, err := Load("../../data")
	if err != nil {
		t.Fatal(err)
	}
	for mode, want := range map[string][]string{
		"pos":        {"bert_base", "mobilebert"},
		"semantic":   {"multilingual_minilm"},
		"dependency": {"en_ewt_electra_base_dependency"},
	} {
		if got := c.DefaultStems(mode); !slices.Equal(got, want) {
			t.Errorf("DefaultStems(%q) = %q, want %q", mode, got, want)
		}
	}
}

func TestLoadOverridesByName(t *testing.T) {
	a := write(t, `{"models":[{"name":"x-y","mode":"pos","default":true},{"name":"keep","mode":"pos"}]}`)
	b := write(t, `{"models":[{"name":"x-y","mode":"pos","default":false},{"name":"z.w","mode":"dependency","default":true}]}`)
	c, err := Load(a, filepath.Join(a, "missing"), b)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Models) != 3 {
		t.Fatalf("models = %+v", c.Models)
	}
	if got := c.DefaultStems(""); !slices.Equal(got, []string{"z_w_dependency"}) {
		t.Errorf("DefaultStems = %q", got)
	}
}

func TestLoadBadJSON(t *testing.T) {
	if _, err := Load(write(t, `{`)); err == nil {
		t.Error("want error")
	}
}

func TestStemsForPrefersLanguage(t *testing.T) {
	dir := write(t, `{"models":[
	 {"name":"en-a","mode":"pos","language":"en","default":true},
	 {"name":"xlmr-es","mode":"pos","language":"es"},
	 {"name":"multi","mode":"pos"},
	 {"name":"other","mode":"semantic","language":"es"}]}`)
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := c.StemsFor("pos", "es"), []string{"xlmr_es", "multi", "en_a"}; !slices.Equal(got, want) {
		t.Errorf("StemsFor(pos, es) = %q, want %q", got, want)
	}
	if got, want := c.StemsFor("pos", ""), []string{"en_a"}; !slices.Equal(got, want) {
		t.Errorf("StemsFor(pos, \"\") = %q, want %q", got, want)
	}
}

func TestStemMatchesSlugCases(t *testing.T) {
	raw, err := os.ReadFile("../../data/slug_cases.tsv")
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(raw)), "\n") {
		name, want, _ := strings.Cut(strings.TrimSuffix(line, "\r"), "\t")
		if got := (Model{Name: name}).Stem(); got != want {
			t.Errorf("Stem(%q) = %q, want %q", name, got, want)
		}
	}
}
