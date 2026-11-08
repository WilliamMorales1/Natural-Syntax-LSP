package tokenizer

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Tokenizer is what inference needs from any model tokenizer.
type Tokenizer interface {
	// Tokenize splits text into the words the model tags.
	Tokenize(text string) []WordSpan
	// WordIDs encodes one word from Tokenize into model vocab ids, without special tokens.
	WordIDs(word string) []int64
	// CLSID is the id opening every sequence.
	CLSID() int
	// SEPID is the id closing every sequence.
	SEPID() int
}

// continuer is implemented by tokenizers that mark word starts and can encode a word without the mark.
type continuer interface {
	ContinuationIDs(word string) []int64
}

// Encoding is a subword stream for pre-split words, wrapped in special tokens.
type Encoding struct {
	InputIDs, AttMask, TokTypeIDs []int64
	// SwWordIdx maps each subword to its word index; special tokens get -1.
	SwWordIdx []int
	// SwIsFirst marks each word's first subword.
	SwIsFirst []bool
}

// EncodeWords builds a special-token-wrapped id stream for pre-split words; wrapper entries get SwWordIdx -1.
func EncodeWords(t Tokenizer, words []WordSpan) Encoding {
	return encodeWords(t, words, false)
}

// EncodeWordsContiguous is EncodeWords, but a word that starts where the previous one ended in the source gets no word-start mark, so unspaced text (Japanese, Chinese, punctuation) reaches the model as it was written.
func EncodeWordsContiguous(t Tokenizer, words []WordSpan) Encoding {
	return encodeWords(t, words, true)
}

func encodeWords(t Tokenizer, words []WordSpan, contiguous bool) Encoding {
	inputIDs := []int64{int64(t.CLSID())}
	swWordIdx := []int{-1}
	swIsFirst := []bool{false}
	cont, canCont := t.(continuer)
	for wi, w := range words {
		ids := t.WordIDs(w.Text)
		if contiguous && canCont && wi > 0 && !w.BlockStart && w.Begin == words[wi-1].End {
			ids = cont.ContinuationIDs(w.Text)
		}
		for pi, id := range ids {
			inputIDs = append(inputIDs, id)
			swWordIdx = append(swWordIdx, wi)
			swIsFirst = append(swIsFirst, pi == 0)
		}
	}
	inputIDs = append(inputIDs, int64(t.SEPID()))
	swWordIdx = append(swWordIdx, -1)
	swIsFirst = append(swIsFirst, false)

	n := len(inputIDs)
	attMask := make([]int64, n)
	for i := range attMask {
		attMask[i] = 1
	}
	return Encoding{inputIDs, attMask, make([]int64, n), swWordIdx, swIsFirst}
}

// UnigramTokenizer is a SentencePiece Unigram model loaded from a HF tokenizer.json.
type UnigramTokenizer struct {
	ids       map[string]int32
	scores    []float64
	pieces    []string
	unkID     int
	unkScore  float64
	maxPiece  int
	clsID     int
	sepID     int
	metaspace bool
	opts      options
}

// unigramFile is the subset of a HF tokenizer.json this package reads.
type unigramFile struct {
	Model struct {
		Type  string            `json:"type"`
		UnkID int               `json:"unk_id"`
		Vocab []json.RawMessage `json:"vocab"`
	} `json:"model"`
	PreTokenizer  json.RawMessage `json:"pre_tokenizer"`
	PostProcessor struct {
		Single []struct {
			SpecialToken *struct {
				ID string `json:"id"`
			} `json:"SpecialToken"`
		} `json:"single"`
	} `json:"post_processor"`
}

// isUnigramFile reports whether data is a full HF tokenizer.json with a Unigram model, as opposed to a tokenizer_config.json.
func isUnigramFile(data []byte) bool {
	var probe struct {
		Model struct {
			Type string `json:"type"`
		} `json:"model"`
	}
	return json.Unmarshal(data, &probe) == nil && probe.Model.Type == "Unigram"
}

// LoadUnigram reads a HF tokenizer.json holding a Unigram model.
func LoadUnigram(path string, opts ...Option) (*UnigramTokenizer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load unigram tokenizer: %w", err)
	}
	return parseUnigram(data, opts...)
}

