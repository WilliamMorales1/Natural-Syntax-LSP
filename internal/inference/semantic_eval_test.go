package inference

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode"

	_ "natural-syntax-ls/internal/jaseg"
)

// Semantic quality across languages. Off by default: set NSLS_SEMANTIC_EVAL=1.
//
//	NSLS_ML_DIR   dir with multilingual_minilm.* (default: the config dir)
//
// Each language gets one paragraph of six sentences, every sentence holding one word from each of four groups (animals, food, technology, emotions), as in docs/semantic-example.txt.
// Quality is measured on the 24 target words only, with embeddings taken as the server takes them (contextual, mean of overlapping pieces):
//
//	sep    mean between-group distance / mean within-group distance of embeddings (higher is better)
//	colsep the same on the colors' disk positions after fitting a SemanticPlane to the paragraph, as the server does
//	grp    leave-one-out nearest-centroid group accuracy (chance 0.25)
//	xgrp   group accuracy of the nearest word in another language, mean over language pairs
//	xconc  top-1 accuracy of finding the translation of a word among the other language's 24 words (chance 0.04)

var semGroups = []string{"animals", "food", "technology", "emotions"}

// semEvalDocs: per language a paragraph and its 24 target words, sentence-major (animal, food, tech, emotion).
var semEvalDocs = map[string]struct {
	text  string
	words []string
}{
	"en": {"A fox found some bread near the old computer and seemed happy. Later a deer ate an apple beside the printer, looking scared. An owl watched the cheese on the keyboard and felt angry. The rabbit nibbled honey next to the router, looking calm. A wolf stole the butter from the server room, feeling proud. The bear tasted the soup beside the phone and grew tired.",
		[]string{"fox", "bread", "computer", "happy", "deer", "apple", "printer", "scared", "owl", "cheese", "keyboard", "angry", "rabbit", "honey", "router", "calm", "wolf", "butter", "server", "proud", "bear", "soup", "phone", "tired"}},
	"es": {"Un zorro encontró pan cerca de la vieja computadora y parecía feliz. Luego un ciervo comió una manzana junto a la impresora, asustado. Un búho miró el queso sobre el teclado y se sintió enojado. El conejo mordisqueó miel junto al enrutador, tranquilo. Un lobo robó la mantequilla del servidor, orgulloso. El oso probó la sopa junto al teléfono y se puso cansado.",
		[]string{"zorro", "pan", "computadora", "feliz", "ciervo", "manzana", "impresora", "asustado", "búho", "queso", "teclado", "enojado", "conejo", "miel", "enrutador", "tranquilo", "lobo", "mantequilla", "servidor", "orgulloso", "oso", "sopa", "teléfono", "cansado"}},
	"fr": {"Un renard a trouvé du pain près du vieil ordinateur et semblait heureux. Plus tard un cerf a mangé une pomme à côté de l'imprimante, effrayé. Un hibou regardait le fromage sur le clavier et se sentait fâché. Le lapin grignotait du miel près du routeur, calme. Un loup a volé le beurre dans la salle du serveur, fier. L'ours a goûté la soupe à côté du téléphone et s'est senti fatigué.",
		[]string{"renard", "pain", "ordinateur", "heureux", "cerf", "pomme", "imprimante", "effrayé", "hibou", "fromage", "clavier", "fâché", "lapin", "miel", "routeur", "calme", "loup", "beurre", "serveur", "fier", "ours", "soupe", "téléphone", "fatigué"}},
	"ja": {"キツネは古いコンピューターの近くでパンを見つけて、嬉しそうだった。その後、シカはプリンターの横でリンゴを食べて、怖がっていた。フクロウはキーボードの上のチーズを見て、怒っていた。ウサギはルーターのそばでハチミツをかじり、穏やかだった。オオカミはサーバーからバターを盗んで、誇らしげだった。クマは電話の隣でスープを味わい、疲れていた。",
		[]string{"キツネ", "パン", "コンピューター", "嬉し", "シカ", "リンゴ", "プリンター", "怖", "フクロウ", "チーズ", "キーボード", "怒", "ウサギ", "ハチミツ", "ルーター", "穏やか", "オオカミ", "バター", "サーバー", "誇", "クマ", "スープ", "電話", "疲れ"}},
	// ja2 avoids katakana, which the XLM-R vocabulary splits into one-character pieces.
	"ja2": {"狐は古い電話の近くで米を見つけて、嬉しそうだった。その後、鹿は機械の横で肉を食べて、怖がっていた。梟は画面の上の魚を見て、怒っていた。兎は電池のそばで茶を飲み、穏やかだった。狼は時計の下で塩を盗んで、誇らしげだった。熊は電線の隣で卵を味わい、疲れていた。",
		[]string{"狐", "米", "電話", "嬉し", "鹿", "肉", "機械", "怖", "梟", "魚", "画面", "怒", "兎", "茶", "電池", "穏やか", "狼", "塩", "時計", "誇", "熊", "卵", "電線", "疲れ"}},
	"zh": {"狐狸在旧电脑旁找到了面包，看起来很高兴。后来鹿在打印机旁吃了苹果，显得很害怕。猫头鹰看着键盘上的奶酪，感到很生气。兔子在路由器旁啃着蜂蜜，神情平静。狼从服务器室偷走了黄油，觉得很骄傲。熊在手机旁尝了尝汤，变得很疲倦。",
		[]string{"狐狸", "面包", "电脑", "高兴", "鹿", "苹果", "打印机", "害怕", "猫头鹰", "奶酪", "键盘", "生气", "兔子", "蜂蜜", "路由器", "平静", "狼", "黄油", "服务器", "骄傲", "熊", "汤", "手机", "疲倦"}},
}

