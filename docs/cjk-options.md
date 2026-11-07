# Chinese and Japanese options

Status: research notes, nothing implemented. Items marked (unverified) come from model names or memory, not from a run.

## Root problem

Current ja/zh models (`wietsedv/xlm-roberta-base-ft-udpos28-{ja,zh}`) were trained on UD data split into words. Server feeds raw unsegmented text, so the Unigram tokenizer invents pieces (`私は`, `在北京`, `キ ツ ネ`). Results: POS 74% (ja) and 79% (zh) against 96% for es/fr. Same cause hurts semantic mode (katakana) and Wiktionary lookup.

Fix order: get real words first, then pick models. Every option below is either a way to get words or a model that consumes them.

## Measured results (2026-11-07)

Colab CPU, first 300 UD test sentences (3-60 words), input is the joined text with no spaces. Seg F1 = exact word span match. UPOS acc = share of gold words whose span and tag both match. Script: not kept in repo; logic is span match on character offsets.

| Lang / treebank | Case | Seg F1 | UPOS acc | ms/sent (CPU) |
| --------------- | ---- | ------ | -------- | ------------- |
| ja GSD | XLM-R ja, raw pieces (current) | 50.3 | 26.4 | 166 |
| ja GSD | XLM-R ja, gold words | 100 | 98.0 | 242 |
| ja GSD | XLM-R ja, fugashi + unidic-lite words | 99.2 | 97.3 | 242 |
| ja GSD | Yasuoka roberta-small-japanese-char-luw-upos | 73.7 | 63.1 | 29 |
| ja GSD | Yasuoka roberta-base-japanese-char-luw-upos | 75.0 | 65.9 | 196 |
| ja GSD | Yasuoka modernbert-small-japanese-char-luw-upos | 74.2 | 65.2 | 45 |
| ja GSD | Yasuoka bert-base-japanese-upos | 56.2 | 41.7 | 137 |
| zh GSDSimp | XLM-R zh, raw pieces (current) | 61.2 | 40.9 | 173 |
| zh GSDSimp | XLM-R zh, gold words | 100 | 92.5 | 246 |
| zh GSDSimp | XLM-R zh, jieba words | 80.3 | 70.5 | 239 |
| zh GSDSimp | Yasuoka chinese-roberta-base-upos | 98.3 | 91.1 | 196 |
| zh GSDSimp | Yasuoka deberta-base-chinese-upos | 98.1 | 91.0 | 797 |
| zh GSDSimp | ckiplab albert-tiny-chinese-ws | 68.8 | n/a | 15 |
| zh GSDSimp | ckiplab bert-tiny-chinese-ws | 60.0 | n/a | 15 |
| zh GSD (Trad.) | XLM-R zh raw / gold / jieba | 62.0 / 100 / 75.0 | 41.6 / 92.6 / 66.4 | |
| zh GSD (Trad.) | Yasuoka chinese-roberta-base-upos | 98.3 | 91.1 | 194 |
| zh GSD (Trad.) | ckiplab albert-tiny-chinese-ws | 80.9 | n/a | 15 |
| segmenter only | fugashi (MeCab) + unidic-lite on ja GSD | 99.2 | | |
| segmenter only | jieba on zh GSDSimp / GSD | 80.3 / 75.0 | | |

Conclusions:
- Word-level scoring is much harsher than the per-character scores in PLANS.md (74% / 79%). Current ja/zh pipeline gets 26% / 41% of words exactly right.
- ja: confirmed. UniDic segmentation + existing XLM-R UPOS model gives 97.3%, within 0.7 of the gold-word ceiling. The Yasuoka LUW models score low here because LUW words differ from GSD short-unit words (73-75 seg F1), not because they are bad taggers.
- zh: Yasuoka `chinese-roberta-base-upos` is the best joint option (98.3 F1, 91.1 UPOS) and needs no new Go dependency. jieba segmentation loses over 20 UPOS points. CKIP tiny is fast but its standard differs from UD.
- Caveats: the ja segmenter is MeCab with `unidic-lite` (Python), used as a stand-in for kagome with UniDic. kagome output was not run. `unidic-lite` is 2.1.2, the same release kagome-dict-uni builds from per its README. Subset of 300 sentences, CPU timings only, no int8 test. Not tested: gse, BudouX, diaparser ja/zh, semantic models (E1-E6).

