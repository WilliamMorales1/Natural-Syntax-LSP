# Neovim

The server is a standard stdio LSP (`textDocument/semanticTokens/full` + `textDocument/hover`), so it works with any LSP client. POS and dependency coloring use standard semantic tokens and hover works in all three modes. Semantic-mode colors use a custom `$/nls/semanticColors` notification, handled by the config below.

## Setup

Export the models and install the binary first (see [Installation](../README.md#installation)).

`scripts/setup.sh` installs the binary to `~/.local/bin` (`$XDG_BIN_HOME` if set); `scripts/setup.ps1` installs it to `%LOCALAPPDATA%\Programs\natural-syntax-ls` and adds that to your user `PATH`. With it on `PATH`, `cmd = { "natural-syntax-ls" }` is enough.

Copy this into `init.lua` (or `require` it from a file in your config). Needs Neovim 0.11+. It defines the server config, the semantic-mode color handler, `:NSMode`, and `:NSLang`.

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

If you use `mason-lspconfig`, the `vim.lsp.enable` call is still needed: its `automatic_enable` only covers servers Mason installed. With LazyVim, put the snippet in `lua/config/options.lua` or any file under `lua/plugins/` inside a `config` function.

On Neovim 0.10, `vim.lsp.config` does not exist. Register the server through `nvim-lspconfig` (`require("lspconfig.configs")`, `default_config = { cmd, filetypes, root_dir }`, then `require("lspconfig").natural_syntax_ls.setup({})`) and attach the `$/nls/semanticColors` handler in `setup({ handlers = ... })`. `:NSMode` and `:NSLang` are not supported there; change `cmd` and run `:LspRestart`.

## Switching modes

Mode and model are fixed when the server starts, picked by the `-mode`, `-model`, `-vocab` args in `cmd` (same flags as `cmd/natural-syntax-ls`'s CLI). `-vocab` defaults to `<model>_vocab.txt` next to the model, so it is rarely needed. Pass `-model` whenever `-mode` isn't `pos`: without it the server auto-picks a POS model first.

| Mode         | `cmd`                                                                                                                                  |
| ------------ | -------------------------------------------------------------------------------------------------------------------------------------- |
| `pos`        | `{ "natural-syntax-ls" }`                                                                                                              |
| `semantic`   | `{ "natural-syntax-ls", "-mode", "semantic", "-model", vim.fn.expand("~/.config/natural-syntax-ls/models/multilingual_minilm/multilingual_minilm.onnx") }` |
| `dependency` | `{ "natural-syntax-ls", "-mode", "dependency", "-model", vim.fn.expand("~/.config/natural-syntax-ls/models/en_ewt_electra_base_dependency/en_ewt_electra_base_dependency.onnx") }` |

Swap `bert_base` for `mobilebert`, `xlmr_es`, `xlmr_fr`, `xlmr_ru`, `xlmr_ja`, `xlmr_ar`, `xlmr_de`, `xlmr_it`, `xlmr_pt` or `chinese_roberta_upos` (`-model` also works in `pos` mode), and the dependency model for `es_ancora_mbert_dependency`, `fr_sequoia_camembert_large_dependency` or `ru_syntagrus_DeepPavlov_dependency` (the slug keeps the model name's case), to use other models.

To switch at runtime, use `:NSMode pos|semantic|dependency` (defined above).

## Semantic mode colors in Neovim

Semantic mode emits no semantic tokens. The server pushes each word's hex color in a custom `$/nls/semanticColors` notification instead (`{ uri, tokens = { { line, character, length, color } } }`, columns in codepoints). Neovim drops unknown notifications, so the config above includes a handler that paints them as extmarks.

The server re-pushes after every edit, so colors catch up once inference finishes. Tune them with `initializationOptions = { semantic_lightness = 0.6, semantic_chroma = 0.14 }` (lower lightness for light colorschemes).

`-idle-unload 5m` (the default) frees a model's weights once it has gone that long without an edit to analyse, so an idle Neovim holds about 70 MB instead of 0.4-2 GB; the next edit reloads it in about a second. `0` keeps models loaded.

`-threads N` sets ONNX Runtime threads per chunk; the default (0) uses the physical core count, capped at 4, and runs chunks in parallel on any cores left over.

POS tags map to standard semantic token types, so colors come from your colorscheme's `@lsp.type.*` highlight groups. Link any that render plain, e.g. `vim.api.nvim_set_hl(0, "@lsp.type.function", { link = "Function" })`.

## Hover definition language

The default is `detect,en`: the language detected in the hovered line, then English. Set `vim.g.natural_syntax_ls_wiktionary_language = "es,en"` before the server starts, or pass `init_options = { wiktionary_language = "es,en" }` in the LSP config. `:NSLang CODE` restarts the server on one language (defined above).

Values: a code (`en` default, `es`, `fr`, `ja`, `zh`, ...), `model` (the model's language, else `en`), `auto` (best across languages), or a comma-separated priority list (`es,en`: first language with a definition wins).

## Forcing a language

`:NSLang es` (also `fr`, `de`, `ja`, ...) restarts the server for one language, keeping the current mode:

- the model becomes that language's entry in `models.json` for the mode (for example `xlmr-es` in `pos`), else a language-neutral model, else the default;
- detection is switched off, so the tokenizer rules in `lang/es.lua`-style scripts and the hover lookup treat all text as that language;
- hover definitions use `es,en`.

`:NSLang auto` returns to per-line detection and the default models; with an English model, a document in another language is switched to that language's installed model (see settings.md, Language detection). `:NSMode` keeps the forced language. The server flag is `-lang CODE`.