// findWord returns the rune range of the first occurrence of word in text, requiring letter boundaries for scripts written with spaces.
func findWord(text, word string) (int, int, bool) {
	rt, rw := []rune(text), []rune(word)
	cjk := unicode.Is(unicode.Han, rw[0]) || unicode.Is(unicode.Katakana, rw[0]) || unicode.Is(unicode.Hiragana, rw[0])
	for i := 0; i+len(rw) <= len(rt); i++ {
		if string(rt[i:i+len(rw)]) != word {
			continue
		}
		if !cjk && ((i > 0 && unicode.IsLetter(rt[i-1])) || (i+len(rw) < len(rt) && unicode.IsLetter(rt[i+len(rw)]))) {
			continue
		}
		return i, i + len(rw), true
	}
	return 0, 0, false
}

type semEmbeds struct {
	targets [][]float32 // 24 unit vectors
	disk    [][2]float32
}

func semEmbed(t *testing.T, m *EmbeddingModel, lang string) semEmbeds {
	t.Helper()
	d := semEvalDocs[lang]
	tokens, units, err := m.EmbedChunk(m.Tokenize(d.text))
	if err != nil {
		t.Fatal(err)
	}
	var plane SemanticPlane
	plane.Fit(units)
	var out semEmbeds
	for _, w := range d.words {
		b, e, ok := findWord(d.text, w)
		if !ok {
			t.Fatalf("%s: %q not in text", lang, w)
		}
		var sum []float32
		n := 0
		for i, tok := range tokens {
			if int(tok.OffsetBegin) < e && int(tok.OffsetEnd) > b {
				if sum == nil {
					sum = make([]float32, len(units[i]))
				}
				for j, x := range units[i] {
					sum[j] += x
				}
				n++
			}
		}
		if n == 0 {
			t.Fatalf("%s: no token overlaps %q", lang, w)
		}
		v := l2Normalize(sum)
		out.targets = append(out.targets, v)
		x, y := plane.diskPoint(v)
		out.disk = append(out.disk, [2]float32{x, y})
	}
	return out
}

func chord(a, b []float32) float64 {
	var s float64
	for i := range a {
		d := float64(a[i] - b[i])
		s += d * d
	}
	return math.Sqrt(s)
}

func cosine(a, b []float32) float64 { return float64(dot(a, b)) }

func sepRatio(n int, dist func(i, j int) float64) float64 {
	var within, between float64
	var nw, nb int
	for i := range n {
		for j := i + 1; j < n; j++ {
			if i%4 == j%4 {
				within += dist(i, j)
				nw++
			} else {
				between += dist(i, j)
				nb++
			}
		}
	}
	return (between / float64(nb)) / (within / float64(nw))
}

// groupAcc is leave-one-out nearest-centroid accuracy over the 24 words.
func groupAcc(v [][]float32) float64 {
	hit := 0
	for i := range v {
		best, bestSim := -1, -2.0
		for g := range 4 {
			c := make([]float32, len(v[i]))
			for j := range v {
				if j != i && j%4 == g {
					axpy(1, v[j], c)
				}
			}
			if s := cosine(l2Normalize(c), v[i]); s > bestSim {
				best, bestSim = g, s
			}
		}
		if best == i%4 {
			hit++
		}
	}
	return float64(hit) / float64(len(v))
}

