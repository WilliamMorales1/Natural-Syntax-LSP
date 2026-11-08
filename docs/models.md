# Models

| Setup name (`--model`, from `models.json`) | `naturalSyntaxLs.model` | Size | Mode | Languages | Notes |
| ---------------------- | ----------------------- | ---- | ---- | --------- | ----- |
| `bert-base` (default) | `bert-base` | ~430 MB | POS | en | Higher accuracy (multilingual vocab inflates size) |
| `mobilebert` | `mobilebert` | ~105 MB | POS | en | Faster startup |
| `xlmr-es` | `xlmr-es` | ~280 MB | POS | es | XLM-R UPOS, int8; 96.7% on AnCora |
| `xlmr-fr` | `xlmr-fr` | ~280 MB | POS | fr | XLM-R UPOS, int8; 96.2% on GSD |
| `xlmr-ru` | `xlmr-ru` | ~280 MB | POS | ru | XLM-R UPOS (Apache-2.0), int8; 93.0% on GSD (fp32 93.5%) |
| `xlmr-ja` | `xlmr-ja` | ~280 MB | POS | ja | XLM-R UPOS, int8; 96.2% on GSD with kagome words (fp32 97.1%) |
| `chinese-roberta-upos` | `chinese-roberta-upos` | ~410 MB | POS | zh | `KoichiYasuoka/chinese-roberta-base-upos` (Apache-2.0), whole words; 91.6% of characters on GSDSimp, fp32 only |
| `multilingual-minilm` (default for semantic) | `multilingual-minilm` | ~470 MB | Semantic | 50+ | `paraphrase-multilingual-MiniLM-L12-v2`, 384-dim |
| `en_ewt.electra-base` (default for dependency) | `en_ewt.electra-base` | ~470 MB | Dependency | en | diaparser biaffine UD parser (ELECTRA-base encoder) |
| `es_ancora.mbert` | `es_ancora.mbert` | ~750 MB | Dependency | es | fp32 only; int8 drops UAS from 91 to 42 |
| `fr_sequoia.camembert-large` | `fr_sequoia.camembert-large` | ~350 MB | Dependency | fr | int8; UAS/LAS 90.5 / 86.0 |
| `ru_syntagrus.DeepPavlov` | `ru_syntagrus.DeepPavlov` | ~750 MB | Dependency | ru | fp32 only; UAS/LAS 90.8 / 80.4 on GSD (int8 81.8 / 67.8) |

The former `all-mpnet-base-v2` and `all-MiniLM-L6-v2` semantic models were replaced by the multilingual one: about half the memory of mpnet and 2-3x faster per edit.

One setting, `naturalSyntaxLs.model`, picks the model for whichever mode is active. Its meaning depends on `naturalSyntaxLs.mode`: a POS model name for `pos`, an embedding model name for `semantic`, or a diaparser catalog name for `dependency`. Model files on disk (`models/{slug}/{slug}.onnx` and `{slug}_vocab.txt` or `{slug}_tokenizer.json`, or `models/{slug}_dependency/{slug}_dependency.onnx` and its tokenizer file) are derived from this value using the same slug rule as `export_model.py` (`-`, `.`, `/` → `_`), so it must match what you exported. `export_model.py --int8` quantizes in place.

Export commands: [install.md](install.md#exporting-models).

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
| es, fr, ru | Dependency | `--mode dependency --model es_ancora.mbert`, `fr_sequoia.camembert-large` or `ru_syntagrus.DeepPavlov` (needs `diaparser`) |

Japanese text that contains kana is split into UniDic short-unit words by [kagome](https://github.com/ikawaha/kagome) (the word standard of UD Japanese-GSD), so the Japanese POS model tags real words: 97.1% of characters on GSD, up from 74.4% with tokenizer pieces. kagome's dictionary is embedded in the binary (+37 MB) and decoded on the first Japanese text: start-up goes from 1.1 s to 3.2 s and RSS from about 450 MB to about 1 GB, and edits are 40-50% slower (int8: 137 vs 94 ms). int8 Japanese POS keeps 96.2% (fp32 97.1%). Build with `-tags nojaseg` to leave it out; Japanese then falls back to tokenizer pieces. Kanji-only text cannot be told from Chinese and also uses pieces.

The XLM-R Chinese model tags the multi-character pieces of its SentencePiece vocabulary, which can fuse a particle or preposition with its noun (79.2% of characters on GSDSimp). Prefer `chinese-roberta-upos` (`KoichiYasuoka/chinese-roberta-base-upos`): it tags each character as `B-`, `I-` or a plain UPOS label, and the server joins `I-` characters to the word before them, so tokens are whole words (`北京`, `大学`, `自然`, `语言`) and 91.6% of characters get the right tag. The vocab file is found next to the model. int8 (`scripts/quantize_model.py`) cuts it from 438 MB to 104 MB and RSS from 397 to 218 MB, but accuracy drops to 86.5%, so keep fp32 unless memory is tight. Neovim edit latency: 108 ms fp32, 81 ms int8. Traditional Chinese scored the same in the Python test (91.1% word level). No Japanese or Chinese dependency parser is wired up yet.
