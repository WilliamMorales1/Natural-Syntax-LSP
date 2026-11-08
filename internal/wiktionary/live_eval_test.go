package wiktionary

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	nlang "natural-syntax-ls/internal/lang"
	"natural-syntax-ls/internal/lang/builtin"
	"natural-syntax-ls/internal/postag"
	"natural-syntax-ls/internal/tokenizer"
)

// Live accuracy check against the Wiktionary API and UD test treebanks. Off by default: NSLS_WIKT_EVAL=1.
//
//	NSLS_WIKT_N    unique words per language and mode (default 100)
//	NSLS_WIKT_OUT  optional TSV path for every lookup
//	NSLS_CFG_DIR   tokenizer files for pipeline tokens (default ~/.config/natural-syntax-ls)

var udFiles = map[string]string{
	"en": "UD_English-EWT/master/en_ewt-ud-test.conllu",
	"es": "UD_Spanish-AnCora/master/es_ancora-ud-test.conllu",
	"fr": "UD_French-GSD/master/fr_gsd-ud-test.conllu",
	"ja": "UD_Japanese-GSD/master/ja_gsd-ud-test.conllu",
	"zh": "UD_Chinese-GSD/master/zh_gsd-ud-test.conllu",
	"ru": "UD_Russian-GSD/master/ru_gsd-ud-test.conllu",
}

type udWord struct{ form, upos, rel string }

func loadUD(t *testing.T, lang string) (words []udWord, texts []string) {
	t.Helper()
	cache, _ := os.UserCacheDir()
	path := filepath.Join(cache, "natural-syntax-ls", "ud", filepath.Base(udFiles[lang]))
	if _, err := os.Stat(path); err != nil {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		resp, err := http.Get("https://raw.githubusercontent.com/UniversalDependencies/" + udFiles[lang])
		if err != nil || resp.StatusCode != 200 {
			t.Skipf("cannot fetch treebank %s: %v", lang, err)
		}
		defer resp.Body.Close()
		f, _ := os.Create(path)
		_, _ = io.Copy(f, resp.Body)
		f.Close()
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if s, ok := strings.CutPrefix(line, "# text = "); ok {
			texts = append(texts, s)
			continue
		}
		if line == "" || line[0] == '#' {
			continue
		}
		c := strings.Split(line, "\t")
		if len(c) < 8 || strings.ContainsAny(c[0], ".-") {
			continue
		}
		words = append(words, udWord{c[1], c[3], c[7]})
	}
	return words, texts
}

// sections lists the Wiktionary headings that count as correct for each UPOS tag.
var sections = map[string][]string{
	"NOUN": {"noun"}, "PROPN": {"proper noun", "noun"}, "VERB": {"verb", "participle"}, "AUX": {"verb", "auxiliary verb"},
	"ADJ": {"adjective", "participle"}, "ADV": {"adverb"}, "ADP": {"preposition", "postposition", "particle", "prefix"},
	"DET": {"article", "determiner", "pronoun", "adjective", "numeral"}, "PRON": {"pronoun", "determiner"},
	"CCONJ": {"conjunction"}, "SCONJ": {"conjunction", "preposition", "pronoun"}, "NUM": {"numeral", "number", "noun", "symbol"},
	"INTJ": {"interjection"}, "PART": {"particle", "adverb", "preposition", "suffix", "postposition", "verb"},
}

type liveFetcher struct {
	client *Client
	mu     sync.Mutex
	pages  map[string]map[string][]entry
	errs   map[string]bool
	last   time.Time
	calls  int
}

func (lf *liveFetcher) get(ctx context.Context, word string) (map[string][]entry, error) {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	if p, ok := lf.pages[word]; ok {
		return p, nil
	}
	if lf.errs[word] {
		return nil, fmt.Errorf("cached miss")
	}
	var p map[string][]entry
	var err error
	for try := range 4 {
		if d := 150*time.Millisecond - time.Since(lf.last); d > 0 {
			time.Sleep(d)
		}
		lf.last = time.Now()
		lf.calls++
		p, err = lf.client.fetchRemote(ctx, word)
		if err == nil || !strings.Contains(err.Error(), "429") {
			break
		}
		time.Sleep(time.Duration(2<<try) * time.Second)
	}
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			lf.errs[word] = true
		}
		return nil, err
	}
	lf.pages[word] = p
	return p, nil
}

func hasLetter(s string) bool { return strings.IndexFunc(s, unicode.IsLetter) >= 0 }

// sectionOf finds which section of the language bucket produced def.
func sectionOf(payload map[string][]entry, lang, def string) string {
	if i := strings.Index(def, ")* "); strings.HasPrefix(def, "*(") && i > 0 {
		def = def[i+3:]
	}
	for _, b := range []string{lang, "en"} {
		for _, e := range payload[b] {
			for _, d := range e.Definitions {
				if stripHTML(d.Definition) == def {
					return strings.ToLower(e.PartOfSpeech)
				}
			}
		}
	}
	return ""
}

type stat struct {
	n, hit, sect, sectN int
	miss, wrong         []string
}

