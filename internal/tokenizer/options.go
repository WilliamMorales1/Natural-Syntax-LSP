package tokenizer

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
	mergers    []WordMerger
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
	return func(o *options) { o.mergers = append(o.mergers, m) }
}