func parseUnigram(data []byte, opts ...Option) (*UnigramTokenizer, error) {
	var f unigramFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	if f.Model.Type != "Unigram" {
		return nil, fmt.Errorf("tokenizer model type %q, want Unigram", f.Model.Type)
	}
	t := &UnigramTokenizer{
		ids:       make(map[string]int32, len(f.Model.Vocab)),
		scores:    make([]float64, len(f.Model.Vocab)),
		pieces:    make([]string, len(f.Model.Vocab)),
		unkID:     f.Model.UnkID,
		metaspace: strings.Contains(string(f.PreTokenizer), `"Metaspace"`),
		opts:      newOptions(opts),
	}
	minScore := 0.0
	for i, raw := range f.Model.Vocab {
		var entry []json.RawMessage
		if err := json.Unmarshal(raw, &entry); err != nil || len(entry) != 2 {
			return nil, fmt.Errorf("bad unigram vocab entry %d", i)
		}
		var piece string
		var score float64
		if json.Unmarshal(entry[0], &piece) != nil || json.Unmarshal(entry[1], &score) != nil {
			return nil, fmt.Errorf("bad unigram vocab entry %d", i)
		}
		t.pieces[i], t.scores[i] = piece, score
		if _, dup := t.ids[piece]; !dup {
			t.ids[piece] = int32(i)
		}
		minScore = math.Min(minScore, score)
		t.maxPiece = max(t.maxPiece, len([]rune(piece)))
	}
	// SentencePiece scores unknown characters ten below the worst real piece so they only win as a last resort.
	t.unkScore = minScore - 10
	special := func(i int) int {
		if i < len(f.PostProcessor.Single) && f.PostProcessor.Single[i].SpecialToken != nil {
			if id, ok := t.ids[f.PostProcessor.Single[i].SpecialToken.ID]; ok {
				return int(id)
			}
		}
		return -1
	}
	t.clsID = special(0)
	t.sepID = special(len(f.PostProcessor.Single) - 1)
	if t.clsID < 0 || t.sepID < 0 {
		return nil, fmt.Errorf("tokenizer.json has no single-sequence special tokens")
	}
	return t, nil
}

// CLSID returns the id that opens a sequence ([CLS] or <s>).
func (t *UnigramTokenizer) CLSID() int { return t.clsID }

// SEPID returns the id that closes a sequence ([SEP] or </s>).
func (t *UnigramTokenizer) SEPID() int { return t.sepID }

// Pieces returns the surface strings of ids, for debugging.
func (t *UnigramTokenizer) Pieces(ids []int64) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = t.pieces[id]
	}
	return out
}

// WordIDs encodes one pre-split word, with a leading ▁ for models that mark word starts.
func (t *UnigramTokenizer) WordIDs(word string) []int64 { return t.wordIDs(word, t.metaspace) }

// ContinuationIDs encodes a word that continues the previous one with no space between, so it carries no ▁.
func (t *UnigramTokenizer) ContinuationIDs(word string) []int64 { return t.wordIDs(word, false) }

func (t *UnigramTokenizer) wordIDs(word string, mark bool) []int64 {
	norm, _ := normalizeRunes(word)
	if mark {
		norm = append([]rune{'▁'}, norm...)
	}
	segs := t.viterbi(norm)
	out := make([]int64, len(segs))
	for i, s := range segs {
		out[i] = int64(s.id)
	}
	return out
}

type segment struct {
	from, to int // rune range in the viterbi input
	id       int
}

