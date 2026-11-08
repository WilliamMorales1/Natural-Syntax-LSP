// Package arabic splits Arabic clitics off written words into the syntactic words of UD Arabic-PADT, which the Arabic models were trained on.
package arabic

import (
	"strings"

	"natural-syntax-ls/internal/tokenizer"
)

// CliticMerger splits a conjunction (و ف), a preposition (ب ل ك), the future marker س and a pronoun suffix off each word.
// A split needs the model vocabulary to back it: the rest is a likelier word than the whole, or the whole is no vocabulary word in any common inflection.
func CliticMerger(score tokenizer.VocabScore) tokenizer.WordMerger {
	s := splitter{score}
	return func(_ []rune, words []tokenizer.WordSpan) []tokenizer.WordSpan {
		var out []tokenizer.WordSpan
		for _, w := range words {
			pieces := s.split(w.Text)
			if len(pieces) < 2 {
				out = append(out, w)
				continue
			}
			for i, p := range pieces {
				out = append(out, tokenizer.WordSpan{
					Text:       p.text,
					Begin:      w.Begin + uint32(p.begin),
					End:        w.Begin + uint32(p.end),
					BlockStart: w.BlockStart && i == 0,
				})
			}
		}
		return out
	}
}

type piece struct {
	text       string
	begin, end int // rune offsets in the word
}

type splitter struct{ score tokenizer.VocabScore }

var (
	pronouns = set("ه ها هم هما هن ك كم كما كن ي نا ني")
	// hosts take a pronoun suffix of any length; hamza-less spellings are common in news text.
	hosts = set("في على علي من عن إلى الى إلي الي مع أن ان إن لكن عند بين لدى لدي حول مثل عبر نحو منذ خلال أمام امام بعد قبل ضد دون لأن لان كأن كان ل ب ك")
	// standalone words follow و or ف as a separate word even though the joined form is in the vocabulary.
	standalone = set("هو هي هم هما هن أنا انا نحن أنت انت أنتم انتم ذلك هذا هذه تلك هؤلاء لا لم لن قد إن ان أن كان كانت ما لكن هناك هنا إذا اذا ثم لقد في على من عن إلى الى مع التي الذي الذين اللذين أيضا ايضا كذلك بعد قبل كل كما حتى أو او إذ اذ")
	endings    = []string{"ية", "ين", "ون", "ات", "ان", "تا", "يا", "ة", "ي", "ا", "ى", "هما", "هم", "ها", "ه", "نا"}
)

func set(words string) map[string]bool {
	m := map[string]bool{}
	for w := range strings.FieldsSeq(words) {
		m[w] = true
	}
	return m
}

func isMark(r rune) bool { return r >= 0x64B && r <= 0x652 || r == 0x670 || r == 0x640 }

func isLetter(r rune) bool { return r >= 0x621 && r <= 0x64A || r == 0x671 }

// split returns the word's pieces, or nil to keep it whole.
func (s splitter) split(word string) []piece {
	src := []rune(word)
	// Decide on bare letters; origin maps each back so marks stay with their letter.
	var bare []rune
	var origin []int
	for i, r := range src {
		switch {
		case isMark(r):
		case isLetter(r):
			bare = append(bare, r)
			origin = append(origin, i)
		default:
			return nil
		}
	}
	if len(bare) < 3 {
		return nil
	}
	var cuts []int // bare offsets where a new piece starts
	articleDropped := false
	r := bare
	at := 0
	cut := func(n int) {
		at += n
		cuts = append(cuts, at)
		r = r[n:]
	}
	if len(r) >= 3 && (r[0] == 'و' || r[0] == 'ف') {
		rest := string(r[1:])
		if standalone[rest] || s.better(rest, string(r)) || !s.known(string(r)) && len(r) > 3 {
			cut(1)
		}
	}
	switch {
	case len(r) >= 4 && r[0] == 'ل' && r[1] == 'ل':
		// لل is ل plus the article with its alef dropped.
		cut(1)
		articleDropped = true
	case len(r) >= 3 && (r[0] == 'ب' || r[0] == 'ل' || r[0] == 'ك'):
		rest := string(r[1:])
		if strings.HasPrefix(rest, "ال") && len(r) > 4 || s.better(rest, string(r)) || !s.known(string(r)) && len(r) > 5 {
			cut(1)
		}
	case len(r) >= 4 && r[0] == 'س' && strings.ContainsRune("يتنأا", r[1]):
		if s.better(string(r[1:]), string(r)) || !s.known(string(r)) {
			cut(1)
		}
	}
	if !articleDropped {
		if n := s.suffix(string(r)); n > 0 {
			cuts = append(cuts, at+len(r)-n)
		}
	}
	if len(cuts) == 0 {
		return nil
	}
	var out []piece
	from := 0
	for _, c := range append(cuts, len(bare)) {
		begin, end := 0, len(src)
		if from > 0 {
			begin = origin[from]
		}
		if c < len(bare) {
			end = origin[c]
		}
		out = append(out, piece{text: string(src[begin:end]), begin: begin, end: end})
		from = c
	}
	if articleDropped {
		out[1].text = "ا" + out[1].text
	}
	return out
}

// suffix returns the rune length of a pronoun suffix to split off host, or 0.
func (s splitter) suffix(host string) int {
	if standalone[host] {
		return 0
	}
	// A pronoun suffix never follows the article, so an ال word ending in ه is the word itself.
	definite := strings.HasPrefix(host, "ال")
	r := []rune(host)
	for n := 3; n >= 1; n-- {
		if len(r) <= n || !pronouns[string(r[len(r)-n:])] {
			continue
		}
		suf := string(r[len(r)-n:])
		stem := string(r[:len(r)-n])
		if hosts[stem] {
			return n
		}
		if len(r) == n+1 {
			continue
		}
		// One-letter ي and ك end too many nouns and adjectives to split on the vocabulary alone.
		if definite || n == 1 && suf != "ه" {
			continue
		}
		if s.better(stem, host) || !s.known(host) && len(r)-n >= 3 {
			return n
		}
		if t, ok := strings.CutSuffix(stem, "ت"); ok && s.better(t+"ة", host) {
			return n
		}
	}
	return 0
}

// better reports whether rest is a vocabulary word scoring above whole.
func (s splitter) better(rest, whole string) bool {
	rs, ok := s.score(rest)
	if !ok || len([]rune(rest)) < 3 {
		return false
	}
	ws, ok := s.score(whole)
	return !ok || rs > ws
}

// known reports whether word, or its stem under a common inflection ending or pronoun suffix, is a vocabulary word.
func (s splitter) known(word string) bool {
	if _, ok := s.score(word); ok {
		return true
	}
	for _, end := range endings {
		stem, ok := strings.CutSuffix(word, end)
		if !ok || len([]rune(stem)) < 3 {
			continue
		}
		if _, ok := s.score(stem); ok {
			return true
		}
		if _, ok := s.score(stem + "ة"); ok {
			return true
		}
	}
	return false
}
