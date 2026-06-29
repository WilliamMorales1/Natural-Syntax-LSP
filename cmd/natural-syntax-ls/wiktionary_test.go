package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- test helpers ---

func makeWiktResponse(entries []wiktDef) map[string][]wiktDef {
	return map[string][]wiktDef{"en": entries}
}

func makeEntry(pos, lang, def string) wiktDef {
	return wiktDef{
		PartOfSpeech: pos,
		Language:     lang,
		Definitions: []struct {
			Definition string `json:"definition"`
		}{{Definition: def}},
	}
}

// serveWikt builds a mock Wiktionary HTTP server.
// responses maps word → wiktionary payload.
func serveWikt(t *testing.T, responses map[string]map[string][]wiktDef) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		word := strings.TrimPrefix(r.URL.Path, "/api/rest_v1/page/definition/")
		log.Printf("[mock-wikt] GET %s → word=%q", r.URL.Path, word)
		resp, ok := responses[word]
		if !ok {
			log.Printf("[mock-wikt] 404 for word=%q", word)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("[mock-wikt] encode error: %v", err)
		}
	}))
	return srv
}

// patchWiktClient replaces the global HTTP client so requests go to the mock server.
func patchWiktClient(srv *httptest.Server) func() {
	orig := wiktionaryClient
	wiktionaryClient = &http.Client{
		Transport: &prefixRewriter{base: srv.URL, inner: http.DefaultTransport},
	}
	return func() { wiktionaryClient = orig }
}

// prefixRewriter rewrites https://en.wiktionary.org → mock server URL.
type prefixRewriter struct {
	base  string
	inner http.RoundTripper
}

func (p *prefixRewriter) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	orig := cloned.URL.String()
	rewritten := strings.Replace(orig, "https://en.wiktionary.org", p.base, 1)
	log.Printf("[rewriter] %s → %s", orig, rewritten)
	cloned.URL, _ = cloned.URL.Parse(rewritten)
	cloned.Host = cloned.URL.Host
	return p.inner.RoundTrip(cloned)
}

// --- stripHTML ---

