if vim.g.loaded_natural_syntax_ls or vim.fn.has("nvim-0.11") == 0 then
  return
end
vim.g.loaded_natural_syntax_ls = true

local models = {
  pos = "bert_base.onnx",
  semantic = "mpnet.onnx",
  dependency = "en_ewt_electra_base_dependency.onnx",
}

-- Same model dir as the server, setup.sh, and the VS Code extension.
local function config_dir()
  if vim.fn.has("win32") == 1 then
    return vim.env.APPDATA
  end
  return vim.env.XDG_CONFIG_HOME or vim.fs.joinpath(vim.env.HOME, ".config")
end

vim.api.nvim_create_user_command("NlsMode", function(opts)
  local dir = vim.fs.joinpath(config_dir(), "natural-syntax-ls")
  -- Disabling stops the clients; re-enabling is what re-attaches open buffers.
  vim.lsp.enable("natural_syntax_ls", false)
  -- Stopping is async; enabling before exit lets the dying client claim the buffers.
  vim.wait(5000, function()
    return #vim.lsp.get_clients({ name = "natural_syntax_ls" }) == 0
  end, 50)
  local ns = vim.api.nvim_create_namespace("natural_syntax_ls_colors")
  for _, buf in ipairs(vim.api.nvim_list_bufs()) do
    vim.api.nvim_buf_clear_namespace(buf, ns, 0, -1)
  end
  vim.lsp.config("natural_syntax_ls", {
    cmd = { "natural-syntax-ls", "-mode", opts.args, "-model", vim.fs.joinpath(dir, models[opts.args]) },
  })
  vim.lsp.enable("natural_syntax_ls")
end, {
  nargs = 1,
  complete = function()
    return vim.tbl_keys(models)
  end,
})
