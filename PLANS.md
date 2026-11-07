# Multilingual support plan

Goal: add useful Spanish, French, Japanese, and Chinese support without changing default English behavior. Chinese means Mandarin/Standard Chinese initially; document dialect and script coverage explicitly.

## Decisions

- Tokenization follows the model's own tokenizer (vocab plus pretokenization rules from its `tokenizer_config.json`). No separate dictionary or morphology segmenter.
- Japanese and Chinese words are the model's own Unigram pieces (multi-character), never per character, for models with a SentencePiece tokenizer. Per-character stays only for char-vocab WordPiece models (CKIP, Yasuoka bert-base-japanese-upos).
- Wiktionary lookup uses the tokenizer's span text as given.
- WordPiece (BERT) and SentencePiece Unigram (XLM-R, CamemBERT) tokenizers are supported in Go. SentencePiece BPE (mmBERT, Llama-style) is not.
- `naturalSyntaxLs.model` stays the only selector; language is derived from model metadata.

## Done

- Dependency sentence splitting ends on `。 ！ ？ ｡ ‼ ⁇ ⁈ ⁉`; CJK closing brackets (`」』）】〕｣`) stay with their sentence. Tests in `internal/inference/predictor_test.go`.
- `internal/tokenizer`: optional `<slug>_tokenizer.json` sidecar (copy of HF `tokenizer_config.json`). With it, each Han/kana character is its own word (default on when `tokenize_chinese_chars` is absent, as in HF; also on for `subword_tokenizer_type: character`) and apostrophes split like HF. Without it, behavior is unchanged.
- `BERTTokenizer.Tokenize`, exposed via `inference.TextTokenizer`; `lspserver/registry.go` uses the model's tokenizer when available.
- Tests: sidecar CJK splitting and offsets, HF apostrophe splitting, multilingual sentence splits.

- `internal/tokenizer/unigram.go`: pure-Go Unigram tokenizer from a HF `tokenizer.json` (Viterbi, NFKC per cluster with offset map, Metaspace, fused unknowns, `<s>`/`</s>` from the post-processor). Ids match HF `tokenizers` on a 38-word es/fr/ja/zh/emoji/kana fixture for both XLM-R and Yasuoka vocabs (`NSLS_UNIGRAM_REF`).
- `tokenizer.Tokenizer` interface; `inference` models use it for BERT and Unigram alike. `tokenizer.Open` picks Unigram when `<slug>_tokenizer.json` is a full tokenizer.json.
- Unigram word splitting: whitespace and punctuation as in HF, French elision (`l'`, `qu'`, `aujourd'hui` kept whole), and every Han/kana run segmented into the pieces Viterbi picks.
- `postag.ParseLabel` maps Penn and Universal POS (with B-/I- and `+` compounds) onto existing token types. `NewPOSModel` no longer falls back to the English label order when a sidecar-tokenizer model lacks `_labels.json`; it errors.
- `scripts/export_model.py`: `--mode pos|semantic|dependency` with any HF repo id; writes `_tokenizer.json` (Unigram tokenizer.json, or WordPiece tokenizer_config.json beside `_vocab.txt`), also for dependency exports.
- `cmd`: vocab lookup falls back to `<model>_tokenizer.json`; any model name containing `minilm` gets 384 dims.
- Tests in `internal/inference/multilingual_test.go` (run with `NSLS_ML_DIR`).

## Tested models (exported, run through Go)

| Mode | Lang | Model | Result |
| ---- | ---- | ----- | ------ |
| POS | es, fr, ja, zh | `wietsedv/xlm-roberta-base-ft-udpos28-{es,fr,ja,zh}` (Apache-2.0, UPOS, 1.1 GB fp32 / 280 MB int8) | es, fr good. ja, zh tag Unigram pieces: usable, noisy where a piece fuses a particle or preposition (`私は` tagged ADV, `在北京` as one proper noun). int8 changes 1 of ~30 tags. |
| Semantic | all | `sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2`, 384-dim | cat/car word embeddings: same concept 0.92-0.99 across en/es/fr/ja/zh, different concept 0.17-0.25 |
| Dependency | es | diaparser `es_ancora.mbert` | correct heads and relations on test sentences |
| Dependency | fr | diaparser `fr_sequoia.camembert-large` (catalog name; 1.4 GB) | correct on test sentences, elision handled |
| Dependency | ja | diaparser `ja` default is `cl-tohoku-bert` and needs MeCab (`fugashi`, `ipadic`) | not usable without a dictionary segmenter |
| Dependency | zh | diaparser `zh_ctb7.electra-base` | not tested: Stanford labels, needs mapping |

