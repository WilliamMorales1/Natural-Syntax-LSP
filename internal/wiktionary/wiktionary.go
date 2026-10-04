// Package wiktionary fetches and formats word definitions from the Wiktionary REST API.
package wiktionary

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"natural-syntax-ls/internal/postag"
)

var client = &http.Client{Timeout: 8 * time.Second}

type cacheEntry struct {
	def, url string
}

var cache sync.Map // key: lowercase word:pos:deprel → cacheEntry

var (
	htmlTagRe    = regexp.MustCompile(`<[^>]+>`)
	styleBlockRe = regexp.MustCompile(`(?s)<style[^>]*>.*?</style>`)
)

// entry is one part-of-speech section of a Wiktionary definition response.
type entry struct {
	PartOfSpeech string  `json:"partOfSpeech"`
	Language     string  `json:"language"`
	Definitions  []sense `json:"definitions"`
}

type sense struct {
	Definition string `json:"definition"`
}

// fetch returns the definition API payload for word, keyed by language code.
func fetch(word string) (map[string][]entry, error) {
	req, err := http.NewRequest(http.MethodGet, "https://en.wiktionary.org/api/rest_v1/page/definition/"+url.PathEscape(word), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "natural-syntax-ls/1.0 (https://github.com/wsm5224/NLSyntaxHighlighting-Go)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wiktionary %q: %s", word, resp.Status)
	}
	var payload map[string][]entry
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// FetchDef returns a definition for word and its Wiktionary page, matching the word's POS if possible, trying exact case then lowercase.
// In dependency mode there is no POS tag (pos == postag.O); deprel is used instead to guess the Wiktionary category.
func FetchDef(word string, pos postag.PartOfSpeech, deprel postag.Deprel) (def, pageURL string, ok bool) {
	key := strings.ToLower(word) + ":" + pos.String() + ":" + deprel.String()
	if v, ok := cache.Load(key); ok {
		e := v.(cacheEntry)
		return e.def, e.url, true
	}
	def, pageURL, ok = fetchDefUncached(word, pos, deprel)
	if ok {
		cache.Store(key, cacheEntry{def, pageURL})
	}
	return def, pageURL, ok
}

func fetchDefUncached(word string, pos postag.PartOfSpeech, deprel postag.Deprel) (string, string, bool) {
	lower := strings.ToLower(word)

	if pos == postag.POS || (deprel == postag.DepCase && lower == "'s") {
		word = "-'s"
		lower = "-'s"
	}

	target := posCategory(pos)
	if pos == postag.O {
		target = deprelCategory(deprel)
	}
	numeralGlyph := (pos == postag.CD || pos == postag.LS || deprel == postag.DepNummod) && isNumeralGlyph(lower)
	if numeralGlyph {
		target = "Symbol" // Translingual numeral entries use partOfSpeech="Symbol"
	}

	forms := []string{word}
	if word != lower {
		forms = append(forms, lower)
	}
	// Capitalized pages can lack the tagged POS (sentence-initial "Said" is only a proper noun), so try lowercase before settling.
	var fallbackDef, fallbackURL string
	for _, form := range forms {
		payload, err := fetchPayload(form)
		if err != nil {
			continue
		}
		def, matched := pickDef(payload["en"], target, numeralGlyph)
		if def == "" {
			continue
		}
		pageURL := "https://en.wiktionary.org/wiki/" + url.PathEscape(form)
		if matched || target == "" {
			return def, pageURL, true
		}
		if fallbackDef == "" {
			fallbackDef, fallbackURL = def, pageURL
		}
	}
	return fallbackDef, fallbackURL, fallbackDef != ""
}

// fetchPayload is fetch, swappable in tests.
var fetchPayload = fetch

