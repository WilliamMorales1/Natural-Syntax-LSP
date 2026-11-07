# Models

| Setup name (`--model`) | `naturalSyntaxLs.model` | Size | Mode | Languages | Notes |
| ---------------------- | ----------------------- | ---- | ---- | --------- | ----- |
| `bert-base` (default) | `bert-base` | ~430 MB | POS | en | Higher accuracy (multilingual vocab inflates size) |
| `mobilebert` | `mobilebert` | ~105 MB | POS | en | Faster startup |
| `pos-es` | `xlmr-es` | ~280 MB | POS | es | XLM-R UPOS, int8; 96.7% on AnCora |
| `pos-fr` | `xlmr-fr` | ~280 MB | POS | fr | XLM-R UPOS, int8; 96.2% on GSD |
| `multilingual` (default for semantic) | `multilingual-minilm` | ~470 MB | Semantic | 50+ | `paraphrase-multilingual-MiniLM-L12-v2`, 384-dim |
| `dependency` (default for dependency) | `en_ewt.electra-base` | ~470 MB | Dependency | en | diaparser biaffine UD parser (ELECTRA-base encoder) |
| `dependency-es` | `es_ancora.mbert` | ~750 MB | Dependency | es | fp32 only; int8 drops UAS from 91 to 42 |
| `dependency-fr` | `fr_sequoia.camembert-large` | ~350 MB | Dependency | fr | int8; UAS/LAS 90.5 / 86.0 |

The former `all-mpnet-base-v2` and `all-MiniLM-L6-v2` semantic models were replaced by the multilingual one: about half the memory of mpnet and 2-3x faster per edit.

One setting, `naturalSyntaxLs.model`, picks the model for whichever mode is active. Its meaning depends on `naturalSyntaxLs.mode`: a POS model name for `pos`, an embedding model name for `semantic`, or a diaparser catalog name for `dependency`. Model file names on disk (`{slug}.onnx` and `{slug}_vocab.txt` or `{slug}_tokenizer.json`, or `{slug}_dependency.onnx` and its tokenizer file) are derived from this value using the same slug rule as `export_model.py` (`-`, `.`, `/` → `_`), so it must match what you exported. `export_model.py --int8` quantizes in place.

Export commands: [install.md](install.md#exporting-models).

## Cost on a ThinkPad X280

Measured through Neovim (`internal/e2e`, `NSLS_NVIM_BENCH=1`): time to first result including model load, median one-character edit in a 32-line document, and server RSS.

| Model | Ready | Edit | RSS |
| ----- | ----- | ---- | --- |
| en `bert-base` POS | 0.45 s | 93 ms | 397 MB |
| es / fr POS (int8) | 1.1 s | 62 ms | 450 MB |
| en `mpnet` semantic (removed) | 0.42 s | 90 ms | 385 MB |
| multilingual MiniLM semantic | 0.8 s | 26-43 ms | 203 MB |
| en electra dependency | 0.8 s | 130 ms | 684 MB |
| es mBERT dependency (fp32) | 1.1 s | 130 ms | 919 MB |
| fr CamemBERT dependency (int8) | 1.2 s | 244 ms | 492 MB |

Start-up is slower than English because of the 250k-piece vocabulary and tokenizer load. The French parser is about twice as slow per edit as the English one.

## Semantic quality across languages

`NSLS_SEMANTIC_EVAL=1 NSLS_ML_DIR=... go test ./internal/inference -run TestSemanticQuality -v` (`semantic_eval_test.go`). Each language has a paragraph in the style of [semantic-example.txt](semantic-example.txt): 24 target words in four groups (animals, food, technology, emotions). `sep` is mean between-group over mean within-group embedding distance, `colsep` the same on the colors after the per-document plane fit, `grp` leave-one-out nearest-centroid group accuracy (chance 0.25).

| Model / language | sep | colsep | grp |
| ---------------- | --- | ------ | --- |
| multilingual en | 1.29 | 3.74 | 1.00 |
| multilingual es | 1.32 | 3.73 | 0.96 |
| multilingual fr | 1.30 | 2.41 | 1.00 |
| multilingual zh | 1.24 | 1.37 | 0.88 |
| multilingual ja (katakana words) | 1.02 | 0.93 | 0.17 |
| multilingual ja (kanji words) | 1.16 | 1.05 | 0.79 |
| mpnet en (removed) | 1.29 | 3.95 | 1.00 |
| English MiniLM en (removed) | 1.20 | 4.63 | 1.00 |

Across languages, the nearest word in another language has the right group 90% of the time (en, es, fr, zh at 88-100%; pairs involving ja at 50-96%) and is the actual translation 65% of the time (chance 4%).

- English quality matches mpnet on this test; colors are slightly less separated than with either removed English model.
- es and fr match English. zh is correct but its colors separate less.
- Japanese is unreliable for katakana words: the XLM-R vocabulary splits them into one-character pieces (`キ ツ ネ`), so embeddings cluster by script, not meaning. Words written in kanji work (0.79).

## Other languages (experimental)

The Spanish, French, and multilingual models above have setup names. Anything else (Japanese and Chinese POS, other treebanks) can be exported by hand: use an explicit `--mode` and a Hugging Face repo id; the model name on disk is the repo id with `-`, `.`, `/` → `_`. SentencePiece models ship `{slug}_tokenizer.json` instead of `{slug}_vocab.txt`. Run the server with `-model <path>.onnx`; the tokenizer file is found next to it.

| Language | Mode | Export |
| -------- | ---- | ------ |
| es, fr, ja, zh | POS (UPOS mapped onto the Penn token types) | `python scripts/export_model.py DIR --mode pos --model wietsedv/xlm-roberta-base-ft-udpos28-es` (also `-fr`, `-ja`, `-zh`) |
| all | Semantic | `--mode semantic --model sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2` |
| es, fr | Dependency | `--mode dependency --model es_ancora.mbert` or `fr_sequoia.camembert-large` (needs `diaparser`) |

Japanese and Chinese are split into the multi-character pieces of the model's SentencePiece vocabulary, not single characters. Pieces can fuse a particle or preposition with its noun, so tags there are less reliable than for Spanish and French. No Japanese or Chinese dependency parser is wired up yet. Hover text still shows English Penn descriptions.
