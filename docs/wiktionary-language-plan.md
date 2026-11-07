# Wiktionary language selection plan

Goal: hover definitions in English by default. User can pick any supported language, or `auto` to take the top result across languages.

## Current state

- `wiktionary.FetchDef(word, pos, deprel)` reads only `payload["en"]` (`internal/wiktionary/wiktionary.go`).
- Cache key is `word:pos:deprel`, with no language.
- Only setting is `wiktionary_definitions` (bool). Wired in `initOptions` (`internal/lspserver/server.go:139`), `vscode-extension/extension.js:125,161`, `package.json:80`.
- API response is keyed by language code. Each bucket holds entries with a `language` display name.
- `A` and `a` have empty `es` bucket sections for verbs. Spanish `a` is not in the data. English-only code never sees other buckets.

## Setting

New option `naturalSyntaxLs.wiktionaryLanguage`, string, default `"en"`.

| Value | Behavior |
| ----- | -------- |
| `"en"` | Default. English bucket only (today's behavior). |
| ISO code (`"es"`, `"fr"`, `"ja"`, `"zh"`, ...) | That bucket only. |
| `"model"` | Language from the loaded model metadata (es model gives `es`). Falls back to `en` when model is multilingual. |
| `"auto"` | Top result across languages (multilingual option). |

`wiktionary_definitions` stays the on/off switch.

Server init option: `wiktionary_language` (string). VS Code, Neovim (`lsp/natural_syntax_ls.lua`, `plugin/`), docs/settings.md all pass it through.

## Supported code list

- Start with `en`, `es`, `fr`, `ja`, `zh` (languages with models, per `docs/models.md`).
- Accept any code the API returns. Unknown code gives no definition, not an error.
- Validate in the extension with an `enum` plus free-form fallback (`anyOf`) so new codes need no release.

## Lookup logic

`FetchDef(word, lang, pos, deprel)`:

1. Resolve `lang`. `model` maps to model language. Empty maps to `en`.
2. Fetch payload once per word (existing `fetchPayload`, swappable in tests).
3. Fixed language: `entries = payload[lang]`. Run existing `pickDef` unchanged.
4. `auto`: rank buckets, then `pickDef` on the winner.
   - Prefer model language first when known.
   - Then `en`.
   - Then remaining buckets in API order, skipping `other`.
   - First bucket giving a non-empty definition wins. A bucket matching the target POS beats a non-matching one (same `matched` flag `pickDef` already returns).
5. Case handling: keep exact case, then lowercase. Run per language.
6. Label the result when language differs from `en`: prefix `*(Spanish)*` using the entry's `language` field. Needed in `auto` so the source is clear.

## POS mapping per language

- `posCategory` and `deprelCategory` return English section names. Wiktionary section names in the API are English for all languages, so mapping stays valid.
- Semantic mode has no POS (`Tag: O`, `embedding_model.go:48`), so `target == ""` and first non-low-priority entry wins. Unchanged.

## Cache

- Key becomes `lang:word:pos:deprel` (lowercase word). `auto` is its own `lang` value.
- Cache stores only successful lookups (existing). Offline stays graceful: no entry, no hover text.
- Wrap in a size cap later if memory matters (see tmpfs/RAM notes). Not needed now.

## Known gaps (from PLANS.md section 6)

- ja and zh pieces such as `私は` are not headwords. Lookup returns nothing. Separate task: try dropping trailing particles, or skip.
- Non-Latin URL escaping: `url.PathEscape` already encodes UTF-8. Add tests.
- Translingual entries: keep as extra bucket tier for every fixed language (digits, symbols).

## Files to change

1. `internal/wiktionary/wiktionary.go`: `lang` param, bucket selection, `auto` ranking, language label, cache key.
2. `internal/wiktionary/wiktionary_test.go`: tests below.
3. `internal/lspserver/server.go`: `WiktionaryLanguage *string` in `initOptions`, `atomic.Value` on server, pass to `FetchDef` at `handleHover`. Model language from registry metadata for `model`.
4. `internal/lspserver/server_test.go`: option parsing and hover call.
5. `vscode-extension/package.json`, `extension.js`: setting and `opts.wiktionary_language`.
6. `lsp/natural_syntax_ls.lua`, `plugin/natural_syntax_ls.lua`: init option.
7. `docs/settings.md`, `docs/neovim.md`, `README.md`: document values.
8. `PLANS.md`: mark section 6 items done.

## Tests

- `en` default returns same result as today (regression).
- Fixed `es` for `a`: preposition from the `es` bucket (mock payload).
- Missing bucket: no result, no panic.
- `auto` order: model language, then `en`, then others; `other` skipped.
- `auto` label present for non-English, absent for English.
- Cache separation: same word under `en` and `es` returns different values.
- `model` resolves per loaded model; multilingual model falls back to `en`.
- Non-Latin word escaping in page URL (`https://en.wiktionary.org/wiki/...`).
- Setting change at runtime via `workspace/didChangeConfiguration`, if supported; otherwise on init only (state this in docs).

## Order of work

1. Core `lang` param plus cache key, default `en`. Existing tests stay green.
2. Server option and plumbing.
3. `auto` ranking and label.
4. Editor settings and docs.
5. ja/zh headword handling (separate PR).

## Open decisions

- Should `auto` prefer model language before `en`? Plan says yes.
- Language label format in hover. Plan says italic prefix.
- Runtime reconfiguration vs init-only. Check how `wiktionary_definitions` is handled today and match it.
