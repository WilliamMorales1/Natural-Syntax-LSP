package wiktionary

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

// The REST definition endpoint returns 404 for pages it cannot parse (Chinese {{head|zh|hanzi}}, Japanese {{ja-see}} stubs), so those are read from wikitext.

var langCodes = map[string]string{
	"English": "en", "Translingual": "en", "Spanish": "es", "French": "fr", "Japanese": "ja", "Chinese": "zh",
}

var (
	seeRe      = regexp.MustCompile(`\{\{(?:ja|zh)-see\|([^|}=]+)`)
	linkRe     = regexp.MustCompile(`\[\[(?:[^\]|]*\|)?([^\]]*)\]\]`)
	labelRe    = regexp.MustCompile(`\{\{(?:lb|lbl|label|context)\|[^|}]*\|([^}]*)\}\}`)
	glossRe    = regexp.MustCompile(`\{\{(?:l|m|mention|link|zh-l|ja-l|n-g|non-gloss|gloss|gl|w|zh-m|q|qualifier|i)\|(?:[a-z-]+\|)?([^|}]*)(?:\|[^}]*)?\}\}`)
	templateRe = regexp.MustCompile(`\{\{[^{}]*\}\}`)
	quoteRe    = regexp.MustCompile(`'{2,}`)
	headTplRe  = regexp.MustCompile(`^\{\{head\|[a-z-]+\|([a-z ]+)`)
	headingRe  = regexp.MustCompile(`^(={2,6})\s*([^=]+?)\s*={2,6}\s*$`)
)

func fetchWikitext(ctx context.Context, word string) (string, error) {
	q := url.Values{"action": {"query"}, "titles": {word}, "prop": {"revisions"}, "rvprop": {"content"}, "rvslots": {"main"}, "format": {"json"}, "formatversion": {"2"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://en.wiktionary.org/w/api.php?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("wiktionary wikitext %q: %s", word, resp.Status)
	}
	var r struct {
		Query struct {
			Pages []struct {
				Revisions []struct {
					Slots struct {
						Main struct {
							Content string `json:"content"`
						} `json:"main"`
					} `json:"slots"`
				} `json:"revisions"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	if len(r.Query.Pages) == 0 || len(r.Query.Pages[0].Revisions) == 0 {
		return "", fmt.Errorf("wiktionary %q: missing", word)
	}
	return r.Query.Pages[0].Revisions[0].Slots.Main.Content, nil
}

// fetchViaWikitext builds a REST-shaped payload from page wikitext, following one {{ja-see}} or {{zh-see}} redirect.
func fetchViaWikitext(ctx context.Context, word string, depth int) (map[string][]entry, error) {
	text, err := fetchWikitext(ctx, word)
	if err != nil {
		return nil, err
	}
	payload := parseWikitext(text)
	if len(payload) == 0 && depth < 2 {
		if m := seeRe.FindStringSubmatch(text); m != nil && m[1] != word {
			if p, err := fetch(ctx, strings.TrimSpace(m[1])); err == nil {
				return p, nil
			}
			return fetchViaWikitext(ctx, strings.TrimSpace(m[1]), depth+1)
		}
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("wiktionary %q: no definitions", word)
	}
	return payload, nil
}

// parseWikitext extracts "# " definition lines under each language and part-of-speech heading.
func parseWikitext(text string) map[string][]entry {
	out := map[string][]entry{}
	var lang, pos string
	var cur *entry
	flush := func() {
		if cur != nil && len(cur.Definitions) > 0 {
			code := langCodes[cur.Language]
			if code == "" {
				code = "other"
			}
			out[code] = append(out[code], *cur)
		}
		cur = nil
	}
	for _, line := range strings.Split(text, "\n") {
		if m := headingRe.FindStringSubmatch(line); m != nil {
			flush()
			name := m[2]
			if len(m[1]) == 2 {
				lang, pos = name, ""
			} else if strings.EqualFold(name, "Definitions") {
				pos, cur = "Hanzi", &entry{PartOfSpeech: "Hanzi", Language: lang}
			} else if posNames[strings.ToLower(name)] {
				pos = name
				cur = &entry{PartOfSpeech: name, Language: lang}
			} else {
				pos = ""
			}
			continue
		}
		if m := headTplRe.FindStringSubmatch(line); m != nil && cur != nil && cur.PartOfSpeech == "Hanzi" && posNames[m[1]] {
			cur.PartOfSpeech = strings.ToUpper(m[1][:1]) + m[1][1:]
		}
		if cur == nil || pos == "" || !strings.HasPrefix(line, "# ") {
			continue
		}
		if d := cleanWikitext(line[2:]); d != "" {
			cur.Definitions = append(cur.Definitions, sense{Definition: d})
		}
	}
	flush()
	return out
}

var posNames = map[string]bool{
	"noun": true, "proper noun": true, "verb": true, "adjective": true, "adverb": true, "preposition": true,
	"postposition": true, "pronoun": true, "conjunction": true, "interjection": true, "particle": true,
	"numeral": true, "determiner": true, "article": true, "classifier": true, "counter": true, "prefix": true,
	"suffix": true, "symbol": true, "character": true, "han character": true, "phrase": true, "proverb": true,
	"idiom": true, "adnominal": true, "auxiliary verb": true, "participle": true, "letter": true,
}

func cleanWikitext(s string) string {
	for range 4 {
		next := labelRe.ReplaceAllString(s, "($1) ")
		next = glossRe.ReplaceAllString(next, "$1")
		next = templateRe.ReplaceAllString(next, "")
		if next == s {
			break
		}
		s = next
	}
	s = strings.ReplaceAll(s, "(|", "(")
	s = strings.ReplaceAll(s, "|", ", ")
	s = linkRe.ReplaceAllString(s, "$1")
	s = quoteRe.ReplaceAllString(s, "")
	s = htmlTagRe.ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), " ")
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") && strings.Count(s, "(") == 1 {
		return "" // a label with no gloss
	}
	return strings.TrimSpace(s)
}

// supplementHan adds the Chinese or Japanese sections from wikitext when the REST payload lacks them, as it does for pages mixing parsable and unparsable languages.
func supplementHan(ctx context.Context, word string, payload map[string][]entry) {
	if !strings.ContainsFunc(word, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
		return
	}
	if len(payload["zh"]) > 0 && len(payload["ja"]) > 0 {
		return
	}
	text, err := fetchWikitext(ctx, word)
	if err != nil {
		return
	}
	for code, es := range parseWikitext(text) {
		if (code == "zh" || code == "ja") && len(payload[code]) == 0 {
			payload[code] = es
		}
	}
}