func TestLiveAccuracy(t *testing.T) {
	if os.Getenv("NSLS_WIKT_EVAL") == "" {
		t.Skip("NSLS_WIKT_EVAL unset")
	}
	n := 100
	if v := os.Getenv("NSLS_WIKT_N"); v != "" {
		fmt.Sscan(v, &n)
	}
	wl := nlang.WiktionaryLangs(builtin.Languages("../../data/lang"))
	if os.Getenv("NSLS_WIKT_NOFORMS") != "" { // baseline without the Forms hooks
		for i := range wl {
			wl[i].Forms = nil
		}
	}
	lf := &liveFetcher{client: NewClient(wl...), pages: map[string]map[string][]entry{}, errs: map[string]bool{}}
	lf.client.fetch = lf.get

	cfg := os.Getenv("NSLS_CFG_DIR")
	if cfg == "" {
		home, _ := os.UserHomeDir()
		cfg = filepath.Join(home, ".config", "natural-syntax-ls")
	}
	tokFiles := map[string]string{
		"en": "bert_base_vocab.txt", "es": "xlmr_es_tokenizer.json", "fr": "xlmr_fr_tokenizer.json",
		"ja": "multilingual_minilm_tokenizer.json", "zh": "multilingual_minilm_tokenizer.json",
		"ru": "xlmr_ru_tokenizer.json",
	}

	var out *os.File
	if p := os.Getenv("NSLS_WIKT_OUT"); p != "" {
		out, _ = os.Create(p)
		defer out.Close()
	}
	type row struct {
		lang, mode string
		s          stat
	}
	var rows []row

	langs := []string{"en", "es", "fr", "ja", "zh"}
	if v := os.Getenv("NSLS_WIKT_LANGS"); v != "" {
		langs = strings.Fields(v)
	}
	for _, lang := range langs {
		words, texts := loadUD(t, lang)

		// Unique content words in treebank order, evenly strided down to n.
		seen := map[string]bool{}
		var uniq []udWord
		for _, w := range words {
			if _, ok := sections[w.upos]; !ok || !hasLetter(w.form) {
				continue
			}
			key := strings.ToLower(w.form) + "/" + w.upos
			if !seen[key] {
				seen[key] = true
				uniq = append(uniq, w)
			}
		}
		var sample []udWord
		for i := range min(n, len(uniq)) {
			sample = append(sample, uniq[i*len(uniq)/min(n, len(uniq))])
		}

		run := func(mode string, ws []udWord, tag func(udWord) (postag.PartOfSpeech, postag.Deprel), check bool) {
			var s stat
			for _, w := range ws {
				pos, rel := tag(w)
				def, _, ok, _ := lf.client.lookup(t.Context(), w.form, lang, "", pos, rel)
				s.n++
				sec := ""
				if ok {
					s.hit++
					for _, form := range []string{w.form, strings.ToLower(w.form)} {
						if p, err := lf.get(t.Context(), form); err == nil {
							if sec = sectionOf(p, lang, def); sec != "" {
								break
							}
						}
					}
				} else {
					s.miss = append(s.miss, w.form)
				}
				good := false
				if check && ok && sec != "" && sec != "hanzi" { // hanzi and lemma-page hits carry no checkable section
					s.sectN++
					for _, c := range sections[w.upos] {
						if sec == c {
							good = true
						}
					}
					if good {
						s.sect++
					} else {
						s.wrong = append(s.wrong, fmt.Sprintf("%s(%s≠%s)", w.form, w.upos, sec))
					}
				}
				if out != nil {
					fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%v\t%q\t%s\t%v\n", lang, mode, w.form, w.upos, ok, sec, w.rel, good)
				}
			}
			rows = append(rows, row{lang, mode, s})
		}

		// pos: gold UPOS stands in for the POS model's tag.
		run("pos", sample, func(w udWord) (postag.PartOfSpeech, postag.Deprel) {
			p, _ := postag.ParseLabel(w.upos)
			return p, postag.DepRoot
		}, true)
		// dependency: no POS, only the deprel guess.
		run("dep", sample, func(w udWord) (postag.PartOfSpeech, postag.Deprel) {
			r, ok := postag.ParseDeprel(w.rel)
			if !ok {
				r = postag.DepRoot
			}
			return postag.O, r
		}, true)
		// semantic: no POS, no deprel; any section is accepted.
		run("semantic", sample, func(udWord) (postag.PartOfSpeech, postag.Deprel) { return postag.O, postag.DepRoot }, false)

		// pipeline: words the shipped tokenizer cuts from raw sentences, looked up with no tag.
		tk, err := tokenizer.Open(filepath.Join(cfg, tokFiles[lang]), nlang.TokenizerOptions(builtin.Languages("../../data/lang"), "")...)
		if err != nil {
			t.Logf("%s: pipeline tokens skipped: %v", lang, err)
			continue
		}
		seenP := map[string]bool{}
		var piped []udWord
		for _, txt := range texts {
			for _, ws := range tk.Tokenize(txt) {
				k := strings.ToLower(ws.Text)
				if hasLetter(ws.Text) && !seenP[k] {
					seenP[k] = true
					piped = append(piped, udWord{form: ws.Text})
				}
			}
		}
		var pick []udWord
		for i := range min(n, len(piped)) {
			pick = append(pick, piped[i*len(piped)/min(n, len(piped))])
		}
		run("pipeline-tok", pick, func(udWord) (postag.PartOfSpeech, postag.Deprel) { return postag.O, postag.DepRoot }, false)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n%-4s %-13s %4s %7s %9s\n", "lang", "mode", "n", "hit%", "section%")
	for _, r := range rows {
		sec := "-"
		if r.s.sectN > 0 {
			sec = fmt.Sprintf("%.1f", 100*float64(r.s.sect)/float64(r.s.sectN))
		}
		fmt.Fprintf(&b, "%-4s %-13s %4d %7.1f %9s\n", r.lang, r.mode, r.s.n, 100*float64(r.s.hit)/float64(max(r.s.n, 1)), sec)
	}
	t.Log(b.String())
	for _, r := range rows {
		cut := func(x []string) []string { return x[:min(len(x), 25)] }
		t.Logf("%s/%s MISS: %s", r.lang, r.mode, strings.Join(cut(r.s.miss), " "))
		t.Logf("%s/%s WRONG: %s", r.lang, r.mode, strings.Join(cut(r.s.wrong), " "))
	}
	t.Logf("api calls: %d", lf.calls)
}
