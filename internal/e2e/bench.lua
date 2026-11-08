-- Driven by nvim_bench_test.go: nvim --clean --headless -l bench.lua cfg.json
local cfg = vim.json.decode(table.concat(vim.fn.readfile(arg[1]), "\n"))
local uv = vim.uv
local semantic = cfg.mode == "semantic"

local function now()
	return uv.hrtime() / 1e6
end

local sys = uv.os_uname().sysname

-- Returns current and peak resident MB; peak is nil where the OS keeps no high-water mark (macOS).
local function mem_mb(pid)
	if sys == "Linux" then
		local cur, peak
		for l in io.lines("/proc/" .. pid .. "/status") do
			cur = cur or tonumber(l:match("^VmRSS:%s+(%d+) kB"))
			peak = peak or tonumber(l:match("^VmHWM:%s+(%d+) kB"))
		end
		return cur / 1024, peak / 1024
	elseif sys:match("Windows") then
		local out = vim.system({
			"powershell",
			"-NoProfile",
			"-Command",
			"$p = Get-Process -Id " .. pid .. "; \"$($p.WorkingSet64) $($p.PeakWorkingSet64)\"",
		}):wait().stdout
		local cur, peak = out:match("(%d+)%s+(%d+)")
		return tonumber(cur) / 2 ^ 20, tonumber(peak) / 2 ^ 20
	end
	local out = vim.system({ "ps", "-o", "rss=", "-p", tostring(pid) }):wait().stdout
	return tonumber(out:match("%d+")) / 1024, nil
end

local sampled_peak = 0

local function sample(pid)
	local cur, peak = mem_mb(pid)
	sampled_peak = math.max(sampled_peak, cur, peak or 0)
	return cur
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
		init_options = { wiktionary_definitions = cfg.wiktionary },
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
	res.rss_ready_mb = sample(pid)

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
		sample(pid)
	end
	res.edit_ms = stats(edits)

	local cand = {}
	for i, l in ipairs(cfg.long) do
		if #l > 12 then
			cand[#cand + 1] = i - 1
		end
	end
	-- First pass may fetch from Wiktionary; the second repeats the same words from the server's cache.
	local function hover_pass()
		local hovers, hits, defs = {}, 0, 0
		for i = 1, cfg.hovers do
			local row = cand[(i * 7) % #cand + 1]
			-- Spread over the line so most hovers land on a word not yet looked up.
			local width = vim.str_utfindex(cfg.long[row + 1], "utf-16")
			local ms, h = req("textDocument/hover", {
				textDocument = { uri = long_uri },
				position = { line = row, character = (i * 11 + 5) % width },
			}, long)
			hovers[i] = ms
			if h then
				hits = hits + 1
				if h.contents.value:find("[Wiktionary](", 1, true) then
					defs = defs + 1
				end
			end
		end
		return stats(hovers), hits, defs
	end
	res.hover_ms, res.hover_hits, res.hover_defs = hover_pass()
	res.hover_warm_ms = hover_pass()

	res.rss_end_mb = sample(pid)
	res.peak_mb = sampled_peak
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
