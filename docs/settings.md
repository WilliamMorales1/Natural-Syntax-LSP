# Settings and colors

## Settings

| Setting                                 | Default                     | Description |
| --------------------------------------- | --------------------------- | ----------- |
| `naturalSyntaxLs.serverPath`            | `""` (bundled binary)       | Path to the Go binary |
| `naturalSyntaxLs.mode`                  | `pos`                       | `pos`, `semantic`, or `dependency` |
| `naturalSyntaxLs.model`                 | `bert-base`                 | Model name; meaning depends on `mode`, e.g. `xlmr-es`, `multilingual-minilm`, `fr_sequoia.camembert-large` (see [models.md](models.md)) |
| `naturalSyntaxLs.filetypes`             | `["plaintext", "markdown"]` | Language IDs to activate on |
| `naturalSyntaxLs.scoreThreshold`        | `null` (0.333)              | Minimum confidence to highlight (POS mode) |
| `naturalSyntaxLs.tokenMapUpdate`        | `{}`                        | Override POS → token type mappings |
| `naturalSyntaxLs.wiktionaryDefinitions` | `true`                      | Show Wiktionary definitions in hover |
| `naturalSyntaxLs.wiktionaryLanguage`    | `"detect,en"`               | Hover definition language: `detect` (language of the hovered line, found by the built-in detector), a code (`en`, `es`, `fr`, `ja`, `zh`, ...), `model` (model's language, else `en`), `auto` (best across languages), or a priority list like `detect,en` (first language with a definition wins). Applies immediately; Neovim: `:NSLang es` |
| `naturalSyntaxLs.langDir`               | `""`                        | Extra directory of language `*.lua` scripts (see `docs/adding-a-language.md`). Read at server start |
| `naturalSyntaxLs.semanticLightness`     | `0.75`                      | OKLCH lightness for semantic colors (0–1); increase for light themes |
| `naturalSyntaxLs.semanticChroma`        | `0.14`                      | OKLCH chroma of the most vivid semantic colors |

## Language detection

The server embeds a 1.7 MB language detector, about 11 microseconds per line; `ExampleDetect` in `internal/langid` lists its languages. Nothing needs configuring:

- Hover looks the word up in the language of its line (`detect`). A line too short to tell borrows the nearest of the five lines above that can; failing that the whole document's language, then English.
- With an English model configured (the default), a document detected as another language is analysed by that language's installed model for the current mode (`models.json`), loaded in the background on first use, alongside the English model for documents open at start (`switch/pos/es` bench: first Spanish tags at about 1.0 s, against 1.3 s loading one after the other); the document shows no tags until it is ready (or until it fails to load, then the English model analyses it). If no open document needs the English model once it has loaded, its weights are freed until one does. At most two extra models are kept, and like the configured one each frees its weights after `-idle-unload` (default 5 minutes) without use, then reloads on the next edit; a text too short to tell keeps the model it had. Any non-English or multilingual `model` setting, and `:NSLang CODE`, disable switching.
- Language-specific tokenizer rules (French elisions such as `l'ami`) apply only to lines detected as that language. When the language cannot be told, as with very short text, every rule applies.

Retrain on Tatoeba per-language dumps (`<iso3>_sentences.tsv.bz2` from `downloads.tatoeba.org/exports/per_language/<iso3>/`) plus optional extra text, one sentence per line in `<code>.txt` (for example the `# text =` lines of UD treebank train files):

```
go run ./internal/langid/train -data TATOEBA_DIR -extra EXTRA_DIR -out internal/langid/linear.bin -epochs 6 -n 150000 -crop 0.6
```

`NSLS_LANGID_DIR=TATOEBA_DIR go test ./internal/langid -run HeldOut -v` reports held-out accuracy by text length and a confusion matrix (99-100% on whole sentences, 85-99% on texts of 6-12 characters); `NSLS_LANGID_UD=DIR` with `<code>.test.txt` files scores out-of-domain text. Add `NSLS_LANGID_LINEAR=FILE` to score a candidate model without installing it. To add a language, add its row to `langs` in `internal/langid/train/main.go` and to the test tables, then retrain.

## Colors

- **POS**: one token type per Universal class, colored by your theme ([tags.md](tags.md#pos-classes)). Hover shows the part of speech.
- **Dependency**: one token type and modifier pair per UD relation ([tags.md](tags.md#ud-relations)). Hover shows the relation to the head and the words under `dependents:` (marked `# has dependents` when they have their own).
- **Semantic**: hex colors pushed in a custom `$/nls/semanticColors` notification and drawn as decorations, independent of theme. Words used alike get similar hues; colors are per document and stay steady as you type. `semanticLightness` sets the shared lightness (raise it for light themes), `semanticChroma` the most vivid words' intensity. Hover shows the hex code. Method and costs: [semantic-colors.md](semantic-colors.md).

Every mode adds a Wiktionary definition to hover when one exists.

## Logging

The server logs to stderr through `log/slog`: warnings and errors by default, debug with `NLS_LOG=debug`. An unknown `-mode` value exits with an error. `exit` without a prior `shutdown` exits with code 1, per the LSP spec.

## Testing

Open [test.txt](examples/test.txt) after installation; words color within about 10 s (POS), 5 s (semantic) or 15 s (dependency). [semantic-example.txt](examples/semantic-example.txt) gives animals, food, technology and feelings their own color families.

### Automated tests

`go test ./...` runs the unit tests and the `Example` functions, which double as reference docs. These need models or a long run, so they skip unless enabled:

| Env | Test | What it checks |
| --- | ---- | -------------- |
| `NSLS_ML_DIR` | `internal/inference` multilingual tests | Sample sentences per language through the exported models |
| `NSLS_EVAL=1`, `NSLS_ML_DIR`, `NSLS_ML_QDIR` | `TestEval` | POS accuracy and dependency UAS/LAS on UD test treebanks (downloaded on first run) |
| `NSLS_WIKT_EVAL=1` | `internal/wiktionary` `TestLiveAccuracy` | Live Wiktionary hits on UD treebank words (`udFiles`) |
| `NSLS_SEMANTIC_EVAL=1`, `NSLS_ML_DIR` | `TestSemanticQualityAcrossLanguages` | Word-group separation within and across languages (the table in models.md covers en, es, fr, zh, ja; ru is in the test but not yet in that table) |
| `NSLS_MODEL_COST=1` (or `NLS_TAG_DUMP=DIR`) | `internal/inference` `TestModelCost` | Wall and CPU time per mode for a long document and one sentence; `NLS_TAG_DUMP` also writes each mode's tokens for before/after diffs |
| `NSLS_NVIM_BENCH=1`, `NSLS_ML_DIR` | `internal/e2e` `TestNvimBench` | Startup, open, edit and hover latency (Wiktionary on; `NSLS_BENCH_WIKT=0` off) and server memory, through a real headless Neovim on Linux, macOS or Windows; `NSLS_BENCH_ONLY` filters cases |

The model tests in `internal/inference` load each installed model once per run and share it across tests.

`go test -short -race ./...` is the CI check (`.github/workflows/ci.yml`). Fuzz targets run with `go test ./internal/lspserver -run xxx -fuzz FuzzApplyContentChange`; others are `FuzzParseCPUList` (`internal/inference`) and `FuzzStripHTML` (`internal/wiktionary`).

Run heavy ones under `oomguard run -- env VAR=... go test ...` on a small machine. Results: [models.md](models.md).
