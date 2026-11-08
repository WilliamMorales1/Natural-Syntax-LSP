# Natural Syntax LSP

Parts-of-speech, semantic-embedding, or dependency-parse highlighting for English, Spanish, French, German, Italian, Portuguese, Russian, Arabic, Japanese and Chinese in VS Code or Neovim, via a local Go LSP server running ONNX inference. Hover over any word to see its tag (and, in dependency mode, its relation and dependents) plus a Wiktionary definition.

![screenshot showing color highlighting and hover](docs/screenshotExample.png)

Three modes:

- **POS** (default): BERT/MobileBERT tags each word's part of speech (other languages via XLM-R or RoBERTa models); your theme picks the colors.
- **Semantic**: a multilingual sentence-embedding model (50+ languages) colors words by meaning in context; words used alike get similar colors.
- **Dependency**: [diaparser](https://github.com/Unipisa/diaparser) UD parser colors words by syntactic relation (`nsubj`, `obj`, `amod`, …), for English, Spanish, French, German, Italian, Russian, Japanese and Chinese.

Per-language models, accuracy and licenses: [models.md](docs/models.md). Semantic mode is measured on en, es, fr, ja and zh. The server detects each line's language, so one document can mix languages; hover definitions follow the detected language.

## Dependencies

- Prebuilt releases: none beyond the editor; ONNX Runtime is bundled.
- Model export: Python 3 with `torch`, `transformers`, `onnx`, `onnxscript`, `onnxruntime`, `requests` (plus `diaparser` for dependency mode). `scripts/setup.sh` installs them, into a venv, uv or conda env if the system Python is externally managed.
- Building from source: Go 1.26+ with cgo (`github.com/yalue/onnxruntime_go`; `scripts/setup.sh` fetches the ONNX Runtime shared library), Node.js and npm for the VS Code extension (`vscode-languageclient`, packaged with `vsce`).
- Neovim 0.11+ for the Neovim plugin.

## Installation

Every install path needs the ONNX model files (100-750 MB each), which no package ships. Export them first: [docs/install.md](docs/install.md#exporting-models).

**VS Code / VSCodium**: download the `.vsix` for your platform from the [latest release](../../releases/latest). It bundles the server and ONNX Runtime (Windows x64, Linux x64, macOS arm64).

```bash
code --install-extension natural-syntax-ls-*.vsix
```

**Standalone binary**: each release has `natural-syntax-ls_<platform>.tar.gz` (`.zip` on Windows) holding the server (all languages), ONNX Runtime, the language scripts (`lang/`), the model catalog (`models.json`) and `LICENSE` side by side. Extract anywhere and put the binary on your `PATH`; a symlink is fine, the server finds its files beside the real binary.

**Neovim 0.11+**: needs the server binary on `PATH`. The release archive above works. Config snippet (LSP setup, semantic colors, `:NSMode`, `:NSLang`) is in [docs/neovim.md](docs/neovim.md).

## Docs

- [Install from source, export models, troubleshooting](docs/install.md)
- [Models, languages, accuracy, latency and memory](docs/models.md)
- [Settings, language detection, colors, testing](docs/settings.md)
- [POS tag and UD relation color tables](docs/tags.md)
- [Adding a language](docs/adding-a-language.md)
- [Neovim manual setup](docs/neovim.md)
- [How semantic colors work](docs/semantic-colors.md)
- [Open work](docs/PLANS.md)
- [Example texts per language](docs/examples/)

## Credits

Chinese Traditional/Simplified lookup uses data derived from [OpenCC](https://github.com/BYVoid/OpenCC) (Apache-2.0). See [NOTICE](NOTICE).
