# Models

| Setup name (`--model`, from `models.json`) | `naturalSyntaxLs.model` | Size | Mode | Languages | Notes |
| ---------------------- | ----------------------- | ---- | ---- | --------- | ----- |
| `bert-base` (default) | `bert-base` | ~430 MB | POS | en | Higher accuracy (multilingual vocab inflates size) |
| `mobilebert` | `mobilebert` | ~105 MB | POS | en | Faster startup |
| `xlmr-es` | `xlmr-es` | ~280 MB | POS | es | XLM-R UPOS, int8; 96.7% on AnCora |
| `xlmr-fr` | `xlmr-fr` | ~280 MB | POS | fr | XLM-R UPOS, int8; 96.2% on GSD |
| `xlmr-ru` | `xlmr-ru` | ~280 MB | POS | ru | XLM-R UPOS (Apache-2.0), int8; 93.0% on GSD (fp32 93.5%) |
| `xlmr-ja` | `xlmr-ja` | ~280 MB | POS | ja | XLM-R UPOS, int8; 96.2% on GSD with kagome words (fp32 97.1%) |
| `xlmr-ar` | `xlmr-ar` | ~280 MB | POS | ar | XLM-R UPOS, int8; 90.5% of all PADT characters with clitic splitting (84.8% without) (Apache-2.0) |
| `xlmr-de` | `xlmr-de` | ~280 MB | POS | de | XLM-R UPOS, int8; 93.3% on GSD (Apache-2.0) |
| `xlmr-it` | `xlmr-it` | ~280 MB | POS | it | XLM-R UPOS, int8; 97.1% on ISDT (Apache-2.0) |
| `xlmr-pt` | `xlmr-pt` | ~280 MB | POS | pt | XLM-R UPOS, int8; 95.6% on GSD (Apache-2.0) |
| `chinese-roberta-upos` | `chinese-roberta-upos` | ~410 MB | POS | zh | `KoichiYasuoka/chinese-roberta-base-upos` (Apache-2.0), whole words; 91.6% of characters on GSDSimp, fp32 only |
| `multilingual-minilm` (default for semantic) | `multilingual-minilm` | ~470 MB | Semantic | 50+ | `paraphrase-multilingual-MiniLM-L12-v2`, 384-dim |
| `en_ewt.electra-base` (default for dependency) | `en_ewt.electra-base` | ~470 MB | Dependency | en | diaparser biaffine UD parser (ELECTRA-base encoder) |
| `es_ancora.mbert` | `es_ancora.mbert` | ~750 MB | Dependency | es | fp32 only; int8 drops UAS from 91 to 42 |
| `fr_sequoia.camembert-large` | `fr_sequoia.camembert-large` | ~350 MB | Dependency | fr | int8; UAS/LAS 90.5 / 86.0 |
| `ru_syntagrus.DeepPavlov` | `ru_syntagrus.DeepPavlov` | ~750 MB | Dependency | ru | fp32 only; UAS/LAS 90.8 / 80.4 on GSD (int8 81.8 / 67.8) |
| `de_hdt.dbmdz-bert-base` | `de_hdt.dbmdz-bert-base` | ~120 MB | Dependency | de | int8; UAS/LAS 82.9 / 75.9 on GSD (fp32 83.0 / 76.1); trained on HDT, so the lower score partly reflects the treebank change |
| `it_isdt.dbmdz-electra-xxl` | `it_isdt.dbmdz-electra-xxl` | ~120 MB | Dependency | it | int8; UAS/LAS 95.6 / 93.9 on ISDT (fp32 95.8 / 94.0) |
| `zh_ud.mbert` | `zh_ud.mbert` | ~470 MB | Dependency | zh | Character-level diaparser trained here (CC BY-SA 4.0, [Hydragon123/zh-ud-diaparser](https://huggingface.co/Hydragon123/zh-ud-diaparser)); fp32 only; UAS/LAS 86.9 / 83.3 on the full GSD test set (500 sentences) |
| `ja_gsd.mbert` | `ja_gsd.mbert` | ~120 MB | Dependency | ja | int8; UAS/LAS 89.5 / 85.5 on the full GSD test set (543 sentences) with kagome words. Needs `fugashi` and `unidic-lite` at export time |

The former `all-mpnet-base-v2` and `all-MiniLM-L6-v2` semantic models were replaced by the multilingual one: about half the memory of mpnet and 2-3x faster per edit.

One setting, `naturalSyntaxLs.model`, picks the model for whichever mode is active. Its meaning depends on `naturalSyntaxLs.mode`: a POS model name for `pos`, an embedding model name for `semantic`, or a diaparser catalog name for `dependency`. Model files on disk (`models/{slug}/{slug}.onnx` and `{slug}_vocab.txt` or `{slug}_tokenizer.json`, or `models/{slug}_dependency/{slug}_dependency.onnx` and its tokenizer file) are derived from this value using the same slug rule as `export_model.py` (`-`, `.`, `/` → `_`), so it must match what you exported. `export_model.py --int8` quantizes in place.

Export commands: [install.md](install.md#exporting-models).

## Licenses and tag inventories

Checked against the Hugging Face model cards and the UD treebank READMEs. The server itself is under the repository `LICENSE`; the exported weights keep the license of their source.

| Model | Source license | Tag inventory |
| ----- | -------------- | ------------- |
| `bert-base` | CC BY-NC 3.0 (**non-commercial**) | Penn Treebank, 46 labels including punctuation |
| `mobilebert` | MIT | Penn Treebank, 34 labels |
| `xlmr-es`, `xlmr-fr`, `xlmr-ru`, `xlmr-ja`, `xlmr-ar`, `xlmr-de`, `xlmr-it`, `xlmr-pt` | Apache-2.0 | Universal POS, 17 tags |
| `chinese-roberta-upos` | Apache-2.0 | Universal POS with `B-`/`I-` word pieces, 45 labels |
| `multilingual-minilm` | Apache-2.0 | none (embeddings) |
| diaparser parsers (`en_ewt`, `es_ancora`, `fr_sequoia`, `ru_syntagrus`, `ja_gsd`, `de_hdt`, `it_isdt`) | diaparser is MIT; weights follow the training treebank: EWT CC BY-SA 4.0, AnCora CC BY 4.0, Sequoia LGPL-LR, SynTagRus **CC BY-NC-SA 4.0**, Japanese-GSD CC BY-SA 4.0, HDT CC BY-SA 4.0, ISDT **CC BY-NC-SA 3.0** | UD relations, see [tags.md](tags.md) |
| `zh_ud.mbert` | CC BY-SA 4.0 (trained on Chinese-GSD, CC BY-SA 4.0) | UD relations |

Other tag sets: label files may also use EAGLES (`NCMS000`) or CKIP (`Na`, `VC`) names, which decode to the Universal classes (`internal/postag/tagsets.go`). The CKIP transformers models are GPL-3.0 and are not in the catalog. A label that fits no inventory decodes as `O` and is logged when the model loads.

## Cost on a ThinkPad X280

Measured through Neovim (`internal/e2e`, `NSLS_NVIM_BENCH=1`): time to first result including model load, median one-character edit in a 32-line document, and server RSS.

| Model | Ready | Edit | RSS |
| ----- | ----- | ---- | --- |
| en `bert-base` POS | 0.45 s | 93 ms | 397 MB |
| es / fr POS (int8) | 1.1 s | 62 ms | 450 MB |
| ru POS (int8) | 1.0 s | 60 ms | 457 MB |
| ja POS (int8, kagome words) | 3.3 s | 133 ms | 1037 MB |
| zh POS (`chinese-roberta-upos`, fp32) | 0.46 s | 120 ms | 404 MB |
| en `mpnet` semantic (removed) | 0.42 s | 90 ms | 385 MB |
| multilingual MiniLM semantic | 0.8 s | 26-43 ms | 203 MB |
| en electra dependency | 0.8 s | 130 ms | 684 MB |
| es mBERT dependency (fp32) | 1.1 s | 130 ms | 919 MB |
| fr CamemBERT dependency (int8) | 1.2 s | 244 ms | 492 MB |
| ja mBERT dependency (int8, kagome words) | 2.7 s | 92 ms | 779 MB |
| zh character-level dependency (fp32) | 0.9 s | 161 ms | 675 MB |

Start-up is slower than English because of the 250k-piece vocabulary and tokenizer load. The French parser is about twice as slow per edit as the English one.

## Semantic quality across languages

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
- Pieces are still the unit that gets a color: a katakana word split into several pieces shows several colors, though they now come from the right context.

## Other languages

The models in the table above have setup names. Anything else (other treebanks, other UD languages) can be exported by hand: use an explicit `--mode` and a Hugging Face repo id; the model name on disk is the repo id with `-`, `.`, `/` → `_`. SentencePiece models ship `{slug}_tokenizer.json` instead of `{slug}_vocab.txt`. Run the server with `-model <path>.onnx`; the tokenizer file is found next to it.

| Language | Mode | Export |
| -------- | ---- | ------ |
| es, fr, ru, ja, zh | POS (UPOS mapped onto the Penn token types; all but zh are catalog entries `xlmr-<code>`) | `python scripts/export_model.py DIR --mode pos --model wietsedv/xlm-roberta-base-ft-udpos28-es` (also `-fr`, `-ru`, `-ja`, `-zh`) |
| zh | POS, whole words (character-level B-/I- tagger) | `chinese-roberta-upos` in the catalog (Apache-2.0; 91.6% of characters on GSDSimp vs 79.2% for the XLM-R zh model) |
| all | Semantic | `--mode semantic --model sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2` |
| es, fr, ru, ja | Dependency | `--mode dependency --model es_ancora.mbert`, `fr_sequoia.camembert-large`, `ru_syntagrus.DeepPavlov` or `ja_gsd.mbert` (needs `diaparser`) |
| zh | Dependency, character level | `--mode dependency --model zh_ud.mbert` (downloads the checkpoint from Hugging Face; retrain with `scripts/train_zh_dependency.py`, see below) |

Japanese text that contains kana is split into UniDic short-unit words by [kagome](https://github.com/ikawaha/kagome) (the word standard of UD Japanese-GSD), so the Japanese POS model tags real words: 97.1% of characters on GSD, up from 74.4% with tokenizer pieces. kagome's dictionary is embedded in the binary (+37 MB) and decoded on the first Japanese text: start-up goes from 1.1 s to 3.2 s and RSS from about 450 MB to about 1 GB, and edits are 40-50% slower (int8: 137 vs 94 ms). int8 Japanese POS keeps 96.2% (fp32 97.1%). Build with `-tags nojaseg` to leave it out; Japanese then falls back to tokenizer pieces. Kanji-only text cannot be told from Chinese and also uses pieces.

`KoichiYasuoka/deberta-base-japanese-wikipedia-luw-upos` (CC BY-SA 4.0) was tested as a segmenter-free alternative and not added. It tags long-unit words with `B-`/`I-` labels over Unigram pieces, which the server already joins, and exports cleanly (ONNX matches PyTorch to 6e-6, 499 MB fp32). On the first 300 test sentences, joining `I-` pieces: 95.8% of characters and word F1 93.4 on UD Japanese-GSDLUW (int8 94.9 / 92.5), but 85.0% and F1 71.8 against GSD's short units, which the shipped kagome path (97.1%) and the `ja_gsd` parser use. Its tokenizer also emits an `[UNK]` piece at the start of the text and before `。`.

The XLM-R Chinese model tags the multi-character pieces of its SentencePiece vocabulary, which can fuse a particle or preposition with its noun (79.2% of characters on GSDSimp). Prefer `chinese-roberta-upos` (`KoichiYasuoka/chinese-roberta-base-upos`): it tags each character as `B-`, `I-` or a plain UPOS label, and the server joins `I-` characters to the word before them, so tokens are whole words (`北京`, `大学`, `自然`, `语言`) and 91.6% of characters get the right tag. The vocab file is found next to the model. int8 (`scripts/quantize_model.py`) cuts it from 438 MB to 104 MB and RSS from 397 to 218 MB, but accuracy drops to 86.5%, so keep fp32 unless memory is tight. Neovim edit latency: 108 ms fp32, 81 ms int8. Traditional Chinese scored the same in the Python test (91.1% word level).

### Arabic clitics

UD Arabic-PADT, which the Arabic model was trained on, splits a written word such as `وبالكتاب` into syntactic words (`و`, `ب`, `الكتاب`). Without a split the model tags the whole written word once: 16% of PADT test words carry a clitic, and only 39.8% of their characters got their own word's tag. `tokenizer.arabic_clitics` in `data/lang/arabic.lua` splits off a conjunction (`و ف`), a preposition (`ب ل ك`, with `لل` read as `ل` plus `ال`), the future `س` and a pronoun suffix (`ه ها هم ...`). `ExampleParseUnigram` in `internal/tokenizer` shows the split on a tiny vocabulary. It needs no extra files: a split is taken when the rest is a likelier whole word in the model's own SentencePiece vocabulary than the written word, or when the written word is in the vocabulary under no common inflection, plus short lists of function words (`فيها`, `وهو`). Marks stay with their letter.

Measured on the PADT sentences whose words align with the text (int8 model, per character, clitic parts scored by their own tag):

| Set | Clitic boundaries P / R | All characters | Clitic-word characters | Clitic-free words |
| --- | ----------------------- | -------------- | ---------------------- | ----------------- |
| test, 312 sentences, rules tuned here | 89.8 / 90.3 | 84.8 to 90.5% | 39.9 to 82.2% | 93.4 to 92.2% |
| dev, held out | 82.6 / 86.7 | 84.8 to 89.2% | 45.2 to 79.4% | |

The cost is false splits of rare words and names that start with `و ف ب ل ك` (`وقائية`, `بريطانيا`), which lowers clitic-free words by about a point. Hover looks up the host word, so live Wiktionary hits on the pipeline's words rose from 62% to 72%. Set `arabic_clitics = false` in a copied script to keep written words whole.

### Japanese and Chinese dependency parsers

Japanese uses the released diaparser `ja_gsd.mbert` (Tohoku BERT, WordPiece). Its config names a mecab word tokenizer, so the WordPiece tokenizer leaves Han/kana runs whole and cuts them with the kagome segmenter, the same words UD Japanese-GSD uses. Without kagome (`-tags nojaseg`) each character becomes a word and accuracy collapses. On the first 300 GSD test sentences: UAS/LAS 89.3 / 84.9 (int8 89.2 / 84.9), 88.4 / 84.1 when words must also match the gold spans.

diaparser publishes no Chinese parser, so `scripts/train_zh_dependency.py` trains one on UD Chinese-GSD (Traditional) and GSDSimp (CC BY-SA 4.0) with `hfl/chinese-roberta-wwm-ext` as encoder (about 20 minutes on a Colab T4). The parser works on characters and finds word boundaries itself: the tokenizer's units (one Han character, one punctuation mark, one letter/digit run) are the parser tokens, and a multi-unit word links each unit but the last to the last with the relation `wp`. The server merges `wp` chains into whole-word tokens, and the last unit supplies the word's head and relation, so no separate segmenter is needed and labels are native UD (no mapping). Training drops about 5% of sentences whose units straddle two gold words. On the first 300 test sentences, counting a word only when its span matches the gold word: UAS/LAS 85.6 / 82.3 on GSD and 85.5 / 82.2 on GSDSimp. int8 drops that to about 80 / 76, so keep fp32 (~470 MB). Setup exports it from the Hugging Face repo, which also holds the training script.

## Definition lookup coverage

`NSLS_WIKT_EVAL=1 go test ./internal/wiktionary -run TestLiveAccuracy -v` looks up 100 unique content words per language from the UD test treebank (POS mode) and counts the words with a Wiktionary definition (hit) and, of those, how many matched the model's POS section.

| Language | Hit | POS section match | Misses are mostly |
| -------- | --- | ----------------- | ----------------- |
| it | 93% | 90% | proper nouns |
| pt | 87% | 92% | proper nouns, rare forms |
| ru | 86% | 98% | proper nouns (was 73% before adjective lemma forms) |
| de | 85% | 92% | compounds, inflected verbs |
| ar | 84% | 92% | inflected forms, names (72% of the tokenizer's own words, 62% before clitic splitting) |
| zh | 84% | 91% | rare words, names |


## Startup and latency per language

Through headless Neovim on the ThinkPad X280 (i7-8650U), Wiktionary definitions on, one fresh server per row: `NSLS_NVIM_BENCH=1 NSLS_BENCH_OUT=table.md go test ./internal/e2e -run TestNvimBench` (`NSLS_BENCH_WIKT=0` turns definitions off). Ready is server start to first tokens. Japanese is slowest (3 s) because the kagome dictionary loads with it. Hover is 20 requests spread over the long document; the first lookup of a word goes to Wiktionary (the max column, 75-1200 ms on home broadband), repeats come from the server's cache (cached column). Definitions counts hovers that showed one, out of hovers that landed on a token. ru dependency was not exported locally.

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

### Other platforms

The `e2e` CI job runs the English POS row on each GitHub Actions runner (`.github/workflows/ci.yml`, job summary). macOS keeps no peak RSS counter, so its peak is the largest sample taken between steps.

| Runner | Ready (ms) | Open long (ms) | Edit median (ms) | Hover max / cached (ms) | Definitions | RSS ready (MB) | Peak RSS (MB) |
| ------ | ---------- | -------------- | ---------------- | ----------------------- | ----------- | -------------- | ------------- |
| X280, Linux | 458 | 744 | 97 | 75 / 0.6 | 16/16 | 405 | 423 |
| ubuntu-latest | 474 | 581 | 75 | 416 / 0.11 | 16/16 | 445 | 474 |
| macos-latest | 595 | 1192 | 163 | 376 / 0.12 | 16/16 | 404 | 424 |
| windows-latest | 1152 | 1094 | 143 | 525 / 0.22 | 16/16 | 396 | 420 |
