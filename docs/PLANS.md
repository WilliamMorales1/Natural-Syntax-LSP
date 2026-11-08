# Open work

Only unfinished items are listed. Shipped behavior and measurements live in `docs/models.md`, `docs/settings.md` and `docs/adding-a-language.md`.

## Multilingual

Constraints that still apply: tokenization follows the model's own tokenizer; SentencePiece BPE (mmBERT, Llama-style) is unsupported; `naturalSyntaxLs.model` is the only selector.

### Quality gaps

- Katakana words in semantic mode show one color per piece. Kagome gives whole words but one word was still off.
- ja int8 POS with kagome words: not evaluated.
- ja dependency parser: diaparser `ja_gsd.mbert` on kagome words. Not tried.
- zh dependency parser: `zh_ctb7` uses Stanford labels, so a label mapping is needed.
- `KoichiYasuoka/deberta-base-japanese-wikipedia-luw-upos`: untested (LUW B-/I- tags over char-sized Unigram pieces, would need piece merging after tagging).

### POS labels

- EAGLES and CKIP tag inventories are not mapped onto Universal classes.
- Unknown labels still decode as `O`.

### Definition lookup

- ja/zh pieces such as `私は` find no headword. Needs lemma lookup.

### Export and setup

- Test `setup.ps1` on Windows (no pwsh available so far).
- Hidden size is still inferred from "minilm" in the file name. Use explicit metadata.
- The model slug rule is duplicated in Python, PowerShell, bash and JS.
- Record license and tag inventory per model in `docs/models.md` (XLM-R UPOS models are Apache-2.0, CKIP is GPL-3.0).

### Validation

- Hover returned a result for 18 of 20 positions in semantic mode (English and multilingual). Find out why.
- Benchmark on a platform other than the X280, and with hover on (Wiktionary on).
- VS Code checks: semantic tokens, hover, edits, definitions.
- Run `go test ./...` and the opt-in tests before marking a language supported.

### Acceptance

- Each claimed language has documented model IDs, licenses, export steps and supported modes.
- Spans come from the model's tokenizer; CJK spans are multi-character Unigram pieces for SentencePiece models and per character otherwise, documented as such.
- POS labels are mapped deliberately, with no accidental English fallback.
- Token spans point to correct source characters after multilingual edits.
- Existing English behavior and configuration stay backward compatible.

## Russian and Arabic

- Wiktionary lemma lookup for inflected forms (ru `спортивной`, ar `الأكبر`, `للعدل`). Most misses are inflected forms and proper nouns.
- Arabic clitic segmentation: decision is to leave pieces. Revisit only if a segmenter fits the size and memory budget.

## Lua-defined languages and models

- Model metadata in Lua: labels, UPOS mapping, hidden size, normalization. `export_model.py` should emit `<model>.lua` next to the ONNX file.
- Per-language startup timing (target under 5 ms).
- `Reload` on script file change (optional).
- Windows check.

## New-language workflow (de, it, pt, ar)

- Ship `xlmr-de|it|pt` catalog entries and run the exports. Add the missing Arabic POS row to `docs/models.md`.
- `export_model.py` docstring lists model names by hand, against the "no model name in Python" rule.
- `:NSLang` completion is a static list.
- Auto-detect never switches the model, so a German file under the default setup gets English POS tags. Only `:NSLang` changes models.
- Arabic hover misses the definite article (`المدرسة`) and tashkeel forms (`كَلْب`). Nothing strips `ال` or tashkeel now that `lang/arabic.lua` is removed.
- Semantic mode has no POS, so homographs pick the wrong sense (`amico`, `faulen`, `كلب`).
- `xlm-roberta-base-ft-udpos28-ar` ships only `pytorch_model.bin`; recent torch refuses `torch.load` on it, so the export may fail. Not run.
- Untested diaparser parsers: `de_hdt.dbmdz-bert-base`, `it_isdt.dbmdz-*`. No pt parser found.
- Dependency mode and `:NSLang` have no match for de/it/pt.

## Language identification

- Hover has no previous-line smoothing. Needs document line history.
- Wikipedia text was not used for training.
- `docs/settings.md` accuracy ranges are rounded.
- 6-12 character accuracy for Latin-script languages is 87-94%.
