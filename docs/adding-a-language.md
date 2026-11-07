# Adding a language

Language-specific code lives under `internal/lang/<name>`. Generic packages (`tokenizer`, `wiktionary`, `inference`, `lspserver`) hold no language names; they take hooks from a `lang.Language` value.

## Layout

| Package | Role |
| ------- | ---- |
| `internal/lang` | `Language` struct, `ForModel`, hook collectors. Imports only `tokenizer` and `postag`. |
| `internal/lang/<name>` | One language: exports `Language() lang.Language`. |
| `internal/lang/builtin` | The list compiled into the binary; `-tags english` shrinks it. |
| `internal/models` | Language-neutral model facts: default file stems, embedding size from file name. |

## Hooks

Every `Language` field but `Code` is optional.

| Field | Use when |
| ----- | -------- |
| `ModelPrefixes` | Models whose file name starts with the prefix are this language's (drives `wiktionaryLanguage: "model"` and `auto`). |
| `Tokenizer` | The generic split cuts words wrongly. `tokenizer.WithWordMerger` rewrites the whitespace/punctuation split (French elision). `tokenizer.WithSegmenter` replaces model pieces for unspaced scripts (Japanese kagome). Unigram tokenizers only. |
| `Wiktionary.Rewrite` | A token's headword differs from its text (English `'s` to `-'s`). |
| `Wiktionary.Forms` | Extra headwords to try (Japanese dictionary forms, Chinese script variants). |
| `Wiktionary.Sections` | Always: the English Wiktionary L2 headings (`"Spanish"`) that map to the language code. |
| `Wiktionary.Wikitext`, `Script` | The REST endpoint cannot parse the language's entries, so read wikitext for words in `Script`. |

## Steps

1. `internal/lang/<name>/<name>.go` with `Language()`. A language with no special rules is only `Code` and `ModelPrefixes` (see `spanish`).
2. Add `<name>.Language()` to `internal/lang/builtin/builtin.go` (and `builtin_english.go` if it needs no heavy assets).
3. Tests next to the hook. Tokenizer hooks are plain functions, so test them without a model (see `french_test.go`).
4. Not Go, one line each:
   - `scripts/export_model.py`: entry in `POS_MODELS` (and a diaparser name for dependency).
   - `scripts/setup.sh`, `scripts/setup.ps1`: `export_*` function, `--model` case, `all` list.
   - `vscode-extension/package.json`: `naturalSyntaxLs.model` enum and description.
   - `docs/models.md`: model, license, accuracy.

## Worked example: French

French needed two things beyond a model:

- `internal/lang/french`: `Language{Code: "fr", ModelPrefixes: ["xlmr_fr", "fr_sequoia"], Tokenizer: [WithWordMerger(MergeElisions)]}`. The generic split cuts `l'ami` into `l`, `'`, `ami`; the merger rejoins it as `l'`, `ami` (UD's words) and keeps `aujourd'hui` whole.
- Model entries in `export_model.py`, both setup scripts and the extension enum (`xlmr-fr`, `fr_sequoia.camembert-large`).

Spanish needed only the second item plus a `Language` with prefixes. Japanese needed a segmenter (kagome) and dictionary-form headwords; Chinese needed script-variant headwords and the wikitext fallback.

## Decide what the language needs

Answer these in order; stop adding hooks as soon as the answer is no.

1. Does the model's own tokenizer already produce the words the model was trained on? If not, add a `WordMerger` (clitics, elision) or `Segmenter` (no spaces).
2. Does Wiktionary file the word under something other than its surface text or lowercase? Add `Forms` (variants, lemmas, stripped diacritics) or `Rewrite` (a fixed suffix page).
3. Does Wiktionary's REST payload lack the language's sections? Set `Wikitext` and `Script`.
4. Does the language have a Wiktionary section? Set `Sections` (a Language with no `Wiktionary` gets none of the lookup).
5. Does the model emit labels `postag.ParseLabel` does not know (non-UPOS, non-Penn)? Extend the mapping; unknown labels decode as `O` (no color), which hides the failure.

## Verifying

- Unit tests for each hook with no model files.
- `NSLS_EVAL=1` accuracy run (`internal/inference/eval_test.go`) against the language's UD test treebank; add it to the table in `docs/models.md`.
- `NSLS_WIKT_EVAL=1` live Wiktionary check (`internal/wiktionary/live_eval_test.go`); add the treebank to `udFiles` there.
- Latency and memory with `NSLS_NVIM_BENCH=1` (`internal/e2e`).
- `go build -tags english ./...` still builds.

## Sentence boundaries

No hook needed. `internal/inference` ends a sentence at any lone Unicode `Sentence_Terminal` word (`. ! ? 。 ？ ؟ ۔`) and keeps following non-ASCII closing brackets and quotes (`」 ） »`) with it. A script whose terminator lacks that property needs a tokenizer or inference change.
