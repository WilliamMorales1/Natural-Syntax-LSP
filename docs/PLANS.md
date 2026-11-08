# Open work

Only unfinished items are listed. Shipped behavior and measurements live in `docs/models.md`, `docs/settings.md` and `docs/adding-a-language.md`.

## Order of work

After finishing each item: run `go test ./...`, commit it, then start the next one without waiting. Drop the item from this file in the same commit.

Blocked-looking items are not blocked: Windows is checked by the GitHub Actions workflow (`.github/workflows/ci.yml`: `setup.ps1`, language scripts, models, tokenizer, server smoke test; green on the `multilingual` branch), heavy exports run on Colab, VS Code is installed.

1. Documentation as testable examples.

## Multilingual

Constraints that still apply: tokenization follows the model's own tokenizer; SentencePiece BPE (mmBERT, Llama-style) is unsupported; `naturalSyntaxLs.model` is the only selector.

### Quality gaps

- Semantic mode colors katakana words whole (キツネ, コンピューター); UniDic short units still split compounds such as スマートフォン into スマート and フォン, matching UD Japanese-GSD.
- zh dependency int8 still loses accuracy (full GSD, UAS/LAS): fp32 86.9 / 83.3; dynamic int8 of the encoder only 82.6 / 77.7; per-channel (all or encoder only) collapses to 8 / 1, so ONNX Runtime's per-channel path is unusable here. Not shipped. Untried: static quantization with calibration, or int8 on the feed-forward layers only.

### Definition lookup

- Hover in Neovim resolves ja and zh words; kagome and the zh model already split `私は` and `学生们的`, so the ja lemma fallback and zh prefix trimming only cover pieces that still carry a particle or suffix (unit-tested, not seen in an editor). zh has no lemmatizer; `-tags nojaseg` builds have no ja lemmas.

### Export and setup


### Validation

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

- Russian misses left are proper nouns; noun and verb inflections have no lemmatizer (adjectives do, `data/lang/russian.lua`). German compounds and inflected verbs miss the same way.
- Arabic clitic splitting falsely splits rare words and names starting with و ف ب ل ك (dev boundary precision 82.6%); a lexicon or a small trained splitter would cut that. Hover misses forms whose hamza seat changes before a suffix (`دمائها` gives `دمائ`).

## New-language workflow

- Model switching follows the whole document's language; a mixed-language file gets one model.
- Arabic hover strips tashkeel, the article and one-letter proclitics (`data/lang/arabic.lua`); inflected forms still miss.
- Semantic mode has no POS, so a homograph gets its page's first non-variant section (`faulen` in `faulen Hund` shows the verb "to rot").
- No Portuguese diaparser parser found; dependency mode has no pt model.

## Performance

- Replace nested for-loops in hot paths (inference pre/post-processing, decoding, span mapping) with matrix multiplication or batched/concurrent equivalents where the result stays identical. Measure ms and RSS before and after with the existing bench.
- Run no model or highlighting until language identification has settled on a document's language. Today work done before identification is wasted; skipping it should save RSS (no model loaded for the wrong language) and startup ms.

## Documentation as testable examples

Move example and reference material that restates code into Go testable examples (`func ExampleXxx` in `example_test.go`, with `// Output:` comments; see https://go.dev/blog/examples). `go test ./...` then checks it, and `go doc` / pkg.go.dev show it beside the API. Replace each moved doc section with a one-line pointer to the example. Keep prose that explains why or covers non-Go users (Lua authors, editor setup).

- `docs/tags.md` POS and UD tables duplicate `internal/tokenmap/pos_bits.go` and `deprel_bits.go`. Add `ExampleDeprelBits` and a POS example in `internal/tokenmap` printing tag, class and token type for a few rows; generate or test the full tables against `tokenmap.NewDefault()` so they cannot drift.
- `docs/adding-a-language.md` "Schema" Lua block and "Worked example: French": add `ExampleParse` in `internal/lang/luaload` that parses a minimal script (Spanish-style) and one with `tokenizer.elisions`, printing the resulting `lang.Language` fields and a split of `l'ami`. Keep the field table; point the schema to the example.
- `docs/adding-a-language.md` "Decide what the language needs" names `WordMerger`, `Segmenter`, `Forms`, `Rewrite`, `Wikitext`, `Script` as Go fields; check they still match `lang.Language` and move the Go side to an `ExampleLanguage` in `internal/lang`. Lua authors keep the Lua-field wording.
- `docs/adding-a-language.md` "Sentence boundaries": add an example in `internal/inference` splitting `Hi. 「はい。」 ok؟ next` to show terminators and trailing closers.
- `internal/postag`: `ExampleParseLabel` covering Penn, UPOS, `B-`/`I-` prefixes and an unknown label decoding as `O` (the silent failure noted in "Decide what the language needs").
- `internal/tokenizer`: example for elision merge and Arabic clitic split on a tiny in-memory vocabulary, replacing prose examples in `docs/models.md` where they only show input and output.
- Leave as prose: `docs/semantic-colors.md` derivation, install, Neovim and VS Code setup, settings, benchmark tables.
- Verify with `go test ./...` and `go doc ./internal/<pkg>`; examples need no model files, so they run in CI on all three runners.

## Language identification

- Wikipedia text was not used for training.
- `docs/settings.md` accuracy ranges are rounded.
- 6-12 character accuracy for Latin-script languages is 87-94%.
