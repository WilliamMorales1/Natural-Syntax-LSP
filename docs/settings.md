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
| `naturalSyntaxLs.wiktionaryLanguage`    | `"en"`                      | Hover definition language: a code (`en`, `es`, `fr`, `ja`, `zh`, ...), `model` (model's language, else `en`), or `auto` (best across languages). Read at server start |
| `naturalSyntaxLs.langDir`               | `""`                        | Extra directory of language `*.lua` scripts (see `docs/adding-a-language.md`). Read at server start |
| `naturalSyntaxLs.semanticLightness`     | `0.75`                      | OKLCH lightness for semantic colors (0–1); increase for light themes |
| `naturalSyntaxLs.semanticChroma`        | `0.14`                      | OKLCH chroma of the most vivid semantic colors |

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
| `NSLS_SEMANTIC_EVAL=1`, `NSLS_ML_DIR` | `TestSemanticQualityAcrossLanguages` | Word-group separation within and across en, es, fr, ja, zh, ru, ar |
| `NSLS_NVIM_BENCH=1`, `NSLS_ML_DIR` | `internal/e2e` `TestNvimBench` | Startup, open, edit and hover latency and server memory, through a real headless Neovim; `NSLS_BENCH_ONLY` filters cases |

`go test -short -race ./...` is the CI check (`.github/workflows/ci.yml`). Fuzz targets run with `go test ./internal/lspserver -run xxx -fuzz FuzzApplyContentChange`; others are `FuzzParseCPUList` (`internal/inference`) and `FuzzStripHTML` (`internal/wiktionary`).

Run heavy ones under `oomguard run -- env VAR=... go test ...` on a small machine. Results: [models.md](models.md).
