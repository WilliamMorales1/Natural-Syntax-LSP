// Package tokenizer does BERT-style basic tokenization + WordPiece encoding.
package tokenizer

import (
	"bufio"
	"os"
	"slices"
	"strings"
	"unicode"
)

// WordSpan is a word and its char-offset span within the source text.
type WordSpan struct {
	Text  string
	Begin uint32
	End   uint32
	// BlockStart marks the first word of a line that opens a new markdown block (list item, heading, table row, code line, ...), which always starts a new sentence.
	BlockStart bool
}

// BERTTokenizer does basic BERT tokenization + WordPiece.
type BERTTokenizer struct {
	vocab map[string]int
	unkID int
	clsID int
	sepID int
	// uncased is true for "do_lower_case" BERT vocabs, which need input lowercased first or capitalized words wordpiece to [UNK].
	uncased bool
}

// Load reads a BERT vocab.txt file and builds a tokenizer from it.
func Load(vocabPath string) (*BERTTokenizer, error) {
	f, err := os.Open(vocabPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	vocab := make(map[string]int)
	sc := bufio.NewScanner(f)
	id := 0
	upperEntries := 0
	for sc.Scan() {
		tok := sc.Text()
		vocab[tok] = id
		id++
		if tok != strings.ToLower(tok) && !strings.HasPrefix(tok, "[") {
			upperEntries++
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return &BERTTokenizer{
		vocab: vocab,
		unkID: vocab["[UNK]"],
		clsID: vocab["[CLS]"],
		sepID: vocab["[SEP]"],
		// Cased vocabs have thousands of uppercase entries; uncased have essentially none outside special tokens, so 1% is a safe margin.
		uncased: upperEntries < len(vocab)/100,
	}, nil
}

// CLSID returns the vocab id of [CLS].
func (t *BERTTokenizer) CLSID() int { return t.clsID }

// VocabID looks up a wordpiece's vocab id, falling back to [UNK] if not found.
func (t *BERTTokenizer) VocabID(piece string) int {
	if id, ok := t.vocab[piece]; ok {
		return id
	}
	return t.unkID
}

// TokenizeWords encodes a pre-split word slice into a [CLS]-prefixed, [SEP]-suffixed subword stream; [CLS]/[SEP] entries get swWordIdx -1.
func (t *BERTTokenizer) TokenizeWords(words []WordSpan) (inputIDs, attMask, tokTypeIDs []int64, swWordIdx []int, swIsFirst []bool) {
	inputIDs = []int64{int64(t.clsID)}
	swWordIdx = []int{-1}
	swIsFirst = []bool{false}

	for wi, w := range words {
		for pi, piece := range t.WordPiece(w.Text) {
			inputIDs = append(inputIDs, int64(t.VocabID(piece)))
			swWordIdx = append(swWordIdx, wi)
			swIsFirst = append(swIsFirst, pi == 0)
		}
	}
	inputIDs = append(inputIDs, int64(t.sepID))
	swWordIdx = append(swWordIdx, -1)
	swIsFirst = append(swIsFirst, false)

	n := len(inputIDs)
	return inputIDs, slices.Repeat([]int64{1}, n), make([]int64, n), swWordIdx, swIsFirst
}

// WordPiece splits a single word into WordPiece subword tokens.
func (t *BERTTokenizer) WordPiece(word string) []string {
	if t.uncased {
		word = strings.ToLower(word)
	}
	if _, ok := t.vocab[word]; ok {
		return []string{word}
	}
	// Greedy longest-match-first, as in BERT's reference WordPiece.
	runes := []rune(word)
	var result []string
	for start := 0; start < len(runes); {
		end := len(runes)
		for ; end > start; end-- {
			sub := string(runes[start:end])
			if start > 0 {
				sub = "##" + sub
			}
			if _, ok := t.vocab[sub]; ok {
				result = append(result, sub)
				break
			}
		}
		if end == start {
			return []string{"[UNK]"}
		}
		start = end
	}
	return result
}

// BasicTokenize splits text on whitespace/punctuation while tracking char offsets.
func BasicTokenize(text string) []WordSpan {
	var words []WordSpan
	runes := []rune(text)
	n := len(runes)
	var lines lineClassifier
	blockLine := lines.next(runes)
	i := 0
	for i < n {
		lineFirst := i == 0
		// skip whitespace
		for i < n && unicode.IsSpace(runes[i]) {
			if runes[i] == '\n' {
				blockLine = lines.next(runes[i+1:])
				lineFirst = true
			}
			i++
		}
		if i >= n {
			break
		}
		start := i
		// collect until whitespace
		for i < n && !unicode.IsSpace(runes[i]) {
			i++
		}
		// Split run on punctuation boundaries.
		spans := splitPunct(runes[start:i], uint32(start))
		if lineFirst && blockLine && len(spans) > 0 {
			spans[0].BlockStart = true
		}
		words = append(words, spans...)
	}
	return words
}

// lineKind is the markdown block construct a line opens.
type lineKind int

const (
	lineText lineKind = iota
	lineBlank
	lineList
	lineHeading
	lineRule // thematic break or setext underline
	lineTable
	lineFence
)

// lineClassifier tracks enough markdown state across lines to tell which lines open a new block.
type lineClassifier struct {
	prev    lineKind
	inFence bool
}

// next classifies the line at the start of rest and reports whether its first word starts a new block.
func (c *lineClassifier) next(rest []rune) bool {
	line := rest
	if j := slices.Index(rest, '\n'); j >= 0 {
		line = rest[:j]
	}
	kind := classifyLine(line)
	var block bool
	switch {
	case c.inFence:
		// Code lines are never prose continuations of each other.
		block = true
		if kind == lineFence {
			c.inFence = false
		}
		kind = lineFence
	case kind == lineFence:
		block, c.inFence = true, true
	case kind == lineText || kind == lineBlank:
		// Lists and quotes allow lazy continuation lines; single-line blocks don't.
		block = c.prev == lineHeading || c.prev == lineRule || c.prev == lineTable || c.prev == lineFence
	default:
		block = true
	}
	c.prev = kind
	return block
}

// classifyLine reports which block construct a single line (without its newline) opens.
func classifyLine(line []rune) lineKind {
	s := strings.TrimLeft(string(line), " \t\r")
	// Blockquote markers are containers; classify what's inside them.
	for strings.HasPrefix(s, ">") {
		s = strings.TrimLeft(s[1:], " \t")
	}
	s = strings.TrimRight(s, " \t\r")
	switch {
	case s == "":
		return lineBlank
	case strings.HasPrefix(s, "```") || strings.HasPrefix(s, "~~~"):
		return lineFence
	case isRule(s):
		return lineRule
	case strings.HasPrefix(s, "|"):
		return lineTable
	}
	if h := strings.TrimLeft(s, "#"); len(s)-len(h) <= 6 && h != s && (h == "" || h[0] == ' ' || h[0] == '\t') {
		return lineHeading
	}
	if strings.ContainsRune("-*+", rune(s[0])) && hasMarkerGap(s[1:]) {
		return lineList
	}
	d := strings.TrimLeft(s, "0123456789")
	if digits := len(s) - len(d); digits >= 1 && digits <= 9 && d != "" && (d[0] == '.' || d[0] == ')') && hasMarkerGap(d[1:]) {
		return lineList
	}
	return lineText
}

// hasMarkerGap reports whether the text after a list marker begins with the whitespace (or end of line) that makes it a marker.
func hasMarkerGap(after string) bool {
	return after == "" || after[0] == ' ' || after[0] == '\t'
}

// isRule reports whether s is a thematic break (three or more -, * or _) or a setext underline (= or - run).
func isRule(s string) bool {
	compact := strings.ReplaceAll(s, " ", "")
	if len(compact) < 3 && !strings.HasPrefix(compact, "=") {
		return false
	}
	return strings.Trim(compact, compact[:1]) == "" && strings.ContainsAny(compact[:1], "-*_=")
}

// splitPunct splits a whitespace-free run into words and punctuation, keeping "'s" and mid-word apostrophes attached.
func splitPunct(runes []rune, offset uint32) []WordSpan {
	var spans []WordSpan
	emit := func(from, to int) {
		if to > from {
			spans = append(spans, WordSpan{
				Text:  string(runes[from:to]),
				Begin: offset + uint32(from),
				End:   offset + uint32(to),
			})
		}
	}
	start := 0
	n := len(runes)
	for i := 0; i < n; {
		r := runes[i]
		isPossessiveApostrophe := r == '\'' &&
			i+1 < n && (runes[i+1] == 's' || runes[i+1] == 'S') &&
			(i+2 >= n || !unicode.IsLetter(runes[i+2]))
		isMidWordApostrophe := r == '\'' && i > 0 && i+1 < n &&
			unicode.IsLetter(runes[i-1]) && unicode.IsLetter(runes[i+1]) &&
			!isPossessiveApostrophe
		switch {
		case isPossessiveApostrophe:
			emit(start, i)
			emit(i, i+2)
			i += 2
			start = i
		case (unicode.IsPunct(r) || unicode.IsSymbol(r)) && !isMidWordApostrophe:
			emit(start, i)
			emit(i, i+1)
			i++
			start = i
		default:
			i++
		}
	}
	emit(start, n)
	return spans
}

// IsAllPunct reports whether s consists entirely of punctuation/symbol runes.
func IsAllPunct(s string) bool {
	return s != "" && !strings.ContainsFunc(s, func(r rune) bool {
		return !unicode.IsPunct(r) && !unicode.IsSymbol(r)
	})
}