## Accuracy (UD test treebanks, first 200 sentences each)

Run: `NSLS_EVAL=1 NSLS_ML_DIR=... NSLS_ML_QDIR=... go test ./internal/inference -run TestEval -v` (`eval_test.go`; downloads treebanks on first run; `scripts/quantize_model.py` makes the int8 copies). POS is per source character on Penn-mapped tags.

| Task | Lang | Treebank | fp32 | int8 |
| ---- | ---- | -------- | ---- | ---- |
| POS | es | AnCora | 96.8% | 96.7% |
| POS | fr | GSD | 96.4% | 96.2% |
| POS | ja | GSD | 97.1% (kagome words; 74.4% with pieces) | 96.2% |
| POS | zh | GSD | 79.2% | 78.6% |
| POS | zh (Yasuoka char B-/I-) | GSDSimp | 91.6% | 86.5% |
| Dependency UAS/LAS | es | AnCora | 91.1 / 86.0 | 41.9 / 23.6 |
| Dependency UAS/LAS | fr | GSD | 90.4 / 85.7 | 90.5 / 86.0 |

int8 is safe for XLM-R POS (ja 97.1 to 96.2) and the French parser, but costs 5 points on the character-level Chinese tagger (91.6 to 86.5): per-channel quantization broke it (20.7%) and keeping the classifier or edge layers in fp32 did not help, so use fp32 there. Do not quantize the Spanish mBERT parser. ja and zh are limited by piece segmentation not matching UD words.

## Latency and memory (Neovim end to end)

Run: `NSLS_NVIM_BENCH=1 NSLS_ML_DIR=... NSLS_ML_QDIR=... go test -count=1 ./internal/e2e -run TestNvimBench -v` (`internal/e2e`; off by default; wrap in `oomguard run -- env VAR=... go test ...`, since the unit does not inherit env). Each case is a fresh `nvim --clean --headless` with the real LSP client and a fresh server. X280, 4 threads auto. Ready = spawn to first analysed 4-line doc (includes model load). Open long = 32-line doc, all chunks new. Edit = 1-char insert in the long doc, median of 10. RSS in MB; hover under 1 ms everywhere (Wiktionary off).

| Case | Ready ms | Open long ms | Edit ms | RSS ready | Peak |
| ---- | -------- | ------------ | ------- | --------- | ---- |
| pos en bert_base | 501 | 1077 | 130 | 397 | 414 |
| pos es int8 | 1103 | 520 | 62 | 449 | 558 |
| pos fr int8 | 1103 | 541 | 62 | 459 | 560 |
| pos ja int8 | 1238 | 655 | 86 | 454 | 558 |
| pos zh int8 | 1349 | 956 | 88 | 451 | 558 |
| pos ja XLM-R int8, kagome words | 3243 | 1065 | 137 | 1010 | 1033 |
| pos ja XLM-R fp32, kagome words | 3151 | 1400 | 180 | 1015 | 1038 |
| pos ja XLM-R int8, pieces (`-tags nojaseg`) | 1065 | 708 | 94 | 444 | 556 |
| pos ja XLM-R fp32, pieces (`-tags nojaseg`) | 1030 | 962 | 128 | 458 | 474 |
| pos zh XLM-R int8 | 1045 | 692 | 89 | 456 | 563 |
| pos zh XLM-R fp32 | 1044 | 1142 | 147 | 452 | 475 |
| pos zh Yasuoka char int8 (86.5%) | 338 | 614 | 81 | 218 | 232 |
| pos zh Yasuoka char fp32 (91.6%) | 468 | 830 | 108 | 397 | 414 |
| semantic MiniLM (es/fr/ja/zh) | 750-855 | 35-69 | 26-43 | ~203 | ~217 |
| dependency en electra | 852 | 1005 | 135 | 667 | 675 |
| dependency es mBERT fp32 | 1131 | 1015 | 130 | 919 | 1114 |
| dependency fr int8 | 1175 | 1830 | 244 | 492 | 502 |
| dependency fr fp32 | 2590 | 3073 | 400 | 1944 | 1952 |

