package langid

import (
	_ "embed"
	"encoding/binary"
	"errors"
	"math"
	"sync"
	"unicode"
	"unicode/utf8"
)

// linearMagic starts a model file.
const linearMagic = 0xFF

const (
	linearMaxN = 5
	// wordPrefix separates word features from character n-grams in the hash input.
	wordPrefix = 1
)

//go:embed linear.bin
var linearData []byte

// linear is a softmax regression over hashed character n-grams and words, with int8 weights.
type linear struct {
	codes   []string
	bits    uint8
	scales  []float32 // per language dequantization factor
	biases  []float32
	weights []int8 // (1<<bits) buckets of len(codes) weights
}

var loadLinear = sync.OnceValues(func() (*linear, error) { return parseLinear(linearData) })

// parseLinear reads the format written by internal/langid/train.
func parseLinear(b []byte) (*linear, error) {
	bad := errors.New("langid: corrupt linear model")
	if len(b) < 4 || b[0] != linearMagic {
		return nil, bad
	}
	m := &linear{bits: b[1]}
	nl := int(b[2])
	b = b[3:]
	for range nl {
		if len(b) < 1 || len(b) < 1+int(b[0]) {
			return nil, bad
		}
		m.codes = append(m.codes, string(b[1:1+int(b[0])]))
		b = b[1+int(b[0]):]
	}
	if m.bits == 0 || m.bits > 24 || len(b) < 8*nl+(nl<<m.bits) {
		return nil, bad
	}
	for i := range nl {
		m.scales = append(m.scales, math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:])))
	}
	b = b[4*nl:]
	for i := range nl {
		m.biases = append(m.biases, math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:])))
	}
	b = b[4*nl:]
	m.weights = make([]int8, nl<<m.bits)
	for i := range m.weights {
		m.weights[i] = int8(b[i])
	}
	return m, nil
}

// Hash maps a feature to a bucket in a table of 1<<bits entries (FNV-1a).
func Hash(feature []byte, bits uint8) uint32 {
	h := uint32(2166136261)
	for _, c := range feature {
		h = (h ^ uint32(c)) * 16777619
	}
	return h & (1<<bits - 1)
}

// Features calls emit with every feature of text: lowercased-letter character n-grams of 1 to 5 runes over space-padded text, plus each word. It returns the letter count.
// The training tool uses it too, so model and detector see the same features. emit's argument is only valid during the call.
func Features(text string, emit func(feature []byte)) (letters int) {
	runes := make([]rune, 0, len(text)+2)
	runes = append(runes, ' ')
	for _, r := range text {
		switch {
		case unicode.IsLetter(r):
			runes = append(runes, unicode.ToLower(r))
			letters++
		case runes[len(runes)-1] != ' ':
			runes = append(runes, ' ')
		}
	}
	if runes[len(runes)-1] != ' ' {
		runes = append(runes, ' ')
	}
	var buf []byte
	for i := range runes {
		buf = buf[:0]
		for n := 1; n <= linearMaxN && i+n <= len(runes); n++ {
			buf = utf8.AppendRune(buf, runes[i+n-1])
			emit(buf)
		}
	}
	start := -1
	for i, r := range runes {
		switch {
		case r != ' ' && start < 0:
			start = i
		case r == ' ' && start >= 0:
			buf = append(buf[:0], wordPrefix)
			buf = append(buf, string(runes[start:i])...)
			emit(buf)
			start = -1
		}
	}
	return letters
}

// scores returns the per-language logits for text and the letter count.
func (m *linear) scores(text string) ([]float64, int) {
	nl := len(m.codes)
	out := make([]float64, nl)
	feats := 0
	letters := Features(text, func(f []byte) {
		feats++
		row := m.weights[int(Hash(f, m.bits))*nl:][:nl]
		for l, w := range row {
			out[l] += float64(w) * float64(m.scales[l])
		}
	})
	if feats == 0 {
		return out, letters
	}
	for l := range out {
		out[l] = out[l]/float64(feats) + float64(m.biases[l])
	}
	return out, letters
}

// minProb is the softmax probability the best language needs to be reported.
const minProb = 0.5

// detect returns the best language and its softmax probability, or "" when the text is too short or the best is under minProb.
func (m *linear) detect(text string) (string, float64) {
	logits, letters := m.scores(text)
	if letters < minLetters {
		return "", 0
	}
	best := 0
	peak := math.Inf(-1)
	for l, v := range logits {
		if v > peak {
			best, peak = l, v
		}
	}
	var sum float64
	for _, v := range logits {
		sum += math.Exp(v - peak)
	}
	p := 1 / sum
	if p < minProb {
		return "", p
	}
	return m.codes[best], p
}
