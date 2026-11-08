// Package wiktionary fetches and formats word definitions from the Wiktionary REST API.
package wiktionary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	nlang "natural-syntax-ls/internal/lang"
	"natural-syntax-ls/internal/postag"
)

// userAgent identifies this client to Wikimedia; override at build time with -ldflags "-X natural-syntax-ls/internal/wiktionary.userAgent=...".
var userAgent = "natural-syntax-ls/1.0 (https://github.com/WilliamMorales1/Natural-Syntax-LSP)"

type cacheEntry struct {
	def, url string
}

const (
	// maxCacheEntries bounds each definition cache so a long editor session cannot grow it without limit.
	maxCacheEntries = 4096
	// missTTL is how long a definitive "no entry" answer is remembered.
	missTTL = 10 * time.Minute
)

// ErrNotFound reports that Wiktionary has no usable entry for a word.
var ErrNotFound = errors.New("wiktionary: no entry")

// StatusError is an unexpected HTTP status from the Wiktionary API.
type StatusError struct {
	Word       string
	StatusCode int
	Status     string
}

func (e *StatusError) Error() string { return fmt.Sprintf("wiktionary %q: %s", e.Word, e.Status) }

// Client looks up Wiktionary definitions and caches the results; it is safe for concurrent use.
type Client struct {
	HTTP *http.Client

	// fetch replaces the network lookup in tests.
	fetch func(ctx context.Context, word string) (map[string][]entry, error)
	now   func() time.Time
	langs []nlang.Wiktionary

	mu     sync.Mutex
	hits   map[string]cacheEntry // key: lang:modelLang:lowercase word:pos:deprel
	misses map[string]time.Time  // key -> expiry; only definitive misses
}

// NewClient returns a Client with an 8 second HTTP timeout and the given per-language behavior.
func NewClient(langs ...nlang.Wiktionary) *Client {
	return &Client{
		langs:  langs,
		HTTP:   &http.Client{Timeout: 8 * time.Second},
		now:    time.Now,
		hits:   map[string]cacheEntry{},
		misses: map[string]time.Time{},
	}
}

func (c *Client) fetchPayload(ctx context.Context, word string) (map[string][]entry, error) {
	if c.fetch != nil {
		return c.fetch(ctx, word)
	}
	return c.fetchRemote(ctx, word)
}

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

// fetchRemote returns the definition API payload for word, keyed by language code.
func (c *Client) fetchRemote(ctx context.Context, word string) (map[string][]entry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://en.wiktionary.org/api/rest_v1/page/definition/"+url.PathEscape(word), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return c.fetchViaWikitext(ctx, word, 0)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &StatusError{Word: word, StatusCode: resp.StatusCode, Status: resp.Status}
	}
	var payload map[string][]entry
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	c.supplementWikitext(ctx, word, payload)
	return payload, nil
}

// Auto is the lang value that takes the best definition across all languages.
const Auto = "auto"

// FetchDef returns a definition for word and its Wiktionary page, matching the word's POS if possible, trying exact case then lowercase.
// lang is a Wiktionary language code ("en" when empty) or Auto; modelLang is tried first under Auto.
// In dependency mode there is no POS tag (pos == postag.O); deprel is used instead to guess the Wiktionary category.
func (c *Client) FetchDef(ctx context.Context, word, lang, modelLang string, pos postag.PartOfSpeech, deprel postag.Deprel) (def, pageURL string, ok bool) {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" {
		lang = "en"
	}
	modelLang = strings.ToLower(modelLang)
	if lang != Auto {
		modelLang = ""
	}
	key := lang + ":" + modelLang + ":" + strings.ToLower(word) + ":" + pos.String() + ":" + deprel.String()
	c.mu.Lock()
	e, hit := c.hits[key]
	if exp, miss := c.misses[key]; miss && !hit {
		if c.now().Before(exp) {
			c.mu.Unlock()
			return "", "", false
		}
		delete(c.misses, key)
	}
	c.mu.Unlock()
	if hit {
		return e.def, e.url, true
	}
	def, pageURL, ok, definitive := c.lookup(ctx, word, lang, modelLang, pos, deprel)
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case ok:
		if len(c.hits) >= maxCacheEntries {
			clear(c.hits)
		}
		c.hits[key] = cacheEntry{def, pageURL}
	case definitive:
		if len(c.misses) >= maxCacheEntries {
			clear(c.misses)
		}
		c.misses[key] = c.now().Add(missTTL)
	}
	return def, pageURL, ok
}

