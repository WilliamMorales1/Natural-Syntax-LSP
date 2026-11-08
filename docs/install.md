# Installation

Every install needs ONNX model files. No package ships them, so export them first.

## Requirements

| To | Needs |
| -- | ----- |
| Use a release | Only the editor. ONNX Runtime is bundled. |
| Export models | Python 3. `scripts/setup.sh` installs `torch`, `transformers`, `onnx`, `onnxscript`, `onnxruntime`, `requests` and `diaparser`, into a venv, uv or conda env if system Python is externally managed. |
| Build the server | Go 1.26+ with cgo. |
| Build the VS Code extension | Node.js and npm. |
| Use Neovim | Neovim 0.11+. |

## Export models

From a repo checkout:

```bash
scripts/setup.sh                    # bert-base POS model (~430 MB)
scripts/setup.sh --model xlmr-es    # any setup name from models.md
scripts/setup.sh --model all        # every model
scripts/setup.sh --list             # print the catalog and exit
```

On Windows: `scripts/setup.ps1 -Model <name>` and `-List`.

The script also:

- downloads the ONNX Runtime library;
- copies `data/lang/` and `data/models.json` into the config dir, keeping files you edited;
- builds the server and installs it to `~/.local/bin` (`$XDG_BIN_HOME` if set; Windows: `%LOCALAPPDATA%\Programs\natural-syntax-ls`, added to your `PATH`).

Models already exported are skipped. Catalog entries marked `int8` are quantized on export, about 4x smaller. See [models.md](models.md).

### Config directory

| OS | Path |
| -- | ---- |
| Linux, macOS | `$XDG_CONFIG_HOME/natural-syntax-ls/`, default `~/.config/natural-syntax-ls/` |
| Windows | `%APPDATA%\natural-syntax-ls\` |

Layout:

```
models/<slug>/    one directory per model
lib/              ONNX Runtime library
lang/             language scripts
models.json       model catalog
```

A flat layout from older installs still works.

## Install a release

**VS Code / VSCodium**: download the `.vsix` for your platform from the latest release (Windows x64, Linux x64, macOS arm64). It bundles the server and ONNX Runtime.

```bash
code --install-extension natural-syntax-ls-*.vsix
```

**Standalone binary**: `natural-syntax-ls_<platform>.tar.gz` (`.zip` on Windows) holds the server, ONNX Runtime, `lang/`, `models.json` and `LICENSE`. Extract anywhere and put the binary on `PATH`. A symlink works; the server finds its files beside the real binary.

**Neovim**: use the standalone binary, then follow [neovim.md](neovim.md).

## Build from source

```bash
scripts/setup.sh   # exports bert-base, fetches ONNX Runtime, builds and installs the server

cd vscode-extension
npm install
npx vsce package
code --install-extension natural-syntax-ls-*.vsix
```

Manual release-style build:

```bash
CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" ./cmd/natural-syntax-ls/
```

The server reads `lang/` and `models.json` from beside the executable. Copy `data/lang` and `data/models.json` next to it. Put the ONNX Runtime library next to it, in `lib/`, or at `$ORT_LIB_PATH`.

### Use your own binary in VS Code

```json
"naturalSyntaxLs.serverPath": "/path/to/natural-syntax-ls"
```

## Troubleshooting

**POS mode tags almost every word as "Foreign word", with low confidence.** The exported `models/bert_base/bert_base.onnx` (or `.onnx.data`) is stale or corrupt. Re-export with `scripts/setup.sh --model bert-base`, or use `mobilebert` meanwhile.

**Nothing is highlighted.** Check the server log on stderr. Set `NLS_LOG=debug` for more detail. See [settings.md](settings.md#logging).