Literature checked:
- Omura and Wakasa, "Word Delimitation Issues in UD Japanese" (UDW 2021, aclanthology 2021.udw-1.12): GSD uses UniDic short-unit-word delimitation; MeCab+UniDic reaches 96.84% delimitation accuracy; UPOS with gold segmentation 96.8-97.4% and 89.3% from raw text in UDPipe 2.5. Consistent with the table above.
- Ruri (arXiv 2409.07737, J. NLP 33(2)): v3 drops MeCab pre-tokenization, uses a 100K-token SentencePiece vocabulary on ModernBERT-Ja; 30m model has 256-dim output.
- Model2Vec / potion-multilingual-128M: distilled from BGE-M3 on 101 languages. Quantized ports exist (int8, ONNX); an 512 MB fp32 size was reported for one port, too large to ship without pruning.
- Joint Chinese segmentation + POS via span labeling (arXiv 2112.09488) and Word Segmentation for Asian Languages (arXiv 2407.19400) are possible reading for a self-trained model; not evaluated.
- CKIP Transformers ACL demo paper not found by search; accuracy figures there unverified.

### Kagome (Go, pure) on UD Japanese-GSD, 300 sentences

Seg F1, same metric as above. Program: `~/kagome-eval/main.go` (scratch, not in repo). kagome v2.11.0, X280.

| Dictionary | Seg F1 | Tokenize | Heap after load | Notes |
| ---------- | ------ | -------- | --------------- | ----- |
| kagome-dict/uni (UniDic) | 97.6 | 0.05 ms/sent | 366 MB | fugashi + unidic-lite scored 99.2, so kagome UniDic loses 1.6 points |
| kagome-dict/ipa (IPADIC) | 92.2 | 0.03 ms/sent | 88 MB | IPADIC word standard differs from GSD |

- Binary with both dictionaries embedded: 61 MB. Dictionary load time rounds to 0 s (dictionary is a static in-memory structure), so cost is RSS, not start-up.
- Expected UPOS with kagome UniDic words: below the 97.3% measured with fugashi; not measured end to end.
- 366 MB heap for UniDic is large next to the 400-560 MB RSS of the POS models. Check RSS of a real binary before committing; IPADIC at 88 MB loses 5 F1.

### Semantic candidates (24 target words, 4 groups; T4 GPU)

Same word lists as `semantic_eval_test.go`. `sep` and `grp` as defined there (colsep not computed). contextual = mean of token states overlapping the word's true character span in the full paragraph. isolated = each word encoded alone with the model's sentence pooling. static = no context. Script: not kept in repo.

| Model | ja (katakana) sep / grp | ja2 (kanji) | zh | en / es / fr (grp) |
| ----- | ----------------------- | ----------- | -- | ------------------ |
| MiniLM-L12 multilingual, contextual | 1.36 / 1.00 | 1.26 / 1.00 | 1.35 / 1.00 | 1.00 / 0.96 / 1.00 |
| MiniLM-L12 multilingual, isolated | 1.17 / 0.79 | 1.16 / 0.79 | 1.20 / 1.00 | |
| ruri-v3-30m, contextual | 1.14 / 1.00 | 1.11 / 0.79 | | |
| ruri-v3-30m, isolated | 1.10 / 0.96 | 1.06 / 0.83 | | |
| bge-small-zh-v1.5, contextual | | | 1.21 / 1.00 | |
| bge-small-zh-v1.5, isolated | | | 1.11 / 0.96 | |
| static-embedding-japanese | 1.03 / 0.79 | 1.02 / 0.71 | | en 0.42 |
| potion-multilingual-128M (static) | 1.05 / 0.58 | 1.06 / 0.79 | 1.06 / 0.88 | 0.88 / 0.75 / 0.71 |
| LaBSE, isolated | 1.16 / 0.96 | 1.20 / 0.96 | 1.23 / 0.96 | 1.00 / 1.00 / 0.88 |

