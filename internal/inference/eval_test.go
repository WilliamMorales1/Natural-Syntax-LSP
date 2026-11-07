package inference

import (
	"bufio"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"natural-syntax-ls/internal/postag"
)

// Accuracy evaluation against Universal Dependencies test treebanks. Off by default: set NSLS_EVAL=1.
//
//	NSLS_ML_DIR    exported fp32 models (required)
//	NSLS_ML_QDIR   quantized copies, evaluated alongside when set
//	NSLS_EVAL_N    sentences per treebank (default 200)
//	NSLS_UD_DIR    treebank cache (default: user cache dir); missing files are downloaded from GitHub
//
// POS accuracy is per source character on the Penn-mapped tags, so models whose words differ from UD tokens (ja, zh) are scored fairly.

type udToken struct {
	id, head   int
	form       string
	upos, rel  string
	mwt        bool
	begin, end int // rune offsets in text, -1 if not located
}

type udSentence struct {
	text string
	toks []udToken
}

var udTreebanks = map[string]string{
	"es": "UD_Spanish-AnCora/master/es_ancora-ud-test.conllu",
	"fr": "UD_French-GSD/master/fr_gsd-ud-test.conllu",
	"ja": "UD_Japanese-GSD/master/ja_gsd-ud-test.conllu",
	"zh": "UD_Chinese-GSD/master/zh_gsd-ud-test.conllu",
}

func udDir(t *testing.T) string {
	if d := os.Getenv("NSLS_UD_DIR"); d != "" {
		return d
	}
	c, err := os.UserCacheDir()
	if err != nil {
		t.Skip("no cache dir")
	}
	return filepath.Join(c, "natural-syntax-ls", "ud")
}

func loadTreebank(t *testing.T, lang string, limit int) []udSentence {
	t.Helper()
	rel := udTreebanks[lang]
	path := filepath.Join(udDir(t), filepath.Base(rel))
	if _, err := os.Stat(path); err != nil {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		resp, err := http.Get("https://raw.githubusercontent.com/UniversalDependencies/" + rel)
		if err != nil || resp.StatusCode != 200 {
			t.Skipf("cannot download %s: %v", rel, err)
		}
		defer resp.Body.Close()
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(f, resp.Body); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var out []udSentence
	var cur udSentence
	flush := func() {
		if cur.text != "" && len(cur.toks) > 0 && locate(&cur) {
			out = append(out, cur)
		}
		cur = udSentence{}
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() && len(out) < limit {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "# text = "):
			cur.text = strings.TrimPrefix(line, "# text = ")
		case line == "":
			flush()
		case line[0] == '#':
		default:
			c := strings.Split(line, "\t")
			if len(c) < 8 || strings.Contains(c[0], ".") {
				continue
			}
			tok := udToken{form: c[1], upos: c[3], rel: c[7], begin: -1}
			if strings.Contains(c[0], "-") {
				tok.mwt = true
			} else {
				tok.id, _ = strconv.Atoi(c[0])
				tok.head, _ = strconv.Atoi(c[6])
			}
			cur.toks = append(cur.toks, tok)
		}
	}
	flush()
	return out
}

// locate fills token rune offsets by scanning forms through the text; multiword-token parts are marked unusable.
func locate(s *udSentence) bool {
	runes := []rune(s.text)
	pos := 0
	for i := range s.toks {
		tok := &s.toks[i]
		form := []rune(tok.form)
		found := -1
		for j := pos; j+len(form) <= len(runes) && j < pos+20; j++ {
			if string(runes[j:j+len(form)]) == tok.form {
				found = j
				break
			}
		}
		if found < 0 {
			return false
		}
		tok.begin, tok.end = found, found+len(form)
		// A multiword token's parts are subforms of its surface form, so the cursor stays at its start.
		if !tok.mwt {
			pos = tok.end
		}
	}
	// Words inside a multiword range have no reliable own span.
	for i, tok := range s.toks {
		if !tok.mwt {
			continue
		}
		for j := i + 1; j < len(s.toks) && !s.toks[j].mwt && s.toks[j].begin < tok.end; j++ {
			s.toks[j].mwt = true
		}
	}
	return true
}

func evalDirs(t *testing.T) map[string]string {
	dirs := map[string]string{"fp32": multilingualDir(t)}
	if q := os.Getenv("NSLS_ML_QDIR"); q != "" {
		dirs["int8"] = q
	}
	return dirs
}