Japanese with kagome: ready time triples (3.2 s vs 1.1 s; the UniDic dictionary loads on the first Japanese text) and RSS more than doubles (1010 vs 450 MB), because the dictionary adds about 370 MB heap and 550 MB RSS. Edits are 40-50% slower (137 vs 94 ms int8) with the same GC settings (GOGC 100 or 400 gave the same), most likely because word-by-word input gives each particle its own word-start mark and more subwords per sentence. The Chinese character model fp32 is 108 ms per edit at 397 MB; int8 is 81 ms at 218 MB but loses 5 points. Bench cases `pos/ja`, `pos/zh`, `pos/zh-char` are in `internal/e2e`; ja and zh files are expected in `NSLS_ML_DIR`.

int8 POS is faster than fp32 (edit 62 vs 90-120 ms) at about the same RSS; peak is higher (~560 vs ~470) from load-time quantization buffers. The French parser at int8 is 4x smaller in RAM and 1.6x faster than fp32. All shippable candidates fit under 1.2 GB.

Semantic quality across languages: see `docs/models.md`.

## Model candidates (earlier survey)

Vocab and config downloaded; Go tokenization checked against each vocab. No ONNX export or inference run yet.

| Mode       | Lang   | Model                                                        | Vocab                  | Labels                  | Status                                                              |
| ---------- | ------ | ------------------------------------------------------------ | ---------------------- | ----------------------- | ------------------------------------------------------------------- |
| POS        | es     | `mrm8488/bert-spanish-cased-finetuned-pos`                   | WordPiece 31k          | EAGLES                  | tokenizes like HF                                                   |
| POS        | zh     | `ckiplab/bert-base-chinese-pos` (GPL-3.0)                    | WordPiece 21k          | CKIP                    | per-char                                                            |
| POS        | ja     | `KoichiYasuoka/bert-base-japanese-upos` (CC-BY-SA)           | char vocab 6k, no `##` | UPOS + B-/I-            | per-char                                                            |
| POS        | fr     | `gilf/french-camembert-postag-model`                         | SentencePiece Unigram  | -                       | tokenizer now loadable; superseded by udpos28-fr                    |
| POS        | all    | none found                                                   | -                      | -                       | see options below                                                   |
| Semantic   | all    | `sentence-transformers/distiluse-base-multilingual-cased-v2` | mBERT cased 119k       | -                       | DistilBERT, 768-dim, no `token_type_ids` (export script wraps this) |
| Semantic   | all    | `paraphrase-multilingual-MiniLM-L12-v2`                      | SentencePiece Unigram  | -                       | tested, works; LaBSE too large (501k vocab)                         |
| Dependency | es, ja | diaparser `es_ancora.mbert`, `ja_gsd.mbert`                  | mBERT                  | UD                      | usable via `export_model.py --model`                                |
| Dependency | zh     | diaparser `zh_ptb.hfl`                                       | HFL Chinese BERT       | Stanford (CTB7), not UD | needs label mapping                                                 |
| Dependency | fr     | diaparser `fr_sequoia.camembert-large`                       | SentencePiece Unigram  | UD                      | tested, works                                                       |

Multilingual POS decision: the per-language XLM-R UPOS models (option b below) won. Option (a) is no longer needed. Not tested: `KoichiYasuoka/deberta-base-japanese-wikipedia-luw-upos` (LUW B-/I- tags over char-sized Unigram pieces; would need piece merging after tagging).

Original options: (a) fine-tune `bert-base-multilingual-cased` on UD (es_ancora, fr_gsd, ja_gsd, zh_gsd) on Colab for one UPOS model covering all four languages, or (b) write a Go SentencePiece tokenizer to unlock per-language XLM-R UPOS models (`wietsedv/xlm-roberta-base-ft-udpos28-*`) and CamemBERT.

## Shipped (commit 9e45381)

- Setup: `setup.sh`/`setup.ps1 --model bert-base|mobilebert|pos-es|pos-fr|multilingual|dependency|dependency-es|dependency-fr|all`; skip-if-present; `_tokenizer.json` or `_vocab.txt` accepted.
- `export_model.py --int8`; aliases `xlmr-es`, `xlmr-fr`, `multilingual-minilm`. `mpnet` and `minilm` removed; semantic default is `multilingual-minilm`.
- VS Code model enum and `_tokenizer.json` fallback, Neovim `:NlsMode` semantic file, server fallback list, docs.
- Spanish parser is fp32 (int8 breaks it). Against English: es parser uses 919 MB vs 684 MB, fr parser edits take 244 ms vs 130 ms, POS and semantic start-up is slower (see `docs/models.md`).
- Off-by-default tests: accuracy (`TestEval`), semantic quality across languages, Neovim latency and memory (`internal/e2e`). Env vars in `docs/settings.md`.

