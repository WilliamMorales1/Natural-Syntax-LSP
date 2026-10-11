-- Default config for vim.lsp.enable("natural_syntax_ls") on Neovim 0.11+; override with vim.lsp.config.
local ns = vim.api.nvim_create_namespace("natural_syntax_ls_colors")

return {
  cmd = { "natural-syntax-ls" },
  filetypes = { "text", "markdown" },
  root_dir = function(_, on_dir)
    on_dir(vim.fn.getcwd())
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
}
