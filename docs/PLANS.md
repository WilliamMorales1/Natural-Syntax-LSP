# Open work

Only unfinished items are listed. Shipped behavior and measurements live in `docs/models.md`, `docs/settings.md` and `docs/adding-a-language.md`.

## Order of work

After finishing each item: run `go test ./...`, commit it, then start the next one without waiting. Drop the item from this file in the same commit.

Blocked-looking items are not blocked: Windows is checked by the GitHub Actions workflow (`.github/workflows/ci.yml`: `setup.ps1`, language scripts, models, tokenizer, server smoke test; green on the `multilingual` branch), heavy exports run on Colab, VS Code is installed.

1. Auto-detect never switches the model (German file gets English tags).
2. Wiktionary lemma lookup for ru and ar inflected forms.
3. Lua model metadata: `export_model.py` emits `<model>.lua`; per-language startup timing; `Reload`.
4. New-language workflow: de and it dependency parsers (diaparser), pt parser search.
5. Language identification: previous-line smoothing, docs ranges, short-text accuracy, training data.
6. Benchmark on another platform, with hover on.
7. Semantic homographs without POS; `deberta-luw-upos` test; Arabic clitic segmentation (revisit only if a segmenter fits).

## Multilingual

Constraints that still apply: tokenization follows the model's own tokenizer; SentencePiece BPE (mmBERT, Llama-style) is unsupported; `naturalSyntaxLs.model` is the only selector.

### Quality gaps

- Semantic mode colors katakana words whole (キツネ, コンピューター); UniDic short units still split compounds such as スマートフォン into スマート and フォン, matching UD Japanese-GSD.
- Dependency mode has no POS, so definitions pick senses from the relation alone: ja `私` (nsubj) shows the noun sense "a personal or private matter", `本` and `読み` show rare senses. Use the lemma or a coarse POS guess.
- zh dependency int8 still loses accuracy (full GSD, UAS/LAS): fp32 86.9 / 83.3; dynamic int8 of the encoder only 82.6 / 77.7; per-channel (all or encoder only) collapses to 8 / 1, so ONNX Runtime's per-channel path is unusable here. Not shipped. Untried: static quantization with calibration, or int8 on the feed-forward layers only.
- `KoichiYasuoka/deberta-base-japanese-wikipedia-luw-upos`: untested (LUW B-/I- tags over char-sized Unigram pieces, would need piece merging after tagging).

### Definition lookup

- Hover in Neovim resolves ja and zh words; kagome and the zh model already split `私は` and `学生们的`, so the ja lemma fallback and zh prefix trimming only cover pieces that still carry a particle or suffix (unit-tested, not seen in an editor). zh has no lemmatizer; `-tags nojaseg` builds have no ja lemmas.

### Export and setup


### Validation

- Benchmark on a platform other than the X280, and with hover on (Wiktionary on).
- VS Code (1.138, extension host) passes for ja and zh in pos and dependency modes (semantic tokens, hover with Wiktionary, edits) and for hover in semantic mode. Not observed: semantic-mode color decorations, which the extension draws from a server notification and the API cannot read.
- The extension only attaches to `file` scheme documents, so untitled buffers get no highlighting.
- Opt-in tests (`NSLS_ML_DIR=~/.config/natural-syntax-ls/models`) pass for es, fr, ja POS, semantic and es/fr dependency. Not run: ja, zh and ru dependency (models not exported locally), the `NSLS_EVAL` treebank evals, langid and Unigram reference tests.

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
- `Reload` on script file change (optional).

## New-language workflow (de, it, pt)

- Auto-detect never switches the model, so a German file under the default setup gets English POS tags. Only `:NSLang` changes models.
- Arabic hover now strips tashkeel, the article and one-letter proclitics (`data/lang/arabic.lua`); inflected forms and other clitics still miss.
- Semantic mode has no POS, so homographs pick the wrong sense (`amico`, `faulen`, `كلب`).
- Untested diaparser parsers: `de_hdt.dbmdz-bert-base`, `it_isdt.dbmdz-*`. No pt parser found.
- Dependency mode and `:NSLang` have no match for de/it/pt.

## Language identification

- Hover has no previous-line smoothing. Needs document line history.
- Wikipedia text was not used for training.
- `docs/settings.md` accuracy ranges are rounded.
- 6-12 character accuracy for Latin-script languages is 87-94%.