// viterbi finds the highest-scoring segmentation of runes into vocab pieces.
func (t *UnigramTokenizer) viterbi(runes []rune) []segment {
	n := len(runes)
	if n == 0 {
		return nil
	}
	best := make([]float64, n+1)
	prev := make([]int, n+1)
	prevID := make([]int, n+1)
	for i := 1; i <= n; i++ {
		best[i] = math.Inf(-1)
	}
	for end := 1; end <= n; end++ {
		for start := max(0, end-t.maxPiece); start < end; start++ {
			id, ok := t.ids[string(runes[start:end])]
			if !ok || best[start] == math.Inf(-1) {
				continue
			}
			if s := best[start] + t.scores[id]; s > best[end] {
				best[end], prev[end], prevID[end] = s, start, int(id)
			}
		}
		// Single unknown character as fallback, scored below any real piece.
		if best[end] == math.Inf(-1) {
			best[end], prev[end], prevID[end] = best[end-1]+t.unkScore, end-1, t.unkID
		}
	}
	var segs []segment
	for end := n; end > 0; end = prev[end] {
		segs = append(segs, segment{prev[end], end, prevID[end]})
	}
	slices.Reverse(segs)
	// HF fuses a run of unknown characters into a single unknown token.
	fused := segs[:1]
	for _, s := range segs[1:] {
		if last := &fused[len(fused)-1]; s.id == t.unkID && last.id == t.unkID {
			last.to = s.to
		} else {
			fused = append(fused, s)
		}
	}
	return fused
}

// Tokenize splits text into words like the WordPiece path does, applies the configured word mergers, then breaks each CJK run into words: with an applicable Segmenter its words, otherwise the Unigram pieces the model would see, since those scripts have no spaces.
func (t *UnigramTokenizer) Tokenize(text string) []WordSpan {
	runes := []rune(text)
	var seg Segmenter
	for _, s := range t.opts.segmenters {
		if s.Applies(runes) {
			seg = s
			break
		}
	}
	words := basicTokenize(text, false, true)
	for _, merge := range t.opts.mergers {
		words = merge(runes, words)
	}
	var out []WordSpan
	for _, w := range words {
		out = append(out, t.splitCJK(runes, w, seg)...)
	}
	return out
}

// splitCJK cuts out maximal Han/kana runs from w and segments each with Unigram.
func (t *UnigramTokenizer) splitCJK(runes []rune, w WordSpan, seg Segmenter) []WordSpan {
	src := runes[w.Begin:w.End]
	hasCJK := slices.ContainsFunc(src, isCJK)
	if !hasCJK {
		return []WordSpan{w}
	}
	var parts []WordSpan
	flush := func(from, to int) {
		if to > from {
			parts = append(parts, WordSpan{Text: string(src[from:to]), Begin: w.Begin + uint32(from), End: w.Begin + uint32(to)})
		}
	}
	for i := 0; i < len(src); {
		j := i
		cjk := isCJK(src[i])
		for j < len(src) && isCJK(src[j]) == cjk {
			j++
		}
		if !cjk {
			flush(i, j)
			i = j
			continue
		}
		if seg != nil {
			if ranges := seg.Segment(string(src[i:j])); coversRun(ranges, j-i) {
				for _, r := range ranges {
					flush(i+r[0], i+r[1])
				}
				i = j
				continue
			}
		}
		norm, origin := normalizeRunes(string(src[i:j]))
		for _, s := range t.viterbi(norm) {
			flush(i+origin[s.from], i+originEnd(origin, s.to, j-i))
		}
		i = j
	}
	if len(parts) > 0 {
		parts[0].BlockStart = w.BlockStart
	}
	return parts
}

// coversRun reports whether ranges tile [0, n) in order with no gap, the only segmentation worth trusting.
func coversRun(ranges [][2]int, n int) bool {
	pos := 0
	for _, r := range ranges {
		if r[0] != pos || r[1] <= r[0] {
			return false
		}
		pos = r[1]
	}
	return pos == n
}

// originEnd maps an exclusive normalized rune end back to an exclusive original rune end.
func originEnd(origin []int, to, srcLen int) int {
	if to >= len(origin) {
		return srcLen
	}
	return origin[to]
}

// normalizeRunes applies NFKC cluster by cluster and returns the result with, for each output rune, the index of the source rune that produced it.
func normalizeRunes(s string) (out []rune, origin []int) {
	src := []rune(s)
	for i := 0; i < len(src); {
		j := i + 1
		for j < len(src) && unicode.In(src[j], unicode.Mn, unicode.Me, unicode.Mc) {
			j++
		}
		for _, r := range norm.NFKC.String(string(src[i:j])) {
			if r == 0 || (unicode.IsControl(r) && !unicode.IsSpace(r)) {
				continue
			}
			out = append(out, r)
			origin = append(origin, i)
		}
		i = j
	}
	return out, origin
}
