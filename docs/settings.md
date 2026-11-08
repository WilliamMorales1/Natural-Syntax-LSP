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

The server embeds a small language detector (a linear model over hashed character n-grams and words, 1.7 MB, about 11 microseconds per line; en, fr, es, de, it, pt, ru, ja, zh, ar, ca, uk, ko). Nothing needs configuring:

- Hover looks the word up in the language of its line (`detect`), falling back to the whole document's language, then English.
- Language-specific tokenizer rules (French elisions such as `l'ami`) apply only to lines detected as that language. When the language cannot be told, as with very short text, every rule applies.

Retrain on Tatoeba per-language dumps (`<iso3>_sentences.tsv.bz2` from `downloads.tatoeba.org/exports/per_language/<iso3>/`) plus optional extra text, one sentence per line in `<code>.txt` (for example the `# text =` lines of UD treebank train files):

```
go run ./internal/langid/train -data TATOEBA_DIR -extra EXTRA_DIR -out internal/langid/linear.bin -epochs 6 -n 150000 -crop 0.6
```

`NSLS_LANGID_DIR=TATOEBA_DIR go test ./internal/langid -run HeldOut -v` reports held-out accuracy by text length and a confusion matrix (99-100% on whole sentences, 85-99% on texts of 6-12 characters); `NSLS_LANGID_UD=DIR` with `<code>.test.txt` files scores out-of-domain text. Add `NSLS_LANGID_LINEAR=FILE` to score a candidate model without installing it. To add a language, add its row to `langs` in `internal/langid/train/main.go` and to the test tables, then retrain.

## POS mode colors

Tags map to VS Code semantic token types; color comes from your theme. Full list: [tags.md](tags.md#pos-tags).

Hover shows the Part of Speech and a Wiktionary definition (when available).

## Semantic mode colors

Colors are pushed via a custom `$/nls/semanticColors` LSP notification and applied as VS Code `TextEditorDecorationType` decorations (full hex, not theme-limited).

For each document, the server lays every word out on a 2D map, choosing the view where that document's words are most spread out. A word's direction on the map sets its hue and its distance from the middle sets how vivid it is. Words used in similar ways land close together, so they get similar colors. The map is updated as you type and lined up with the previous one, so colors stay steady. Colors are per document and don't depend on your theme.

All colors share one lightness. `semanticLightness` (default 0.75) sets it; increase it for light themes. `semanticChroma` (default 0.14) sets the most vivid words' intensity.

How it works, in depth (the math, the code, and its time and memory costs): [semantic-colors.md](semantic-colors.md).

Hover shows the hex color code and a Wiktionary definition (when available).

## Dependency mode colors

Each word is colored by its Universal Dependencies relation, one relation per token type/modifier pair. Full list: [tags.md](tags.md#ud-relations).

Hover shows the word's relation to its head, then the words that depend on it under `dependents:` (marked `# has dependents` when they have their own), plus a Wiktionary definition (when available).

## Logging

The server logs to stderr through `log/slog`: warnings and errors by default, debug with `NLS_LOG=debug`. An unknown `-mode` value exits with an error. `exit` without a prior `shutdown` exits with code 1, per the LSP spec.

## Testing

Open [test.txt](examples/test.txt) in VS Code after installation. For semantic mode, [semantic-example.txt](examples/semantic-example.txt) shows animals, food, technology and feelings each getting their own color family.

Words color within ~10 seconds (POS), ~5 seconds (semantic) or ~15 seconds (dependency).

### Automated tests

`go test ./...` runs the unit tests. These need models or a long run, so they skip unless enabled:

| Env | Test | What it checks |
| --- | ---- | -------------- |
| `NSLS_ML_DIR` | `internal/inference` multilingual tests | Sample sentences per language through the exported models |
| `NSLS_EVAL=1`, `NSLS_ML_DIR`, `NSLS_ML_QDIR` | `TestEval` | POS accuracy and dependency UAS/LAS on UD test treebanks (downloaded on first run) |
| `NSLS_SEMANTIC_EVAL=1`, `NSLS_ML_DIR` | `TestSemanticQualityAcrossLanguages` | Word-group separation within and across en, es, fr, ja, zh, ru |
| `NSLS_NVIM_BENCH=1`, `NSLS_ML_DIR` | `internal/e2e` `TestNvimBench` | Startup, open, edit and hover latency and server memory, through a real headless Neovim; `NSLS_BENCH_ONLY` filters cases |

`go test -short -race ./...` is the CI check (`.github/workflows/ci.yml`). Fuzz targets run with `go test ./internal/lspserver -run xxx -fuzz FuzzApplyContentChange`; others are `FuzzParseCPUList` (`internal/inference`) and `FuzzStripHTML` (`internal/wiktionary`).

Run heavy ones under `oomguard run -- env VAR=... go test ...` on a small machine. Results: [models.md](models.md).
