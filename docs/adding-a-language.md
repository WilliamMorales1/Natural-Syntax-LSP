# Adding a language

A language is a Lua script. The server loads every `*.lua` from these directories, in order, and a later file replaces an earlier one with the same `code`: `<exe dir>/lang/`, `<model dir>/lang/`, `~/.config/natural-syntax-ls/lang/`, then `naturalSyntaxLs.langDir` (`--lang-dir`). No Go build needed; edits are picked up within about two seconds, with no restart: the server re-reads the scripts, rebuilds the tokenizers and Wiktionary rules and re-analyses open documents. Restart only to change which directories are read.

The shipped scripts are in `data/lang/` in the repo (English, French, Spanish, Russian, Arabic). `scripts/setup.sh` and `setup.ps1` copy them into the config dir without overwriting edited files; release archives and the extension carry them in `<exe dir>/lang/`. The binary embeds none: without scripts it has only the Go languages. Japanese (kagome dictionary) and Chinese (conversion tables) stay Go packages because they need native assets; `-tags nojaseg` drops the kagome dictionary.

Generic packages (`tokenizer`, `wiktionary`, `inference`, `lspserver`) hold no language names; they take hooks from a `lang.Language` value, which `internal/lang/luaload` builds from the script.

## Schema

```lua
return {
  code = "fr",                              -- required, Wiktionary language code
  model_prefixes = { "xlmr_fr", "fr_sequoia" }, -- lowercase model file-name prefixes
  tokenizer = {                             -- closed set of named flags, implemented in Go
    elisions = {
      clitics = { "l", "d", "qu" },         -- glued to a following apostrophe: l'  d'  qu'
      words = { "aujourd'hui" },            -- apostrophe words kept whole
    },
  },
  wiktionary = {
    code = "fr",
    sections = { "French" },                -- English Wiktionary L2 headings
    -- optional: wikitext = true, script = "Han" reads wikitext for words in that Unicode script
    rewrite = function(word, pos, deprel) end, -- Penn tag and UD relation as strings; return a headword or nil
    forms = function(word) return {} end,   -- extra headwords to try
  },
}
```

Unknown fields, wrong types and unknown script names fail that script only, with an error naming file and field; the server logs it and keeps running. `go doc -all ./internal/lang/luaload` shows `ExampleParse` (a parsed script and a field error) and `go doc -all ./internal/tokenizer` shows `ExampleParseUnigram` (`elisions` and `arabic_clitics` on a tiny vocabulary); `go test ./...` checks both.

## Sandbox

- Available: base functions (no `load*`, `dofile`, `require`, `print`, `setfenv`, `collectgarbage`), `table`, `string` (no `dump`; `rep` capped at 64 KB), `math`, and a `utf8` table: `chars(s)` (array of characters), `codepoint(ch)`, `char(n)`, `len(s)`. Lua strings are bytes, so use `utf8` for non-ASCII text.
- Not available: `os`, `io`, `package`, `debug`, `coroutine`. No file, network or process access.
- Load has a 2 s limit and each hook call 200 ms. A hook that errors, loops or panics returns nothing for that call and is logged.
- Trust model: scripts sit beside downloaded models, so treat them like config, not like code you would run unreviewed. The sandbox limits damage but cannot cap heap use beyond `string.rep`.
- Keep per-token work out of Lua. Only per-lookup hooks (`rewrite`, `forms`) are scripted; tokenizer behavior is chosen by flag.

## Hooks

Every field but `code` is optional.

| Field | Use when |
| ----- | -------- |
| `model_prefixes` | Models whose file name starts with the prefix are this language's (drives `wiktionaryLanguage: "model"` and `auto`). |
| `tokenizer.elisions` | The generic split cuts clitics wrongly (French `l'ami`). Unigram tokenizers only. |
| `tokenizer.arabic_clitics` | `true` splits Arabic conjunction, preposition, future and pronoun clitics off written words, guided by the model's vocabulary (`docs/models.md`). Unigram tokenizers only. |
| `wiktionary.rewrite` | A token's headword differs from its text (English `'s` to `-'s`). |
| `wiktionary.forms` | Extra headwords to try (stripped diacritics, `ё` variants). |
| `wiktionary.sections` | Always: the English Wiktionary L2 headings (`"Spanish"`) that map to the language code. |
| `wiktionary.wikitext`, `script` | The REST endpoint cannot parse the language's entries, so read wikitext for words in `script`. |

