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
| POS | ja | GSD | 74.4% | 73.2% |
| POS | zh | GSD | 79.2% | 78.6% |
| Dependency UAS/LAS | es | AnCora | 91.1 / 86.0 | 41.9 / 23.6 |
| Dependency UAS/LAS | fr | GSD | 90.4 / 85.7 | 90.5 / 86.0 |

int8 is safe for POS and the French parser. Do not quantize the Spanish mBERT parser. ja and zh are limited by piece segmentation not matching UD words.

## Latency and memory (Neovim end to end)

Run: `NSLS_NVIM_BENCH=1 NSLS_ML_DIR=... NSLS_ML_QDIR=... go test -count=1 ./internal/e2e -run TestNvimBench -v` (`internal/e2e`; off by default; wrap in `oomguard run -- env VAR=... go test ...`, since the unit does not inherit env). Each case is a fresh `nvim --clean --headless` with the real LSP client and a fresh server. X280, 4 threads auto. Ready = spawn to first analysed 4-line doc (includes model load). Open long = 32-line doc, all chunks new. Edit = 1-char insert in the long doc, median of 10. RSS in MB; hover under 1 ms everywhere (Wiktionary off).

| Case | Ready ms | Open long ms | Edit ms | RSS ready | Peak |
| ---- | -------- | ------------ | ------- | --------- | ---- |
| pos en bert_base | 501 | 1077 | 130 | 397 | 414 |
| pos es int8 | 1103 | 520 | 62 | 449 | 558 |
| pos fr int8 | 1103 | 541 | 62 | 459 | 560 |
| pos ja int8 | 1238 | 655 | 86 | 454 | 558 |
| pos zh int8 | 1349 | 956 | 88 | 451 | 558 |
| semantic MiniLM (es/fr/ja/zh) | 750-855 | 35-69 | 26-43 | ~203 | ~217 |
| dependency en electra | 852 | 1005 | 135 | 667 | 675 |
| dependency es mBERT fp32 | 1131 | 1015 | 130 | 919 | 1114 |
| dependency fr int8 | 1175 | 1830 | 244 | 492 | 502 |
| dependency fr fp32 | 2590 | 3073 | 400 | 1944 | 1952 |

int8 POS is faster than fp32 (edit 62 vs 90-120 ms) at about the same RSS; peak is higher (~560 vs ~470) from load-time quantization buffers. The French parser at int8 is 4x smaller in RAM and 1.6x faster than fp32. All shippable candidates fit under 1.2 GB.

Semantic quality across languages: see `docs/models.md`. Open issue: katakana words in Japanese split into one-character pieces and color by script; needs piece merging or a dictionary segmenter.

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

## Remaining work

### 1. Model and mode matrix

- Shipped: `setup.sh`/`setup.ps1 --model pos-es|pos-fr|multilingual|dependency-es|dependency-fr`, `export_model.py --int8`, VS Code enum, Neovim plugin, docs. Semantic default is now `multilingual-minilm` (mpnet and minilm removed). Spanish parser is fp32, others int8; es parser RSS (919 MB vs 684) and fr parser edit latency (244 vs 130 ms) are worse than English. Setup scripts untested on Windows.
- Record language, mode, model ID, tag inventory, tokenizer requirements, size, license, and limitations in `docs/models.md`.

### 2. Normalize POS labels

- UPOS and B-/I- labels now map onto Penn tags (`postag.ParseLabel`). EAGLES and CKIP inventories still unmapped.
- Missing labels sidecar now errors for sidecar-tokenizer models. Unknown labels still decode as `O`.
- Hover text still shows the Penn description (a UPOS `NOUN` reads "Noun, singular or mass"). Needs language-neutral wording without changing English output.

### 3. Tokenization

- Export script writes the `_tokenizer.json` sidecar (done).
- Wiktionary lookup for Japanese and Chinese pieces: pieces such as `私は` are not dictionary headwords.
- Test offsets for combining marks, full-width punctuation, and mid-document edits.
- Spanish and French: test curly quotes, contractions, hyphens.

### 4. Sentence and chunk boundaries

- Review POS and semantic chunking (`SplitChunks`) with CJK text.
- Test Markdown block and paragraph splitting in each script.

### 5. Export and setup

- Extend `scripts/export_model.py`, `scripts/setup.sh`, `scripts/setup.ps1` for the chosen models: tokenizer sidecar, label metadata, validation, skip-if-present.
- Prefer explicit metadata over filename heuristics for slugs, autodetection, hidden size, and sidecar lookup.
- Users download weights themselves; do not bundle in releases.

### 6. Definition lookup

- Language-aware Wiktionary lookup (es, fr, ja, zh) using the language from model metadata, spans as given by the tokenizer.
- Cache by language plus word and tag; keep graceful offline behavior.
- Test non-Latin URL escaping, case handling, missing entries.

### 7. Validation

- Sample sentences per language with expected spans, offsets, tags, and relations.
- Export smoke tests: ONNX inputs and outputs, vocab, label, and tokenizer sidecars, load through Go.
- VS Code and Neovim checks for semantic tokens, hover, edits, definitions.
- Benchmark startup, latency, and memory; document tradeoffs.
- Run `go test ./...` and per-artifact smoke tests before marking a language supported.

## Acceptance criteria

- Each claimed language has documented model IDs, licenses, export steps, and supported modes.
- Spans come from the model's tokenizer; CJK spans are multi-character Unigram pieces where the model has a SentencePiece tokenizer, and per character otherwise, documented as such.
- POS labels are mapped deliberately, with no accidental English fallback.
- Token spans point to correct source characters after multilingual edits.
- Existing English behavior and configuration stay backward compatible.
