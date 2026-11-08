package tokenizer

import "slices"

// Segmenter splits scripts written without spaces into words, replacing the model's own pieces when it applies.
type Segmenter interface {
	// Applies reports whether the segmenter should handle runs of text, e.g. because it is in the segmenter's language.
	Applies(text []rune) bool
	// Segment returns rune ranges tiling run in order, or nil to fall back to the model's pieces.
	Segment(run string) [][2]int
}

// WordMerger rewrites the whitespace and punctuation split of text, for words the generic split cuts wrongly in one language.
type WordMerger func(text []rune, words []WordSpan) []WordSpan

// Option adds language-specific behavior to a tokenizer.
type Option func(*options)

type options struct {
	segmenters []Segmenter
	mergers    []scopedMerger
	detect     func(text string) string
	scope      string // language code applied to mergers added while building a WithLanguage group
}

// scopedMerger is a merger that runs only on text detected as lang ("" runs everywhere).
type scopedMerger struct {
	lang  string
	merge WordMerger
}

func newOptions(opts []Option) options {
	var o options
	for _, f := range opts {
		f(&o)
	}
	return o
}

// WithSegmenter adds a segmenter; the first one that applies to a text wins.
func WithSegmenter(s Segmenter) Option {
	return func(o *options) { o.segmenters = append(o.segmenters, s) }
}

// WithWordMerger adds a merger, run in the order given.
func WithWordMerger(m WordMerger) Option {
	return func(o *options) { o.mergers = append(o.mergers, scopedMerger{o.scope, m}) }
}

// WithLanguage limits the mergers in opts to lines whose detected language is code; see WithDetector.
func WithLanguage(code string, opts ...Option) Option {
	return func(o *options) {
		prev := o.scope
		o.scope = code
		for _, f := range opts {
			f(o)
		}
		o.scope = prev
	}
}

// WithDetector sets the function that names a text's language by its Wiktionary code, or "" when unsure.
// Without one, or when it cannot tell, mergers limited by WithLanguage run on every line.
func WithDetector(detect func(text string) string) Option {
	return func(o *options) { o.detect = detect }
}

// docSample caps how much of a text is read to guess the language of lines too short to tell.
const docSample = 4000

// merge runs the mergers over words, choosing the language-limited ones per line by the line's detected language, else the previous line's, else the text's.
func (o *options) merge(runes []rune, words []WordSpan) []WordSpan {
	scoped := o.detect != nil && slices.ContainsFunc(o.mergers, func(m scopedMerger) bool { return m.lang != "" })
	if !scoped {
		for _, m := range o.mergers {
			words = m.merge(runes, words)
		}
		return words
	}
	whole := o.detect(string(runes[:min(len(runes), docSample)]))
	var out []WordSpan
	prev := whole
	for i := 0; i < len(words); {
		begin := lineStart(runes, int(words[i].Begin))
		end := lineEnd(runes, int(words[i].Begin))
		j := i + 1
		for j < len(words) && int(words[j].Begin) < end {
			j++
		}
		code := o.detect(string(runes[begin:end]))
		if code == "" {
			code = prev
		}
		prev = code
		group := slices.Clone(words[i:j])
		for _, m := range o.mergers {
			// With no language known at all, run everything as if no detector were set.
			if m.lang == "" || code == "" || m.lang == code {
				group = m.merge(runes, group)
			}
		}
		out = append(out, group...)
		i = j
	}
	return out
}

func lineStart(runes []rune, at int) int {
	for at > 0 && runes[at-1] != '\n' {
		at--
	}
	return at
}

func lineEnd(runes []rune, at int) int {
	for at < len(runes) && runes[at] != '\n' {
		at++
	}
	return at
}
