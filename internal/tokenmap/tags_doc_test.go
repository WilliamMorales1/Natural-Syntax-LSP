package tokenmap

import (
	"os"
	"strings"
	"testing"

	"natural-syntax-ls/internal/postag"
)

// tagsTable returns the cells of the docs/tags.md table under heading.
func tagsTable(t *testing.T, heading string) [][]string {
	t.Helper()
	data, err := os.ReadFile("../../docs/tags.md")
	if err != nil {
		t.Fatal(err)
	}
	_, after, ok := strings.Cut(string(data), "\n## "+heading+"\n")
	if !ok {
		t.Fatalf("no %q section", heading)
	}
	var rows [][]string
	for line := range strings.SplitSeq(after, "\n") {
		if strings.HasPrefix(line, "## ") {
			break
		}
		if !strings.HasPrefix(line, "|") || strings.HasPrefix(line, "|---") {
			continue
		}
		var cells []string
		for c := range strings.SplitSeq(strings.Trim(line, "|"), "|") {
			cells = append(cells, strings.ReplaceAll(strings.TrimSpace(c), "`", ""))
		}
		rows = append(rows, cells)
	}
	if len(rows) < 2 {
		t.Fatalf("%q table is empty", heading)
	}
	return rows[1:] // header row
}

func TestTagsDocPOSTable(t *testing.T) {
	m := NewDefault()
	seen := map[postag.PartOfSpeech]bool{}
	for _, row := range tagsTable(t, "POS classes") {
		class, penn, upos, want := row[0], row[1], row[2], row[3]
		check := func(label string, pos postag.PartOfSpeech) {
			if got := m.Get(pos).String(); got != want {
				t.Errorf("%s: token type %q, docs say %q", label, got, want)
			}
			if got := pos.Description(); got != class {
				t.Errorf("%s: class %q, docs say %q", label, got, class)
			}
		}
		for tag := range strings.SplitSeq(penn, ", ") {
			pos, ok := postag.ParsePartOfSpeech(tag)
			if !ok {
				t.Errorf("docs list unknown Penn tag %q", tag)
				continue
			}
			seen[pos] = true
			check(tag, pos)
		}
		for label := range strings.SplitSeq(upos, ", ") {
			pos, ok := postag.ParseLabel(label)
			if !ok {
				t.Errorf("docs list unknown UPOS %q", label)
				continue
			}
			check(label, pos)
		}
	}
	for i := range postag.NumPartsOfSpeech {
		if pos := postag.PartOfSpeech(i); pos != postag.O && !seen[pos] {
			t.Errorf("Penn tag %s missing from docs/tags.md", pos)
		}
	}
}

func TestTagsDocDeprelTable(t *testing.T) {
	rows := tagsTable(t, "UD relations")
	if len(rows) != int(postag.NumDeprels) {
		t.Errorf("docs list %d relations, code has %d", len(rows), postag.NumDeprels)
	}
	for _, row := range rows {
		rel, ok := postag.ParseDeprel(row[1])
		if !ok {
			t.Errorf("docs list unknown relation %q", row[1])
			continue
		}
		if got, want := DeprelBits(rel).String(), row[4]; got != want {
			t.Errorf("%s: token type %q, docs say %q", row[1], got, want)
		}
	}
}
