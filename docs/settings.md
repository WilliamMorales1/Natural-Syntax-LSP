# Settings and colors

## Settings

| Setting                                 | Default                     | Description |
| --------------------------------------- | --------------------------- | ----------- |
| `naturalSyntaxLs.serverPath`            | `""` (bundled binary)       | Path to the Go binary |
| `naturalSyntaxLs.mode`                  | `pos`                       | `pos`, `semantic`, or `dependency` |
| `naturalSyntaxLs.model`                 | `bert-base`                 | Model name; meaning depends on `mode` (see [models.md](models.md)) |
| `naturalSyntaxLs.filetypes`             | `["plaintext", "markdown"]` | Language IDs to activate on |
| `naturalSyntaxLs.scoreThreshold`        | `null` (0.333)              | Minimum confidence to highlight (POS mode) |
| `naturalSyntaxLs.tokenMapUpdate`        | `{}`                        | Override POS → token type mappings |
| `naturalSyntaxLs.wiktionaryDefinitions` | `true`                      | Show Wiktionary definitions in hover |
| `naturalSyntaxLs.semanticLightness`     | `0.75`                      | OKLCH lightness for semantic colors (0–1); increase for light themes |
| `naturalSyntaxLs.semanticChroma`        | `0.14`                      | OKLCH chroma of the most vivid semantic colors |

## POS mode colors

Tags map to VS Code semantic token types; color comes from your theme. Full list: [posList.md](posList.md).

Hover shows the Part of Speech and a Wiktionary definition (when available).

## Semantic mode colors

Colors are pushed via a custom `$/nls/semanticColors` LSP notification and applied as VS Code `TextEditorDecorationType` decorations (full hex, not theme-limited).

For each document, the server lays every word out on a 2D map, choosing the view where that document's words are most spread out. A word's direction on the map sets its hue and its distance from the middle sets how vivid it is. Words used in similar ways land close together, so they get similar colors. The map is updated as you type and lined up with the previous one, so colors stay steady. Colors are per document and don't depend on your theme.

All colors share one lightness. `semanticLightness` (default 0.75) sets it; increase it for light themes. `semanticChroma` (default 0.14) sets the most vivid words' intensity.

How it works, in depth (the math, the code, and its time and memory costs): [semantic-colors.md](semantic-colors.md).

Hover shows the hex color code and a Wiktionary definition (when available).

## Dependency mode colors

Each word is colored by its Universal Dependencies relation, one relation per token type/modifier pair. Full list: [deprelList.md](deprelList.md).

Hover shows the word's relation to its head, then the words that depend on it under `dependents:` (marked `# has dependents` when they have their own), plus a Wiktionary definition (when available).

## Testing

Open [test.txt](test.txt) in VS Code after installation. For semantic mode, [semantic-example.txt](semantic-example.txt) shows animals, food, technology and feelings each getting their own color family.

Words color within ~10 seconds (POS), ~5 seconds (semantic) or ~15 seconds (dependency).
