--- natural_syntax_ls.colors is an optional linguistic palette for POS and dependency mode, replacing the colorscheme's code colors.
local M = {}

-- Hue follows the nominal-verbal category squish (Ross 1972; Givon's time-stability scale):
-- red (proper/common nouns) > orange > yellow (numerals) > green (adjectives) > cyan > blue (adverbs) > violet (verbs),
-- in even OKLCH steps. Function words keep their position's hue, dimmer and grayer; tags off the scale are grey.
-- The number after each tag is its 0-1 position, nominal to verbal.
M.palettes = {
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

local function set(palette, filetypes, clear)
	for group, hl in pairs(palette or {}) do
		local kind = group:find(".", 1, true) and "typemod" or "type"
		for _, ft in ipairs(filetypes) do
			vim.api.nvim_set_hl(0, "@lsp." .. kind .. "." .. group .. "." .. ft, clear and {} or hl)
		end
	end
end

local function apply()
	local ns = require("natural_syntax_ls")
	local fts = ns.filetypes()
	for _, p in pairs(M.palettes) do
		set(p, fts, true)
	end
	set(M.palettes[ns.mode()], fts)
	-- The fallback token is comment+deprecated; empty per-filetype groups stop the colorscheme's italic/strikethrough leaking in.
	for _, ft in ipairs(fts) do
		vim.api.nvim_set_hl(0, "@lsp.type.comment." .. ft, {})
		vim.api.nvim_set_hl(0, "@lsp.mod.deprecated." .. ft, {})
	end
end

--- setup applies the palette now, after every :colorscheme and after every mode switch; opts.palettes overrides entries per mode.
function M.setup(opts)
	for mode, p in pairs((opts or {}).palettes or {}) do
		M.palettes[mode] = vim.tbl_extend("force", M.palettes[mode] or {}, p)
	end
	apply()
	local group = vim.api.nvim_create_augroup("natural_syntax_ls_colors", { clear = true })
	vim.api.nvim_create_autocmd("ColorScheme", { group = group, callback = apply })
	vim.api.nvim_create_autocmd("User", { group = group, pattern = "NaturalSyntaxLsMode", callback = apply })
end

return M
