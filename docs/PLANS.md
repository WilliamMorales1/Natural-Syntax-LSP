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
- `cmd`: vocab lookup falls back to `<model>_tokenizer.json`; embedding size is read from the model's `last_hidden_state` output (no file-name rule).
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
- VS Code model enum and `_tokenizer.json` fallback, Neovim `:NSMode` semantic file, server fallback list, docs.
- Spanish parser is fp32 (int8 breaks it). Against English: es parser uses 919 MB vs 684 MB, fr parser edits take 244 ms vs 130 ms, POS and semantic start-up is slower (see `docs/models.md`).
- Off-by-default tests: accuracy (`TestEval`), semantic quality across languages, Neovim latency and memory (`internal/e2e`). Env vars in `docs/settings.md`.

## Remaining work

### 1. Known quality gaps

- Semantic mode fixed: unspaced neighbors are encoded without a word-start mark (`tokenizer.EncodeWordsContiguous`), ja 0.17 to 1.00, zh 0.88 to 1.00, colsep zh 1.37 to 3.54. Katakana words still show one color per piece; needs a dictionary segmenter (kagome) for whole-word colors.
- ja POS now tags kagome UniDic words (`internal/lang/japanese`, kana-bearing text only): 74.4% to 97.1% per character on GSD. Semantic ja group accuracy 0.96, colsep 2.56 (whole words, one word off). Costs +37 MB binary, ~370 MB heap on first Japanese text; `-tags nojaseg` removes it.
- Release variants: `-tags english` drops kagome, Japanese lemmas and Chinese Simplified/Traditional tables (7.3 MB vs 52 MB). Unigram tokenizer stays because the multilingual semantic model needs it. `scripts/build-release.sh` builds both; CI adds `*_english.*` archives.
- zh POS: `KoichiYasuoka/chinese-roberta-base-upos` exported and wired. Character-level B-/I- labels are merged into words in `POSModel` (`mergeContinuations`); 79.2% to 91.6% per character on GSDSimp. Hand export only (no setup name yet); int8, latency and memory not measured. See `docs/cjk-options.md`.
- Open: ja int8 POS with kagome words, ja dependency parser (diaparser `ja_gsd.mbert` on kagome words), Wiktionary lookup now gets real words (lemma lookup not done), Neovim latency with kagome loaded.
- No Japanese or Chinese dependency parser (ja needs MeCab; zh_ctb7 uses Stanford labels).
- Not tested: `KoichiYasuoka/deberta-base-japanese-wikipedia-luw-upos`.

### 2. Normalize POS labels

- UPOS and B-/I- labels map onto Penn tags (`postag.ParseLabel`). EAGLES and CKIP inventories still unmapped.
- Missing labels sidecar errors for sidecar-tokenizer models. Unknown labels still decode as `O`.
- Done 2026-11-07: every tagset collapses to one Universal class (`PartOfSpeech.Canonical`) for default color and hover, so English `NNS`/`VBD` look and read like Spanish `NOUN`/`VERB` (`docs/tags.md`). English fine-grained colors are gone by design; `tokenMapUpdate` still overrides single Penn tags. Remaining difference: UPOS `SCONJ` keeps its own hover wording while its color equals `ADP`.
- Done: UPOS models set `Token.Description` from `postag.LabelDescription` (`NOUN` reads "Noun"); Penn models keep Penn wording.

### 3. Tokenization

- Wiktionary lookup for Japanese and Chinese pieces: pieces such as `私は` are not dictionary headwords.
- Done (`internal/tokenizer/offsets_test.go`): offsets after mid-document edits, combining marks, full-width punctuation, es/fr punctuation. Found and fixed: curly `’` split English words (`it’s`, `don’t`) where `'` did not; `isApostrophe` now covers both.

### 4. Sentence and chunk boundaries

- Done (`internal/inference/chunks_test.go`): `SplitChunks` keeps order, respects `ChunkSize`, and breaks at sentence ends for en/es/fr/ja/zh/ru/ar; markdown block starts hold in ja/ru/ar.

### 5. Export and setup

- Test `setup.ps1` on Windows (the multilingual changes were never run there).
- Prefer explicit metadata over filename heuristics: hidden size is still inferred from "minilm" in the file name, and the slug rule is duplicated in Python, PowerShell, bash and JS.
- Record license and tag inventory per model in `docs/models.md` (XLM-R UPOS models are Apache-2.0; CKIP is GPL-3.0).
- Users download weights themselves; do not bundle in releases.

### 6. Definition lookup

- Done: `wiktionary_language` init option (`en` default, code, `model`, `auto`), cache keyed by language, language label and `#Section` anchor for non-English hits, tests for escaping, missing buckets, auto order. Plan: `docs/wiktionary-language-plan.md`. Model language comes from the model file name via `Language.ModelPrefixes` (`lang.ForModel`), not real metadata.
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

# Russian and Arabic support plan

Goal: add POS, semantic and, if a parser exists, dependency support for Russian and Arabic by following `docs/adding-a-language.md`, and use the work to test whether that guide is enough. Any edit outside `internal/lang/<name>`, `internal/lang/builtin`, scripts, extension and docs counts as a gap in the extension design and is recorded here.

