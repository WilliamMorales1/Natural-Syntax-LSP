-- natural-syntax-ls client for lazy.nvim: server config, semantic colors, :NSMode and :NSLang.
-- Edit opts in the spec at the bottom. Additions such as the linguistic palette (docs/neovim.md) go above the final return.
local M = {}

local name = "natural_syntax_ls"
local ns = vim.api.nvim_create_namespace("natural_syntax_ls_colors")
local modes = { "pos", "semantic", "dependency" }

local state = {
	cmd = "natural-syntax-ls",
	mode = "pos",
	language = "",
	filetypes = { "text", "markdown" },
	models = {},
	init_options = {},
}

-- Semantic mode sends hex colors in a custom notification instead of semantic tokens.
local function paint(_, result)
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
end

local function cmd()
	local c = { state.cmd, "-mode", state.mode }
	if state.language ~= "" then
		vim.list_extend(c, { "-lang", state.language })
	end
	if state.models[state.mode] then
		vim.list_extend(c, { "-model", state.models[state.mode] })
	end
	return c
end

local function init_options()
	local o = vim.deepcopy(state.init_options)
	if state.language ~= "" and o.wiktionary_language == nil then
		o.wiktionary_language = state.language .. ",en"
	end
	-- An empty table encodes as a JSON array, which the server rejects.
	return next(o) and o or vim.empty_dict()
end

local function config_dir()
	if vim.fn.has("win32") == 1 then
		return vim.fs.joinpath(vim.env.APPDATA, "natural-syntax-ls")
	end
	return vim.fs.joinpath(vim.env.XDG_CONFIG_HOME or vim.fs.joinpath(vim.env.HOME, ".config"), "natural-syntax-ls")
end

--- languages lists the language codes in the model catalogs the server reads.
function M.languages()
	local dirs = { config_dir() }
	local exe = vim.fn.exepath(state.cmd)
	if exe ~= "" then
		local bin = vim.fs.dirname(vim.uv.fs_realpath(exe) or exe)
		vim.list_extend(dirs, { bin, vim.fs.joinpath(bin, "..", "data") })
	end
	local codes, seen = {}, {}
	for _, d in ipairs(dirs) do
		local f = io.open(vim.fs.joinpath(d, "models.json"))
		if f then
			local ok, cat = pcall(vim.json.decode, f:read("*a"))
			f:close()
			for _, m in ipairs(ok and cat.models or {}) do
				if m.language and m.language ~= vim.NIL and not seen[m.language] then
					seen[m.language] = true
					codes[#codes + 1] = m.language
				end
			end
		end
	end
	table.sort(codes)
	return codes
end

local function restart()
	vim.lsp.enable(name, false)
	-- Stopping is async; enabling before exit lets the dying client claim the buffers.
	vim.wait(5000, function()
		return #vim.lsp.get_clients({ name = name }) == 0
	end, 50)
	for _, buf in ipairs(vim.api.nvim_list_bufs()) do
		vim.api.nvim_buf_clear_namespace(buf, ns, 0, -1)
	end
	vim.lsp.config(name, { cmd = cmd(), init_options = init_options() })
	vim.lsp.enable(name)
	vim.api.nvim_exec_autocmds("User", { pattern = "NaturalSyntaxLsMode", data = { mode = state.mode } })
end

--- mode returns the running mode.
function M.mode()
	return state.mode
end

--- filetypes returns the filetypes the server attaches to.
function M.filetypes()
	return state.filetypes
end

--- set_mode restarts the server in mode: pos, semantic or dependency.
function M.set_mode(mode)
	if not vim.tbl_contains(modes, mode) then
		vim.notify("natural-syntax-ls: unknown mode " .. mode, vim.log.levels.ERROR)
		return
	end
	state.mode = mode
	restart()
end

--- set_language restarts the server forced to one language code, or per-line detection for "" or "auto".
function M.set_language(code)
	code = code:lower()
	state.language = code == "auto" and "" or code
	restart()
end

--- setup registers and enables the server; opts: cmd, filetypes, mode, language, models, init_options.
function M.setup(opts)
	opts = opts or {}
	state.cmd = opts.cmd or state.cmd
	state.mode = opts.mode or state.mode
	state.language = opts.language or state.language
	state.models = opts.models or state.models
	state.init_options = opts.init_options or state.init_options
	state.filetypes = opts.filetypes or state.filetypes

	vim.lsp.config(name, {
		cmd = cmd(),
		filetypes = state.filetypes,
		init_options = init_options(),
		root_dir = function(_, on_dir)
			on_dir(vim.fn.getcwd())
		end,
		-- One server per Neovim; matching on root_dir would start another (and load its models again) after every :cd.
		reuse_client = function(client, config)
			return client.name == config.name and not client:is_stopped()
		end,
		handlers = { ["$/nls/semanticColors"] = paint },
	})
	vim.lsp.enable(name)

	vim.api.nvim_create_user_command("NSMode", function(o)
		M.set_mode(o.args)
	end, {
		nargs = 1,
		complete = function()
			return modes
		end,
	})
	vim.api.nvim_create_user_command("NSLang", function(o)
		M.set_language(o.args)
	end, {
		nargs = 1,
		complete = function()
			return vim.list_extend({ "auto" }, M.languages())
		end,
	})
	vim.api.nvim_exec_autocmds("User", { pattern = "NaturalSyntaxLsMode", data = { mode = state.mode } })
end

-- Lets keymaps elsewhere call require("natural_syntax_ls").set_mode(...).
package.loaded["natural_syntax_ls"] = M

-- Paste additions here, above the return.

return {
	"natural-syntax-ls",
	virtual = true,
	opts = {},
	config = function(_, opts)
		M.setup(opts)
	end,
}
