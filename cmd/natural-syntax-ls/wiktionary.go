package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
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

// fetchWiktionaryDef returns (definition, wiktionary URL, ok).
// Tries to match the word's POS; falls back to first available definition.
// Tries exact case first, then lowercase.
func fetchWiktionaryDef(word string, pos PartOfSpeech) (string, string, bool) {
	lower := strings.ToLower(word)
	cacheKey := lower + ":" + pos.String()
	if v, ok := wiktCache.Load(cacheKey); ok {
		e := v.(wiktCacheEntry)
		return e.def, e.url, e.ok
	}
	def, url, ok := fetchWiktionaryDefUncached(word, pos)
	if ok {
		wiktCache.Store(cacheKey, wiktCacheEntry{def, url, true})
	}
	return def, url, ok
}

func fetchWiktionaryDefUncached(word string, pos PartOfSpeech) (string, string, bool) {
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
	numeralGlyph := (pos == POS_CD || pos == POS_LS) && isNumeralGlyph(lower)
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

	// For numeral glyphs: Translingual before English in every tier.
	// For words: English before Translingual.
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

func posToWiktCategory(pos PartOfSpeech) string {
	switch pos {
	case POS_NN, POS_NNS, POS_NNP, POS_NNPS:
		return "Noun"
	case POS_CD:
		return "Numeral"
	case POS_VB, POS_VBD, POS_VBG, POS_VBN, POS_VBP, POS_VBZ, POS_MD:
		return "Verb"
	case POS_JJ, POS_JJR, POS_JJS:
		return "Adjective"
	case POS_RB, POS_RBR, POS_RBS:
		return "Adverb"
	case POS_IN, POS_TO:
		return "Preposition"
	case POS_DT, POS_PDT, POS_WDT:
		return "Article"
	case POS_PRP, POS_WP:
		return "Pronoun"
	case POS_CC:
		return "Conjunction"
	case POS_UH:
		return "Interjection"
	case POS_RP:
		return "Particle"
	default:
		return ""
	}
}
