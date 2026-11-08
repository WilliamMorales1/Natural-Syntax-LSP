package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Latency and memory benchmark through a real Neovim LSP client. Off by default: set NSLS_NVIM_BENCH=1.
//
//	NSLS_ML_DIR         dir with the multilingual models from scripts/setup.sh (default: the config dir; cases whose files are missing are skipped)
//	NSLS_BENCH_ONLY     substring filter on case names, e.g. "pos/es"
//	NSLS_BENCH_SERVER   prebuilt server binary (default: built from this checkout)
//	NSLS_BENCH_OUT      also write the result table here as markdown
//	NSLS_BENCH_TIMEOUT  per-request timeout (default 120s)
//
// English baselines come from the default model dir (~/.config/natural-syntax-ls). Each case runs in a fresh Neovim and server, so peak RSS is per model.
// Wrap the run in oomguard: the French parser alone is 1.4 GB on disk.

type benchCase struct {
	name, mode, lang, model, dir string
}

var paragraphs = map[string][]string{
	"es": {
		"El niño comió manzanas rojas en la cocina mientras su madre preparaba la cena.",
		"Después de la comida, la familia salió a pasear por el parque y vio muchos pájaros en los árboles.",
		"María vive en Madrid desde hace cinco años y trabaja en una pequeña editorial del centro.",
		"Cuando llegó el invierno, las calles se llenaron de luces y todos esperaban la Navidad con alegría.",
	},
	"fr": {
		"Le chat noir mange la souris dans le jardin pendant que les enfants jouent près de la maison.",
		"L'ami d'aujourd'hui qu'il a vu hier est très gentil et il parle souvent de ses voyages.",
		"Marie habite à Paris depuis cinq ans et travaille dans une petite maison d'édition du centre-ville.",
		"Quand l'hiver est arrivé, les rues se sont remplies de lumières et tout le monde attendait Noël.",
	},
	"ja": {
		"私は東京大学で自然言語処理を勉強しています。毎日図書館で長い時間を過ごします。",
		"今日は晴れですが、明日は雨かもしれません。傘を持って出かけたほうがいいでしょう。",
		"彼女は五年前から京都に住んでいて、小さな出版社で働いています。",
		"冬が来ると、街は明かりでいっぱいになり、みんなが新年を楽しみに待っていました。",
	},
	"zh": {
		"我喜欢在北京大学学习自然语言处理，每天都在图书馆里度过很长时间。",
		"今天天气很好，但是明天可能会下雨，所以出门最好带一把伞。",
		"她五年前搬到上海，现在在一家小出版社工作，同事们都很友好。",
		"冬天来了以后，街道上到处都是灯光，大家都高兴地等待着新年。",
	},
	"ru": {
		"Мальчик ел красные яблоки на кухне, пока его мать готовила ужин.",
		"После обеда вся семья пошла гулять в парк и увидела много птиц на деревьях.",
		"Мария живёт в Москве уже пять лет и работает в маленьком издательстве в центре города.",
		"Когда наступила зима, улицы наполнились огнями, и все с радостью ждали Новый год.",
	},
	"en": {
		"The North Wind and the Sun were disputing which was the stronger, when a traveler came along wrapped in a warm cloak.",
		"They agreed that the one who first succeeded in making the traveler take his cloak off should be considered stronger than the other.",
		"Then the North Wind blew as hard as he could, but the more he blew the more closely did the traveler fold his cloak around him.",
		"And so the North Wind was obliged to confess that the Sun was the stronger of the two.",
	},
}

// docs builds a one-paragraph document and an eight-paragraph one, as lines.
func docs(lang string) (short, long []string) {
	p := paragraphs[lang]
	// Numbered so no paragraph repeats: the server caches analysed chunks by text.
	for i := range 8 {
		for _, l := range p {
			long = append(long, fmt.Sprintf("(%d) %s", i+1, l))
		}
		long = append(long, "")
	}
	return p, long
}

type benchResult struct {
	Error       string  `json:"error"`
	InitMS      float64 `json:"init_ms"`
	ReadyMS     float64 `json:"ready_ms"`
	OpenLongMS  float64 `json:"open_long_ms"`
	TokensShort float64 `json:"tokens_short"`
	TokensLong  float64 `json:"tokens_long"`
	Edit        spread  `json:"edit_ms"`
	Hover       spread  `json:"hover_ms"`
	HoverHits   int     `json:"hover_hits"`
	RSSReadyMB  float64 `json:"rss_ready_mb"`
	RSSEndMB    float64 `json:"rss_end_mb"`
	PeakMB      float64 `json:"peak_mb"`
}

type spread struct{ Min, Median, Max float64 }