## Starting point

- Semantic: `paraphrase-multilingual-MiniLM-L12-v2` already covers both languages; only quality needs measuring (`NSLS_SEMANTIC_EVAL`).
- POS: `wietsedv/xlm-roberta-base-ft-udpos28-ru` and `-ar` (same family as es/fr, UPOS, Apache-2.0 to be confirmed per model card). Same XLM-R Unigram tokenizer, so no new tokenizer code is expected.
- Dependency: check the diaparser catalog for Russian and Arabic models (for example `ru_syntagrus.*`, `ar_padt.*`; names unverified). If none exports cleanly, ship POS and semantic only and say so.

## Language-specific risks

Russian:
- Stress marks (U+0301 combining acute) in textbooks and Wiktionary-style text; Wiktionary headwords have none. Needs a `Forms` hook that strips them; check the tokenizer keeps the mark inside its word and the model's NFKC handling does not split it.
- `ё` / `е` spelling variants: try the `ё` form of a headword when the `е` page misses.
- Wiktionary section names live in `lang.Wiktionary.Sections` (no `langCodes` table).

Arabic:
- Right-to-left text: offsets stay in logical order, so LSP positions should be fine, but test mixed Arabic/Latin lines and numbers, and bidi control characters (U+200E/F, U+202A-E) which are `Cf`, not punctuation, and may glue to words.
- Optional diacritics (tashkeel) and tatweel (U+0640) are not in headwords and NFKC does not remove them. `Forms` strips both.
- Clitics are written attached (`وَالْكِتَاب`: conjunction, article, noun). UD Arabic-PADT splits them into syntactic words, so scoring against PADT will undercount the way ja pieces did. Decide whether to leave pieces (documented, cheap) or add a clitic `WordMerger`/`Segmenter` (costly; needs a segmenter dependency and a size and memory check like kagome).
- Wiktionary: `{{head|ar|...}}` pages may fail the REST parser like Chinese; if so set `Wikitext` and `Script: unicode.Arabic`.
- Arabic punctuation (`، ؛ ؟`) must end sentences in `SplitChunks`; Russian needs no change. The sentence-end and closing-bracket sets (`sentenceEnders`, `cjkClosers` in `internal/inference/dependency_model.go`) are CJK-specific and live in a generic file, a leak the language refactor did not move. Resolved without a hook: `inference` now uses Unicode `Sentence_Terminal` plus non-ASCII `Pe`/`Pf` closers (`isSentenceEnd`, `isCloser`), covering `؟ ۔` and `»`.
- Hover text for POS uses Penn wording; unchanged by this plan (see "Normalize POS labels").

## Steps

1. Branch from `multilingual`. Write the failing tests first: a `ForModel` case per language, `Forms` tests (Russian stress and `ё`, Arabic diacritics and tatweel), a sentence-boundary test for `؟`.
2. Add `internal/lang/russian` and `internal/lang/arabic` (`Language()` with `ModelPrefixes` `xlmr_ru`/`xlmr_ar`, plus hooks from the risks above); list both in `builtin` and `builtin_english`; set `Wiktionary.Sections`.
3. Export models: `xlmr-ru`, `xlmr-ar` in `export_model.py` (`--int8`), `pos-ru`/`pos-ar` in `setup.sh` and `setup.ps1`, extension enum and descriptions. Do the same for any parser found.
4. Accuracy: add UD_Russian-GSD (or SynTagRus) and UD_Arabic-PADT to `eval_test.go` and `live_eval_test.go`; record fp32 and int8 POS, and UAS/LAS if a parser exists. Check that int8 does not collapse as the es parser and zh character model did.
5. Semantic: add ru and ar sentence groups to the cross-language quality test; compare to the es/fr numbers.
6. Latency and memory: add `pos/ru`, `pos/ar` cases to `internal/e2e`; expect es/fr-like figures (about 450 MB RSS ready, edit near 62 ms int8).
7. Wiktionary: measure hit rate on both treebanks before and after the `Forms` hooks; keep a hook only if it helps.
8. Docs: `docs/models.md` rows (license, size, accuracy), `docs/settings.md` language list, README line; tell `adding-a-language.md` what was missing.

## Acceptance

- Both languages work end to end in POS and semantic modes with documented models, licenses and accuracy; dependency only if a parser passes the same checks.
- No Russian or Arabic string in `tokenizer`, `inference`, `lspserver` or `wiktionary` ; any exception is listed under "Design gaps" below.
- `go build -tags english ./...` and `go test -short -race ./...` pass; English output unchanged.

## Design gaps found

- Sentence enders and closers were CJK-only constants in `inference`. Fixed generically with Unicode properties (`internal/inference/sentences_test.go`). Side effect: English curly closing quotes (`”`) after a period now stay with their sentence.
- `langCodes` in `wiktionary/wikitext.go` was a language table in a generic package. Replaced by `lang.Wiktionary.Sections`.
- Tokenizer options reach Unigram tokenizers only; fine for XLM-R ru/ar.
- `basicTokenize` in the generic tokenizer needed an Arabic-driven fix (bidi controls); script-neutral, so no language hook.