// pickDef returns the best definition from entries and whether it came from an entry matching target.
func pickDef(entries []entry, target string, numeralGlyph bool) (string, bool) {
	firstNonEmpty := func(e *entry) string {
		for _, d := range e.Definitions {
			if s := stripHTML(d.Definition); s != "" {
				return s
			}
		}
		return ""
	}

	type bucket struct{ trans, main []*entry }
	var matched, normal, lowPri bucket
	addTo := func(b *bucket, e *entry) {
		if strings.EqualFold(e.Language, "Translingual") {
			b.trans = append(b.trans, e)
		} else {
			b.main = append(b.main, e)
		}
	}
	for i := range entries {
		e := &entries[i]
		lpos := strings.ToLower(e.PartOfSpeech)
		switch {
		case strings.EqualFold(e.PartOfSpeech, target), target == "Noun" && lpos == "proper noun":
			addTo(&matched, e)
		case lowPriPOS[lpos]:
			addTo(&lowPri, e)
		default:
			addTo(&normal, e)
		}
	}

	// Numeral glyphs: Translingual before English in every tier; words: reverse.
	ordered := func(b bucket) []*entry {
		if numeralGlyph {
			return append(b.trans, b.main...)
		}
		return append(b.main, b.trans...)
	}

	for _, e := range ordered(matched) {
		if def := firstNonEmpty(e); def != "" {
			return def, true
		}
	}
	for _, e := range append(ordered(normal), ordered(lowPri)...) {
		if def := firstNonEmpty(e); def != "" {
			return def, false
		}
	}
	return "", false
}

// lowPriPOS are Wiktionary sections ranked after every other non-matching section.
var lowPriPOS = map[string]bool{"symbol": true, "letter": true, "prefix": true, "suffix": true, "affix": true}

var htmlEntityRe = regexp.MustCompile(`&[a-zA-Z]+;|&#[0-9]+;`)

var htmlEntities = map[string]string{
	"&amp;": "&", "&lt;": "<", "&gt;": ">", "&quot;": `"`, "&apos;": "'",
	"&nbsp;": " ", "&ndash;": "–", "&mdash;": "—", "&lsquo;": "'", "&rsquo;": "'",
	"&ldquo;": "“", "&rdquo;": "”",
}

func stripHTML(s string) string {
	s = styleBlockRe.ReplaceAllString(s, "")
	s = htmlTagRe.ReplaceAllString(s, "")
	s = htmlEntityRe.ReplaceAllStringFunc(s, func(e string) string {
		if v, ok := htmlEntities[e]; ok {
			return v
		}
		return e
	})
	return strings.TrimSpace(s)
}

func isNumeralGlyph(word string) bool {
	return word != "" && !strings.ContainsFunc(word, func(r rune) bool { return r < '0' || r > '9' })
}

// posCategory maps a POS tag to the Wiktionary section name it should match.
func posCategory(pos postag.PartOfSpeech) string {
	switch pos {
	case postag.NN, postag.NNS, postag.NNP, postag.NNPS:
		return "Noun"
	case postag.CD:
		return "Numeral"
	case postag.VB, postag.VBD, postag.VBG, postag.VBN, postag.VBP, postag.VBZ, postag.MD:
		return "Verb"
	case postag.JJ, postag.JJR, postag.JJS:
		return "Adjective"
	case postag.RB, postag.RBR, postag.RBS:
		return "Adverb"
	case postag.IN, postag.TO:
		return "Preposition"
	case postag.DT, postag.PDT, postag.WDT:
		return "Article"
	case postag.PRP, postag.WP:
		return "Pronoun"
	case postag.CC:
		return "Conjunction"
	case postag.UH:
		return "Interjection"
	case postag.RP:
		return "Particle"
	default:
		return ""
	}
}

// deprelCategory guesses a Wiktionary POS category from a UD dependency relation,
// since dependency mode has no POS tag (deprels are not POS tags: e.g. nsubj can be a noun
// or pronoun, root can be a verb or noun). Best-effort based on the relation's typical filler.
func deprelCategory(rel postag.Deprel) string {
	switch rel {
	case postag.DepNsubj, postag.DepObj, postag.DepIobj, postag.DepObl, postag.DepNmod,
		postag.DepAppos, postag.DepCompound, postag.DepFlat, postag.DepList, postag.DepVocative,
		postag.DepExpl, postag.DepClf, postag.DepCsubj:
		return "Noun"
	case postag.DepAmod:
		return "Adjective"
	case postag.DepAdvmod:
		return "Adverb"
	case postag.DepAux, postag.DepCop, postag.DepXcomp, postag.DepCcomp, postag.DepAdvcl, postag.DepAcl:
		return "Verb"
	case postag.DepDet:
		return "Article"
	case postag.DepCase, postag.DepMark:
		return "Preposition"
	case postag.DepCc:
		return "Conjunction"
	case postag.DepNummod:
		return "Numeral"
	case postag.DepDiscourse:
		return "Interjection"
	default:
		return ""
	}
}
