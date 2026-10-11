# Natural Syntax Highlighting

Colors prose by part of speech, contextual meaning, or dependency relation, using a local language server running ONNX models. Hover any word for its tag plus a Wiktionary definition.

![color highlighting and hover](https://raw.githubusercontent.com/WilliamMorales1/Natural-Syntax-LSP/master/docs/screenshotExample.png)

- **POS mode** (default): BERT/MobileBERT part-of-speech tags, colored by your theme.
- **Semantic mode**: words used alike get similar colors (MPNet or MiniLM embeddings).
- **Dependency mode**: Universal Dependencies relations from a biaffine parser.

Everything runs locally. The server binary and ONNX Runtime are bundled (Windows x64, Linux x64, macOS arm64).

## Setup

The models are too large to bundle. Export the ones you want once, from a checkout of the [repository](https://github.com/WilliamMorales1/Natural-Syntax-LSP):

```bash
scripts/setup.sh                     # POS, bert-base (~430 MB)
scripts/setup.sh --model mobilebert  # POS, lighter (~105 MB)
scripts/setup.sh --model mpnet       # semantic (~440 MB)
scripts/setup.sh --model minilm      # semantic, lighter (~92 MB)
scripts/setup.sh --model dependency  # dependency (~470 MB)
```

On Windows use `scripts/setup.ps1 -Model <name>`. Models go to `%APPDATA%\natural-syntax-ls\` or `~/.config/natural-syntax-ls/`.

Then pick a mode with `naturalSyntaxLs.mode` and a matching `naturalSyntaxLs.model`.

## Settings

| Setting | Default | Description |
| --- | --- | --- |
| `naturalSyntaxLs.mode` | `pos` | `pos`, `semantic`, or `dependency` |
| `naturalSyntaxLs.model` | `bert-base` | Model for the active mode |
| `naturalSyntaxLs.filetypes` | `["plaintext", "markdown"]` | Language IDs to activate on |
| `naturalSyntaxLs.serverPath` | `""` | Custom server binary (empty = bundled) |
| `naturalSyntaxLs.scoreThreshold` | `null` (0.333) | Minimum POS confidence to highlight |
| `naturalSyntaxLs.tokenMapUpdate` | `{}` | Override POS to token type mappings |
| `naturalSyntaxLs.wiktionaryDefinitions` | `true` | Wiktionary definitions in hover |
| `naturalSyntaxLs.semanticLightness` | `0.75` | OKLCH lightness of semantic colors |
| `naturalSyntaxLs.semanticChroma` | `0.14` | OKLCH chroma of semantic colors |

Full docs, Neovim setup, and tag lists: [GitHub](https://github.com/WilliamMorales1/Natural-Syntax-LSP).
