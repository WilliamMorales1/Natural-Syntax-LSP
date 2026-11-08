# Installing from source and exporting models

## Exporting models

Every install path needs the ONNX model files, which no package ships (100-750 MB each). Catalog entries marked `int8` are quantized on export, 4x smaller; [models.md](models.md) lists which and why others stay fp32. From a repo checkout (installs Python deps itself):

```bash
scripts/setup.sh                    # bert-base POS model (~430 MB)
scripts/setup.sh --model xlmr-es    # any setup name from models.md
scripts/setup.sh --model all        # every model
scripts/setup.sh --list             # print the catalog (data/models.json) and exit
```

(`scripts/setup.ps1 -Model <...>` and `-List` on Windows, same values.) The script also downloads the ONNX Runtime library, copies `data/lang/` and `models.json` into the config dir without overwriting edited files, builds the server and installs it to `~/.local/bin` (`$XDG_BIN_HOME` if set; on Windows `%LOCALAPPDATA%\Programs\natural-syntax-ls`, added to your user `PATH`). A model already exported is skipped.

Model files go to `%APPDATA%\natural-syntax-ls\` (Windows) or `$XDG_CONFIG_HOME/natural-syntax-ls/`, default `~/.config/natural-syntax-ls/` (Linux and macOS). Layout: `models/<slug>/` per model, `lib/` for the ONNX Runtime library, `lang/` for language scripts, `models.json` for the catalog. A flat layout from older installs is still read. Model details: [models.md](models.md).

## Building from source

Requires Go 1.26+ with cgo and Python 3. Node.js and npm are needed only for the VS Code extension.

```bash
scripts/setup.sh   # exports bert-base, fetches ONNX Runtime, builds and installs the binary

cd vscode-extension
npm install
npx vsce package
code --install-extension natural-syntax-ls-*.vsix
```

Every build includes the kagome Japanese dictionary (about 48 MB binary instead of 11 MB; it is decoded only when Japanese text appears, see [models.md](models.md#other-languages)). To leave it out, build with `-tags nojaseg`; Japanese then falls back to tokenizer pieces (74% instead of 97% POS accuracy).

Release-style build: `CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" ./cmd/natural-syntax-ls/`. The server reads `lang/` and `models.json` from beside the executable, so copy `data/lang` and `data/models.json` next to it, and put the ONNX Runtime library next to it or in `lib/` (or set `ORT_LIB_PATH`).

To use your own binary instead of the bundled one, set in VS Code settings:

```json
"naturalSyntaxLs.serverPath": "/path/to/natural-syntax-ls"
```

The standalone binary may be symlinked onto `PATH`; the server finds ONNX Runtime next to the real file.

## Troubleshooting

If POS mode tags nearly every word as "Foreign word" or other nonsense tags with low confidence, the exported `models/bert_base/bert_base.onnx` (and `.onnx.data`) is likely stale or corrupt. Re-export with `scripts/setup.sh --model bert-base` (or `scripts/setup.ps1 -Model bert-base`), or switch to `mobilebert` in the meantime.