Read with care:
- Root cause of the Go test's low ja/zh scores, found and reproduced (2026-11-07). `TestSemanticQualityAcrossLanguages` run locally matches `docs/models.md` exactly (ja2 0.79, zh 0.88). The server's word list for ja is single-character fragments (`キ | ツ | ネ | は | 古い | コンピュータ | ー ...`, from Viterbi over each Han/kana run), and `tokenizer.EncodeWords` puts a word-start marker (`▁`) before every word, so the model reads `▁キ ▁ツ ▁ネ`. Colab ablation with MiniLM and the exact server word list: as separate words ja 0.17, ja2 0.79, zh 0.875 (identical to Go); same words over the natural text with no inserted markers ja 1.00, ja2 1.00, zh 1.00. So the model and vocabulary are fine. The fault is the input encoding: for models that were not trained on pre-split words (semantic MiniLM), encode the original text once and only use word spans to pool.
- The same encoding matters for POS: XLM-R UPOS models were trained on real words with a marker per word, so there the fix is real words (kagome/Yasuoka), not pieces.
- Differences between models inside one row group are small on 24 words (one word = 0.04 grp). Only gaps of 0.15+ are meaningful.
- Static models (potion, static-embedding-japanese) are clearly worse than contextual ones. static-embedding-japanese is English-poor (0.42) and mediocre on Japanese (0.79).
- ruri-v3-30m gives no gain over MiniLM contextual on this test. Its isolated-word score on katakana (0.96) beats MiniLM isolated (0.79), which matters only if the server embeds words alone.
- LaBSE isolated is the most even isolated model (0.88-1.00 everywhere) but has the 501k vocabulary the project already rejected for size.
- bge-small-zh is not better than MiniLM contextual (1.00 both). No reason to add it for this metric. colsep, the weaker zh metric, was not measured.

## A. Get word boundaries

| Option | Lang | Cost | Notes |
| ------ | ---- | ---- | ----- |
| A1. kagome v2 + UniDic (`ikawaha/kagome`, `kagome-dict`) | ja | New Go dep, pure Go, MIT. UniDic embeds tens of MB (unverified exact size); IPADIC is smaller | Short-unit words, matches UD Japanese-GSD segmentation. Also gives POS, lemma, reading. Dictionary download could be a setup step like model weights |
| A2. `go-ego/gse` | zh (some ja) | New Go dep, Apache-2.0, pure Go (unverified) | Jieba port: dictionary plus HMM, has POS. Segmentation standard is jieba, not UD-GSD. Last release found: v0.80.3, Aug 2024 |
| A3. Neural char segmenter via existing ONNX path | zh | No new Go code. `ckiplab/albert-tiny-chinese-ws` (~4M params), `bert-tiny-chinese-ws`, GPL-3.0 | B/I tags per char, CKIP standard (Traditional-leaning). Most accurate small option. Reuses `postag.ParseLabel` style decoding |
| A4. Yasuoka char LUW models | ja | No new Go code. `roberta-small-japanese-char-luw-upos` (CC-BY-SA), `modernbert-small-japanese-char-luw-upos` (Apache-2.0) | One model gives segmentation and UPOS together as B-/I- tags per char. Long-unit words: fewer fragments, but compounds like `国立国語研究所` become one unit. ModernBERT needs ONNX export check |
| A5. Self-made dictionary Viterbi in Go | ja, zh | ~300 lines plus data. Dictionary from CC-CEDICT (zh) and JMdict or Wiktionary headwords (ja) | Longest-match or unigram-frequency lattice. Weak without frequencies. Doubles as definition source (see D). Good fallback with zero model cost |
| A6. BudouX port (`sg0hsmt/budoux-go` or ~60 lines self-made) | ja, zh | Tiny JSON model | Phrase level (bunsetsu-like), not words. Useful for chunking and line breaks, not for POS |
| A7. Self-trained segmenter | ja, zh | Colab training on UD GSD (ja), GSDSimp (zh), export ONNX or Go BiLSTM/CRF | Last resort. A3 and A4 already cover it |

## B. POS given words

B1. Keep the XLM-R UPOS models, change input only. Segment with A1/A2/A3, tokenize each word separately (Metaspace prefix per word, label on first piece), as in training. Expected: close to es/fr (~95%+), unverified. Needs a word-list entry point in `internal/inference` and the Unigram tokenizer. Lowest effort, highest return. Test first on 200 GSD sentences using gold UD words, to measure the ceiling before writing segmenter code.

B2. Joint seg+POS models (A4 for ja). One ONNX pass, no Go dependency. Chinese equivalents: `KoichiYasuoka/chinese-roberta-base-upos`, `deberta-base-chinese-upos` (char tokens, B-/I- style, unverified), `ckiplab/albert-tiny-chinese-pos` (CKIP tags, no UPOS, needs mapping, GPL-3.0).

B3. kagome UniDic tags mapped to UPOS in Go, no model. Fast (<5 ms), ~90% on content words, weak on ambiguity. Good "light mode".

