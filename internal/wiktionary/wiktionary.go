// Package wiktionary fetches and formats word definitions from the Wiktionary REST API.
package wiktionary

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"natural-syntax-ls/internal/postag"
)

var wiktionaryClient = &http.Client{Timeout: 8 * time.Second}

type wiktCacheEntry struct {
	def string
	url string
	ok  bool
}

var wiktCache sync.Map // key: lowercase word → wiktCacheEntry
var htmlTagRe = regexp.MustCompile(`<[^>]+>`)
var styleBlockRe = regexp.MustCompile(`(?s)<style[^>]*>.*?</style>`)

type wiktDef struct {
	PartOfSpeech string `json:"partOfSpeech"`
	Language     string `json:"language"`
	Definitions  []struct {
		Definition string `json:"definition"`
	} `json:"definitions"`
}

func wiktFetch(word string) (map[string][]wiktDef, error) {
	apiURL := fmt.Sprintf("https://en.wiktionary.org/api/rest_v1/page/definition/%s", word)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "natural-syntax-ls/1.0 (https://github.com/wsm5224/NLSyntaxHighlighting-Go)")
	resp, err := wiktionaryClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return nil, fmt.Errorf("request failed")
	}
	defer resp.Body.Close()
	var payload map[string][]wiktDef
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// FetchDef returns (definition, wiktionary URL, ok), matching the word's POS if possible, trying exact case then lowercase.
func FetchDef(word string, pos postag.PartOfSpeech) (string, string, bool) {
	lower := strings.ToLower(word)
	cacheKey := lower + ":" + pos.String()
	if v, ok := wiktCache.Load(cacheKey); ok {
		e := v.(wiktCacheEntry)
		return e.def, e.url, e.ok
	}
	def, url, ok := fetchDefUncached(word, pos)
	if ok {
		wiktCache.Store(cacheKey, wiktCacheEntry{def, url, true})
	}
	return def, url, ok
}

func fetchDefUncached(word string, pos postag.PartOfSpeech) (string, string, bool) {
	lower := strings.ToLower(word)

	resolved := word
	payload, err := wiktFetch(word)
	if err != nil || len(payload["en"]) == 0 {
		if word == lower {
			return "", "", false
		}
		payload, err = wiktFetch(lower)
		if err != nil {
			return "", "", false
		}
		resolved = lower
	}

	entries := payload["en"]
	if len(entries) == 0 {
		return "", "", false
	}

	target := posToWiktCategory(pos)
	numeralGlyph := (pos == postag.POS_CD || pos == postag.POS_LS) && isNumeralGlyph(lower)
	if numeralGlyph {
		target = "Symbol" // Translingual numeral entries use partOfSpeech="Symbol"
	}
	pageURL := fmt.Sprintf("https://en.wiktionary.org/wiki/%s", resolved)

	// firstNonEmpty returns the first non-empty stripped definition from an entry.
	firstNonEmpty := func(e *wiktDef) string {
		for _, d := range e.Definitions {
			if s := stripHTML(d.Definition); s != "" {
				return s
			}
		}
		return ""
	}

	lowPriPOS := map[string]bool{"symbol": true, "letter": true, "prefix": true, "suffix": true, "affix": true}

	type bucket struct{ trans, main []*wiktDef }
	var matched, normal, lowPri bucket
	addTo := func(b *bucket, e *wiktDef) {
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
		case strings.EqualFold(e.PartOfSpeech, target):
			addTo(&matched, e)
		case lowPriPOS[lpos]:
			addTo(&lowPri, e)
		default:
			addTo(&normal, e)
		}
	}

	// Numeral glyphs: Translingual before English in every tier; words: reverse.
	ordered := func(b bucket) []*wiktDef {
		if numeralGlyph {
			return append(b.trans, b.main...)
		}
		return append(b.main, b.trans...)
	}

	candidates := append(append(ordered(matched), ordered(normal)...), ordered(lowPri)...)
	for _, e := range candidates {
		if def := firstNonEmpty(e); def != "" {
			return def, pageURL, true
		}
	}
	return "", pageURL, false
}

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
	if len(word) == 0 {
		return false
	}
	for _, ch := range word {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func posToWiktCategory(pos postag.PartOfSpeech) string {
	switch pos {
	case postag.POS_NN, postag.POS_NNS, postag.POS_NNP, postag.POS_NNPS:
		return "Noun"
	case postag.POS_CD:
		return "Numeral"
	case postag.POS_VB, postag.POS_VBD, postag.POS_VBG, postag.POS_VBN, postag.POS_VBP, postag.POS_VBZ, postag.POS_MD:
		return "Verb"
	case postag.POS_JJ, postag.POS_JJR, postag.POS_JJS:
		return "Adjective"
	case postag.POS_RB, postag.POS_RBR, postag.POS_RBS:
		return "Adverb"
	case postag.POS_IN, postag.POS_TO:
		return "Preposition"
	case postag.POS_DT, postag.POS_PDT, postag.POS_WDT:
		return "Article"
	case postag.POS_PRP, postag.POS_WP:
		return "Pronoun"
	case postag.POS_CC:
		return "Conjunction"
	case postag.POS_UH:
		return "Interjection"
	case postag.POS_RP:
		return "Particle"
	default:
		return ""
	}
}
