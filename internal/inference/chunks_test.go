package inference

import (
	"strings"
	"testing"

	"natural-syntax-ls/internal/tokenizer"
)

// TestSplitChunksScripts checks chunking keeps every word in order, never exceeds ChunkSize, and breaks only between sentences, in each script.
func TestSplitChunksScripts(t *testing.T) {
	for _, c := range []struct{ name, para string }{
		{"english", "The dog runs fast. It is late! Is it?"},
		{"spanish", "¿Dónde está el gato? ¡Aquí está! Es negro."},
		{"french", "Le chat dort. L’ami arrive « vite ». Fin."},
		{"japanese", "今日は晴れです。明日は雨かもしれません。「行く」と言った。"},
		{"chinese", "今天天气很好。明天会下雨！你去吗？"},
		{"russian", "Он сказал «да». Потом ушёл. Почему?"},
		{"arabic", "ذهب الولد إلى المدرسة. هل عاد؟ نعم."},
	} {
		text := strings.Repeat(c.para+" ", 60) + "\n\n" + strings.Repeat(c.para+" ", 60)
		words := tokenizer.BasicTokenize(text)
		chunks := SplitChunks(text, words)
		if len(chunks) < 2 {
			t.Errorf("%s: %d chunks for %d words", c.name, len(chunks), len(words))
		}
		i := 0
		for _, ch := range chunks {
			if len(ch) > ChunkSize {
				t.Errorf("%s: chunk of %d words", c.name, len(ch))
			}
			for _, w := range ch {
				if w != words[i] {
					t.Fatalf("%s: word %d is %+v, want %+v", c.name, i, w, words[i])
				}
				i++
			}
		}
		if i != len(words) {
			t.Errorf("%s: chunks hold %d of %d words", c.name, i, len(words))
		}
	}
}

// TestSplitChunksMarkdownBlocks checks list items and headings in each script open new sentences, so short blocks share a chunk but never a sentence.
func TestSplitChunksMarkdownBlocks(t *testing.T) {
	for name, text := range map[string]string{
		"english":  "# Title\n- one item\n- two item\n\npara text here",
		"japanese": "# 題名\n- 一つ目\n- 二つ目\n\n本文です",
		"russian":  "# Заголовок\n- первый\n- второй\n\nтекст",
		"arabic":   "# عنوان\n- الأول\n- الثاني\n\nنص",
	} {
		words := tokenizer.BasicTokenize(text)
		var starts int
		for _, w := range words {
			if w.BlockStart {
				starts++
			}
		}
		if starts != 3 {
			t.Errorf("%s: %d block starts, want 3 (two items and the paragraph; the first word needs no mark)", name, starts)
		}
		var sents int
		for _, ch := range SplitChunks(text, words) {
			sents += len(splitSentences(ch))
		}
		if sents < starts+1 {
			t.Errorf("%s: %d sentences for %d block starts", name, sents, starts)
		}
	}
}
