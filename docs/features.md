# Features

The server is a standard stdio LSP. It answers `textDocument/semanticTokens/full` and `textDocument/hover`, so any LSP client works. Semantic mode also sends a custom notification (see below).

## Modes

Pick one with `naturalSyntaxLs.mode` (VS Code) or `-mode` (server flag).

### POS

Each word gets one token type per Universal POS class, so a noun looks the same in every language. Your theme picks the colors. Tables: [tags.md](tags.md#pos-classes).

Hover shows the part of speech.

### Dependency

Each word gets a token type and modifier pair for its UD relation to its head. Tables: [tags.md](tags.md#ud-relations).

Hover shows the relation and the words under `dependents:`. A dependent with its own dependents is marked `# has dependents`.

### Semantic

Each word gets a color from its contextual embedding. Words used alike get similar hues. Colors are per document and stay steady while you type. They ignore your theme.

- `semanticLightness` sets the shared lightness. Raise it for light themes.
- `semanticChroma` sets how vivid the strongest colors are.

Hover shows the hex code. Colors are not semantic tokens: the server pushes them in `$/nls/semanticColors` (`{ uri, tokens: [{ line, character, length, color }] }`, columns in codepoints) after each edit. VS Code draws them as decorations; Neovim needs the handler in [neovim.md](neovim.md).

How it works: [design/semantic-colors.md](design/semantic-colors.md).

## Hover definitions

Every mode adds a Wiktionary definition to hover when one exists. `naturalSyntaxLs.wiktionaryLanguage` picks the language:

| Value | Meaning |
| ----- | ------- |
| `detect` | Language of the hovered line |
| `en`, `es`, ... | That language |
| `model` | The model's language, else `en` |
| `auto` | Best match across languages |
| `detect,en` (default) | Priority list; first language with a definition wins |

Lookup tries the word, then forms from the language script (lemmas, variants, stripped diacritics). Known gaps:

- Chinese has no lemmatizer.
- Semantic mode has no POS, so a homograph shows its page's first section. Dependency mode picks the section from the relation.

Coverage per language: [benchmarks.md](benchmarks.md#dictionary-coverage).

## Language detection

A built-in 1.7 MB detector labels each line (about 11 µs per line). Nothing needs configuring.

- **Hover** uses the line's language. A line too short to tell borrows the nearest of the five lines above, then the document's language, then English.
- **Tokenizer rules** (French elisions such as `l'ami`, Arabic clitics) apply only to lines in their language. If the language is unknown, every rule applies.
- **Model switching**: with an English model configured (the default), a document detected as another language is analysed by that language's installed model for the current mode. It loads in the background; the document shows no tags until it is ready. The model follows the whole document, so a mixed file gets one model.

Model switching rules:

- At most two extra models stay loaded.
- Every model frees its weights after `-idle-unload` (default 5 minutes) without use, and reloads on the next edit.
- An unused English model is freed until a document needs it.
- A non-English or multilingual `model` setting, `-lang`, or `:NSLang CODE` turns switching off.

Detector accuracy: [benchmarks.md](benchmarks.md#language-detection).