// lookup queries Wiktionary uncached; definitive is false when a transient error (timeout, 429, 5xx) may have hidden an entry.
func (c *Client) lookup(ctx context.Context, word, lang, modelLang string, pos postag.PartOfSpeech, deprel postag.Deprel) (def, pageURL string, ok, definitive bool) {
	lower := strings.ToLower(word)

	for _, l := range c.langs {
		if l.Code == lang && l.Rewrite != nil {
			if w := l.Rewrite(word, pos, deprel); w != "" {
				word, lower = w, strings.ToLower(w)
			}
		}
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
	// Language hooks add headwords the word's own page may live under, tried last.
	for _, l := range c.langs {
		if l.Forms == nil || (l.Code != lang && lang != Auto) {
			continue
		}
		for _, f := range l.Forms(word) {
			if !slices.Contains(forms, f) && f != lower {
				forms = append(forms, f)
			}
		}
	}
	// Capitalized pages can lack the tagged POS (sentence-initial "Said" is only a proper noun), so try lowercase before settling.
	var fallbackDef, fallbackURL string
	definitive = true
	for _, form := range forms {
		payload, err := c.fetchPayload(ctx, form)
		if err != nil {
			if !errors.Is(err, ErrNotFound) {
				definitive = false
			}
			continue
		}
		def, language, matched := pickLang(payload, lang, modelLang, target, numeralGlyph)
		if def == "" {
			continue
		}
		def, pageURL := labelled(def, language, form)
		if matched || target == "" {
			return def, pageURL, true, true
		}
		if fallbackDef == "" {
			fallbackDef, fallbackURL = def, pageURL
		}
	}
	return fallbackDef, fallbackURL, fallbackDef != "", definitive
}

// pickLang chooses the definition for lang (or the best across buckets under Auto) and reports its entry language.
func pickLang(payload map[string][]entry, lang, modelLang, target string, numeralGlyph bool) (def, language string, matched bool) {
	if lang != Auto {
		entries := payload[lang]
		if lang != "en" {
			// Translingual entries (symbols, digits) live in the en bucket.
			for _, e := range payload["en"] {
				if strings.EqualFold(e.Language, "Translingual") {
					entries = append(slices.Clip(entries), e)
				}
			}
		}
		return pickDef(entries, target, numeralGlyph)
	}
	var order []string
	seen := map[string]bool{"other": true}
	add := func(code string) {
		if code != "" && !seen[code] {
			seen[code] = true
			order = append(order, code)
		}
	}
	add(modelLang)
	add("en")
	for _, code := range slices.Sorted(maps.Keys(payload)) {
		add(code)
	}
	var fbDef, fbLang string
	for _, code := range order {
		d, l, m := pickDef(payload[code], target, numeralGlyph)
		if d == "" {
			continue
		}
		if m || target == "" {
			return d, l, true
		}
		if fbDef == "" {
			fbDef, fbLang = d, l
		}
	}
	return fbDef, fbLang, false
}

// labelled prefixes def with its language and links the language section when it is not English.
func labelled(def, language, form string) (string, string) {
	pageURL := "https://en.wiktionary.org/wiki/" + url.PathEscape(form)
	if language == "" || strings.EqualFold(language, "English") || strings.EqualFold(language, "Translingual") {
		return def, pageURL
	}
	return "*(" + language + ")* " + def, pageURL + "#" + url.PathEscape(strings.ReplaceAll(language, " ", "_"))
}

// pickDef returns the best definition from entries, its entry language, and whether the entry matched target.
func pickDef(entries []entry, target string, numeralGlyph bool) (string, string, bool) {
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
		case strings.EqualFold(e.PartOfSpeech, target), target == "Noun" && lpos == "proper noun",
			(target == "Verb" || target == "Adjective") && lpos == "participle",
			target == "Preposition" && (lpos == "particle" || lpos == "postposition"),
			(target == "Particle" || target == "Preposition") && lpos == "hanzi" && grammaticalGloss(e):
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
			return def, e.Language, true
		}
	}
	for _, e := range slices.Concat(ordered(normal), ordered(lowPri)) {
		if def := firstNonEmpty(e); def != "" {
			return def, e.Language, false
		}
	}
	return "", "", false
}

var grammarGlossRe = regexp.MustCompile(`(?i)\b(?:marker|particle|nominali[sz]er|auxiliary|possessive|genitive|topic|complementi[sz]er)\b`)

// grammaticalGloss reports whether an entry's first senses read as a function word. Chinese pages label every sense "Hanzi", so the gloss is the only signal.
func grammaticalGloss(e *entry) bool {
	for _, d := range e.Definitions[:min(len(e.Definitions), 3)] {
		if grammarGlossRe.MatchString(stripHTML(d.Definition)) {
			return true
		}
	}
	return false
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
		return html.UnescapeString(e)
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
