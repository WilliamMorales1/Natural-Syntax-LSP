# Language notes

Per-language choices behind the [catalog](../models.md#catalog): segmenters, tokenizer rules, quantization. Adding a language: [contributing/adding-a-language.md](../contributing/adding-a-language.md).

## Japanese

### POS

Japanese text that contains kana is split into UniDic short-unit words by [kagome](https://github.com/ikawaha/kagome) (the word standard of UD Japanese-GSD), so the Japanese POS model tags real words: 97.1% of characters on GSD, against 74.4% when it tags tokenizer pieces. The cost: kagome's dictionary is embedded in the binary (+37 MB) and decoded on the first Japanese text, which adds about 2 s to start-up and about 550 MB of RSS, and edits are 40-50% slower (int8: 137 vs 94 ms). int8 Japanese POS keeps 96.2% (fp32 97.1%). Kanji-only text cannot be told from Chinese and also uses pieces.

### Dependency

Japanese uses the released diaparser `ja_gsd.mbert` (Tohoku BERT, WordPiece). Its config names a mecab word tokenizer, so the WordPiece tokenizer leaves Han/kana runs whole and cuts them with the kagome segmenter, the same words UD Japanese-GSD uses. On the full GSD test set (int8): UAS/LAS 89.5 / 85.5. int8 costs about 0.1 UAS.

### Rejected: long-unit DeBERTa

`KoichiYasuoka/deberta-base-japanese-wikipedia-luw-upos` (CC BY-SA 4.0) was tested as a segmenter-free alternative and not added. It tags long-unit words with `B-`/`I-` labels over Unigram pieces, which the server already joins, and exports cleanly (ONNX matches PyTorch to 6e-6, 499 MB fp32). On the first 300 test sentences, joining `I-` pieces: 95.8% of characters and word F1 93.4 on UD Japanese-GSDLUW (int8 94.9 / 92.5), but 85.0% and F1 71.8 against GSD's short units, which the shipped kagome path (97.1%) and the `ja_gsd` parser use. Its tokenizer also emits an `[UNK]` piece at the start of the text and before `。`.

## Chinese

### POS

The XLM-R Chinese model tags the multi-character pieces of its SentencePiece vocabulary, which can fuse a particle or preposition with its noun (79.2% of characters on GSDSimp). Prefer `chinese-roberta-upos` (`KoichiYasuoka/chinese-roberta-base-upos`): it tags each character as `B-`, `I-` or a plain UPOS label, and the server joins `I-` characters to the word before them, so tokens are whole words (`北京`, `大学`, `自然`, `语言`) and 91.6% of characters get the right tag. The vocab file is found next to the model. int8 (`scripts/quantize_model.py`) cuts it from 438 MB to 104 MB and RSS from 397 to 218 MB, but accuracy drops to 86.5%, so the catalog ships fp32. Neovim edit latency: 108 ms fp32, 81 ms int8. Traditional Chinese text scores about the same.

### Dependency

diaparser publishes no Chinese parser, so `scripts/train_dependency.py` trains one on UD Chinese-GSD (Traditional) and GSDSimp (CC BY-SA 4.0) with `hfl/chinese-roberta-wwm-ext` as encoder (about 20 minutes on a Colab T4). The parser works on characters and finds word boundaries itself: the tokenizer's units (one Han character, one punctuation mark, one letter/digit run) are the parser tokens, and a multi-unit word links each unit but the last to the last with the relation `wp`. The server merges `wp` chains into whole-word tokens, and the last unit supplies the word's head and relation, so no separate segmenter is needed and labels are native UD (no mapping). Training drops about 5% of sentences whose units straddle two gold words. A word counts only when its span matches the gold word. GSD and GSDSimp score within 0.1 of each other. Quantizing every weight costs about 6 UAS. The errors compound in the middle feed-forward layers, so the catalog entry (`int8_only`, `int8_fp32` in `models.json`) quantizes the encoder's projections except the feed-forward layers of blocks 6-8 (full GSD test set, UAS/LAS):

| Weights in int8 | Size | UAS / LAS |
| --------------- | ---- | --------- |
| none (fp32) | 459 MB | 86.9 / 83.3 |
| encoder, FFN of blocks 6-8 fp32 (shipped) | 247 MB | 85.9 / 81.9 |
| encoder blocks 0-5 | 332 MB | 86.0 / 82.2 |
| feed-forward layers only | 289 MB | 82.6 / 77.6 |
| feed-forward of one block (6, 7 or 8) | 444 MB | 86.4-86.8 / 82.5-83.2 |
| feed-forward only, `reduce_range` | 289 MB | 63.4 / 52.1 |

Server RSS falls from 666 to 347 MB. For full accuracy, export without `--int8`. Setup exports it from the Hugging Face repo, which also holds the training script.

## Arabic

UD Arabic-PADT, which the Arabic model was trained on, splits a written word such as `وبالكتاب` into syntactic words (`و`, `ب`, `الكتاب`). Without a split the model tags the whole written word once: 16% of PADT test words carry a clitic, and only 39.8% of their characters got their own word's tag. `tokenizer.arabic_clitics` in `data/lang/arabic.lua` splits off a conjunction (`و ف`), a preposition (`ب ل ك`, `لل` as `ل` plus `ال`), the future `س` and a pronoun suffix, taking a split only where the model's own vocabulary backs it (`ExampleParseUnigram` in `internal/tokenizer`). Marks stay with their letter.

Measured on the PADT sentences whose words align with the text (int8 model, per character, clitic parts scored by their own tag):

| Set | Clitic boundaries P / R | All characters | Clitic-word characters | Clitic-free words |
| --- | ----------------------- | -------------- | ---------------------- | ----------------- |
| test, 312 sentences, rules tuned here | 89.8 / 90.3 | 84.8 to 90.5% | 39.9 to 82.2% | 93.4 to 92.2% |
| dev, held out | 82.6 / 86.7 | 84.8 to 89.2% | 45.2 to 79.4% | |

The cost is false splits of rare words and names that start with `و ف ب ل ك` (`وقائية`, `بريطانيا`), which lowers clitic-free words by about a point. Hover looks up the host word, not the written word with its clitics. Hover forms in `arabic.lua` also restore what a suffix or spelling changes: a seated hamza or `ت` back to `ء` or `ة` (`دمائ` to `دماء`), a dropped hamza on an initial alef (`ادارة` to `إدارة`), tanween alef, tatweel and the sound feminine plural (`جماعات` to `جماعة`). Coverage: [benchmarks.md](../benchmarks.md#dictionary-coverage). Set `arabic_clitics = false` in a copied script to keep written words whole.

## Portuguese

diaparser has no Portuguese parser either. `train_dependency.py --lang pt` trains one on UD Portuguese-Bosque and GSD with BERTimbau (`neuralmind/bert-base-portuguese-cased`, WordPiece), 30 epochs in about 40 minutes on a Colab T4. The server sees written words, so each contraction (`do` = `de` + `o`, `dá-lo`) is one training word that takes the head and relation of its part attached outside it; a written word the tokenizer cuts at a hyphen or other punctuation (`quarta-feira`) is joined back with `wp` as in Chinese. int8 costs 0.5 UAS and 1.2 LAS (fp32 486 MB, int8 124 MB).

## Dependency scoring

Dependency evals (`TestEvalDependency`) score written words the same way, so a contraction (es `del`, fr `du`, it `della`, pt `do`) counts once and sentences with contractions are kept. Scoring the parts separately instead would drop most such sentences; es, fr and de change by under a point either way, while Italian LAS is about 2 points lower on written words.