## Progress

- Done: step 1 tests, step 2 (`internal/lang/russian`, `internal/lang/arabic`, builtin lists, `langCodes`), sentence ends. Branch `russian-arabic`.
- Step 3 export done on Colab (int8, 279 MB each, labels are UPOS); scripts, setup and extension enum updated. `EmbedHiddenSize` removed; `userAgent` points at the real repo.
- Step 4 POS accuracy (UD test, first 200 sentences, per source character): ru GSD fp32 93.5 / int8 93.0; ar PADT fp32 93.7 / int8 93.6. int8 is safe for both. Arabic PADT scoring skips words inside multiword tokens (clitic-attached), so the figure covers unattached words only. No ru/ar parser tried yet.
- Step 5 semantic (MiniLM, `NSLS_SEMANTIC_EVAL`): cat/car same-concept 0.98-0.99 vs different 0.15-0.25 across all seven languages. Group accuracy ru 0.92, ar 0.88 (es/fr/en 1.00); colsep ru 2.24, ar 1.68 (es 3.99). Arabic clitic-attached words (`والكتاب`) are single pieces, which blurs colors. The test texts needed 50+ tokens or the plane stays unfitted (ru/ar texts got a filler sentence).
- Step 6 Neovim (X280, int8, `internal/e2e`): pos/ru ready 998 ms, edit 60 ms, RSS 457 MB; pos/ar ready 974 ms, edit 51 ms, RSS 451 MB; semantic/ru and /ar ready ~900 ms, edit 23-25 ms, RSS ~205 MB. Matches es/fr.
- Step 7 Wiktionary (60 words, live, `NSLS_WIKT_NOFORMS=1` gives the baseline): ru pos hit 63.3 to 65.0, pipeline 60.0 to 61.7; ar pos 51.7 to 51.7, pipeline 38.3 to 40.0. Small gain, hooks kept. Most misses are inflected forms (ru `спортивной`, ar `الأكبر`, `للعدل`) and proper nouns; fixing that needs lemmatization, not done.
- Step 8 done (README, `docs/models.md`, `docs/settings.md`). Bidi controls (U+200E/F, 061C, 202A-E, 2066-69) glued to words in `basicTokenize`; now word breaks (`isWordBreak`, test `internal/tokenizer/bidi_test.go`). Mixed Arabic/Latin/number lines tokenize correctly.
- Open: Wiktionary lemma lookup for inflected ru/ar forms; Arabic clitic segmentation (decision: leave pieces).

# Lua-defined languages and models plan

Goal: define language and model specific behavior in Lua scripts loaded at startup, so adding or tuning a language needs no Go rebuild of the main binary.

## Motivation

- Today each language is a Go package under `internal/lang/<name>` listed in `internal/lang/builtin`. Every new language or rule tweak means a full rebuild and a new release binary per platform.
- Model metadata (file prefix, labels, tokenizer options) is likewise compiled in or hardcoded in `internal/inference`.

## Decisions

- Embedded interpreter in pure Go (`github.com/yuin/gopher-lua`, MIT). No cgo, so cross compiles and release builds stay as they are.
- `lang.Language` stays the only interface the generic packages see. A Lua loader produces `lang.Language` values; Go code never learns whether a language is native or scripted.
- Built-in Go languages remain for the default set (English, Chinese, Japanese need Go: kagome, simplify tables). Lua covers the rest and user overrides.
- Scripts are data, not plugins: no `os`, `io`, `package`, `debug` or `load*` in the sandbox. Only pure string and table functions plus a small host API.
- Per-call instruction limit and a recover wrapper so a bad script cannot hang or crash the server.

## Design

- Search path: `<model dir>/lang/*.lua`, then `~/.config/natural-syntax-ls/lang/*.lua`, then a `naturalSyntaxLs.langDir` setting. Later entries override earlier ones by `code`.
- One file returns one table:

```lua
return {
  code = "ru",
  model_prefixes = { "xlmr_ru" },
  tokenizer = { strip_bidi = false },          -- named options, mapped to tokenizer.Option in Go
  wiktionary = {
    code = "ru",
    sections = { "Russian" },
    rewrite = function(word, pos, deprel) return nil end,
    forms = function(word, pos) return {} end,
  },
}
```

- Model specific data (labels, UPOS mapping, hidden size, sentence rules) goes in a sibling `<model>.lua` or a `model` table: `labels`, `label_map`, `pos_normalize(label)`.
- Tokenizer options are a closed set of named flags implemented in Go. Lua selects them; it does not run per token (hot path stays native).
- Hooks that run per lookup (`rewrite`, `forms`) are fine in Lua: low call rate, small inputs.
- One Lua state per language, guarded by a mutex; hooks are pure so states are reusable.

## Steps