func evalLimit() int {
	if n, err := strconv.Atoi(os.Getenv("NSLS_EVAL_N")); err == nil && n > 0 {
		return n
	}
	return 200
}

func TestEvalPOS(t *testing.T) {
	if os.Getenv("NSLS_EVAL") == "" {
		t.Skip("NSLS_EVAL unset")
	}
	dirs := evalDirs(t)
	for _, tc := range []struct{ lang, prefix, vocab string }{
		{"es", udposPrefix + "es", "_tokenizer.json"},
		{"fr", udposPrefix + "fr", "_tokenizer.json"},
		{"ja", udposPrefix + "ja", "_tokenizer.json"},
		{"zh", udposPrefix + "zh", "_tokenizer.json"},
		{"zh-char", yasukoZHPrefix, "_vocab.txt"},
	} {
		lang := strings.SplitN(tc.lang, "-", 2)[0]
		sents := loadTreebank(t, lang, evalLimit())
		for variant, dir := range dirs {
			base := filepath.Join(dir, tc.prefix)
			m, err := NewPOSModel(base+".onnx", base+tc.vocab)
			if err != nil {
				t.Logf("%s %s: skipped: %v", tc.lang, variant, err)
				continue
			}
			var right, total int
			for _, s := range sents {
				gold := make([]int, len([]rune(s.text)))
				for i := range gold {
					gold[i] = -1
				}
				for _, tok := range s.toks {
					if p, ok := postag.ParseLabel(tok.upos); ok && p != postag.O && !tok.mwt {
						for r := tok.begin; r < tok.end; r++ {
							gold[r] = int(p)
						}
					}
				}
				toks, err := m.PredictChunk(m.Tokenize(s.text))
				if err != nil {
					t.Fatal(err)
				}
				for _, tok := range toks {
					for r := tok.OffsetBegin; r < tok.OffsetEnd; r++ {
						if gold[r] >= 0 {
							total++
							if gold[r] == int(tok.Tag) {
								right++
							}
						}
					}
				}
			}
			m.Close()
			t.Logf("POS %s %-4s %d sentences: %.1f%% of %d characters", tc.lang, variant, len(sents), 100*float64(right)/float64(max(total, 1)), total)
		}
	}
}

func TestEvalDependency(t *testing.T) {
	if os.Getenv("NSLS_EVAL") == "" {
		t.Skip("NSLS_EVAL unset")
	}
	dirs := evalDirs(t)
	for lang, slug := range map[string]string{"es": "es_ancora_mbert", "fr": "fr_sequoia_camembert_large"} {
		sents := loadTreebank(t, lang, evalLimit())
		for variant, dir := range dirs {
			base := filepath.Join(dir, slug+"_dependency")
			vocab := base + "_vocab.txt"
			if _, err := os.Stat(vocab); err != nil {
				vocab = base + "_tokenizer.json"
			}
			m, err := NewDependencyModel(base+".onnx", vocab)
			if err != nil {
				t.Logf("%s %s: skipped: %v", lang, variant, err)
				continue
			}
			var uas, las, total int
			for _, s := range sents {
				byBegin := map[uint32]int{}
				for i, tok := range s.toks {
					if !tok.mwt && tok.rel != "punct" {
						byBegin[uint32(tok.begin)] = i
					}
				}
				idBegin := map[int]int{0: -1}
				for _, tok := range s.toks {
					if !tok.mwt {
						idBegin[tok.id] = tok.begin
					}
				}
				predicted, err := m.PredictChunk(m.Tokenize(s.text))
				if err != nil {
					t.Fatal(err)
				}
				for _, p := range predicted {
					gi, ok := byBegin[p.OffsetBegin]
					if !ok {
						continue
					}
					g := s.toks[gi]
					total++
					headOK := (g.head == 0 && !p.HasHead) || (p.HasHead && idBegin[g.head] == int(p.HeadOffsetBegin))
					if headOK {
						uas++
						if want, ok := postag.ParseDeprel(strings.SplitN(g.rel, ":", 2)[0]); ok && want == p.Deprel {
							las++
						}
					}
				}
			}
			m.Close()
			t.Logf("DEP %s %-4s %d sentences: UAS %.1f%% LAS %.1f%% over %d words", lang, variant, len(sents), 100*float64(uas)/float64(max(total, 1)), 100*float64(las)/float64(max(total, 1)), total)
		}
	}
}
