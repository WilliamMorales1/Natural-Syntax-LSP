# Testing

## Quick checks

```bash
go test ./...                     # unit tests and Example functions
go test -short -race ./...        # what CI runs (.github/workflows/ci.yml)
```

`Example` functions double as reference docs, e.g. `ExampleParse` in `internal/lang/luaload` and `ExampleParseLabel` in `internal/postag`.

## Manual check

Open [test.txt](../examples/test.txt) after installing. Words color within about 10 s (POS), 5 s (semantic) or 15 s (dependency). [semantic-example.txt](../examples/semantic-example.txt) gives animals, food, technology and feelings their own color families.

In a VS Code extension test, read POS and dependency tokens and hover back with `vscode.provideDocumentSemanticTokens` and `vscode.executeHoverProvider`. Semantic colors are decorations, which the API cannot read; check them by eye.

## Opt-in tests

These need models or a long run, so they skip unless enabled:

| Env | Test | What it checks |
| --- | ---- | -------------- |
| `NSLS_ML_DIR` | `internal/inference` multilingual tests | Sample sentences per language through the exported models |
| `NSLS_EVAL=1`, `NSLS_ML_DIR`, `NSLS_ML_QDIR` | `TestEval` | POS accuracy and dependency UAS/LAS on UD test treebanks (downloaded on first run) |
| `NSLS_WIKT_EVAL=1` | `internal/wiktionary` `TestLiveAccuracy` | Live Wiktionary hits on UD treebank words (`udFiles`) |
| `NSLS_SEMANTIC_EVAL=1`, `NSLS_ML_DIR` | `TestSemanticQualityAcrossLanguages` | Word-group separation within and across languages (en, es, fr, ru, zh, ja) |
| `NSLS_MODEL_COST=1` (or `NLS_TAG_DUMP=DIR`) | `internal/inference` `TestModelCost` | Wall and CPU time per mode for a long document and one sentence; `NLS_TAG_DUMP` also writes each mode's tokens, to diff output across a change |
| `NSLS_NVIM_BENCH=1`, `NSLS_ML_DIR` | `internal/e2e` `TestNvimBench` | Startup, open, edit and hover latency (Wiktionary on; `NSLS_BENCH_WIKT=0` off) and server memory, through a real headless Neovim on Linux, macOS or Windows; `NSLS_BENCH_ONLY` filters cases |

The model tests in `internal/inference` load each installed model once per run and share it across tests.

The model tests load several hundred MB per model; on a small machine, cap memory (for example `systemd-run --user --scope -p MemoryMax=4G go test ...`). Results: [benchmarks.md](../benchmarks.md).

## Fuzzing

```bash
go test ./internal/lspserver -run xxx -fuzz FuzzApplyContentChange
```

Other targets: `FuzzParseCPUList` (`internal/inference`), `FuzzStripHTML` (`internal/wiktionary`).

## Profiling

Where edit time goes (`go test -bench PredictChunk -cpuprofile` in `internal/inference`): over 99% is inside ONNX Runtime. Tokenizing, pooling, decoding and span mapping in Go take about 0.3% (10-20 ms of 3.6 s per 20 dependency runs), so optimizing those loops would not be noticeable. `BenchmarkRegistryOpen` (20 paragraphs, stub model) takes 6 ms, mostly garbage collection.

## Language detector

Retrain on Tatoeba per-language dumps (`<iso3>_sentences.tsv.bz2` from `downloads.tatoeba.org/exports/per_language/<iso3>/`) plus optional extra text, one sentence per line in `<code>.txt` (for example the `# text =` lines of UD treebank train files):

```
go run ./internal/langid/train -data TATOEBA_DIR -extra EXTRA_DIR -out internal/langid/linear.bin -epochs 6 -n 150000 -crop 0.6
```

`NSLS_LANGID_DIR=TATOEBA_DIR go test ./internal/langid -run HeldOut -v` reports held-out accuracy by text length and a confusion matrix; `NSLS_LANGID_UD=DIR` with `<code>.test.txt` files scores out-of-domain text. Add `NSLS_LANGID_LINEAR=FILE` to score a candidate model without installing it. To add a language, add its row to `langs` in `internal/langid/train/main.go` and to the test tables, then retrain.