func nearest(q []float32, pool [][]float32) int {
	best, bestSim := -1, -2.0
	for j, p := range pool {
		if s := cosine(q, p); s > bestSim {
			best, bestSim = j, s
		}
	}
	return best
}

func openSemModel(t *testing.T, base string, dim int) *EmbeddingModel {
	t.Helper()
	InitSemantic(dim)
	vocab := base + "_tokenizer.json"
	if _, err := os.Stat(vocab); err != nil {
		vocab = base + "_vocab.txt"
	}
	m, err := NewEmbeddingModel(base+".onnx", vocab, dim)
	if err != nil {
		t.Skip(err)
	}
	return m
}

func TestSemanticQualityAcrossLanguages(t *testing.T) {
	if os.Getenv("NSLS_SEMANTIC_EVAL") == "" {
		t.Skip("set NSLS_SEMANTIC_EVAL=1")
	}
	dir := os.Getenv("NSLS_ML_DIR")
	if dir == "" {
		dir = testDataDir(t)
	} else {
		testDataDir(t)
	}
	langs := []string{"en", "es", "fr", "ja", "zh"}
	m := openSemModel(t, filepath.Join(dir, "multilingual_minilm"), 384)

	emb := map[string]semEmbeds{}
	accs := map[string]float64{}
	var rows []string
	for _, l := range append(slices.Clone(langs), "ja2") {
		e := semEmbed(t, m, l)
		emb[l] = e
		sep := sepRatio(len(e.targets), func(i, j int) float64 { return chord(e.targets[i], e.targets[j]) })
		col := sepRatio(len(e.disk), func(i, j int) float64 {
			return math.Hypot(float64(e.disk[i][0]-e.disk[j][0]), float64(e.disk[i][1]-e.disk[j][1]))
		})
		acc := groupAcc(e.targets)
		rows = append(rows, fmt.Sprintf("%-22s sep %.2f  colsep %.2f  grp %.2f", "multilingual/"+l, sep, col, acc))
		accs[l] = acc
	}
	m.Close() // one ORT environment at a time

	// English-only baselines on the English paragraph.
	cfg, _ := os.UserConfigDir()
	for _, b := range []struct {
		name string
		dim  int
	}{{"mpnet", 768}, {"minilm", 384}} {
		path := filepath.Join(cfg, "natural-syntax-ls", b.name)
		if _, err := os.Stat(path + ".onnx"); err != nil {
			continue
		}
		bm := openSemModel(t, path, b.dim)
		e := semEmbed(t, bm, "en")
		sep := sepRatio(24, func(i, j int) float64 { return chord(e.targets[i], e.targets[j]) })
		col := sepRatio(24, func(i, j int) float64 {
			return math.Hypot(float64(e.disk[i][0]-e.disk[j][0]), float64(e.disk[i][1]-e.disk[j][1]))
		})
		rows = append(rows, fmt.Sprintf("%-22s sep %.2f  colsep %.2f  grp %.2f", "english/"+b.name+"/en", sep, col, groupAcc(e.targets)))
		bm.Close()
	}
	t.Log("\n" + strings.Join(rows, "\n"))
	for _, l := range []string{"en", "es", "fr", "zh", "ja", "ja2"} {
		if accs[l] < 0.7 {
			t.Errorf("%s: within-language group accuracy %.2f below 0.70", l, accs[l])
		}
	}

	// Cross-language matrix.
	var xg, xc float64
	var pairs int
	var lines []string
	lines = append(lines, "from->to    xgrp  xconc")
	for _, a := range langs {
		for _, b := range langs {
			if a == b {
				continue
			}
			grp, conc := 0, 0
			for i, q := range emb[a].targets {
				j := nearest(q, emb[b].targets)
				if j%4 == i%4 {
					grp++
				}
				if j == i {
					conc++
				}
			}
			g, c := float64(grp)/24, float64(conc)/24
			lines = append(lines, fmt.Sprintf("%s->%s       %.2f  %.2f", a, b, g, c))
			xg += g
			xc += c
			pairs++
		}
	}
	t.Log("\n" + strings.Join(lines, "\n"))
	xg, xc = xg/float64(pairs), xc/float64(pairs)
	t.Logf("mean cross-language: xgrp %.2f xconc %.2f", xg, xc)
	if xg < 0.6 {
		t.Errorf("cross-language group accuracy %.2f below 0.60", xg)
	}
}