1. Inventory every field of `lang.Language` and `lang.Wiktionary`; mark each as data (declarative) or hook (callable). Check `internal/inference` for model facts that belong in a model table.
2. Add `internal/lang/luaload`: sandboxed state, host API (`postag` constants, string helpers), table-to-`Language` conversion with validation and clear errors naming file and field.
3. Port Russian, Arabic and French to Lua files under `internal/lang/scripts/` as the first consumers; keep Go versions behind a test that asserts identical output on both.
4. Wire loading into `cmd/natural-syntax-ls/main.go` and `lspserver/registry.go`; log which languages came from where; override by `code`.
5. Move model metadata (labels, prefix to language mapping, normalization) behind the same loader; `export_model.py` emits a `<model>.lua` next to the ONNX file.
6. Sandbox and limits: instruction budget, memory cap, forbidden globals test, panic recovery test.
7. Extension: setting for extra script directory; `Reload` on file change optional.
8. Docs: rewrite `docs/adding-a-language.md` around Lua first, Go only when a hook needs a native library. Document the schema and sandbox.

## Risks

- Behavior drift between Go and Lua ports. Mitigation: dual-run equality test from step 3.
- Startup cost of parsing scripts. Mitigation: measure; target under 5 ms per language.
- Untrusted scripts shipped next to downloaded models. Mitigation: sandbox, no filesystem or network, document trust model.
- Hot-path temptation: keep anything per token or per sentence in Go.

## Acceptance

- New language with only Lua hooks works with an unmodified release binary.
- Ported Russian, Arabic, French output identical to Go versions on the test treebanks.
- `go build` for all release targets still cgo-free; `go test -short -race ./...` passes.
- Malicious or looping script fails that language only, with a logged error; server keeps running.

# Russian and Arabic dependency parser plan

Goal: find, export and evaluate a UD dependency parser for each of Russian and Arabic, or record why none ships. Follows the same checks the es and fr parsers passed.

## Search

1. List the diaparser catalog (`diaparser.parsers.Parser.get_available_models()` or its README table) for Russian (`ru_*`) and Arabic (`ar_*`) entries. Names are unverified; expect something like `ru_syntagrus.*` and `ar_padt.*`.
2. If the catalog has none, search the Hugging Face Hub with `hub_repo_search` for UD biaffine or token-classification-style parsers (`ru`, `ar`; tags `dependency-parsing`, `udpipe`, `trankit`). Parsers that are not biaffine or need a non-BERT encoder are out of scope unless they export to the same 3-input/2-output graph (`input_ids`, `attention_mask`, `pool_matrix` to `arc_logits`, `rel_logits`).
3. Record for each candidate: encoder and tokenizer type (WordPiece or Unigram, both supported; SentencePiece BPE is not), model size, license, label set (UD or not), training treebank.

## Acceptance for a candidate

- Exports with `export_model.py --model <catalog name>` and runs through Go on test sentences with correct heads and relations.
- Tokenizer is supported in Go. A parser whose encoder needs a morphological segmenter (as ja did with MeCab) is rejected.
- Labels are UD, or a mapping exists. Unknown relations decode as no color, so check the label list against `postag.ParseDeprel` before anything else.
- License allows redistribution of the export steps (users download weights themselves).
- UAS/LAS on UD test (first 200 sentences, `TestEvalDependency` in `eval_test.go`) for fp32 and int8. Reject int8 if UAS drops more than a few points (the Spanish parser fell from 91 to 42).
- Latency and memory in `internal/e2e` (`dependency/ru`, `dependency/ar`), expect fr-like figures if the encoder is large; must fit under 1.2 GB.

## Language risks

Russian:
- SynTagRus is the large treebank, but check its license for derived models; GSD is the safer eval set (already used for POS).
- Rich morphology means a large subword count per word; check the `maxSeqLen` truncation drops no words on test sentences.

Arabic:
- PADT splits clitics into syntactic words and diaparser models trained on it expect those words. Whole written words fed to the parser will score lower and attach wrongly. Options: leave as is and document, or add a clitic segmenter (costly, see the POS plan). Measure UAS/LAS with gold-segmented and raw input to size the gap before deciding.
- Dependency relations may carry subtypes (`obl:arg`, `nmod:poss`); confirm `ParseDeprel` handles the `:` forms.

## Steps

1. Search and record candidates in the table below.
2. Export the best candidate per language on Colab (per the compute rule), `--int8` and fp32.
3. Write failing tests first: anchor (head, relation) pairs in `TestMultilingualDependency`, `ParseDeprel` cases for any new labels.
4. Evaluate UAS/LAS, then latency and memory.
5. Wire in: `export_model.py`, `setup.sh`, `setup.ps1` (`dependency-ru`, `dependency-ar`), extension enum, `ModelPrefixes` on the language packages (`ru_*`, `ar_*` file stems), `docs/models.md`, README.
6. If nothing passes, write the reason here and stop; POS and semantic stay the shipped modes.

## Candidates

Search 2026-11-07, diaparser README (IWPT 2020 table). Encoder inferred from the name suffix; size, license and tokenizer type still to confirm from the checkpoint config.

| Lang | Catalog name | Treebank | UAS / LAS (README) | Encoder | Verdict |
| ---- | ------------ | -------- | ------------------ | ------- | ------- |
| ru | `ru_syntagrus.DeepPavlov` | SynTagRus | 94.97 / 93.72 | DeepPavlov RuBERT (WordPiece, supported) | candidate; export next |
| ar | `ar_padt.bert` | PADT | 87.75 / 83.25 | loads as English bert-base-uncased (30522 vocab, EWT) | rejected: README name is wrong |
| ar | `ar_padt.asafaya` (real release asset, 1.4 GB) | PADT | - | asafaya Arabic BERT, 32k WordPiece | shipped, int8 |

