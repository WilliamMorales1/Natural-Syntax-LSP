package main

import (
	"bufio"
	"os"
	"unicode"
)

// BERTTokenizer does basic BERT tokenization + WordPiece.
type BERTTokenizer struct {
	vocab    map[string]int
	unkID    int
	clsID    int
	sepID    int
	maxChunk int
}

func loadBERTTokenizer(vocabPath string) (*BERTTokenizer, error) {
	f, err := os.Open(vocabPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	vocab := make(map[string]int)
	sc := bufio.NewScanner(f)
	id := 0
	for sc.Scan() {
		vocab[sc.Text()] = id
		id++
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	unk := vocab["[UNK]"]
	cls := vocab["[CLS]"]
	sep := vocab["[SEP]"]
	return &BERTTokenizer{vocab: vocab, unkID: unk, clsID: cls, sepID: sep, maxChunk: 200}, nil
}

type wordSpan struct {
	text  string
	begin uint32
	end   uint32
}

type subwordToken struct {
	id      int32
	wordIdx int // index into words slice
	isFirst bool
}

// Tokenize returns: flat input_ids, attention_mask, token_type_ids (all int64),
// and the word spans for mapping back to char offsets.
func (t *BERTTokenizer) Tokenize(text string) (inputIDs, attMask, tokTypeIDs []int64, words []wordSpan, subwordWordIdx []int, subwordIsFirst []bool) {
	words = basicTokenize(text)

	// Build subwords with [CLS] at start.
	ids := []int64{int64(t.clsID)}
	swWordIdx := []int{-1}
	swIsFirst := []bool{false}

	for wi, w := range words {
		pieces := t.wordPiece(w.text)
		for pi, piece := range pieces {
			id, ok := t.vocab[piece]
			if !ok {
				id = t.unkID
			}
			ids = append(ids, int64(id))
			swWordIdx = append(swWordIdx, wi)
			swIsFirst = append(swIsFirst, pi == 0)
		}
	}
	// [SEP]
	ids = append(ids, int64(t.sepID))
	swWordIdx = append(swWordIdx, -1)
	swIsFirst = append(swIsFirst, false)

	n := len(ids)
	mask := make([]int64, n)
	ttids := make([]int64, n)
	for i := range ids {
		mask[i] = 1
	}
	return ids, mask, ttids, words, swWordIdx, swIsFirst
}

// tokenizeWords encodes a pre-split word slice (no basicTokenize step).
// swWordIdx indices are local to the provided words slice.
func (t *BERTTokenizer) tokenizeWords(words []wordSpan) (inputIDs, attMask, tokTypeIDs []int64, swWordIdx []int, swIsFirst []bool) {
	ids := []int64{int64(t.clsID)}
	swWordIdx = []int{-1}
	swIsFirst = []bool{false}

	for wi, w := range words {
		pieces := t.wordPiece(w.text)
		for pi, piece := range pieces {
			id, ok := t.vocab[piece]
			if !ok {
				id = t.unkID
			}
			ids = append(ids, int64(id))
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

func (t *BERTTokenizer) wordPiece(word string) []string {
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

// basicTokenize splits text on whitespace/punctuation while tracking char offsets.
func basicTokenize(text string) []wordSpan {
	var words []wordSpan
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

func splitPunct(runes []rune, offset uint32) []wordSpan {
	var spans []wordSpan
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
				spans = append(spans, wordSpan{
					text:  string(runes[start:i]),
					begin: offset + uint32(start),
					end:   offset + uint32(i),
				})
			}
			end := i + 2
			spans = append(spans, wordSpan{
				text:  string(runes[i:end]),
				begin: offset + uint32(i),
				end:   offset + uint32(end),
			})
			start = end
			i = end
		} else if (unicode.IsPunct(r) || unicode.IsSymbol(r)) && !isMidWordApostrophe {
			if i > start {
				spans = append(spans, wordSpan{
					text:  string(runes[start:i]),
					begin: offset + uint32(start),
					end:   offset + uint32(i),
				})
			}
			spans = append(spans, wordSpan{
				text:  string(r),
				begin: offset + uint32(i),
				end:   offset + uint32(i + 1),
			})
			start = i + 1
			i++
		} else {
			i++
		}
	}
	if start < len(runes) {
		spans = append(spans, wordSpan{
			text:  string(runes[start:]),
			begin: offset + uint32(start),
			end:   offset + uint32(len(runes)),
		})
	}
	return spans
}

func isAllPunct(s string) bool {
	for _, r := range s {
		if !unicode.IsPunct(r) && !unicode.IsSymbol(r) {
			return false
		}
	}
	return len(s) > 0
}