func TestStripHTML_RemovesTags(t *testing.T) {
	input := `<b>bold</b> and <i>italic</i>`
	want := "bold and italic"
	got := stripHTML(input)
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestStripHTML_DecodesEntities(t *testing.T) {
	cases := []struct{ in, want string }{
		{"&amp;", "&"},
		{"&lt;foo&gt;", "<foo>"},
		{"foo&nbsp;bar", "foo bar"}, // &nbsp; mid-string → space preserved
		{"&ndash;", "–"},
		{"&mdash;", "—"},
	}
	for _, c := range cases {
		got := stripHTML(c.in)
		if got != c.want {
			t.Errorf("stripHTML(%q) = %q want %q", c.in, got, c.want)
		}
	}
}

func TestStripHTML_RemovesStyleBlock(t *testing.T) {
	input := `<style>body{color:red}</style>visible`
	got := stripHTML(input)
	if strings.Contains(got, "color") {
		t.Errorf("style block not removed, got %q", got)
	}
	if !strings.Contains(got, "visible") {
		t.Errorf("wanted 'visible' in output, got %q", got)
	}
}

func TestStripHTML_EmptyInput(t *testing.T) {
	if got := stripHTML(""); got != "" {
		t.Errorf("empty input: got %q", got)
	}
}

// --- posToWiktCategory ---

func TestPosToWiktCategory(t *testing.T) {
	cases := []struct {
		pos  PartOfSpeech
		want string
	}{
		{POS_NN, "Noun"}, {POS_NNS, "Noun"}, {POS_NNP, "Noun"}, {POS_NNPS, "Noun"},
		{POS_VB, "Verb"}, {POS_VBZ, "Verb"}, {POS_MD, "Verb"},
		{POS_JJ, "Adjective"}, {POS_JJR, "Adjective"},
		{POS_RB, "Adverb"}, {POS_RBR, "Adverb"},
		{POS_IN, "Preposition"}, {POS_TO, "Preposition"},
		{POS_PRP, "Pronoun"},
		{POS_CC, "Conjunction"},
		{POS_UH, "Interjection"},
		{POS_DT, "Article"},
	}
	for _, c := range cases {
		got := posToWiktCategory(c.pos)
		if got != c.want {
			t.Errorf("posToWiktCategory(%v) = %q want %q", c.pos, got, c.want)
		}
	}
}

// --- isNumeralGlyph ---

func TestIsNumeralGlyph(t *testing.T) {
	cases := []struct {
		word string
		want bool
	}{
		{"42", true},
		{"0", true},
		{"12345", true},
		{"12a", false},
		{"abc", false},
		{"", false},
	}
	for _, c := range cases {
		got := isNumeralGlyph(c.word)
		if got != c.want {
			t.Errorf("isNumeralGlyph(%q) = %v want %v", c.word, got, c.want)
		}
	}
}

// --- formatHoverContent ---

func TestFormatHoverContent_NoDefinition(t *testing.T) {
	tok := &POSToken{Word: "running", Tag: POS_VBG, Score: 0.95}
	out := formatHoverContent(tok, "", "")
	log.Printf("[test] formatHoverContent no-def: %q", out)
	if !strings.Contains(out, "running") {
		t.Errorf("missing word, got %q", out)
	}
	if !strings.Contains(out, "gerund") {
		t.Errorf("missing pos description 'gerund', got %q", out)
	}
	if strings.Contains(out, "Wiktionary") {
		t.Errorf("should not have Wiktionary link when no def, got %q", out)
	}
	// Score should appear in POS mode
	if !strings.Contains(out, "0.95") {
		t.Errorf("expected score 0.95, got %q", out)
	}
}

func TestFormatHoverContent_WithDefinition(t *testing.T) {
	tok := &POSToken{Word: "apple", Tag: POS_NN, Score: 0.99}
	out := formatHoverContent(tok, "A fruit.", "https://en.wiktionary.org/wiki/apple")
	log.Printf("[test] formatHoverContent with-def: %q", out)
	if !strings.Contains(out, "A fruit.") {
		t.Errorf("missing definition, got %q", out)
	}
	if !strings.Contains(out, "Wiktionary") {
		t.Errorf("missing Wiktionary link, got %q", out)
	}
	if !strings.Contains(out, "apple") {
		t.Errorf("missing word 'apple', got %q", out)
	}
	if !strings.Contains(out, "Noun") {
		t.Errorf("missing POS label 'Noun', got %q", out)
	}
}

// --- fetchWiktionaryDef (with mock HTTP server) ---

func TestFetchWiktionaryDef_MatchesPOS(t *testing.T) {
	srv := serveWikt(t, map[string]map[string][]wiktDef{
		"apple": makeWiktResponse([]wiktDef{
			makeEntry("Noun", "English", "A round fruit."),
			makeEntry("Verb", "English", "To apple something."),
		}),
	})
	defer srv.Close()
	restore := patchWiktClient(srv)
	defer restore()

	def, url, ok := fetchWiktionaryDef("apple", POS_NN)
	log.Printf("[test] MatchesPOS: def=%q url=%q ok=%v", def, url, ok)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if def != "A round fruit." {
		t.Errorf("expected noun def 'A round fruit.', got %q", def)
	}
	if !strings.Contains(url, "apple") {
		t.Errorf("expected url to contain 'apple', got %q", url)
	}
}

func TestFetchWiktionaryDef_FallsBackToAnyDef(t *testing.T) {
	srv := serveWikt(t, map[string]map[string][]wiktDef{
		"quickly": makeWiktResponse([]wiktDef{
			makeEntry("Adverb", "English", "In a quick manner."),
		}),
	})
	defer srv.Close()
	restore := patchWiktClient(srv)
	defer restore()

	// Ask for Noun but only Adverb exists → should still return adverb def
	def, _, ok := fetchWiktionaryDef("quickly", POS_NN)
	log.Printf("[test] FallbackAnyDef: def=%q ok=%v", def, ok)
	if !ok {
		t.Fatal("expected ok=true even with POS mismatch")
	}
	if def == "" {
		t.Error("expected non-empty definition")
	}
}

func TestFetchWiktionaryDef_MissingWord(t *testing.T) {
	srv := serveWikt(t, map[string]map[string][]wiktDef{})
	defer srv.Close()
	restore := patchWiktClient(srv)
	defer restore()

	_, _, ok := fetchWiktionaryDef("zzznonsenseword", POS_NN)
	log.Printf("[test] MissingWord: ok=%v", ok)
	if ok {
		t.Error("expected ok=false for unknown word")
	}
}

func TestFetchWiktionaryDef_CaseInsensitiveFallback(t *testing.T) {
	// "Apple" (capital) → 404, "apple" (lower) → success
	srv := serveWikt(t, map[string]map[string][]wiktDef{
		"apple": makeWiktResponse([]wiktDef{
			makeEntry("Noun", "English", "A fruit."),
		}),
	})
	defer srv.Close()
	restore := patchWiktClient(srv)
	defer restore()

	def, _, ok := fetchWiktionaryDef("Apple", POS_NN)
	log.Printf("[test] CaseInsensitive: def=%q ok=%v", def, ok)
	if !ok {
		t.Fatal("expected ok=true after lowercase fallback")
	}
	if def != "A fruit." {
		t.Errorf("expected 'A fruit.', got %q", def)
	}
}

func TestFetchWiktionaryDef_HTMLStrippedFromDef(t *testing.T) {
	srv := serveWikt(t, map[string]map[string][]wiktDef{
		"bold": makeWiktResponse([]wiktDef{
			makeEntry("Adjective", "English", "<b>Courageous</b> and <i>daring</i>."),
		}),
	})
	defer srv.Close()
	restore := patchWiktClient(srv)
	defer restore()

	def, _, ok := fetchWiktionaryDef("bold", POS_JJ)
	log.Printf("[test] HTMLStripped: def=%q ok=%v", def, ok)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if strings.Contains(def, "<") || strings.Contains(def, ">") {
		t.Errorf("HTML not stripped from definition, got %q", def)
	}
	if !strings.Contains(def, "Courageous") {
		t.Errorf("expected 'Courageous' in stripped def, got %q", def)
	}
}

// --- wiktFetch error handling ---

func TestWiktFetch_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[mock-wikt] returning 404 for %s", r.URL.Path)
		http.NotFound(w, r)
	}))
	defer srv.Close()
	restore := patchWiktClient(srv)
	defer restore()

	_, err := wiktFetch("anythinggg")
	log.Printf("[test] wiktFetch non-200 err=%v", err)
	if err == nil {
		t.Error("expected error for non-200 response")
	}
	if !strings.Contains(fmt.Sprint(err), "request failed") {
		t.Errorf("expected 'request failed' error, got %v", err)
	}
}

