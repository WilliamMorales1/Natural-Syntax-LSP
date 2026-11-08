-- Driven by nvim_bench_test.go: nvim --clean --headless -l bench.lua cfg.json
local cfg = vim.json.decode(table.concat(vim.fn.readfile(arg[1]), "\n"))
local uv = vim.uv
local semantic = cfg.mode == "semantic"

local function now()
	return uv.hrtime() / 1e6
end

local function proc_mb(pid, key)
	for l in io.lines("/proc/" .. pid .. "/status") do
		local v = l:match("^" .. key .. ":%s+(%d+) kB")
		if v then
			return tonumber(v) / 1024
		end
	end
end

local function stats(xs)
	table.sort(xs)
	return { min = xs[1], median = xs[math.ceil(#xs / 2)], max = xs[#xs] }
end

local function run()
	local colors = {} -- uri -> semanticColors pushes seen
	local init_ms
	local t0 = now()

	local buf = vim.api.nvim_create_buf(true, false)
	vim.bo[buf].filetype = "text"
	vim.api.nvim_buf_set_name(buf, vim.fs.joinpath(vim.fn.getcwd(), "short.txt"))
	vim.api.nvim_buf_set_lines(buf, 0, -1, false, cfg.short)
	-- Manual requests below time the server; native highlighting would add duplicate load.
	vim.lsp.semantic_tokens.enable(false)

	local id = vim.lsp.start({
		name = "nsls_bench",
		cmd = cfg.cmd,
		root_dir = vim.fn.getcwd(),
		init_options = { wiktionary_definitions = false },
		flags = { debounce_text_changes = 0 },
		handlers = {
			["$/nls/semanticColors"] = function(_, r)
				colors[r.uri] = (colors[r.uri] or 0) + 1
			end,
		},
		on_init = function()
			init_ms = now() - t0
		end,
	}, { bufnr = buf })
	assert(id, "lsp start failed")
	local client = assert(vim.lsp.get_client_by_id(id))

	local pid
	vim.wait(5000, function()
		pid = vim.api.nvim_get_proc_children(vim.fn.getpid())[1]
		return pid ~= nil
	end, 5)
	assert(pid, "server process not found")

	local function req(method, params, b)
		local t = now()
		local done, res, err = false, nil, nil
		client:request(method, params, function(e, r)
			err, res, done = e, r, true
		end, b)
		assert(
			vim.wait(cfg.timeout_ms, function()
				return done
			end, 1),
			method .. " timed out"
		)
		assert(not err, vim.inspect(err))
		return now() - t, res
	end

	-- Returns ms until the server has analysed the buffer's current text.
	local function analysed(b, before)
		local uri = vim.uri_from_bufnr(b)
		local t = now()
		if semantic then
			assert(
				vim.wait(cfg.timeout_ms, function()
					return (colors[uri] or 0) > before
				end, 1),
				"no semanticColors push"
			)
			return now() - t
		end
		-- Requests sent before the model is ready queue; superseded ones come back empty.
		repeat
			local _, res = req("textDocument/semanticTokens/full", { textDocument = { uri = uri } }, b)
			if res and #res.data > 0 then
				return now() - t, #res.data / 5
			end
			assert(now() - t < cfg.timeout_ms, "no tokens returned")
		until false
	end

	local res = {}
	local _, n = analysed(buf, 0)
	res.ready_ms = now() - t0
	res.init_ms = init_ms
	res.tokens_short = n
	res.rss_ready_mb = proc_mb(pid, "VmRSS")

	local long = vim.api.nvim_create_buf(true, false)
	vim.bo[long].filetype = "text"
	vim.api.nvim_buf_set_name(long, vim.fs.joinpath(vim.fn.getcwd(), "long.txt"))
	vim.api.nvim_buf_set_lines(long, 0, -1, false, cfg.long)
	vim.lsp.buf_attach_client(long, id)
	local long_uri = vim.uri_from_bufnr(long)
	res.open_long_ms, res.tokens_long = analysed(long, 0)

	local row
	for i, l in ipairs(cfg.long) do
		if #l > 40 and i > #cfg.long / 2 then
			row = i - 1
			break
		end
	end
	local edits = {}
	for i = 1, cfg.edits do
		local before = colors[long_uri] or 0
		local t = now()
		-- Always a new text: the server caches analysed chunks, so a revert would be a cache hit.
		vim.api.nvim_buf_set_text(long, row, 10, row, 10, { tostring(i % 10) })
		analysed(long, before)
		edits[i] = now() - t
	end
	res.edit_ms = stats(edits)

	local hovers, hits = {}, 0
	local cand = {}
	for i, l in ipairs(cfg.long) do
		if #l > 12 then
			cand[#cand + 1] = i - 1
		end
	end
	for i = 1, cfg.hovers do
		local ms, h = req("textDocument/hover", {
			textDocument = { uri = long_uri },
			position = { line = cand[(i * 7) % #cand + 1], character = 5 },
		}, long)
		hovers[i] = ms
		if h then
			hits = hits + 1
		end
	end
	res.hover_ms = stats(hovers)
	res.hover_hits = hits

	res.rss_end_mb = proc_mb(pid, "VmRSS")
	res.peak_mb = proc_mb(pid, "VmHWM")
	client:stop(true)
	vim.wait(3000, function()
		return client:is_stopped()
	end, 10)
	return res
end

local ok, out = pcall(run)
if not ok then
	out = { error = tostring(out) }
end
vim.fn.writefile({ vim.json.encode(out) }, cfg.out)
