# Natural Syntax LSP

Parts-of-speech or semantic-embedding highlighting for VS Code via a local Go LSP server running ONNX inference. Hover over any word to see its tag and a Wiktionary definition.

Two modes:

- **POS mode** (default) — BERT/MobileBERT classifies each word's part of speech and maps it to a VS Code semantic token type (color determined by your theme).
- **Semantic mode** — `all-MiniLM-L6-v2` embeds each word into a 384-dim vector, projects it onto a 2D plane, and maps the angle to a continuous HSL color. Similar words get similar colors. Colors are pushed via a custom `$/nls/semanticColors` LSP notification and applied as VS Code `TextEditorDecorationType` decorations (full hex, not theme-limited).

## Setup

Requires Go, Python, Node.js, and Git for Windows (for bash).

```bash
bash scripts/setup.sh
```

Exports the BERT-base POS model to ONNX (~400 MB), downloads the vocabulary, copies the ONNX Runtime DLL, and builds `natural-syntax-ls.exe`. Model files go to `%APPDATA%\natural-syntax-ls\` (Windows) or `~/.config/natural-syntax-ls/` (Linux/macOS).

```bash
bash scripts/setup.sh --model all        # also exports MobileBERT (~100 MB)
bash scripts/setup.sh --model semantic   # exports all-MiniLM-L6-v2 (~22 MB) for semantic mode
```

Install the VS Code extension:

```bash
cd vscode-extension
npm install
npx vsce package
code --install-extension natural-syntax-ls-0.1.0.vsix
```

Set the server path in VS Code settings if it is not on your PATH:

```json
"naturalSyntaxLs.serverPath": "C:\\path\\to\\natural-syntax-ls.exe"
```

### Rebuilding the VSIX

Only needed if you modify files under `vscode-extension/`:

```bash
cd vscode-extension
npx vsce package
code --install-extension natural-syntax-ls-0.1.0.vsix
```

## Models

| Model | Size | Mode | Notes |
|---|---|---|---|
| `bert-base` (default) | ~400 MB | POS | Higher accuracy |
| `mobilebert` | ~100 MB | POS | Faster startup |
| `all-MiniLM-L6-v2` | ~22 MB | Semantic | Continuous color embedding |

Switch POS model via `naturalSyntaxLs.model`. Switch between modes via `naturalSyntaxLs.mode`.

## Settings

| Setting | Default | Description |
|---|---|---|
| `naturalSyntaxLs.serverPath` | `natural-syntax-ls` | Path to the Go binary |
| `naturalSyntaxLs.mode` | `pos` | `pos` or `semantic` |
| `naturalSyntaxLs.model` | `bert-base` | `bert-base` or `mobilebert` (POS mode only) |
| `naturalSyntaxLs.filetypes` | `["plaintext", "markdown"]` | Language IDs to activate on |
| `naturalSyntaxLs.scoreThreshold` | `null` (0.333) | Minimum confidence to highlight (POS mode) |
| `naturalSyntaxLs.tokenMapUpdate` | `{}` | Override POS → token type mappings |
| `naturalSyntaxLs.wiktionaryDefinitions` | `true` | Show Wiktionary definitions in hover |
| `naturalSyntaxLs.semanticLightness` | `0.75` | OKLCH lightness for semantic colors (0–1); increase for light themes |
| `naturalSyntaxLs.semanticChroma` | `0.14` | OKLCH chroma (color intensity) for semantic colors |

## POS Tag Colors (POS mode)

Tags map to VS Code semantic token types; color comes from your theme.

| Tag | Meaning | Token Type |
|---|---|---|
| VB, VBD, VBG, VBP | Verb | `function` |
| VBN, VBZ | Verb (past participle / 3rd person) | `method` |
| NN, NNS | Noun | `variable` |
| NNP, NNPS | Proper noun | `namespace` / `typeParameter` |
| JJ, JJR, JJS | Adjective | `type` / `struct` / `interface` |
| PRP$, WP, WP$ | Possessive / wh-pronoun | `property` / `regexp` |
| RB, RBR, RBS, WRB | Adverb / wh-adverb | `enumMember` |
| IN, RP, SYM, POS | Preposition / particle / symbol | `operator` |
| CC, EX, MD, PRP, TO | Conjunction / modal / pronoun | `keyword` / `modifier` |
| DT, PDT, WDT | Determiner | `macro` |
| CD | Cardinal number | `number` |
| LS | List item marker | `decorator` |
| FW, UH | Foreign word / interjection | `string` |
| O | Other | `comment` |

## Semantic Mode Colors

Each word is embedded by `all-MiniLM-L6-v2`, projected onto a fixed 2D plane via two orthogonal random unit vectors, and the angle maps to a hue in OKLCH color space (perceptually uniform lightness). Similar words get similar hues. The color is not theme-dependent.

Default: OKLCH(0.75, 0.14, hue). Adjust `semanticLightness` and `semanticChroma` for your theme.

Hover shows the hex color code and a Wiktionary definition (when available).

## Test

Open a plaintext file and paste:

> The North Wind and the Sun were disputing which was the stronger, when a traveler came along wrapped in a warm cloak.

Words color within ~10 seconds (POS) or ~5 seconds (semantic). Hover any word to see its tag and Wiktionary definition.

![screenshot showing color highlighting and hover](screenshotExample.png)
