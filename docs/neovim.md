# Neovim

The server is a standard stdio LSP (`textDocument/semanticTokens/full` + `textDocument/hover`), so it works with any LSP client. POS and dependency coloring use standard semantic tokens and hover works in all three modes. Semantic-mode colors use a custom `$/nls/semanticColors` notification that Neovim ignores unless you add the handler under [Semantic mode colors in Neovim](#semantic-mode-colors-in-neovim).

## Setup

Export the models and install the binary first (see [Installation](../README.md#installation)).

`scripts/setup.sh` installs the binary to `~/.local/bin` (`$XDG_BIN_HOME` if set); `scripts/setup.ps1` installs it to `%LOCALAPPDATA%\Programs\natural-syntax-ls` and adds that to your user `PATH`. With it on `PATH`, `cmd = { "natural-syntax-ls" }` is enough.

**Plain Neovim 0.11+** (built-in `vim.lsp.config`, no plugins required), e.g. in `init.lua`:

```lua
vim.lsp.config("natural_syntax_ls", {
  cmd = { "natural-syntax-ls" },
  filetypes = { "text", "markdown" },
  root_dir = function(bufnr, on_dir)
    on_dir(vim.fn.getcwd())
  end,
})
vim.lsp.enable("natural_syntax_ls")
```

If you use `mason-lspconfig`, you still need the `vim.lsp.enable` call — its `automatic_enable` only covers servers Mason installed.

**Neovim 0.10 with `nvim-lspconfig`:**

```lua
local configs = require("lspconfig.configs")
if not configs.natural_syntax_ls then
  configs.natural_syntax_ls = {
    default_config = {
      cmd = { "natural-syntax-ls" },
      filetypes = { "text", "markdown" },
      root_dir = function() return vim.fn.getcwd() end,
    },
  }
end
require("lspconfig").natural_syntax_ls.setup({})
```

**LazyVim:**

```lua
-- lua/plugins/natural-syntax-ls.lua
return {
  "neovim/nvim-lspconfig",
  opts = {
    servers = {
      natural_syntax_ls = {
        mason = false,
        cmd = { "natural-syntax-ls" },
        filetypes = { "text", "markdown" },
        root_dir = function(bufnr, on_dir)
          on_dir(vim.fn.getcwd())
        end,
      },
    },
  },
}
```

## Switching modes

Mode and model are fixed when the server starts, picked by the `-mode`, `-model`, `-vocab` args in `cmd` (same flags as `cmd/natural-syntax-ls`'s CLI). `-vocab` defaults to `<model>_vocab.txt` next to the model, so it is rarely needed. Pass `-model` whenever `-mode` isn't `pos`: without it the server auto-picks a POS model first.

| Mode         | `cmd`                                                                                                                                  |
| ------------ | -------------------------------------------------------------------------------------------------------------------------------------- |
| `pos`        | `{ "natural-syntax-ls" }`                                                                                                              |
| `semantic`   | `{ "natural-syntax-ls", "-mode", "semantic", "-model", vim.fn.expand("~/.config/natural-syntax-ls/mpnet.onnx") }`                      |
| `dependency` | `{ "natural-syntax-ls", "-mode", "dependency", "-model", vim.fn.expand("~/.config/natural-syntax-ls/en_ewt_electra_base_dependency.onnx") }` |

Swap `mpnet.onnx` for `minilm.onnx`, or `bert_base.onnx` for `mobilebert.onnx` (`-model` also works in `pos` mode), to use the lighter models.

To switch at runtime, restart the server with a new `cmd`. On Neovim 0.11+, this adds `:NlsMode pos|semantic|dependency` (put it after the `vim.lsp.config` block above):

```lua
local nls_dir = vim.fn.expand("~/.config/natural-syntax-ls/")
local nls_models = {
  pos = "bert_base.onnx",
  semantic = "mpnet.onnx",
  dependency = "en_ewt_electra_base_dependency.onnx",
}

vim.api.nvim_create_user_command("NlsMode", function(opts)
  for _, client in ipairs(vim.lsp.get_clients({ name = "natural_syntax_ls" })) do
    client:stop()
  end
  local ns = vim.api.nvim_create_namespace("natural_syntax_ls_colors") -- semantic-mode colors, see below
  for _, buf in ipairs(vim.api.nvim_list_bufs()) do
    vim.api.nvim_buf_clear_namespace(buf, ns, 0, -1)
  end
  vim.lsp.config("natural_syntax_ls", {
    cmd = { "natural-syntax-ls", "-mode", opts.args, "-model", nls_dir .. nls_models[opts.args] },
  })
  vim.lsp.enable("natural_syntax_ls") -- re-attaches to open buffers
end, {
  nargs = 1,
  complete = function() return vim.tbl_keys(nls_models) end,
})
```

On Neovim 0.10 with `nvim-lspconfig`, call `require("lspconfig").natural_syntax_ls.setup({ cmd = { ... } })` again with the new `cmd`, then `:LspRestart`.

## Semantic mode colors in Neovim

Semantic mode emits no semantic tokens. The server pushes each word's hex color in a custom `$/nls/semanticColors` notification instead (`{ uri, tokens = { { line, character, length, color } } }`, columns in codepoints). Neovim drops unknown notifications, so add a handler that paints them as extmarks (Neovim 0.11+; `:NlsMode` above clears them on switch):

```lua
local nls_ns = vim.api.nvim_create_namespace("natural_syntax_ls_colors")

vim.lsp.config("natural_syntax_ls", {
  handlers = {
    ["$/nls/semanticColors"] = function(_, result)
      local bufnr = vim.uri_to_bufnr(result.uri)
      if not vim.api.nvim_buf_is_loaded(bufnr) then
        return
      end
      vim.api.nvim_buf_clear_namespace(bufnr, nls_ns, 0, -1)
      local lines = vim.api.nvim_buf_get_lines(bufnr, 0, -1, false)
      for _, tok in ipairs(result.tokens) do
        local text = lines[tok.line + 1]
        if text then
          local group = "NlsColor" .. tok.color:sub(2)
          vim.api.nvim_set_hl(0, group, { fg = tok.color })
          vim.api.nvim_buf_set_extmark(bufnr, nls_ns, tok.line, vim.str_byteindex(text, "utf-32", tok.character, false), {
            end_col = vim.str_byteindex(text, "utf-32", tok.character + tok.length, false),
            hl_group = group,
            strict = false, -- buffer may have changed since the push
          })
        end
      end
    end,
  },
})
```

The server re-pushes after every edit, so colors catch up once inference finishes. Tune them with `initializationOptions = { semantic_lightness = 0.6, semantic_chroma = 0.14 }` (lower lightness for light colorschemes).

`-threads N` sets ONNX Runtime threads per chunk; the default (0) uses the physical core count, capped at 4, and runs chunks in parallel on any cores left over.

POS tags map to standard semantic token types, so colors come from your colorscheme's `@lsp.type.*` highlight groups. Link any that render plain, e.g. `vim.api.nvim_set_hl(0, "@lsp.type.function", { link = "Function" })`.
