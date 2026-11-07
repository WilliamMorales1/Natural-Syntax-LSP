# Natural Syntax LSP

Parts-of-speech, semantic-embedding, or dependency-parse highlighting for VS Code or Neovim via a local Go LSP server running ONNX inference. Hover over any word to see its tag (and, in dependency mode, its relation and dependents) plus a Wiktionary definition.

![screenshot showing color highlighting and hover](docs/screenshotExample.png)

Three modes:

- **POS** (default): BERT/MobileBERT tags each word's part of speech (Spanish and French via XLM-R); your theme picks the colors.
- **Semantic**: a multilingual sentence-embedding model (50+ languages) colors words by meaning in context; words used alike get similar colors.
- **Dependency**: [diaparser](https://github.com/Unipisa/diaparser) UD parser colors words by syntactic relation (`nsubj`, `obj`, `amod`, …), for English, Spanish and French.

Japanese and Chinese POS and all semantic-mode languages work but are experimental: [models.md](docs/models.md).

## Dependencies

- Prebuilt releases: none beyond the editor; ONNX Runtime is bundled.
- Model export: Python with `torch`, `transformers`, `onnx`, `onnxscript`, `onnxruntime`, `requests` (plus `diaparser` for dependency mode). `scripts/setup.sh` installs them.
- Building from source: Go 1.26+ (`github.com/yalue/onnxruntime_go`, needs ONNX Runtime shared library), Node.js and npm for the VS Code extension (`vscode-languageclient`, packaged with `vsce`).
- Neovim 0.11+ for the Neovim plugin.

## Installation

Every install path needs the ONNX model files (100-470 MB each), which no package ships. Export them first: [docs/install.md](docs/install.md#exporting-models).

**VS Code / VSCodium**: download the `.vsix` for your platform from the [latest release](../../releases/latest). It bundles the server and ONNX Runtime (Windows x64, Linux x64, macOS arm64).

```bash
code --install-extension natural-syntax-ls-*.vsix
```

**Standalone binary**: each release has `natural-syntax-ls_<platform>.tar.gz` (`.zip` on Windows) holding the server and ONNX Runtime side by side. `natural-syntax-ls_<platform>_english.*` is an English-only build (about 7 MB binary instead of 50 MB) without Japanese and Chinese support code. Extract anywhere and put the binary on your `PATH`.

**Neovim 0.11+**: the repo is a plugin (LSP config, semantic colors, `:NlsMode`). Needs the server binary on `PATH`. With [lazy.nvim](https://github.com/folke/lazy.nvim):

```lua
{ "WilliamMorales1/Natural-Syntax-LSP", config = function() vim.lsp.enable("natural_syntax_ls") end }
```

## Docs

- [Install from source, export models, troubleshooting](docs/install.md)
- [Models, languages, accuracy, latency and memory](docs/models.md)
- [Settings, colors, testing](docs/settings.md)
- [Neovim manual setup](docs/neovim.md)

## Credits

Chinese Traditional/Simplified lookup uses data derived from [OpenCC](https://github.com/BYVoid/OpenCC) (Apache-2.0). See [NOTICE](NOTICE).
