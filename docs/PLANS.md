# Open work

Only unfinished items are listed. Shipped behavior and measurements live in `docs/models.md`, `docs/settings.md` and `docs/adding-a-language.md`.

## Order of work

After finishing each item: run `go test ./...`, commit it, then start the next one without waiting. Drop the item from this file in the same commit.

Blocked-looking items are not blocked: Windows checks (10) run in the GitHub Actions workflow (`.github/workflows/ci.yml`), heavy exports (18, 14) run on Colab, VS Code is installed for 9.

1. Particle senses: ja `は`, `が` and zh `的` pick the wrong Wiktionary entry. Small, in the code just touched.
2. Hover returns a result for only 18 of 20 positions in semantic mode. Likely a real bug.
3. Unknown labels decode as `O`; map EAGLES and CKIP tags onto Universal classes.
4. Run `go test ./...` and the opt-in tests before any language counts as supported.
5. Hidden size inferred from "minilm" in the file name: use explicit metadata.
6. Model slug rule duplicated in Python, PowerShell, bash and JS: one source.
7. Record license and tag inventory per model in `docs/models.md`.
8. ja and zh dependency: full GSD test set, then hover and semantic-token check.
9. VS Code checks: semantic tokens, hover, edits, definitions, ja/zh hover.
10. Test `setup.ps1` and the Lua script loading on Windows.
11. Auto-detect never switches the model (German file gets English tags).
12. Arabic hover: strip `ال` and tashkeel.
13. Wiktionary lemma lookup for ru and ar inflected forms.
14. zh dependency int8 loses 5 UAS points: quantize encoder only or per-channel.
15. Evaluate ja int8 POS with kagome words.
16. Katakana pieces in semantic mode show one color per piece.
17. Lua model metadata: `export_model.py` emits `<model>.lua`; per-language startup timing; `Reload`.
18. New-language workflow: de/it/pt catalog and exports, ar POS row, static `:NSLang` list, docstring model names, untested ar export and diaparser parsers.
19. Language identification: previous-line smoothing, docs ranges, short-text accuracy, training data.
20. Benchmark on another platform, with hover on.
21. Semantic homographs without POS; `deberta-luw-upos` test; Arabic clitic segmentation (revisit only if a segmenter fits).

## Multilingual

Constraints that still apply: tokenization follows the model's own tokenizer; SentencePiece BPE (mmBERT, Llama-style) is unsupported; `naturalSyntaxLs.model` is the only selector.

### Quality gaps

- Katakana words in semantic mode show one color per piece. Kagome gives whole words but one word was still off.
- ja int8 POS with kagome words: not evaluated.
- ja and zh dependency: measured on 300 sentences only; run the full GSD test set and a hover/semantic-token check in an editor.
- zh dependency int8 loses about 5 points (UAS 85.6 to 80.0). Try quantizing only the encoder, or per-channel.
- `KoichiYasuoka/deberta-base-japanese-wikipedia-luw-upos`: untested (LUW B-/I- tags over char-sized Unigram pieces, would need piece merging after tagging).

### POS labels

- EAGLES and CKIP tag inventories are not mapped onto Universal classes.
- Unknown labels still decode as `O`.

### Definition lookup

- Hover in Neovim resolves ja and zh words; kagome and the zh model already split `私は` and `学生们的`, so the ja lemma fallback and zh prefix trimming only cover pieces that still carry a particle or suffix (unit-tested, not seen in an editor). zh has no lemmatizer; `-tags nojaseg` builds have no ja lemmas.
- Particles pick the wrong sense: ja `は`, `が` show the kana syllable entry, zh `的` shows "bright; clear". Prefer particle/grammar senses when the POS tag is Adposition or Particle.
- Check ja/zh hover in VS Code.

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