Tokenizer behavior that needs a new flag, an unspaced-script segmenter or a native library needs Go: add a named flag in `luaload/options.go`, or a package under `internal/lang/<name>` exporting `Language()` and list it in `internal/lang/builtin/native.go`.

## Steps

1. Write `<code>.lua`. A language with no special rules is only `code`, `model_prefixes` and `wiktionary.code`/`sections` (see `spanish.lua`).
2. To ship it built in, put it in `lang/` and add tests to `internal/lang/builtin/builtin_test.go`.
3. Model catalog: add an entry to `data/models.json` (`name`, `mode`, `source`, `language`, `description`, optional `default` and `int8`). It is read by the server (default model search, from `<exe dir>` then the config dir) and by `scripts/export_model.py`; setup copies it to the config dir without overwriting. Nothing else in Go or Python names models.
4. Not catalog, one line each:
   - `scripts/setup.sh`, `scripts/setup.ps1`: `export_*` function, `--model` case, `all` list (still hardcoded).
   - `vscode-extension/package.json`: `naturalSyntaxLs.model` enum and description (static JSON, still hardcoded).
   - `docs/models.md`: model, license, accuracy.

## Worked example: French

French needed a word merger beyond a model: the generic split cuts `l'ami` into three words, and `elisions` gives UD's words instead (`data/lang/french.lua`; output in `ExampleParseUnigram`). Arabic needed `arabic_clitics` for the same reason.

Spanish needed only prefixes and a Wiktionary section. Russian needed `forms` (stress marks and `ё`). Japanese needed a segmenter (kagome) and dictionary-form headwords; Chinese needed script-variant headwords and the wikitext fallback; both stay in Go.

## Decide what the language needs

Answer these in order; stop adding hooks as soon as the answer is no. Names below are the Go side (`lang.Language`, see `ExampleLanguage` in `internal/lang`); in Lua they are the `tokenizer` flags and the `wiktionary` fields above.

1. Does the model's own tokenizer already produce the words the model was trained on? If not, add a `WordMerger` (clitics, elision) or `Segmenter` (no spaces).
2. Does Wiktionary file the word under something other than its surface text or lowercase? Add `Forms` (variants, lemmas, stripped diacritics) or `Rewrite` (a fixed suffix page).
3. Does Wiktionary's REST payload lack the language's sections? Set `Wikitext` and `Script`.
4. Does the language have a Wiktionary section? Set `Sections` (a Language with no `Wiktionary` gets none of the lookup).
5. Does the model emit labels `postag.ParseLabel` does not know (non-UPOS, non-Penn)? Extend the mapping; unknown labels decode as `O` (no color), which hides the failure.

## Verifying

- Unit tests for each hook with no model files (`internal/lang/builtin/builtin_test.go`).
- `NSLS_EVAL=1` accuracy run (`internal/inference/eval_test.go`) against the language's UD test treebank; add it to the table in `docs/models.md`.
- `NSLS_WIKT_EVAL=1` live Wiktionary check (`internal/wiktionary/live_eval_test.go`); add the treebank to `udFiles` there.
- Latency and memory with `NSLS_NVIM_BENCH=1` (`internal/e2e`).
- `go build -tags nojaseg ./...` still builds.

## Sentence boundaries

No hook needed. `internal/inference` ends a sentence at any lone Unicode `Sentence_Terminal` word and keeps following non-ASCII closing brackets and quotes with it (`Example_sentences` there). A script whose terminator lacks that property needs a tokenizer or inference change.

## Language detection

Hover and language-scoped tokenizer rules rely on the built-in detector. If your language is not among those it knows (see `docs/settings.md`), scripts with `tokenizer` rules apply to every line. To teach the detector a language, add a Tatoeba dump (and optionally UD text) and retrain as described in `docs/settings.md`. Galician was tried and dropped: about 10k sentences were too few to separate it from Spanish and Portuguese (64% held-out accuracy).
