# Installing from source and exporting models

## Exporting models

Every install path needs the ONNX model files, which no package ships (they are 100-750 MB each). Spanish and French POS and the French parser are exported as int8 (`export_model.py --int8`), which is 4x smaller; the Spanish parser must stay fp32. From a repo checkout (installs Python deps itself):

```bash
scripts/setup.sh                     # BERT-base POS model (~430 MB)
scripts/setup.sh --model mobilebert  # lighter POS model (~105 MB)
scripts/setup.sh --model pos-es      # Spanish POS, XLM-R int8 (~280 MB)
scripts/setup.sh --model pos-fr      # French POS, XLM-R int8 (~280 MB)
scripts/setup.sh --model pos-ru      # Russian POS, XLM-R int8 (~280 MB)
scripts/setup.sh --model pos-ar      # Arabic POS, XLM-R int8 (~280 MB)
scripts/setup.sh --model multilingual  # semantic mode, paraphrase-multilingual-MiniLM-L12-v2 (~470 MB)
scripts/setup.sh --model dependency  # dependency mode, en_ewt.electra-base UD parser (~470 MB)
scripts/setup.sh --model dependency-es  # Spanish parser, fp32 (~750 MB)
scripts/setup.sh --model dependency-fr  # French parser, int8 (~350 MB)
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

Every build includes the kagome Japanese dictionary (about 48 MB binary instead of 11 MB, loaded only when Japanese text appears; see [models.md](models.md#other-languages-experimental)). To leave it out, build with `go build -tags nojaseg ./cmd/natural-syntax-ls/`; Japanese then falls back to tokenizer pieces (74% instead of 97% POS accuracy).

For an English-only server, build with `-tags english` (about 7 MB): no kagome dictionary, no Japanese lemmas, no Chinese script conversion. Release archives named `*_english.*` are built this way; `scripts/build-release.sh [version]` builds both variants into `dist/`. Semantic mode still works because the multilingual embedding model needs the Unigram tokenizer, which stays in.

To use your own binary instead of the bundled one, set in VS Code settings:

```json
"naturalSyntaxLs.serverPath": "/path/to/natural-syntax-ls"
```

The standalone binary may be symlinked onto `PATH`; the server finds ONNX Runtime next to the real file.

## Troubleshooting

If POS mode tags nearly every word as "Foreign word" or other nonsense tags with low confidence, the exported `bert_base.onnx` / `bert_base.onnx.data` pair is likely stale or corrupt. Re-export with `scripts/setup.sh --model bert-base` (or `scripts/setup.ps1 -Model bert-base`), or switch to `mobilebert` in the meantime.