## Remaining work

### 1. Known quality gaps

- Semantic mode fixed: unspaced neighbors are encoded without a word-start mark (`tokenizer.EncodeWordsContiguous`), ja 0.17 to 1.00, zh 0.88 to 1.00, colsep zh 1.37 to 3.54. Katakana words still show one color per piece; needs a dictionary segmenter (kagome) for whole-word colors.
- ja POS now tags kagome UniDic words (`internal/jaseg`, kana-bearing text only): 74.4% to 97.1% per character on GSD. Semantic ja group accuracy 0.96, colsep 2.56 (whole words, one word off). Costs +37 MB binary, ~370 MB heap on first Japanese text; `-tags nojaseg` removes it.
- Release variants: `-tags english` drops kagome, Japanese lemmas and Chinese Simplified/Traditional tables (7.3 MB vs 52 MB). Unigram tokenizer stays because the multilingual semantic model needs it. `scripts/build-release.sh` builds both; CI adds `*_english.*` archives.
- zh POS: `KoichiYasuoka/chinese-roberta-base-upos` exported and wired. Character-level B-/I- labels are merged into words in `POSModel` (`mergeContinuations`); 79.2% to 91.6% per character on GSDSimp. Hand export only (no setup name yet); int8, latency and memory not measured. See `docs/cjk-options.md`.
- Open: ja int8 POS with kagome words, ja dependency parser (diaparser `ja_gsd.mbert` on kagome words), Wiktionary lookup now gets real words (lemma lookup not done), Neovim latency with kagome loaded.
- No Japanese or Chinese dependency parser (ja needs MeCab; zh_ctb7 uses Stanford labels).
- Not tested: `KoichiYasuoka/deberta-base-japanese-wikipedia-luw-upos`.

### 2. Normalize POS labels

- UPOS and B-/I- labels map onto Penn tags (`postag.ParseLabel`). EAGLES and CKIP inventories still unmapped.
- Missing labels sidecar errors for sidecar-tokenizer models. Unknown labels still decode as `O`.
- Hover text still shows the Penn description (a UPOS `NOUN` reads "Noun, singular or mass"). Needs language-neutral wording without changing English output.

### 3. Tokenization

- Wiktionary lookup for Japanese and Chinese pieces: pieces such as `私は` are not dictionary headwords.
- Test offsets for combining marks, full-width punctuation, and mid-document edits.
- Spanish and French: test curly quotes, contractions, hyphens.

### 4. Sentence and chunk boundaries

- Review POS and semantic chunking (`SplitChunks`) with CJK text.
- Test Markdown block and paragraph splitting in each script.

### 5. Export and setup

- Test `setup.ps1` on Windows (the multilingual changes were never run there).
- Prefer explicit metadata over filename heuristics: hidden size is still inferred from "minilm" in the file name, and the slug rule is duplicated in Python, PowerShell, bash and JS.
- Record license and tag inventory per model in `docs/models.md` (XLM-R UPOS models are Apache-2.0; CKIP is GPL-3.0).
- Users download weights themselves; do not bundle in releases.

### 6. Definition lookup

- Done: `wiktionary_language` init option (`en` default, code, `model`, `auto`), cache keyed by language, language label and `#Section` anchor for non-English hits, tests for escaping, missing buckets, auto order. Plan: `docs/wiktionary-language-plan.md`. Model language comes from the model file name (`modelLanguage` in `cmd`), not real metadata.
- Open: ja/zh pieces such as `私は` still find no headword.

### 7. Validation

- Hover returned a result for 18 of 20 positions in semantic mode (English and multilingual); find out why.
- Benchmark on a platform other than the X280, and with hover on (Wiktionary on).
- VS Code checks for semantic tokens, hover, edits, definitions; Neovim is covered by `internal/e2e`.
- Run `go test ./...` and the opt-in tests before marking a language supported.

## Acceptance criteria

- Each claimed language has documented model IDs, licenses, export steps, and supported modes.
- Spans come from the model's tokenizer; CJK spans are multi-character Unigram pieces where the model has a SentencePiece tokenizer, and per character otherwise, documented as such.
- POS labels are mapped deliberately, with no accidental English fallback.
- Token spans point to correct source characters after multilingual edits.
- Existing English behavior and configuration stay backward compatible.
