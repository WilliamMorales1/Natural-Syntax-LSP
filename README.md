# Natural Syntax LSP

Parts-of-speech semantic highlighting for VS Code via a local Go LSP server running BERT inference with ONNX Runtime. Hover over any word to see its POS tag and confidence score.

## Setup

Requires Go, Python 3, Node.js, and Git for Windows (for bash).

```bash
bash scripts/setup.sh
```

This exports the BERT-base POS model to ONNX (~400 MB), downloads the vocabulary, copies the ONNX Runtime DLL, and builds the `natural-syntax-ls.exe` binary. Model files go to `%APPDATA%\natural-syntax-ls\` (Windows) or `~/.config/natural-syntax-ls/` (Linux/macOS). To also export MobileBERT (~100 MB, faster):

```bash
bash scripts/setup.sh --model all
```

Then install the VS Code extension:

```bash
cd vscode-extension
npm install
npx vsce package
code --install-extension natural-syntax-ls-0.1.0.vsix
```

### Rebuilding the VSIX

Only needed if you modify files under `vscode-extension/` (e.g. `extension.ts`, `package.json`, or adding new settings/commands). The pre-built `.vsix` in the repo is fine for normal use.

```bash
cd vscode-extension
npm install
npx vsce package
code --install-extension natural-syntax-ls-0.1.0.vsix
```

Set the server path in VS Code settings:

```json
"naturalSyntaxLs.serverPath": "C:\\path\\to\\natural-syntax-ls.exe"
```

The extension defaults to `bert-base`. Highlighting appears ~10 seconds after VS Code loads while the model initializes.

## Models

| Model | Size | Speed | Accuracy |
|---|---|---|---|
| `bert-base` (default) | ~400 MB | ~10s startup | Higher |
| `mobilebert` | ~100 MB | ~10s startup | Good |

Switch via `naturalSyntaxLs.model` in VS Code settings. Run `bash setup.sh --model all` to export both.
These models only work for English, although if you speak a highly spoken language, you can likely find a different model for your own language.

## Settings

| Setting | Default | Description |
|---|---|---|
| `naturalSyntaxLs.serverPath` | `natural-syntax-ls` | Path to the binary |
| `naturalSyntaxLs.model` | `bert-base` | `bert-base` or `mobilebert` |
| `naturalSyntaxLs.filetypes` | `["plaintext", "markdown"]` | Language IDs to activate for |
| `naturalSyntaxLs.scoreThreshold` | `null` (0.333) | Minimum confidence to highlight |
| `naturalSyntaxLs.tokenMapUpdate` | `{}` | Override POS → color mappings |
| `naturalSyntaxLs.wiktionaryDefinitions` | `true` | Show Wiktionary definitions in hover tooltips |

## POS Tag Colors

Tags map to VS Code semantic token types, which inherit colors from your theme:

| Tag | Meaning | Token Type |
|---|---|---|
| NN, NNS | Noun | `parameter` |
| NNP, NNPS | Proper noun | `parameter` + declaration |
| VB, VBD, VBG, VBP | Verb | `function` |
| VBN, VBZ | Verb (past participle / 3rd person) | `method` |
| JJ, JJR, JJS | Adjective | `type` / `struct` / `interface` |
| RB, RBR, RBS | Adverb | `enumMember` |
| DT | Determiner | `string` |
| IN | Preposition / conjunction | `comment` |
| MD | Modal | `keyword` |
| CC | Coordinating conjunction | `keyword` |
| PRP, PRP$ | Pronoun | `property` |
| CD | Cardinal number | `number` |
| TO | to | `keyword` |

## Test

After setup, open a plaintext file and paste this, or you can simply open up this Markdown file after:

> The North Wind and the Sun were disputing which was the stronger, when a traveler came along wrapped in a warm cloak.
> They agreed that the one who first succeeded in making the traveler take his cloak off should be considered stronger than the other.
> Then the North Wind blew as hard as he could, but the more he blew the more closely did the traveler fold his cloak around him;
> and at last the North Wind gave up the attempt. Then the Sun shined out warmly, and immediately the traveler took off his cloak.
> And so the North Wind was obliged to confess that the Sun was the stronger of the two.

Words should appear in different colors within ~10 seconds. Hover over any word to see its part of speech and confidence score.