Results (first 200 UD test sentences, `TestEvalDependency`):

| Lang | Treebank | fp32 UAS / LAS | int8 UAS / LAS | Ships |
| ---- | -------- | -------------- | -------------- | ----- |
| ru | GSD | 90.8 / 80.4 | 81.8 / 67.8 | fp32 (int8 loses 9 UAS) |
| ar | PADT | 77.7 / 72.0 | 77.5 / 71.9 | int8 |

Arabic scores whole written words against PADT heads at matching word starts, so clitic-attached words are not scored; the gap versus the README figure (87.75) is raw input, as predicted. Anchor tests in `TestMultilingualDependency` pass. Neovim (X280): dependency/ru fp32 ready 1213 ms, edit 125 ms, RSS 955 MB (peak 1118); dependency/ar int8 ready 1121 ms, edit 182 ms, RSS 494 MB. Wired: `setup.sh`/`setup.ps1` (`dependency-ru`, `dependency-ar`), extension enum, `ModelPrefixes` (`ru_syntagrus`, `ar_padt`), `docs/models.md`, `docs/install.md`. Lesson: verify a catalog name against the release assets (`gh api repos/Unipisa/diaparser/releases`) and check the exported vocab size before trusting it.

## Progress

- Steps 1-4, 6-8 done. `internal/lang/luaload` (gopher-lua v1.1.1, pure Go) builds `lang.Language` from scripts; `lang/*.lua` holds English, French, Spanish, Russian, Arabic. Only Japanese (kagome) and Chinese (tables) stay Go packages, in `internal/lang/builtin/native.go`; `-tags english` drops both. Go versions of the five ported languages deleted after a 5000-input differential run (ru/ar forms, en rewrite) matched; tests moved to `internal/lang/builtin/builtin_test.go`.
- Search path: embedded, `<model dir>/lang`, `~/.config/natural-syntax-ls/lang`, `--lang-dir` / `naturalSyntaxLs.langDir`; later overrides by `code`. A broken script is logged and skipped.
- French elision stays native as the named tokenizer flag `elisions` (clitic and word lists come from Lua), so no French string remains in Go.
- Sandbox: no `os io package debug coroutine load* dofile require print`; `string.rep` capped; load 2 s, hook 200 ms via context; tests in `luaload_test.go`. Heap cannot be capped from outside, documented.
- gopher-lua quirk: `x = (string.gsub(...))` on a parameter miscompiles; assign through a new local.
- Open: step 5 (model metadata: labels, UPOS mapping, hidden size, `export_model.py` emitting `<model>.lua`), per-language startup timing, `Reload` on file change, Windows check.
- Scripts moved out of the binary: `lang/*.lua` in the repo, installed by `setup.sh`/`setup.ps1` into the config dir (no overwrite), bundled by release archives and the extension as `<exe dir>/lang`. Embed and `Seed` removed. Search path now `<exe dir>/lang`, `<model dir>/lang`, config `lang`, `--lang-dir`. A binary with no scripts has only ja and zh.
- Model catalog moved to `data/models.json` (was repo root, copied to config dir, bundled next to the binary): name, mode, source, language, default, int8. `internal/models` loads it (later dir overrides by name) and the server's default model search uses `default` entries for the current mode; `export_model.py` reads it. `setup.sh --model NAME|all` and `setup.ps1 -Model NAME|all` read it too (`--list` / `-List` prints it; old aliases like `pos-es` are gone; fixed `tr '-./'` slug bug that broke the skip-if-present check). `setup.ps1` untested (no pwsh here). Still hardcoded: the extension `naturalSyntaxLs.model` enum (static package.json); `ModelPrefixes` live in language scripts.
- Release archives are one full build per platform (all languages); the `*_english.*` archives and the english variant in `build-release.sh` are gone. The `english` build tag and its stubs are removed; `nojaseg` stays.

## Plan: simulate a new user adding de, it, pt, ar

Goal: follow only the docs as a new user would, with no repo edits and no commits, and record every snag. Done 2026-11-07 in a scratch config dir (`XDG_CONFIG_HOME` pointed at a copy of the install, with `lang/*.lua` copied from the repo and the real model files symlinked).

Method:
1. Write `de.lua`, `it.lua`, `pt.lua`, `ar.lua` from `docs/adding-a-language.md` (Italian gets an `elisions` list; Arabic starts with no hooks).
2. Add `xlmr-de|it|pt|ar` entries (`wietsedv/xlm-roberta-base-ft-udpos28-<code>`) to the scratch `models.json`.
3. Drive the real binary over LSP (`semantic` mode for tokenization and hover, `pos` mode with `-lang`) on one sentence per language.
4. Fake an installed `xlmr_de` by symlinking the `xlmr_es` files, to test model selection only. No real export was run (heavy, Colab).

