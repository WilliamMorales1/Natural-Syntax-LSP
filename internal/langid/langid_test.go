package langid

import (
	"bufio"
	"compress/bzip2"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDetect(t *testing.T) {
	cases := []struct{ text, want string }{
		{"The quick brown fox jumps over the lazy dog.", "en"},
		{"Le renard brun saute par-dessus le chien paresseux.", "fr"},
		{"El zorro marrón salta sobre el perro perezoso.", "es"},
		{"Der schnelle braune Fuchs springt über den faulen Hund.", "de"},
		{"Быстрая коричневая лиса прыгает через ленивую собаку.", "ru"},
		{"素早い茶色の狐が怠け者の犬を飛び越える。", "ja"},
		{"敏捷的棕色狐狸跳过了懒狗。", "zh"},
		{"敏捷的棕色狐狸跳過了懶狗。", "zh"},
		{"الثعلب البني السريع يقفز فوق الكلب الكسول", "ar"},
		{"", ""},
		{"ok", ""},
		{"1234 5678 !!!", ""},
	}
	for _, c := range cases {
		if got := Detect(c.text); got != c.want {
			t.Errorf("Detect(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

func TestLanguages(t *testing.T) {
	if got := Languages(); len(got) < 6 {
		t.Fatalf("Languages() = %v", got)
	}
}

func BenchmarkDetect(b *testing.B) {
	Detect("warm up")
	s := "Le renard brun saute par-dessus le chien paresseux et il continue de courir dans la forêt."
	for b.Loop() {
		Detect(s)
	}
}

// bucket names a text length class by rune count.
func bucket(n int) string {
	switch {
	case n < 6:
		return "1-5"
	case n <= 12:
		return "6-12"
	case n <= 20:
		return "13-20"
	}
	return "21+"
}

var buckets = []string{"1-5", "6-12", "13-20", "21+"}

// TestHeldOutAccuracy scores the sentences train leaves out (every 20th) from the Tatoeba dumps in NSLS_LANGID_DIR, by length bucket, and prints a confusion matrix ("-" is an abstention).
func TestHeldOutAccuracy(t *testing.T) {
	dir := os.Getenv("NSLS_LANGID_DIR")
	if dir == "" {
		t.Skip("NSLS_LANGID_DIR not set")
	}
	files := []struct{ file, code string }{{"eng", "en"}, {"fra", "fr"}, {"spa", "es"}, {"deu", "de"}, {"ita", "it"}, {"por", "pt"}, {"rus", "ru"}, {"jpn", "ja"}, {"cmn", "zh"}, {"ara", "ar"}, {"cat", "ca"}, {"ukr", "uk"}, {"kor", "ko"}}
	detect := testDetector(t)
	confusion := map[string]map[string]int{}
	type tally struct{ n, ok int }
	byBucket := map[string]map[string]*tally{}
	for _, fc := range files {
		f, err := os.Open(filepath.Join(dir, fc.file+"_sentences.tsv.bz2"))
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(bzip2.NewReader(f))
		sc.Buffer(nil, 1<<20)
		confusion[fc.code] = map[string]int{}
		byBucket[fc.code] = map[string]*tally{}
		var n, ok int
		for j := 0; sc.Scan(); j++ {
			if j%20 != 19 {
				continue
			}
			parts := strings.SplitN(sc.Text(), "\t", 3)
			if len(parts) < 3 {
				continue
			}
			got := detect(parts[2])
			b := bucket(len([]rune(parts[2])))
			if byBucket[fc.code][b] == nil {
				byBucket[fc.code][b] = &tally{}
			}
			tl := byBucket[fc.code][b]
			tl.n++
			n++
			if got == fc.code {
				tl.ok++
				ok++
			}
			if got == "" {
				got = "-"
			}
			confusion[fc.code][got]++
		}
		f.Close()
		var row strings.Builder
		for _, b := range buckets {
			if tl := byBucket[fc.code][b]; tl != nil && tl.n > 0 {
				fmt.Fprintf(&row, " %s: %.1f%% (%d);", b, 100*float64(tl.ok)/float64(tl.n), tl.n)
			}
		}
		t.Logf("%s: %.2f%% of %d;%s", fc.code, 100*float64(ok)/float64(n), n, row.String())
		// Catalan has little Tatoeba data and shares most of its vocabulary with Spanish and French.
		bar := 0.95
		if fc.code == "ca" {
			bar = 0.90
		}
		if float64(ok)/float64(n) < bar {
			t.Errorf("%s accuracy %.2f%% < %.0f%%", fc.code, 100*float64(ok)/float64(n), 100*bar)
		}
	}
	cols := append(Languages(), "-")
	slices.Sort(cols)
	var m strings.Builder
	m.WriteString("confusion (row = truth, column = detected)\n      ")
	for _, c := range cols {
		fmt.Fprintf(&m, "%8s", c)
	}
	m.WriteByte('\n')
	for _, fc := range files {
		fmt.Fprintf(&m, "%5s ", fc.code)
		for _, c := range cols {
			fmt.Fprintf(&m, "%8d", confusion[fc.code][c])
		}
		m.WriteByte('\n')
	}
	t.Log("\n" + m.String())
}

// TestCalibration prints, per confidence band, how often the linear model's best language is right on held-out sentences of 20 characters or fewer; a calibrated model's accuracy sits inside its band.
func TestCalibration(t *testing.T) {
	dir := os.Getenv("NSLS_LANGID_DIR")
	if dir == "" {
		t.Skip("NSLS_LANGID_DIR not set")
	}
	m, err := loadLinear()
	if err != nil {
		t.Fatal(err)
	}
	type band struct{ n, ok int }
	bands := make([]band, 10)
	for file, code := range map[string]string{"eng": "en", "fra": "fr", "spa": "es", "deu": "de", "ita": "it", "por": "pt", "rus": "ru", "jpn": "ja", "cmn": "zh", "ara": "ar"} {
		f, err := os.Open(filepath.Join(dir, file+"_sentences.tsv.bz2"))
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(bzip2.NewReader(f))
		sc.Buffer(nil, 1<<20)
		for j := 0; sc.Scan(); j++ {
			parts := strings.SplitN(sc.Text(), "\t", 3)
			if j%20 != 19 || len(parts) < 3 || len([]rune(parts[2])) > 20 {
				continue
			}
			logits, letters := m.scores(parts[2])
			if letters < minLetters {
				continue
			}
			best, peak := 0, logits[0]
			for l, v := range logits {
				if v > peak {
					best, peak = l, v
				}
			}
			var sum float64
			for _, v := range logits {
				sum += math.Exp(v - peak)
			}
			b := min(int(10/sum), 9)
			bands[b].n++
			if m.codes[best] == code {
				bands[b].ok++
			}
		}
		f.Close()
	}
	for i, b := range bands {
		if b.n > 0 {
			t.Logf("confidence %.1f-%.1f: %.1f%% right of %d", float64(i)/10, float64(i+1)/10, 100*float64(b.ok)/float64(b.n), b.n)
		}
	}
}

// testDetector is Detect, or the linear model file in NSLS_LANGID_LINEAR when set.
func testDetector(t *testing.T) func(string) string {
	path := os.Getenv("NSLS_LANGID_LINEAR")
	if path == "" {
		return Detect
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := parseLinear(b)
	if err != nil {
		t.Fatal(err)
	}
	return func(s string) string { c, _ := m.detect(s); return c }
}

// TestUDOutOfDomain scores the sentences of UD test treebanks (<code>.test.txt in NSLS_LANGID_UD, one per line), text the model never trained on and unlike Tatoeba's short sentences.
func TestUDOutOfDomain(t *testing.T) {
	dir := os.Getenv("NSLS_LANGID_UD")
	if dir == "" {
		t.Skip("NSLS_LANGID_UD not set")
	}
	detect := testDetector(t)
	for _, code := range []string{"en", "fr", "es", "de", "it", "pt", "ru", "ja", "zh", "ar", "ca", "uk", "ko"} {
		b, err := os.ReadFile(filepath.Join(dir, code+".test.txt"))
		if err != nil {
			t.Fatal(err)
		}
		var n, ok, none int
		confused := map[string]int{}
		for _, line := range strings.Split(string(b), "\n") {
			if line = strings.TrimSpace(line); line == "" {
				continue
			}
			n++
			switch got := detect(line); got {
			case code:
				ok++
			case "":
				none++
			default:
				confused[got]++
			}
		}
		t.Logf("%s: %.2f%% of %d, %d abstained, wrong: %v", code, 100*float64(ok)/float64(n), n, none, confused)
	}
}
