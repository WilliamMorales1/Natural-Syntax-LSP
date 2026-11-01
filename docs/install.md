# Installing from source and exporting models

## Exporting models

Every install path needs the ONNX model files, which no package ships (they are 100-470 MB each). From a repo checkout (installs Python deps itself):

```bash
scripts/setup.sh                     # BERT-base POS model (~430 MB)
scripts/setup.sh --model mobilebert  # lighter POS model (~105 MB)
scripts/setup.sh --model mpnet       # semantic mode, all-mpnet-base-v2 (~440 MB)
scripts/setup.sh --model minilm      # semantic mode, all-MiniLM-L6-v2 (~92 MB)
scripts/setup.sh --model dependency  # dependency mode, en_ewt.electra-base UD parser (~470 MB)
scripts/setup.sh --model all         # all models
```

(`scripts/setup.ps1 -Model <...>` on Windows, same values.)

Model files go to `%APPDATA%\natural-syntax-ls\` (Windows) or `$XDG_CONFIG_HOME/natural-syntax-ls/`, default `~/.config/natural-syntax-ls/` (Linux and macOS). Model details: [models.md](models.md).

## Building from source

Requires Go, Python, Node.js.

```bash
scripts/setup.sh   # exports models, builds binary into ~/.local/bin

cd vscode-extension
npm install
npx vsce package
code --install-extension natural-syntax-ls-*.vsix
```

To use your own binary instead of the bundled one, set in VS Code settings:

```json
"naturalSyntaxLs.serverPath": "/path/to/natural-syntax-ls"
```

The standalone binary may be symlinked onto `PATH`; the server finds ONNX Runtime next to the real file.

## Troubleshooting

If POS mode tags nearly every word as "Foreign word" or other nonsense tags with low confidence, the exported `bert_base.onnx` / `bert_base.onnx.data` pair is likely stale or corrupt. Re-export with `scripts/setup.sh --model bert-base` (or `scripts/setup.ps1 -Model bert-base`), or switch to `mobilebert` in the meantime.