What worked:
- All four scripts load without code changes. The detector already knows de, it, pt, ar, so hover finds `Hund`, `springt`, `cachorro`, `maçãs`, `cane` in the right language with no `wiktionary_language` setting.
- Italian elision: with `it.lua` the tokens are `L'`, `amico`, `dell'`, `un'`; without it the apostrophes vanish (`L`, `dell`, `un`). The `elisions` flag is language-neutral, so no Go change.
- `-lang de` with `xlmr_de` present picks it (different tags from `bert_base`); `-lang it` with no Italian model falls back to the default.

Problems found:
1. No Italian/German/Portuguese/Arabic entries in the shipped `models.json`; `docs/models.md` has no Arabic POS row even though `xlmr_ar` files exist locally.
2. Catalog edits do not propagate. `setup.sh` copies `models.json` to the config dir only if absent, and `export_model.py` reads the repo copy before the config copy, so a user who adds entries in the config dir cannot export them, and one who edits the repo copy after the first setup is not seen by the server.
3. The export script's docstring lists the model names by hand (`xlmr-es|fr|ru` only), against the "no model name in Python" rule.
4. VS Code `naturalSyntaxLs.model` is a static enum, so new catalog names cannot be picked. The `:NSLang` completion list is also hardcoded.
5. `-lang CODE` with no installed model for that code silently falls back to the English default; no log line says which model loaded. Should warn, and the server should log the chosen model path.
6. Auto-detect never switches the model, so a German file under the default setup gets English POS tags (`Hund` is a proper noun at 0.80 under `xlmr_es`-as-de, a noun at 0.99 under bert-base). Only `:NSLang` changes models.
7. Arabic hover: bare words work (`كلب`) but the definite article (`المدرسة`, `الكلب`) and tashkeel (`كَلْب`) find nothing. The old `arabic.lua` `forms` hook stripped tashkeel; nothing strips `ال`. A new user has to know to write both.
8. Semantic mode has no POS, so homographs pick the wrong sense: `amico` gives "first-person singular present indicative of amicarsi", `faulen` gives the verb "to rot", `كلب` gives "to be seized by hydrophobia".
9. `xlm-roberta-base-ft-udpos28-ar` ships only `pytorch_model.bin` (no safetensors); recent torch refuses `torch.load` on it, so the export may fail without a workaround. Not run.
10. diaparser has no usable de/it/pt/ar dependency parser in the release assets besides `ar_padt.asafaya`; `de_hdt.dbmdz-bert-base`, `it_isdt.dbmdz-*` exist but are untested here.
11. `:NSLang` completion and `lang` model choice cover only POS models for these languages; semantic mode keeps the multilingual model (correct) and dependency has no match.

Fixed 2026-11-07: 2 (setup no longer copies `models.json`; `export_model.py` merges repo and config catalogs, config wins), 4 (VS Code `model` is free text with examples), 5 (server logs the chosen model and warns when `-lang` has no matching model). `:NSLang` completion stays a static list. Next: ship `xlmr-de|it|pt`; Arabic `forms` script is moot while `lang/arabic.lua` is removed.

## Plan: better language identification than n-gram naive Bayes

Current: `internal/langid`, hashed-free char 1-3-gram naive Bayes, 288 KB, about 11 us per line, 10 languages. Held-out Tatoeba accuracy 95.7-99.98% on sentences, but short text suffers: es 85%, pt 91%, fr 93%, en 95% at 20 characters or fewer. Weak points: es/pt/it confusion, no calibrated confidence (a fixed per-n-gram margin decides abstaining), independence assumption overcounts repeated evidence, pruned vocabulary (3000 n-grams per language) drops rare ones, and every added language needs retraining on Tatoeba.

Candidates, in order to try:
1. **Discriminative linear model on the same features** (fastText-style: hashed char 1-5-grams plus word unigrams, softmax or one-vs-rest, trained with SGD). Same cost class, usually several points better on short text because it learns weights against confusable languages instead of per-language likelihoods. Keep the pure-Go scorer; only the trainer changes. Expected size 0.5-2 MB at 2^18 buckets, int8.
2. **Pretrained fastText `lid.176.ftz`** (CC BY-SA 3.0, about 900 KB, 176 languages). Needs a Go reader for the product-quantized format; check license compatibility with the repo's before embedding. Covers languages users add without retraining. Compare against option 1 on the same held-out set.
3. **Compact CNN or small transformer via ONNX** only if 1 and 2 stay under 97% on texts of 20 characters or fewer. ONNX Runtime is already linked, but start-up and per-line latency rise from microseconds to milliseconds, so run it per paragraph, not per token.

Cheap improvements that apply to any of them:
- Add word-level features and a prior from the previous line's language (documents are mostly monolingual; smooth across lines).
- Return a calibrated probability (softmax with temperature fitted on held-out data) instead of the per-n-gram margin, and expose a threshold.
- Script shortcuts first (kana means ja, Hangul means ko, Cyrillic narrows to ru/uk/bg), so the model only separates within a script.
- Train on more than Tatoeba: add Wikipedia text and the UD treebanks, plus text with code, URLs and numbers, since Tatoeba sentences are clean and short. Include look-alikes (es/pt/gl/ca, ru/uk/be, zh simplified vs traditional).