## C. Dependency parsing

| Option | Lang | Notes |
| ------ | ---- | ----- |
| C1. diaparser `ja_gsd.mbert` fed kagome words | ja | Model already exportable. Failure today is only MeCab segmentation. kagome UniDic SUW matches GSD. Needs word-list entry in `dependency_model.go` |
| C2. diaparser `zh_ptb.hfl` or CTB7 electra | zh | Stanford labels. Needs label map to UD-like set. Segmentation must match CTB (A3 is not CTB, so mismatch risk) |
| C3. Yasuoka `ud-head` or goeswith models (`esupar`) | ja, zh | Head finding as QA or as extra classification head. Would need a custom decoder in Go. Search of the Hub for these names returned nothing, so availability unverified |
| C4. Rule-based bunsetsu dependencies | ja | Japanese is head-final: each phrase (A6 or kagome grouping) attaches to next phrase with a predicate. ~70-80% on bunsetsu (unverified). No model, no labels beyond coarse case-particle roles |
| C5. Self-trained biaffine on UD ja_gsd / zh_gsdsimp | both | Colab, small encoder (`albert-tiny`, `roberta-small-japanese`). Only if C1/C2 fail |

## D. Definition lookup (Wiktionary gap)

`私は` is not a headword, so hover fails. With real words from section A the problem mostly vanishes. Extra options:
- Lemma lookup: kagome gives base form (`食べた` to `食べる`).
- Offline dictionaries as hover source: CC-CEDICT (CC BY-SA, ~10 MB) for zh, JMdict (CC BY-SA, ~60 MB XML) for ja. Gives definition, reading, pinyin in one local lookup, no network, no Wiktionary section parsing. Needs parsers and a download step.

## E. Semantic mode

| Option | Lang | Notes |
| ------ | ---- | ----- |
| E1. Pre-segment words, embed each word separately in current MiniLM | both | Same word-level input fix. Katakana still splits in the 250k XLM-R vocab, but with isolated words the one-char pieces average sensibly less badly (unverified) |
| E2. `cl-nagoya/ruri-v3-30m` (Apache-2.0, ModernBERT, ~30M params, ONNX build `WariHima/ruri-v3-30m-onnx` exists) | ja | Japanese-native SentencePiece vocab, so katakana stays whole. Small. Needs ModernBERT ONNX run check and a tokenizer.json compatibility check with `unigram.go` (unverified which tokenizer type) |
| E3. `hotchpotch/static-embedding-japanese` (MIT) | ja | Static embeddings: token lookup plus mean, no ONNX. Go implementation ~100 lines (safetensors reader plus tokenizer). Instant start, tiny RAM |
| E4. `minishlab/potion-multilingual-128M` (Model2Vec, MIT) | all | Same static approach, covers zh and ja. 128M params, so large table; prune vocab or int8 |
| E5. `BAAI/bge-small-zh-v1.5` (MIT, 33M params, char-level BERT vocab 21k) | zh | Fixes weak zh color separation (colsep 1.37). Plain BERT export, existing code path. ONNX build `Xenova/bge-small-zh-v1.5` exists |
| E6. Word-vector tables: Tencent AI Lab zh, chiVe (Sudachi) ja, fastText cc.zh/ja | zh, ja | Real word-level vectors pruned to ~100k words x 300d int8 (~30 MB), cosine in Go. Needs segmenter from A. Best fit for "color per word" because no context mixing |

## Suggested order

1. Measure the ceiling: run existing ja/zh UPOS models on gold UD words. Decides whether B1 is worth it.
2. Add word-list input path (words plus offsets) to POS and dependency models.
3. ja: kagome + UniDic (A1), feeding B1 and C1. Lemma also fixes D.
4. zh: A3 (`albert-tiny-chinese-ws`) first for no new Go dep, else A2 `gse`.
5. Semantic: E5 for zh, E2 or E3 for ja.
6. Defer: C2 label mapping, C3, C5, A7.

## Open questions

- Licenses: CKIP is GPL-3.0, Yasuoka ja models mostly CC-BY-SA, UniDic is GPL/LGPL/BSD triple (unverified). Weights are user-downloaded, so repo license is unaffected, but record each in `docs/models.md`.
- Dictionary footprint: kagome UniDic embedded in the binary would bloat releases. Prefer separate download.
- Traditional vs Simplified: CKIP is Traditional-trained, XLM-R zh UD is Simplified GSD. Document coverage.