// --- handleHoverQuery (document_registry) ---

func TestHandleHoverQuery_ReturnsToken(t *testing.T) {
	tokens := []POSToken{
		{Word: "Hello", Tag: POS_UH, Score: 0.9, OffsetBegin: 0, OffsetEnd: 5},
		{Word: "world", Tag: POS_NN, Score: 0.85, OffsetBegin: 6, OffsetEnd: 11},
	}
	doc := &document{text: "Hello world", tokens: tokens}
	store := &documentStore{doc: doc}
	dr := &DocumentRegistry{stores: map[string]*documentStore{"file:///test.txt": store}}

	reply := make(chan *POSToken, 1)
	dr.handleHoverQuery("file:///test.txt", 0, 7, reply) // character 7 → "world"
	tok := <-reply
	log.Printf("[test] hover token: %+v", tok)
	if tok == nil {
		t.Fatal("expected non-nil token")
	}
	if tok.Word != "world" {
		t.Errorf("expected 'world', got %q", tok.Word)
	}
}

func TestHandleHoverQuery_UnknownURI(t *testing.T) {
	dr := &DocumentRegistry{stores: map[string]*documentStore{}}
	reply := make(chan *POSToken, 1)
	dr.handleHoverQuery("file:///nonexistent.txt", 0, 0, reply)
	tok := <-reply
	log.Printf("[test] unknown URI token: %v", tok)
	if tok != nil {
		t.Errorf("expected nil token for unknown URI, got %+v", tok)
	}
}

func TestHandleHoverQuery_OutOfBounds(t *testing.T) {
	tokens := []POSToken{
		{Word: "hi", Tag: POS_UH, Score: 0.9, OffsetBegin: 0, OffsetEnd: 2},
	}
	doc := &document{text: "hi", tokens: tokens}
	store := &documentStore{doc: doc}
	dr := &DocumentRegistry{stores: map[string]*documentStore{"file:///test.txt": store}}

	reply := make(chan *POSToken, 1)
	dr.handleHoverQuery("file:///test.txt", 0, 99, reply)
	tok := <-reply
	log.Printf("[test] out-of-bounds token: %v", tok)
	if tok != nil {
		t.Errorf("expected nil for out-of-bounds hover, got %+v", tok)
	}
}

// --- full pipeline: hover query → fetch def → format ---

func TestHoverPipeline_WordWithDefinition(t *testing.T) {
	srv := serveWikt(t, map[string]map[string][]wiktDef{
		"cat": makeWiktResponse([]wiktDef{
			makeEntry("Noun", "English", "A small domesticated carnivorous mammal."),
		}),
	})
	defer srv.Close()
	restore := patchWiktClient(srv)
	defer restore()

	tok := &POSToken{Word: "cat", Tag: POS_NN, Score: 0.95}
	def, url, ok := fetchWiktionaryDef(tok.Word, tok.Tag)
	log.Printf("[test] pipeline: def=%q url=%q ok=%v", def, url, ok)
	if !ok {
		t.Fatal("expected ok=true")
	}
	content := formatHoverContent(tok, def, url)
	log.Printf("[test] pipeline hover content:\n%s", content)

	if !strings.Contains(content, "cat") {
		t.Error("missing word 'cat'")
	}
	if !strings.Contains(content, "A small domesticated") {
		t.Error("missing definition text")
	}
	if !strings.Contains(content, "Wiktionary") {
		t.Error("missing Wiktionary link")
	}
	if !strings.Contains(content, "Noun") {
		t.Error("missing POS label")
	}
}