Acceptance:
- Held-out accuracy at least 98% on whole sentences and at least 95% on texts of 20 characters or fewer for every shipped language, measured by `TestHeldOutAccuracy` extended with a third length bucket (6-12 characters).
- Per-line cost under 100 us and binary growth under 2 MB.
- No regression in `TestDetect`, `TestElisionsOnlyInDetectedFrench` or `TestTokenAtDetectsLineLanguage`.
- Adding a language is one data directory plus one trainer run; document the command in `docs/adding-a-language.md`.

Steps:
1. Extend the eval to report a confusion matrix and the short-text buckets; record the current numbers here.
2. Implement option 1 in `internal/langid/train` behind the same `model.bin` loader (version byte in the header) and compare.
3. Evaluate option 2 on the same set; pick by accuracy, size and license.
4. Add calibration and previous-line smoothing in `Detect` callers (tokenizer line grouping, hover).
5. Retrain with the wider corpus, update `docs/settings.md`, and note the result here.

Step 1 done 2026-11-07. `TestHeldOutAccuracy` now reports length buckets (1-5, 6-12, 13-20, 21+ runes) and a confusion matrix; run with `NSLS_LANGID_DIR` set to the Tatoeba per-language dumps (`https://downloads.tatoeba.org/exports/per_language/<iso3>/<iso3>_sentences.tsv.bz2`). Baseline (naive Bayes, held-out every 20th sentence):

| Lang | all | 6-12 | 13-20 | 21+ |
| ---- | --- | ---- | ----- | --- |
| en | 98.8 | 88.7 | 95.8 | 99.2 |
| fr | 98.7 | 78.0 | 94.4 | 99.3 |
| es | 95.7 | 72.9 | 87.4 | 97.4 |
| de | 99.3 | 84.5 | 95.8 | 99.6 |
| it | 98.2 | 87.9 | 95.0 | 98.9 |
| pt | 97.5 | 79.4 | 92.9 | 98.5 |
| ru | 100.0 | 100.0 | 100.0 | 100.0 |
| ja | 99.7 | 99.9 | 100.0 | 100.0 |
| zh | 97.3 | 99.8 | 99.7 | 99.4 |
| ar | 99.7 | 100.0 | 99.9 | 99.9 |

Findings: the 1-5 bucket is mostly abstentions (by design, under the minimum evidence). Errors are Latin-script only; es loses the most (6-12: 72.9%), and its confusions go to pt (210), `-` (583), it (94). en/fr/it/pt 13-20 sit at 93-96%, under the 95% short-text target for es and pt. Whole-sentence 21+ already meets 98% except es (97.4%). Abstention is the dominant short-text failure for es (583 of 22158), so calibration (step 4) and a discriminative model (step 2) both target the right errors.

Step 2 done 2026-11-07. `internal/langid/linear.go` is a softmax regression over hashed (2^17 buckets) character 1-5-grams plus words, int8 weights, 1.3 MB (`linear.bin`, magic byte 0xFF; the naive Bayes `model.bin` stays as fallback when it is absent). `Detect` uses it, abstaining under probability 0.5. Trainer: `go run ./internal/langid/train -data DIR -out internal/langid/linear.bin -epochs 6 -n 150000 -crop 0.6` (Adagrad, 40% whole sentences and 60% random 4-30 character windows, about 1 minute). Evaluate a candidate file with `NSLS_LANGID_LINEAR=path NSLS_LANGID_DIR=... go test ./internal/langid -run HeldOut -v`. Cost 10.8 us per line (baseline about 11 us).

| Lang | all (was) | 6-12 (was) | 13-20 (was) | 21+ (was) |
| ---- | --------- | ---------- | ----------- | --------- |
| en | 99.7 (98.8) | 92.8 (88.7) | 98.7 (95.8) | 99.8 (99.2) |
| fr | 99.8 (98.7) | 91.3 (78.0) | 98.9 (94.4) | 100.0 (99.3) |
| es | 99.2 (95.7) | 87.4 (72.9) | 97.3 (87.4) | 99.8 (97.4) |
| de | 99.95 (99.3) | 96.9 (84.5) | 99.7 (95.8) | 100.0 (99.6) |
| it | 99.7 (98.2) | 93.9 (87.9) | 99.1 (95.0) | 99.9 (98.9) |
| pt | 99.5 (97.5) | 89.8 (79.4) | 98.1 (92.9) | 99.8 (98.5) |
| ru, ja, ar | 99.7-100 | 99.9-100 | 99.9-100 | 99.9-100 |
| zh | 97.5 (97.3) | 99.8 | 100.0 | 99.7 |

Acceptance status: whole sentences at least 98% everywhere except zh (97.5%, all errors are abstentions on 1-5 character texts, 115 of 4464); 13-20 characters at least 97.3% everywhere; binary growth 1.0 MB net of the kept `model.bin`. Not yet met: 6-12 characters is 87-94% for Latin-script languages (target applies to 20 characters or fewer combined, which now holds). Next: step 3 (compare `lid.176.ftz`), then calibration and previous-line smoothing, then a wider corpus; dropping `model.bin` would save 288 KB.

