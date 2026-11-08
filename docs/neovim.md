# Neovim

Needs Neovim 0.11+ and the server on `PATH` ([install.md](install.md)). `scripts/setup.sh` installs it to `~/.local/bin`; `scripts/setup.ps1` to `%LOCALAPPDATA%\Programs\natural-syntax-ls`.

## Config

Copy into `init.lua`, or `require` it from your config. It sets up the server, the semantic color handler, `:NSMode` and `:NSLang`.

```lua
local ns = vim.api.nvim_create_namespace("natural_syntax_ls_colors")
local dir = vim.fs.joinpath(vim.env.XDG_CONFIG_HOME or vim.fs.joinpath(vim.env.HOME, ".config"), "natural-syntax-ls")
local models = {
  pos = "bert_base/bert_base.onnx",
  semantic = "multilingual_minilm/multilingual_minilm.onnx",
  dependency = "en_ewt_electra_base_dependency/en_ewt_electra_base_dependency.onnx",
}

vim.lsp.config("natural_syntax_ls", {
  cmd = { "natural-syntax-ls" },
  filetypes = { "text", "markdown" },
  init_options = { wiktionary_language = vim.g.natural_syntax_ls_wiktionary_language },
  root_dir = function(_, on_dir)
    on_dir(vim.fn.getcwd())
  end,
  -- One server per Neovim; matching on root_dir would start another (and load its models again) after every :cd.
  reuse_client = function(client, config)
    return client.name == config.name and not client:is_stopped()
  end,
  handlers = {
    -- Semantic mode sends hex colors in a custom notification instead of semantic tokens.
    ["$/nls/semanticColors"] = function(_, result)
      local bufnr = vim.uri_to_bufnr(result.uri)
      if not vim.api.nvim_buf_is_loaded(bufnr) then
        return
      end
      vim.api.nvim_buf_clear_namespace(bufnr, ns, 0, -1)
      local lines = vim.api.nvim_buf_get_lines(bufnr, 0, -1, false)
      for _, tok in ipairs(result.tokens) do
        local text = lines[tok.line + 1]
        if text then
          local group = "NlsColor" .. tok.color:sub(2)
          vim.api.nvim_set_hl(0, group, { fg = tok.color })
          vim.api.nvim_buf_set_extmark(bufnr, ns, tok.line, vim.str_byteindex(text, "utf-32", tok.character, false), {
            end_col = vim.str_byteindex(text, "utf-32", tok.character + tok.length, false),
            hl_group = group,
            strict = false, -- buffer may have changed since the push
          })
        end
      end
    end,
  },
})
vim.lsp.enable("natural_syntax_ls")

-- Restarts the server for a mode and an optional forced language ("" detects per line).
local function restart(mode, language)
  vim.g.natural_syntax_ls_mode = mode
  vim.g.natural_syntax_ls_language = language
  vim.lsp.enable("natural_syntax_ls", false)
  -- Stopping is async; enabling before exit lets the dying client claim the buffers.
  vim.wait(5000, function()
    return #vim.lsp.get_clients({ name = "natural_syntax_ls" }) == 0
  end, 50)
  for _, buf in ipairs(vim.api.nvim_list_bufs()) do
    vim.api.nvim_buf_clear_namespace(buf, ns, 0, -1)
  end
  local cmd = { "natural-syntax-ls", "-mode", mode }
  local wikt = vim.g.natural_syntax_ls_wiktionary_language
  if language ~= "" then
    -- The server picks that language's model from models.json.
    vim.list_extend(cmd, { "-lang", language })
    wikt = language .. ",en"
  else
    vim.list_extend(cmd, { "-model", vim.fs.joinpath(dir, "models", models[mode]) })
  end
  vim.lsp.config("natural_syntax_ls", { cmd = cmd, init_options = { wiktionary_language = wikt } })
  vim.lsp.enable("natural_syntax_ls")
end

vim.api.nvim_create_user_command("NSMode", function(opts)
  restart(opts.args, vim.g.natural_syntax_ls_language or "")
end, { nargs = 1, complete = function() return vim.tbl_keys(models) end })

vim.api.nvim_create_user_command("NSLang", function(opts)
  local code = opts.args:lower()
  restart(vim.g.natural_syntax_ls_mode or "pos", code == "auto" and "" or code)
end, {
  nargs = 1,
  -- Language codes come from the model catalog, so a new language needs no edit here.
  complete = function()
    local codes, seen = { "auto" }, {}
    local f = io.open(vim.fs.joinpath(dir, "models.json"))
    local ok, cat = pcall(vim.json.decode, f and f:read("*a") or "{}")
    if f then f:close() end
    for _, m in ipairs(ok and cat.models or {}) do
      if m.language and not seen[m.language] then
        seen[m.language] = true
        codes[#codes + 1] = m.language
      end
    end
    return codes
  end,
})
```

- **mason-lspconfig**: keep the `vim.lsp.enable` call. `automatic_enable` covers only servers Mason installed.
- **LazyVim**: put the snippet in `lua/config/options.lua`, or in a `config` function under `lua/plugins/`.
- **Neovim 0.10**: no `vim.lsp.config`. Register through `nvim-lspconfig` (`require("lspconfig.configs")` with `default_config = { cmd, filetypes, root_dir }`, then `require("lspconfig").natural_syntax_ls.setup({ handlers = ... })`). `:NSMode` and `:NSLang` do not work; edit `cmd` and run `:LspRestart`.

## Commands

| Command | Effect |
| ------- | ------ |
| `:NSMode pos\|semantic\|dependency` | Restart in that mode, with the model path from `models` |
| `:NSLang es` | Force one language, keep the mode |
| `:NSLang auto` | Back to per-line detection and default models |

`:NSLang CODE` restarts with `-lang CODE`:

- the model becomes that language's `models.json` entry for the mode (e.g. `xlmr-es` for `pos`), else a language-neutral model, else the default;
- detection is off, so tokenizer rules and hover treat all text as that language;
- hover definitions use `CODE,en`.

## Other models

Put the model's slug in `models`: the setup name with `-`, `.`, `/` replaced by `_`, case kept, plus `_dependency` for parsers. See [models.md](models.md#files-on-disk). The tokenizer file is found next to the model.

## Colors

POS and dependency tokens use your colorscheme's `@lsp.type.*` groups. Link any that render plain:

```lua
vim.api.nvim_set_hl(0, "@lsp.type.function", { link = "Function" })
```

Semantic mode sends no semantic tokens. The handler above paints `$/nls/semanticColors` as extmarks, one highlight group per color. Tune with `init_options = { semantic_lightness = 0.6, semantic_chroma = 0.14 }`; lower lightness suits light colorschemes.

## Hover language

Default `detect,en`. Set `vim.g.natural_syntax_ls_wiktionary_language = "es,en"` before the server starts, or pass `init_options = { wiktionary_language = ... }`. Values: [features.md](features.md#hover-definitions).

## Memory

`-idle-unload 5m` (default) frees model weights after five idle minutes, so an idle Neovim holds about 70 MB instead of 0.4-2 GB. The next edit reloads in about a second. Flags: [settings.md](settings.md#server-flags).
