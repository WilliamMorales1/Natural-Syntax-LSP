# Benchmarks

How to rerun each table: [contributing/testing.md](contributing/testing.md). Per-model accuracy is in the [model catalog](models.md#catalog).

## Semantic quality

`NSLS_SEMANTIC_EVAL=1 NSLS_ML_DIR=... go test ./internal/inference -run TestSemanticQualityAcrossLanguages -v` (`semantic_eval_test.go`). Each language has a paragraph in the style of [semantic-example.txt](examples/semantic-example.txt): 24 target words in four groups (animals, food, technology, emotions). `sep` is mean between-group over mean within-group embedding distance, `colsep` the same on the colors after the per-document plane fit, `grp` leave-one-out nearest-centroid group accuracy (chance 0.25).

| Model / language | sep | colsep | grp |
| ---------------- | --- | ------ | --- |
| multilingual en | 1.30 | 3.89 | 1.00 |
| multilingual es | 1.34 | 3.99 | 1.00 |
| multilingual fr | 1.31 | 2.59 | 1.00 |
| multilingual zh | 1.36 | 3.54 | 1.00 |
| multilingual ja (katakana words) | 1.35 | 2.88 | 1.00 |
| multilingual ja (kanji words) | 1.26 | 2.68 | 1.00 |
| mpnet en (removed) | 1.29 | 3.95 | 1.00 |
| English MiniLM en (removed) | 1.20 | 4.63 | 1.00 |

Across languages, the nearest word in another language has the right group 99% of the time and is the actual translation 88% of the time (chance 4%).

- English quality matches mpnet on this test; colors are slightly less separated than with either removed English model.
- es, fr, zh and ja all match English on group accuracy.
- Semantic mode encodes words that touch in the source (Japanese, Chinese, punctuation) without a word-start mark, as the encoder saw them in training. Before this, every tokenizer piece got its own mark, so katakana `キツネ` reached the model as `▁キ ▁ツ ▁ネ` (ja 0.17, zh 0.88).
- Japanese words come from kagome, so a katakana word gets one color (`キツネ`, `コンピューター`). UniDic short units still split compounds such as `スマートフォン` into `スマート` and `フォン`, matching UD Japanese-GSD.

## Dictionary coverage

`NSLS_WIKT_EVAL=1 go test ./internal/wiktionary -run TestLiveAccuracy -v` looks up 100 unique content words per language from the UD test treebank (POS mode) and counts the words with a Wiktionary definition (hit) and, of those, how many matched the model's POS section.

| Language | Hit | POS section match | Misses are mostly |
| -------- | --- | ----------------- | ----------------- |
| it | 93% | 90% | proper nouns |
| pt | 87% | 92% | proper nouns, rare forms |
| ru | 87% | 98% | proper nouns, participles (was 73% before adjective lemma forms; noun and verb endings in `russian.lua` take the tokenizer's own words from 79% to 82%) |
| de | 85% | 92% | compounds, inflected verbs |
| ar | 90% | 93% | broken plurals, names (86% of the tokenizer's own words, 62% before clitic splitting) |
| zh | 84% | 91% | rare words, names |

## Language detection

Held-out Tatoeba accuracy (2026-11-08 dumps, every 20th sentence):

| Text length | ja | zh | ar | ko | de | ru | uk | it | en | pt | fr | es | ca |
| ----------- | -- | -- | -- | -- | -- | -- | -- | -- | -- | -- | -- | -- | -- |
| 6-12 chars | 99.9 | 99.8 | 100 | 99.6 | 95.9 | 95.0 | 94.3 | 93.8 | 93.0 | 88.6 | 88.2 | 86.9 | 54.5 |
| 21+ chars | 100 | 99.7 | 99.9 | 100 | 100 | 100 | 99.9 | 99.9 | 99.8 | 99.9 | 100 | 99.8 | 96.3 |

Short Latin-script lines are the weak spot: Spanish and Portuguese, French and Italian confuse each other, and Catalan has only about 11k training sentences. Texts of 1-5 characters are right 0-67% of the time or abstain, which is why hover borrows a nearby line's language.

## Latency and memory

Through headless Neovim on the ThinkPad X280 (i7-8650U), Wiktionary definitions on, one fresh server per row: `NSLS_NVIM_BENCH=1 NSLS_BENCH_OUT=table.md go test ./internal/e2e -run TestNvimBench` (`NSLS_BENCH_WIKT=0` turns definitions off). Ready is server start to first tokens. Japanese is slowest (3 s) because the kagome dictionary loads with it. Hover is 20 requests spread over the long document; the first lookup of a word goes to Wiktionary (the max column, 75-2500 ms on home broadband), repeats come from the server's cache (cached column). Definitions counts hovers that showed one, out of hovers that landed on a token. The dependency/ru and dependency/pt rows were added later (2026-11-08); dependency/zh is the fp32 parser.

| Case | Ready (ms) | Open long (ms) | Edit median / max (ms) | Hover median / max (ms) | Hover cached (ms) | Definitions | RSS ready (MB) | Peak RSS (MB) |
| ---- | ---------- | -------------- | ---------------------- | ----------------------- | ----------------- | ----------- | -------------- | ------------- |
| pos/en/bert_base | 458 | 744 | 97 / 100 | 1 / 75 | 0.6 | 16/16 | 405 | 423 |
| semantic/en/mpnet | 864 | 77 | 79 / 101 | 4 / 356 | 0.7 | 16/16 | 395 | 415 |
| dependency/en/electra | 875 | 998 | 132 / 171 | 14 / 106 | 0.7 | 16/16 | 663 | 668 |
| pos/es | 1020 | 475 | 65 / 77 | 16 / 313 | 0.4 | 14/15 | 457 | 566 |
| pos/fr | 1031 | 541 | 64 / 68 | 15 / 143 | 0.2 | 18/18 | 457 | 564 |
| pos/ru | 1022 | 474 | 61 / 67 | 15 / 152 | 0.6 | 16/16 | 457 | 564 |
| pos/ja | 3425 | 1033 | 132 / 144 | 15 / 138 | 0.6 | 19/20 | 1013 | 1037 |
| pos/zh | 464 | 853 | 110 / 144 | 15 / 428 | 0.5 | 13/15 | 399 | 418 |
| semantic/es | 964 | 31 | 28 / 36 | 16 / 193 | 0.5 | 14/15 | 214 | 224 |
| semantic/fr | 926 | 49 | 31 / 46 | 15 / 84 | 0.7 | 18/18 | 207 | 225 |
| semantic/ja | 3077 | 38 | 33 / 41 | 15 / 143 | 0.6 | 19/20 | 794 | 804 |
| semantic/zh | 875 | 27 | 22 / 29 | 205 / 1162 | 0.6 | 14/19 | 207 | 219 |
| semantic/ru | 935 | 28 | 24 / 31 | 15 / 331 | 0.6 | 16/16 | 211 | 221 |
| dependency/es | 1211 | 967 | 119 / 125 | 15 / 119 | 0.6 | 14/15 | 934 | 1124 |
| dependency/fr | 1181 | 1959 | 264 / 292 | 15 / 358 | 0.7 | 18/18 | 504 | 517 |
| dependency/ja | 2842 | 746 | 98 / 104 | 15 / 156 | 0.6 | 19/20 | 783 | 786 |
| dependency/zh | 1005 | 1284 | 168 / 198 | 67 / 392 | 0.6 | 12/15 | 640 | 646 |
| dependency/ru | 1098 | 1224 | 157 / 165 | 14 / 220 | 0.1 | 16/16 | 912 | 1123 |
| dependency/pt | 499 | 814 | 109 / 115 | 143 / 2536 | 0.1 | 14/15 | 207 | 211 |

### Other platforms

The `e2e` CI job runs the English POS row on each GitHub Actions runner (`.github/workflows/ci.yml`, job summary). macOS keeps no peak RSS counter, so its peak is the largest sample taken between steps.

| Runner | Ready (ms) | Open long (ms) | Edit median (ms) | Hover max / cached (ms) | Definitions | RSS ready (MB) | Peak RSS (MB) |
| ------ | ---------- | -------------- | ---------------- | ----------------------- | ----------- | -------------- | ------------- |
| X280, Linux | 458 | 744 | 97 | 75 / 0.6 | 16/16 | 405 | 423 |
| ubuntu-latest | 474 | 581 | 75 | 416 / 0.11 | 16/16 | 445 | 474 |
| macos-latest | 595 | 1192 | 163 | 376 / 0.12 | 16/16 | 404 | 424 |
| windows-latest | 1152 | 1094 | 143 | 525 / 0.22 | 16/16 | 396 | 420 |
