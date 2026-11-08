# Models

One setting, `naturalSyntaxLs.model` (`-model` for the server), picks the model for the active mode. Export with `scripts/setup.sh --model <setup name>` ([install.md](install.md#export-models)). The catalog lives in `data/models.json`.

## Catalog

| Setup name | `naturalSyntaxLs.model` | Size | Mode | Languages | Notes |
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
| `zh_ud.mbert` | `zh_ud.mbert` | ~250 MB | Dependency | zh | Character-level diaparser trained here (CC BY-SA 4.0, [Hydragon123/zh-ud-diaparser](https://huggingface.co/Hydragon123/zh-ud-diaparser)); partial int8; UAS/LAS 85.9 / 81.9 on the full GSD test set (500 sentences), fp32 86.9 / 83.3 |
| `pt_ud.bertimbau` | `pt_ud.bertimbau` | ~125 MB | Dependency | pt | Trained here on Bosque and GSD (CC BY-SA 4.0, [Hydragon123/pt-ud-diaparser](https://huggingface.co/Hydragon123/pt-ud-diaparser)); int8; UAS/LAS 92.1 / 87.8 on the full GSD test set (1200 sentences), fp32 92.6 / 89.0 |
| `ja_gsd.mbert` | `ja_gsd.mbert` | ~120 MB | Dependency | ja | int8; UAS/LAS 89.5 / 85.5 on the full GSD test set (543 sentences) with kagome words. Needs `fugashi` and `unidic-lite` at export time |

Accuracy is on UD test treebanks; method in [benchmarks.md](benchmarks.md). Size and accuracy trade-offs per language: [design/languages.md](design/languages.md).

### Dependency export differs from upstream diaparser

`scripts/export_model.py` rebuilds each diaparser parser for ONNX. Two changes: the custom LSTM is replaced by a stock `nn.LSTM` with the same weights (identical output without padding), and the optional attention-mixing term (`use_attentions`) is left out. The second change means the exported parser is not numerically identical to the PyTorch one for models trained with that term. The UAS/LAS figures above are measured on the exported ONNX files, so they include its effect.

## Files on disk

The file name comes from the model name: `-`, `.` and `/` become `_` (the slug).

| Mode | Files |
| ---- | ----- |
| `pos`, `semantic` | `models/<slug>/<slug>.onnx` plus `<slug>_vocab.txt` or `<slug>_tokenizer.json` |
| `dependency` | `models/<slug>_dependency/<slug>_dependency.onnx` plus its tokenizer file |

The name must match what you exported. `export_model.py --int8` quantizes in place.

## Licenses

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
| `pt_ud.bertimbau` | CC BY-SA 4.0 (trained on Portuguese-Bosque and GSD, both CC BY-SA 4.0; encoder `neuralmind/bert-base-portuguese-cased`, MIT) | UD relations |

EAGLES (`NCMS000`) and CKIP (`Na`) labels also decode; `ExampleParseLabel` in `internal/postag` shows every inventory and the `O` fallback, which is logged when a model loads. The CKIP transformers models are GPL-3.0 and are not in the catalog.

## Other models

Anything outside the catalog can be exported by hand:

```bash
python scripts/export_model.py DIR --mode pos|semantic|dependency --model <Hugging Face repo id or diaparser name>
```

Run the server with `-model <path>.onnx`; the tokenizer file is found next to it. `wietsedv/xlm-roberta-base-ft-udpos28-<code>` covers most UD languages for POS.

Tokenizers: WordPiece and SentencePiece Unigram work. SentencePiece BPE (mmBERT, Llama-style) does not.
