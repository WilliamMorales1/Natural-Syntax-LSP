# Adding a language

A language is a Lua script; no Go build needed. The server loads every `*.lua` from `<exe dir>/lang/`, `<model dir>/lang/`, `~/.config/natural-syntax-ls/lang/`, then `naturalSyntaxLs.langDir` (`--lang-dir`), and a later script replaces an earlier one with the same `code` (`ExampleMerge` in `internal/lang/luaload`). Edits are picked up within about two seconds: scripts, tokenizers and Wiktionary rules are rebuilt and open documents re-analysed. Restart only to change which directories are read.

Shipped scripts live in `data/lang/`. Setup copies them into the config dir without overwriting edited files; release archives and the extension carry them in `<exe dir>/lang/`. The binary embeds none. Japanese (kagome dictionary) and Chinese (conversion tables) stay Go packages because they need native assets.

## Schema

The examples in `internal/lang/luaload/example_test.go` are the reference, checked by `go test ./...`:

- `ExampleParse`: a minimal script (`code`, `model_prefixes`, `wiktionary.code`/`sections`/`forms`, like `spanish.lua`) and the error for an unknown field. A bad script fails alone; the server logs it and keeps running.
- `ExampleParse_schema`: every field and what it becomes.
- `ExampleParse_sandbox`: what scripts can call, and what a failing or looping hook returns.
- `ExampleParseUnigram` in `internal/tokenizer`: `elisions` and `arabic_clitics` on a tiny vocabulary.

Every field but `code` is optional. Add one only when needed, in this order:

| Field | Use when |
| ----- | -------- |
| `model_prefixes` | Models whose file name starts with the prefix are this language's (drives `wiktionaryLanguage: "model"` and `auto`). |
| `tokenizer.elisions`, `tokenizer.arabic_clitics` | The model's tokenizer does not produce the words the model was trained on (French `l'ami`, Arabic `وبالكتاب`). Unigram tokenizers only. |
| `wiktionary.sections` | The language has Wiktionary entries: the English Wiktionary L2 headings (`"Spanish"`). Without `wiktionary`, no lookup. |
| `wiktionary.forms`, `rewrite` | Wiktionary files the word under something other than its text or lowercase: variants, lemmas, stripped diacritics (`forms`) or a fixed page (`rewrite`, English `'s` to `-'s`). |
| `wiktionary.wikitext`, `script` | The REST payload lacks the language's sections, so read wikitext for words in `script` (a Unicode script name such as `"Han"`). |

Scripts sit beside downloaded models: treat them like config, not like unreviewed code. The sandbox cannot cap heap use beyond `string.rep`. Keep per-token work out of Lua; tokenizer behavior is chosen by flag.

Needs Go instead: a new tokenizer flag (`luaload/options.go`), an unspaced-script segmenter or a native library (a package under `internal/lang/<name>` exporting `Language()`, listed in `internal/lang/builtin/native.go`; Go hooks shown in `ExampleLanguage` in `internal/lang`), model labels `postag.ParseLabel` does not know (`ExampleParseLabel`; unknown labels decode as `O`, no color), or a sentence terminator without the Unicode `Sentence_Terminal` property (`Example_sentences` in `internal/inference`).

## Steps

1. Write `<code>.lua`. To ship it, put it in `data/lang/` and add tests to `internal/lang/builtin/builtin_test.go`.
2. Add the model to `data/models.json` (`name`, `mode`, `source`, `language`, `description`, optional `default` and `int8`; `int8_only` and `int8_fp32` narrow which MatMul nodes get quantized, by node-name substring). The server and `scripts/export_model.py` read it; nothing else in Go or Python names models.
3. `scripts/setup.sh` and `setup.ps1` read the catalog. By hand: the `naturalSyntaxLs.model` examples in `vscode-extension/package.json`, the language's `model_prefixes`, and the model, license and accuracy in `docs/models.md` (per-language notes in `docs/design/languages.md`). A dependency model that diaparser does not publish can be trained with `scripts/train_dependency.py` (add the language to its `LANGS`).
4. Verify with the opt-in tests in [testing.md](testing.md#opt-in-tests) (`NSLS_EVAL`, `NSLS_WIKT_EVAL` with the treebank in `udFiles`, `NSLS_NVIM_BENCH`), and `go build -tags nojaseg ./...`.
5. If the language detector does not know it (`ExampleDetect` in `internal/langid`), tokenizer rules apply to every line. Retrain it as in [testing.md](testing.md#language-detector). Galician was tried and dropped: about 10k sentences gave 64% held-out accuracy against Spanish and Portuguese.
