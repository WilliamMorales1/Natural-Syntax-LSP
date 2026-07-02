// Package tokenizer does BERT-style basic tokenization + WordPiece encoding.
package tokenizer

import (
	"bufio"
	"os"
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
	unk := vocab["[UNK]"]
	cls := vocab["[CLS]"]
	sep := vocab["[SEP]"]
	// Cased vocabs have thousands of uppercase entries; uncased have essentially none outside special tokens, so 1% is a safe margin.
	uncased := upperEntries < len(vocab)/100
	return &BERTTokenizer{vocab: vocab, unkID: unk, clsID: cls, sepID: sep, uncased: uncased}, nil
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

// Tokenize returns flat input_ids/attention_mask/token_type_ids and word spans for mapping back to char offsets.
func (t *BERTTokenizer) Tokenize(text string) (inputIDs, attMask, tokTypeIDs []int64, words []WordSpan, subwordWordIdx []int, subwordIsFirst []bool) {
	words = BasicTokenize(text)
	ids, mask, ttids, swWordIdx, swIsFirst := t.TokenizeWords(words)
	return ids, mask, ttids, words, swWordIdx, swIsFirst
}

// TokenizeWords encodes a pre-split word slice into a [CLS]-prefixed, [SEP]-suffixed subword stream; [CLS]/[SEP] entries get swWordIdx -1.
func (t *BERTTokenizer) TokenizeWords(words []WordSpan) (inputIDs, attMask, tokTypeIDs []int64, swWordIdx []int, swIsFirst []bool) {
	ids := []int64{int64(t.clsID)}
	swWordIdx = []int{-1}
	swIsFirst = []bool{false}

	for wi, w := range words {
		pieces := t.WordPiece(w.Text)
		for pi, piece := range pieces {
			ids = append(ids, int64(t.VocabID(piece)))
			swWordIdx = append(swWordIdx, wi)
			swIsFirst = append(swIsFirst, pi == 0)
		}
	}
	ids = append(ids, int64(t.sepID))
	swWordIdx = append(swWordIdx, -1)
	swIsFirst = append(swIsFirst, false)

	n := len(ids)
	mask := make([]int64, n)
	ttids := make([]int64, n)
	for i := range ids {
		mask[i] = 1
	}
	return ids, mask, ttids, swWordIdx, swIsFirst
}

// WordPiece splits a single word into WordPiece subword tokens.
func (t *BERTTokenizer) WordPiece(word string) []string {
	if t.uncased {
		word = strings.ToLower(word)
	}
	if _, ok := t.vocab[word]; ok {
		return []string{word}
	}
	runes := []rune(word)
	n := len(runes)
	var result []string
	start := 0
	for start < n {
		end := n
		found := ""
		for end > start {
			sub := string(runes[start:end])
			if start > 0 {
				sub = "##" + sub
			}
			if _, ok := t.vocab[sub]; ok {
				found = sub
				break
			}
			end--
		}
		if found == "" {
			return []string{"[UNK]"}
		}
		result = append(result, found)
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

func splitPunct(runes []rune, offset uint32) []WordSpan {
	var spans []WordSpan
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
		if isPossessiveApostrophe {
			if i > start {
				spans = append(spans, WordSpan{
					Text:  string(runes[start:i]),
					Begin: offset + uint32(start),
					End:   offset + uint32(i),
				})
			}
			end := i + 2
			spans = append(spans, WordSpan{
				Text:  string(runes[i:end]),
				Begin: offset + uint32(i),
				End:   offset + uint32(end),
			})
			start = end
			i = end
		} else if (unicode.IsPunct(r) || unicode.IsSymbol(r)) && !isMidWordApostrophe {
			if i > start {
				spans = append(spans, WordSpan{
					Text:  string(runes[start:i]),
					Begin: offset + uint32(start),
					End:   offset + uint32(i),
				})
			}
			spans = append(spans, WordSpan{
				Text:  string(r),
				Begin: offset + uint32(i),
				End:   offset + uint32(i+1),
			})
			start = i + 1
			i++
		} else {
			i++
		}
	}
	if start < len(runes) {
		spans = append(spans, WordSpan{
			Text:  string(runes[start:]),
			Begin: offset + uint32(start),
			End:   offset + uint32(len(runes)),
		})
	}
	return spans
}

// IsAllPunct reports whether s consists entirely of punctuation/symbol runes.
func IsAllPunct(s string) bool {
	for _, r := range s {
		if !unicode.IsPunct(r) && !unicode.IsSymbol(r) {
			return false
		}
	}
	return len(s) > 0
}
