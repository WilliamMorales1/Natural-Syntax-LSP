# Neovim

Needs Neovim 0.11+, [lazy.nvim](https://github.com/folke/lazy.nvim) and the server on `PATH` ([install.md](install.md)). `scripts/setup.sh` installs it to `~/.local/bin`; `scripts/setup.ps1` to `%LOCALAPPDATA%\Programs\natural-syntax-ls`. The client sets up the server, the semantic color handler, `:NSMode` and `:NSLang`.

## Install

The repo is not a Neovim plugin. The client is one lazy.nvim spec file, `nvim/natural-syntax-ls.lua`, that goes into your config's `lua/plugins/` beside your other specs. `setup.sh` / `setup.ps1` copy it there when `nvim` is on `PATH` (config dir from `stdpath("config")`, so `NVIM_APPNAME` works). A re-run keeps an existing copy, since it holds your opts; delete it and re-run to update. By hand:

```bash
cp nvim/natural-syntax-ls.lua ~/.config/nvim/lua/plugins/
```

Open a `.txt` or `.md` file. `:checkhealth vim.lsp` should list `natural_syntax_ls`.

## Options

Set them in `opts` of the spec at the bottom of `natural-syntax-ls.lua`. Every field is optional. Defaults:

```lua
opts = {
  cmd = "natural-syntax-ls",           -- binary name or path
  filetypes = { "text", "markdown" },
  mode = "pos",                        -- pos | semantic | dependency
  language = "",                       -- "" detects per line; "es" forces Spanish
  models = {},                         -- per-mode -model path, e.g. { pos = "/path/to/model.onnx" }
  init_options = {},                   -- wiktionary_language, semantic_lightness, semantic_chroma, ...
}
```

With `models` empty, the server picks the catalog default for the mode, or the forced language's model. `init_options` keys: [settings.md](settings.md#initialization-options).

- **mason-lspconfig**: no conflict. `setup` calls `vim.lsp.enable` itself.
- **Neovim 0.10**: no `vim.lsp.config`, so the client does not load. Register the server through `nvim-lspconfig` (`require("lspconfig.configs")` with `default_config = { cmd, filetypes, root_dir }`), then edit `cmd` and run `:LspRestart` to switch modes.

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

Completion lists the languages in the catalogs the server reads. The same functions are exposed as `require("natural_syntax_ls").set_mode(mode)` and `.set_language(code)` for keymaps. `setup` and each switch fire `User NaturalSyntaxLsMode`.

## Colors

POS and dependency tokens use your colorscheme's `@lsp.type.*` groups. Link any that render plain:

```lua
vim.api.nvim_set_hl(0, "@lsp.type.function", { link = "Function" })
```

Semantic mode sends no semantic tokens. The client paints `$/nls/semanticColors` as extmarks, one highlight group per color. Tune with `init_options = { semantic_lightness = 0.6, semantic_chroma = 0.14 }`; lower lightness suits light colorschemes.

### Linguistic palette (optional)

Not installed by default. Paste the block below into `natural-syntax-ls.lua`, at the `-- Paste additions here` line above the final `return`. Lua allows nothing after a chunk's `return`, so it cannot go at the very end.

**Why it exists.** The server reuses code token types for tags (nouns are `variable`, verbs `function`, adjectives `type`), so a colorscheme colors prose by rules made for code. Two related tags can get unrelated colors, and two unrelated ones can get the same color. The palette replaces those colors with one hue scale based on the nominal-verbal squish (Ross 1972; Givón's time-stability scale). Hue runs red (proper and common nouns) > orange > yellow (numerals) > green (adjectives) > cyan > blue (adverbs) > violet (verbs), in even OKLCH steps. Function words keep the hue of their position but are dimmer and greyer, and tags off the scale (foreign words, interjections, unspecified relations) are grey. Dependency mode places relations on the same scale: `nsubj`/`obj` sit near the nouns, `amod` with the adjectives, `advmod` with the adverbs, and the clausal relations near the verbs, with `root` in bold.

POS and dependency mode share token types but give them different meanings, so the palette swaps on every `:NSMode`. It also reapplies after `:colorscheme`. It sets only the per-filetype groups (`@lsp.type.variable.text`, ...), which leaves code buffers alone. Edit the hex values in `palettes` to override; keys are `type` or `type.modifier`.

```lua
-- Linguistic palette: one hue scale for POS and dependency tags instead of the colorscheme's code colors.
-- Hue follows the nominal-verbal category squish (Ross 1972; Givon's time-stability scale):
-- red (proper/common nouns) > orange > yellow (numerals) > green (adjectives) > cyan > blue (adverbs) > violet (verbs),
-- in even OKLCH steps. Function words keep their position's hue, dimmer and grayer; tags off the scale are grey.
-- The number after each tag is its 0-1 position, nominal to verbal.
local palettes = {
	pos = {
		namespace = { fg = "#fb8088" }, -- NNP proper noun (0.00)
		variable = { fg = "#fa8469" }, -- NN noun (0.06)
		["keyword.declaration"] = { fg = "#b27870" }, -- PRP pronoun (0.04, fn)
		macro = { fg = "#aa7f59" }, -- DT determiner (0.16, fn)
		number = { fg = "#dd9d15" }, -- CD numeral (0.22)
		operator = { fg = "#8e8c56" }, -- IN adposition (0.32, fn)
		type = { fg = "#81bf59" }, -- JJ adjective (0.42)
		keyword = { fg = "#629777" }, -- CC conjunction (0.50, fn)
		enumMember = { fg = "#24b8fb" }, -- RB adverb (0.78)
		["operator.modification"] = { fg = "#73a5c9" }, -- RP particle (0.80, fn)
		modifier = { fg = "#7688b7" }, -- MD modal (0.90, fn)
		["function"] = { fg = "#9ba0fe" }, -- VB verb (0.94)
		string = { fg = "#868686" }, -- FW foreign word (grey)
		["string.declaration"] = { fg = "#868686" }, -- UH interjection (grey)
		["comment.deprecated"] = { fg = "#868686" }, -- O, unmapped labels (grey)
	},
	dependency = {
		namespace = { fg = "#fb8088" }, -- compound (0.00)
		["namespace.definition"] = { fg = "#fda5a9" }, -- flat (0.00)
		property = { fg = "#fb8274" }, -- nmod (0.04)
		["property.definition"] = { fg = "#fda894" }, -- appos (0.06)
		["string.declaration"] = { fg = "#fecab8" }, -- vocative (0.08)
		["variable.declaration"] = { fg = "#f48c4a" }, -- nsubj (0.12)
		["variable.readonly"] = { fg = "#feab6e" }, -- iobj (0.14)
		variable = { fg = "#ed9235" }, -- obj (0.16)
		["modifier.definition"] = { fg = "#cf810c" }, -- dislocated (0.18)
		["keyword.abstract"] = { fg = "#ae7c5f" }, -- expl (0.12, fn)
		macro = { fg = "#aa7f59" }, -- det (0.16, fn)
		decorator = { fg = "#c1946e" }, -- clf (0.16, fn)
		number = { fg = "#dd9d15" }, -- nummod (0.22)
		parameter = { fg = "#c0ac16" }, -- obl (0.30)
		["operator.modification"] = { fg = "#8e8c56" }, -- case (0.32, fn)
		type = { fg = "#81bf59" }, -- amod (0.42)
		["keyword.modification"] = { fg = "#629777" }, -- cc (0.50, fn)
		["operator.static"] = { fg = "#44c684" }, -- conj (0.50)
		["decorator.modification"] = { fg = "#5ede99" }, -- list (0.50)
		["method.modification"] = { fg = "#04c5b5" }, -- acl (0.60)
		["variable.definition"] = { fg = "#1ebfda" }, -- csubj (0.70)
		enumMember = { fg = "#24b8fb" }, -- advmod (0.78)
		operator = { fg = "#5f8fb3" }, -- mark (0.80, fn)
		["function.async"] = { fg = "#68affd" }, -- xcomp (0.84)
		["keyword.static"] = { fg = "#6d8bb7" }, -- cop (0.86, fn)
		["function.modification"] = { fg = "#80a9fd" }, -- advcl (0.88)
		["modifier.modification"] = { fg = "#a9bffc" }, -- parataxis (0.90)
		["function"] = { fg = "#93a3fc" }, -- ccomp (0.92)
		keyword = { fg = "#909bce" }, -- aux (0.92, fn)
		class = { fg = "#b595fd", bold = true }, -- root (1.00)
		modifier = { fg = "#868686" }, -- dep (grey)
		string = { fg = "#868686" }, -- discourse (grey)
		["namespace.readonly"] = { fg = "#868686" }, -- fixed (grey)
		["namespace.deprecated"] = { fg = "#868686" }, -- goeswith (grey)
		["modifier.deprecated"] = { fg = "#868686" }, -- orphan, reparandum (grey)
		["comment.deprecated"] = { fg = "#868686" }, -- punct, unmapped relations (grey)
	},
}

local function set_palette(palette, clear)
	for group, hl in pairs(palette) do
		local kind = group:find(".", 1, true) and "typemod" or "type"
		for _, ft in ipairs(M.filetypes()) do
			vim.api.nvim_set_hl(0, "@lsp." .. kind .. "." .. group .. "." .. ft, clear and {} or hl)
		end
	end
end

local function apply_palette()
	for _, p in pairs(palettes) do
		set_palette(p, true)
	end
	set_palette(palettes[M.mode()] or {})
	-- The fallback token is comment+deprecated; empty per-filetype groups stop the colorscheme's italic/strikethrough leaking in.
	for _, ft in ipairs(M.filetypes()) do
		vim.api.nvim_set_hl(0, "@lsp.type.comment." .. ft, {})
		vim.api.nvim_set_hl(0, "@lsp.mod.deprecated." .. ft, {})
	end
end

local palette_group = vim.api.nvim_create_augroup("natural_syntax_ls_colors", { clear = true })
vim.api.nvim_create_autocmd("ColorScheme", { group = palette_group, callback = apply_palette })
vim.api.nvim_create_autocmd("User", { group = palette_group, pattern = "NaturalSyntaxLsMode", callback = apply_palette })
```

## Memory

`-idle-unload 5m` (default) frees model weights after five idle minutes, so an idle Neovim holds about 70 MB instead of 0.4-2 GB. The next edit reloads in about a second. Flags: [settings.md](settings.md#server-flags).
