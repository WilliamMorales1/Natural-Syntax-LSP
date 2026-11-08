# Neovim

Needs Neovim 0.11+ and the server on `PATH` ([install.md](install.md)). `scripts/setup.sh` installs it to `~/.local/bin`; `scripts/setup.ps1` to `%LOCALAPPDATA%\Programs\natural-syntax-ls`. The repo is also a Neovim plugin (`lua/natural_syntax_ls/`). It sets up the server, the semantic color handler, `:NSMode` and `:NSLang`.

## Install

**lazy.nvim**:

```lua
{ "WilliamMorales1/Natural-Syntax-LSP", main = "natural_syntax_ls", opts = {} }
```

From a local checkout, swap the repo name for `dir = "/path/to/Natural-Syntax-LSP"`. `setup.sh` prints that line with your path filled in.

**vim.pack** (Neovim 0.12+):

```lua
vim.pack.add({ "https://github.com/WilliamMorales1/Natural-Syntax-LSP" })
require("natural_syntax_ls").setup()
```

**No plugin manager**:

```lua
vim.opt.rtp:append("/path/to/Natural-Syntax-LSP")
require("natural_syntax_ls").setup()
```

Open a `.txt` or `.md` file. `:checkhealth vim.lsp` should list `natural_syntax_ls`.

## Options

Every field is optional. Defaults:

```lua
require("natural_syntax_ls").setup({
  cmd = "natural-syntax-ls",           -- binary name or path
  filetypes = { "text", "markdown" },
  mode = "pos",                        -- pos | semantic | dependency
  language = "",                       -- "" detects per line; "es" forces Spanish
  models = {},                         -- per-mode -model path, e.g. { pos = "/path/to/model.onnx" }
  init_options = {},                   -- wiktionary_language, semantic_lightness, semantic_chroma, ...
})
```

With `models` empty, the server picks the catalog default for the mode, or the forced language's model. `init_options` keys: [settings.md](settings.md#initialization-options).

- **mason-lspconfig**: no conflict. `setup` calls `vim.lsp.enable` itself.
- **Neovim 0.10**: no `vim.lsp.config`, so the plugin does not load. Register the server through `nvim-lspconfig` (`require("lspconfig.configs")` with `default_config = { cmd, filetypes, root_dir }`), then edit `cmd` and run `:LspRestart` to switch modes.

## Commands

| Command | Effect |
| ------- | ------ |
| `:NSMode pos\|semantic\|dependency` | Restart in that mode |
| `:NSLang es` | Force one language, keep the mode |
| `:NSLang auto` | Back to per-line detection and default models |

`:NSLang CODE` restarts with `-lang CODE`:

- the model becomes that language's `models.json` entry for the mode (e.g. `xlmr-es` for `pos`), else a language-neutral model, else the default;
- detection is off, so tokenizer rules and hover treat all text as that language;
- hover definitions use `CODE,en` unless `init_options.wiktionary_language` is set.

Completion lists the languages in the catalogs the server reads. The same functions are exposed as `require("natural_syntax_ls").set_mode(mode)` and `.set_language(code)` for keymaps. Each switch fires `User NaturalSyntaxLsMode`.

## Colors

POS and dependency tokens use your colorscheme's `@lsp.type.*` groups. Link any that render plain:

```lua
vim.api.nvim_set_hl(0, "@lsp.type.function", { link = "Function" })
```

Semantic mode sends no semantic tokens. The plugin paints `$/nls/semanticColors` as extmarks, one highlight group per color. Tune with `init_options = { semantic_lightness = 0.6, semantic_chroma = 0.14 }`; lower lightness suits light colorschemes.

### Linguistic palette (optional)

```lua
require("natural_syntax_ls.colors").setup()
```

Call it after `setup`, e.g. in a lazy.nvim `config` function:

```lua
config = function(_, opts)
  require("natural_syntax_ls").setup(opts)
  require("natural_syntax_ls.colors").setup()
end,
```

**Why it exists.** The server reuses code token types for tags (nouns are `variable`, verbs `function`, adjectives `type`), so a colorscheme colors prose by rules made for code. Two related tags can get unrelated colors, and two unrelated ones can get the same color. The palette replaces those colors with one hue scale based on the nominal-verbal squish (Ross 1972; Givón's time-stability scale). Hue runs red (proper and common nouns) > orange > yellow (numerals) > green (adjectives) > cyan > blue (adverbs) > violet (verbs), in even OKLCH steps. Function words keep the hue of their position but are dimmer and greyer, and tags off the scale (foreign words, interjections, unspecified relations) are grey. Dependency mode places relations on the same scale: `nsubj`/`obj` sit near the nouns, `amod` with the adjectives, `advmod` with the adverbs, and the clausal relations near the verbs, with `root` in bold.

POS and dependency mode share token types but give them different meanings, so the palette swaps on every `:NSMode`. It also reapplies after `:colorscheme`. It sets only the per-filetype groups (`@lsp.type.variable.text`, ...), which leaves code buffers alone. Override entries per mode:

```lua
require("natural_syntax_ls.colors").setup({
  palettes = { pos = { variable = { fg = "#ff8800" } } },
})
```

Keys are `type` or `type.modifier`, as in `require("natural_syntax_ls.colors").palettes`.

## Memory

`-idle-unload 5m` (default) frees model weights after five idle minutes, so an idle Neovim holds about 70 MB instead of 0.4-2 GB. The next edit reloads in about a second. Flags: [settings.md](settings.md#server-flags).
