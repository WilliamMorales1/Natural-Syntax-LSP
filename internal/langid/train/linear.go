package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"natural-syntax-ls/internal/langid"
)

// trainLinear fits softmax regression with Adagrad on hashed features averaged over each text, then writes int8 weights.
func trainLinear(dir, extra, out string, perLang int, bits uint8, epochs int, lr, cropShare, noiseShare float64) {
	nl := len(langs)
	type sample struct {
		text string
		lang int
	}
	var data []sample
	for i, l := range langs {
		for j, s := range read(filepath.Join(dir, l.file+"_sentences.tsv.bz2")) {
			// every 20th sentence is held out for the evaluation test.
			if j%20 == 19 || j >= perLang {
				continue
			}
			data = append(data, sample{s, i})
		}
	}
	if extra != "" {
		for i, l := range langs {
			b, err := os.ReadFile(filepath.Join(extra, l.code+".txt"))
			if err != nil {
				continue
			}
			for _, line := range strings.Split(string(b), "\n") {
				if line = strings.TrimSpace(line); line != "" {
					data = append(data, sample{line, i})
				}
			}
		}
	}
	rng := rand.New(rand.NewPCG(1, 2))
	size := 1 << bits
	w := make([]float32, size*nl)
	g2 := make([]float32, size*nl)
	bias := make([]float32, nl)
	bg2 := make([]float32, nl)
	logits := make([]float64, nl)
	var idx []int

	for ep := range epochs {
		rng.Shuffle(len(data), func(a, b int) { data[a], data[b] = data[b], data[a] })
		var loss float64
		for _, s := range data {
			text := s.text
			if rng.Float64() < cropShare {
				text = crop(rng, text)
			}
			if rng.Float64() < noiseShare {
				text = addNoise(rng, text)
			}
			idx = idx[:0]
			langid.Features(text, func(f []byte) { idx = append(idx, int(langid.Hash(f, bits))) })
			if len(idx) == 0 {
				continue
			}
			inv := float32(1 / float64(len(idx)))
			for l := range logits {
				logits[l] = float64(bias[l])
			}
			for _, h := range idx {
				for l, v := range w[h*nl : h*nl+nl] {
					logits[l] += float64(v) * float64(inv)
				}
			}
			peak := logits[0]
			for _, v := range logits {
				peak = max(peak, v)
			}
			var sum float64
			for l, v := range logits {
				logits[l] = math.Exp(v - peak)
				sum += logits[l]
			}
			for l := range logits {
				logits[l] /= sum
			}
			loss -= math.Log(max(logits[s.lang], 1e-12))
			for l := range nl {
				grad := float32(logits[l])
				if l == s.lang {
					grad--
				}
				bg2[l] += grad * grad
				bias[l] -= float32(lr) * grad / float32(math.Sqrt(float64(bg2[l]))+1e-8)
				for _, h := range idx {
					gw := grad * inv
					k := h*nl + l
					g2[k] += gw * gw
					w[k] -= float32(lr) * gw / float32(math.Sqrt(float64(g2[k]))+1e-8)
				}
			}
		}
		fmt.Printf("epoch %d: loss %.4f\n", ep+1, loss/float64(len(data)))
	}

	scales := make([]float32, nl)
	for l := range nl {
		var peak float32
		for h := range size {
			peak = max(peak, float32(math.Abs(float64(w[h*nl+l]))))
		}
		scales[l] = peak / 127
	}
	b := []byte{0xFF, bits, byte(nl)}
	for _, l := range langs {
		b = append(b, byte(len(l.code)))
		b = append(b, l.code...)
	}
	for _, sc := range scales {
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(sc))
	}
	for _, v := range bias {
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(v))
	}
	for h := range size {
		for l := range nl {
			q := 0.0
			if scales[l] > 0 {
				q = math.Round(float64(w[h*nl+l] / scales[l]))
			}
			b = append(b, byte(int8(max(-127, min(127, q)))))
		}
	}
	if err := os.WriteFile(out, b, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%d buckets, %d bytes\n", size, len(b))
}

// crop cuts a random short window out of text so the model sees the 4-30 character inputs it meets while typing.
func crop(rng *rand.Rand, text string) string {
	target := 4 + rng.IntN(27)
	if words := strings.Fields(text); len(words) >= 3 {
		i := rng.IntN(len(words))
		j, n := i, 0
		for j < len(words) && n < target {
			n += utf8.RuneCountInString(words[j]) + 1
			j++
		}
		return strings.Join(words[i:j], " ")
	}
	r := []rune(text)
	if len(r) <= target {
		return text
	}
	i := rng.IntN(len(r) - target + 1)
	return string(r[i : i+target])
}

var noiseSnippets = []string{
	"https://example.com/docs/index.html", "www.github.com/user/repo", "user@example.org", "foo_bar(baz, 42)", "x := y + 1",
	"if (a == b) { return null; }", "#include <stdio.h>", "SELECT * FROM users;", "2024-11-07 12:30", "3.14159", "1234567",
	"<div class=\"main\">", "```go", "func main() {}", "C:\\Users\\name\\file.txt", "/usr/local/bin", "TODO(fix)", "v1.2.3", "100%", "#hashtag @mention",
}

// addNoise puts a URL, number or code snippet before or after text, as in notes and source comments.
func addNoise(rng *rand.Rand, text string) string {
	n := noiseSnippets[rng.IntN(len(noiseSnippets))]
	if rng.IntN(2) == 0 {
		return n + " " + text
	}
	return text + " " + n
}