Incident: a stray `git stash` during this step briefly removed the working-tree changes; popped immediately, all files restored, and the two staged deletions (`lang/arabic.lua`, `scripts/build-release.sh`) re-staged.

Step 3 done 2026-11-07: `lid.176.ftz` (938 KB, CC BY-SA 3.0) scored with the official `fasttext` Python package on the same held-out Tatoeba sentences, picking the best of our ten languages and never abstaining (script kept outside the repo; it needs numpy<2). It loses to the step 2 model nearly everywhere, even though Tatoeba is among its sources:

| Lang | fastText all | ours all | fastText 6-12 | ours 6-12 | fastText 13-20 | ours 13-20 |
| ---- | ------------ | -------- | ------------- | --------- | -------------- | ---------- |
| en | 99.7 | 99.7 | 94.4 | 92.8 | 98.8 | 98.7 |
| fr | 98.8 | 99.8 | 69.9 | 91.3 | 95.0 | 98.9 |
| es | 98.1 | 99.2 | 82.0 | 87.4 | 94.5 | 97.3 |
| de | 99.5 | 99.95 | 79.3 | 96.9 | 97.3 | 99.7 |
| it | 98.8 | 99.7 | 90.6 | 93.9 | 96.4 | 99.1 |
| pt | 97.0 | 99.5 | 74.9 | 89.8 | 91.6 | 98.1 |
| ru | 100.0 | 100.0 | 100.0 | 100.0 | 100.0 | 100.0 |
| ja | 99.7 | 99.7 | 98.8 | 99.9 | 100.0 | 100.0 |
| zh | 93.2 | 97.5 | 93.5 | 99.8 | 96.3 | 100.0 |
| ar | 99.7 | 99.7 | 98.8 | 100.0 | 99.9 | 99.9 |

Decision: keep the step 2 model. No Go reader for the product-quantized format, and no CC BY-SA obligation. fastText only wins on English 6-12 characters (+1.6 points). Its one advantage, 176 languages without retraining, stays open: adding a language to ours is one data file plus one trainer run. Remaining steps 4 (calibration, previous-line smoothing) and 5 (wider corpus, docs).

Step 4 done 2026-11-07. Calibration: the linear model is trained on log loss, so its softmax is already close to calibrated (`TestCalibration`, texts of 20 characters or fewer, held out): confidence 0.9-1.0 is 99.9% right (51351 texts), 0.8-0.9 is 98.1%, 0.7-0.8 is 94.8%, 0.6-0.7 is 85.0%, 0.5-0.6 is 64.5%, 0.4-0.5 is 55.2%. Slightly underconfident above 0.6 and overconfident below 0.5, so no temperature fit; the 0.5 report threshold (`minProb`) stays. Smoothing: a tokenizer line the detector cannot classify now takes the previous line's language, then the text's (`options.merge`, test in `unigram_test.go`). Hover still falls back to the whole document; it classifies one line on demand, so previous-line smoothing there would need document line history (not done).

Remaining: step 5 (wider corpus: Wikipedia, UD treebanks, text with code and URLs; look-alikes es/pt/gl/ca, zh simplified vs traditional; document the trainer in `docs/adding-a-language.md`, update `docs/settings.md`). Needs data downloads.

Step 5 done 2026-11-07. Data: Tatoeba for 13 languages plus UD train-file text (`# text =` lines) for 13 of them, never the test files. New classes: Catalan (look-alike of es/fr), Ukrainian (ru), Korean. Training also adds a URL, number or code snippet to 15% of samples (`-noise`). Result: `linear.bin` 1.7 MB (14 classes were tried; Galician dropped, see below). Out-of-domain check (`TestUDOutOfDomain`, UD test sentences, never seen in training; abstentions count as wrong): fr 99.5, es 98.7, de 97.7, it 99.6, pt 97.9, ru 100, ja 98.7, zh 99.8, ar 100, ca 98.8, uk 94.2, ko 99.9, en 88.7. English is the weak one: EWT is web text (headings, emails, names), 188 of 2077 abstain and the rest go to de/it/fr; before this step it scored 86.2. Held-out Tatoeba: all original languages unchanged within 0.3 points, except ru 6-12 characters (100 to 95.0) and 13-20 (100 to 98.7), caused by Ukrainian; ca 93.4 overall (Catalan Tatoeba is small; test bar is 90%), uk 99.3, ko 97.4 (errors are 1-5 character texts).

Galician: with `gl` in (Tatoeba plus UD, about 10k sentences) held-out accuracy was 64% on Tatoeba and 74% on UD, mostly mistaken for es/pt, and es lost 0.1 points; dropped. Simplified versus traditional Chinese stays one class (`zh`), as both use the same models and scripts. Not done: Wikipedia text (large downloads; UD gave the out-of-domain test instead), and `docs/settings.md` accuracy ranges are rounded.

Language-id plan complete except the optional removal of `model.bin` (the naive Bayes fallback, 288 KB).

Naive Bayes removed 2026-11-07: `model.bin`, its loader and the `-format` trainer flag are gone; `linear.bin` is the only model (trainer defaults now match the shipped settings). Earlier numbers in this plan that mention `model.bin` describe the baseline.
