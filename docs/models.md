# Models

| Model                           | Size    | Mode       | Notes |
| ------------------------------- | ------- | ---------- | ----- |
| `bert-base-cased` (default)     | ~430 MB | POS        | Higher accuracy (multilingual vocab inflates size) |
| `mobilebert`                    | ~105 MB | POS        | Faster startup |
| `all-mpnet-base-v2` (default)   | ~440 MB | Semantic   | 768-dim, 110M params, best quality |
| `all-MiniLM-L6-v2`              | ~92 MB  | Semantic   | 384-dim, 22M params, faster, lower quality |
| `en_ewt.electra-base` (default) | ~470 MB | Dependency | diaparser biaffine UD parser (ELECTRA-base encoder); pass `--model <diaparser-catalog-name>` to `export_model.py` for other languages/corpora |

One setting, `naturalSyntaxLs.model`, picks the model for whichever mode is active. Its meaning depends on `naturalSyntaxLs.mode`: a POS model name for `pos`, an embedding model name for `semantic`, or a diaparser catalog name for `dependency`. Model file names on disk (`{slug}.onnx` / `{slug}_vocab.txt`, or `{slug}_dependency.onnx` / `{slug}_dependency_vocab.txt`) are derived from this value using the same slug rule as `export_model.py` (`-`, `.`, `/` → `_`), so it must match what you exported.

Export commands: [install.md](install.md#exporting-models).
