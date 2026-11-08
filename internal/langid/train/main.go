// Command train builds internal/langid/linear.bin from Tatoeba per-language sentence dumps (<iso3>_sentences.tsv.bz2) and optional extra text.
//
//	go run ./internal/langid/train -data DIR [-extra DIR] -out internal/langid/linear.bin
package main

import (
	"bufio"
	"compress/bzip2"
	"flag"
	"fmt"
	"os"
	"strings"
)

// langs maps Tatoeba's ISO 639-3 file prefix to the Wiktionary code.
var langs = []struct{ file, code string }{
	{"eng", "en"}, {"fra", "fr"}, {"spa", "es"}, {"deu", "de"}, {"ita", "it"}, {"por", "pt"},
	{"rus", "ru"}, {"jpn", "ja"}, {"cmn", "zh"}, {"ara", "ar"},
	{"cat", "ca"}, {"ukr", "uk"}, {"kor", "ko"},
}

func main() {
	dir := flag.String("data", "", "directory with <iso3>_sentences.tsv.bz2 files")
	extra := flag.String("extra", "", "directory of <code>.txt files, one sentence per line, added to training (for example UD treebank text)")
	out := flag.String("out", "linear.bin", "output model file")
	perLang := flag.Int("n", 150000, "Tatoeba sentences per language")
	bits := flag.Int("bits", 17, "hash table size, log2 buckets")
	epochs := flag.Int("epochs", 6, "passes over the data")
	lr := flag.Float64("lr", 0.5, "Adagrad learning rate")
	crop := flag.Float64("crop", 0.6, "share of samples cut to a short random window")
	noise := flag.Float64("noise", 0.15, "share of samples with a URL, number or code snippet added")
	flag.Parse()
	trainLinear(*dir, *extra, *out, *perLang, uint8(*bits), *epochs, *lr, *crop, *noise)
}

// read returns the sentences of a Tatoeba dump, in file order.
func read(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(bzip2.NewReader(f))
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		if f := strings.SplitN(sc.Text(), "\t", 3); len(f) == 3 {
			out = append(out, f[2])
		}
	}
	return out
}