func benchCases(t *testing.T) []benchCase {
	var cases []benchCase
	cfg, _ := os.UserConfigDir()
	en := filepath.Join(cfg, "natural-syntax-ls")
	cases = append(
		cases,
		benchCase{"pos/en/bert_base", "pos", "en", "bert_base", en},
		benchCase{"semantic/en/mpnet", "semantic", "en", "mpnet", en},
		benchCase{"dependency/en/electra", "dependency", "en", "en_ewt_electra_base_dependency", en},
	)
	ml := os.Getenv("NSLS_ML_DIR")
	if ml == "" {
		ml = en
	}
	for _, l := range []string{"es", "fr", "ru"} {
		cases = append(cases, benchCase{"pos/" + l, "pos", l, "xlmr_" + l, ml})
	}
	cases = append(
		cases,
		benchCase{"pos/ja", "pos", "ja", "xlmr_ja", ml},
		benchCase{"pos/zh", "pos", "zh", "chinese_roberta_upos", ml},
	)
	for _, l := range []string{"es", "fr", "ja", "zh", "ru"} {
		cases = append(cases, benchCase{"semantic/" + l, "semantic", l, "multilingual_minilm", ml})
	}
	cases = append(
		cases,
		benchCase{"dependency/es", "dependency", "es", "es_ancora_mbert_dependency", ml},
		benchCase{"dependency/fr", "dependency", "fr", "fr_sequoia_camembert_large_dependency", ml},
		benchCase{"dependency/ru", "dependency", "ru", "ru_syntagrus_DeepPavlov_dependency", ml},
	)
	return cases
}

func TestNvimBench(t *testing.T) {
	if os.Getenv("NSLS_NVIM_BENCH") == "" {
		t.Skip("set NSLS_NVIM_BENCH=1")
	}
	nvim, err := exec.LookPath("nvim")
	if err != nil {
		t.Skip("nvim not installed")
	}
	timeout := 120 * time.Second
	if s := os.Getenv("NSLS_BENCH_TIMEOUT"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			timeout = d
		}
	}
	work := t.TempDir()
	server := os.Getenv("NSLS_BENCH_SERVER")
	if server == "" {
		server = filepath.Join(work, "natural-syntax-ls")
		if out, err := exec.Command("go", "build", "-o", server, "../../cmd/natural-syntax-ls").CombinedOutput(); err != nil {
			t.Fatalf("build: %v\n%s", err, out)
		}
	}
	driver, err := filepath.Abs("bench.lua")
	if err != nil {
		t.Fatal(err)
	}

	type row struct {
		c benchCase
		r benchResult
	}
	var rows []row
	only := os.Getenv("NSLS_BENCH_ONLY")
	for _, c := range benchCases(t) {
		if only != "" && !strings.Contains(c.name, only) {
			continue
		}
		if c.dir == "" {
			continue
		}
		model := filepath.Join(c.dir, "models", c.model, c.model+".onnx")
		if _, err := os.Stat(model); err != nil {
			t.Logf("skip %s: %v", c.name, err)
			continue
		}
		short, long := docs(c.lang)
		out := filepath.Join(work, strings.ReplaceAll(c.name, "/", "_")+".json")
		cfgPath := out + ".cfg"
		raw, _ := json.Marshal(map[string]any{
			"cmd":        []string{server, "-mode", c.mode, "-model", model},
			"mode":       c.mode,
			"short":      short,
			"long":       long,
			"edits":      10,
			"hovers":     20,
			"timeout_ms": timeout.Milliseconds(),
			"out":        out,
		})
		if err := os.WriteFile(cfgPath, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*timeout)
		cmd := exec.CommandContext(ctx, nvim, "--clean", "--headless", "-l", driver, cfgPath)
		cmd.Dir = work
		output, runErr := cmd.CombinedOutput()
		cancel()
		var r benchResult
		data, readErr := os.ReadFile(out)
		if readErr != nil {
			t.Errorf("%s: no result (%v): %s", c.name, runErr, output)
			continue
		}
		if err := json.Unmarshal(data, &r); err != nil {
			t.Errorf("%s: bad result %q: %v", c.name, data, err)
			continue
		}
		if r.Error != "" {
			t.Errorf("%s: %s", c.name, r.Error)
			continue
		}
		rows = append(rows, row{c, r})
		t.Logf("%-22s ready %6.0f ms  open-long %6.0f ms  edit %5.0f ms  hover %4.2f ms (%d/20 hits)  rss %5.0f MB  peak %5.0f MB",
			c.name, r.ReadyMS, r.OpenLongMS, r.Edit.Median, r.Hover.Median, r.HoverHits, r.RSSReadyMB, r.PeakMB)
	}

	if path := os.Getenv("NSLS_BENCH_OUT"); path != "" && len(rows) > 0 {
		var b strings.Builder
		b.WriteString("| Case | Ready (ms) | Open long (ms) | Edit median / max (ms) | Hover median (ms) | RSS ready (MB) | Peak RSS (MB) |\n")
		b.WriteString("| ---- | ---------- | -------------- | ---------------------- | ----------------- | -------------- | ------------- |\n")
		for _, x := range rows {
			fmt.Fprintf(&b, "| %s | %.0f | %.0f | %.0f / %.0f | %.0f | %.0f | %.0f |\n",
				x.c.name, x.r.ReadyMS, x.r.OpenLongMS, x.r.Edit.Median, x.r.Edit.Max, x.r.Hover.Median, x.r.RSSReadyMB, x.r.PeakMB)
		}
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			t.Error(err)
		}
	}
}
