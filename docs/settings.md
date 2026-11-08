# Settings

## VS Code

| Setting | Default | Description |
| ------- | ------- | ----------- |
| `naturalSyntaxLs.mode` | `pos` | `pos`, `semantic` or `dependency` |
| `naturalSyntaxLs.model` | `bert-base` | Model for the active mode, e.g. `xlmr-es`, `multilingual-minilm`, `fr_sequoia.camembert-large`. See [models.md](models.md) |
| `naturalSyntaxLs.filetypes` | `["plaintext", "markdown"]` | Language IDs to activate on |
| `naturalSyntaxLs.serverPath` | `""` (bundled) | Path to the server binary |
| `naturalSyntaxLs.scoreThreshold` | `null` (0.333) | Minimum confidence to highlight (POS) |
| `naturalSyntaxLs.tokenMapUpdate` | `{}` | Override Penn tag to token type mappings. See [tags.md](tags.md) |
| `naturalSyntaxLs.wiktionaryDefinitions` | `true` | Show Wiktionary definitions in hover |
| `naturalSyntaxLs.wiktionaryLanguage` | `"detect,en"` | Definition language. See [features.md](features.md#hover-definitions). Applies immediately |
| `naturalSyntaxLs.langDir` | `""` | Extra directory of language `*.lua` scripts. Read at server start |
| `naturalSyntaxLs.semanticLightness` | `0.75` | OKLCH lightness of semantic colors (0-1) |
| `naturalSyntaxLs.semanticChroma` | `0.14` | OKLCH chroma of the most vivid semantic colors |

## Server flags

`natural-syntax-ls [flags]`. Mode and model are fixed at start.

| Flag | Default | Description |
| ---- | ------- | ----------- |
| `-mode` | `pos` | `pos`, `semantic` or `dependency`. Unknown values exit with an error |
| `-model` | default POS model | Path to a `.onnx` file. Pass it whenever `-mode` is not `pos` (unless `-lang` is set) |
| `-vocab` | next to model | Path to the model's `_vocab.txt` or `_tokenizer.json` |
| `-lang` | `""` (detect) | Force one language; picks its model from `models.json` when `-model` is unset |
| `-lang-dir` | `""` | Extra language script directory, loaded last |
| `-idle-unload` | `5m` | Free a model's weights after this long unused (`0` = never) |
| `-threads` | `0` | ONNX Runtime threads per chunk. `0`: physical cores, capped at 4; leftover cores run chunks in parallel |
| `-stdio` | | Accepted for client compatibility; stdio is always used |

## Initialization options

Clients other than VS Code pass these in `initializationOptions`:

| Option | Same as |
| ------ | ------- |
| `wiktionary_language` | `wiktionaryLanguage` |
| `wiktionary_definitions` | `wiktionaryDefinitions` |
| `score_threshold` | `scoreThreshold` |
| `token_map_update` | `tokenMapUpdate` |
| `semantic_lightness` | `semanticLightness` |
| `semantic_chroma` | `semanticChroma` |

## Environment

| Variable | Effect |
| -------- | ------ |
| `NLS_LOG=debug` | Debug logging |
| `ORT_LIB_PATH` | ONNX Runtime library location |
| `XDG_CONFIG_HOME` | Base of the config directory |

Test-only variables: [contributing/testing.md](contributing/testing.md).

## Logging

The server logs to stderr through `log/slog`: warnings and errors by default, debug with `NLS_LOG=debug`. `exit` without a prior `shutdown` exits with code 1, per the LSP spec.
