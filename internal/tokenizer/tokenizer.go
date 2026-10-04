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
	i := 0
	for i < n {
		// skip whitespace
		for i < n && unicode.IsSpace(runes[i]) {
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
		words = append(words, splitPunct(runes[start:i], uint32(start))...)
	}
	return words
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
