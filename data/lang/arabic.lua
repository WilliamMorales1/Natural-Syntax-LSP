local TASHKEEL = {} -- U+064B..U+0652, the dagger alef U+0670 and tatweel U+0640
for cp = 0x64B, 0x652 do
	TASHKEEL[utf8.char(cp)] = true
end
TASHKEEL[utf8.char(0x670)] = true
TASHKEEL[utf8.char(0x640)] = true

local ALEF = utf8.char(0x627)
local AL = ALEF .. utf8.char(0x644) -- ال
local LL = utf8.char(0x644) .. utf8.char(0x644) -- لل: ل + ال with the alef dropped
local PROCLITICS = { 0x648, 0x641, 0x628, 0x643, 0x644 } -- و ف ب ك ل
local FATHATAN = utf8.char(0x64B)
local AT = ALEF .. utf8.char(0x62A) -- ات
local TAA_MARBUTA = utf8.char(0x629)

local function startswith(s, prefix)
	return string.sub(s, 1, #prefix) == prefix
end

local function endswith(s, suffix)
	return #s > #suffix and string.sub(s, -#suffix) == suffix
end

-- A pronoun suffix moves a final hamza onto a seat and turns ة into ت (دمائها, مدرستها); headwords keep the bare form.
local BEFORE_SUFFIX = {
	[utf8.char(0x626)] = utf8.char(0x621), -- ئ to ء
	[utf8.char(0x624)] = utf8.char(0x621), -- ؤ to ء
	[utf8.char(0x62A)] = TAA_MARBUTA, -- ت to ة
}

local function unsuffixed(word)
	local chars = utf8.chars(word)
	local last = BEFORE_SUFFIX[chars[#chars]]
	if #chars < 3 or not last then
		return nil
	end
	chars[#chars] = last
	return table.concat(chars)
end

-- Variants of a word with its article and marks gone, most likely headword first.
local function stem_forms(core, tanween)
	local out = {}
	local n = utf8.len(core)
	-- Accusative tanween is written on an extra alef (ارتفاعاً); the headword has neither.
	if tanween and endswith(core, ALEF) and n > 3 then
		core = string.sub(core, 1, #core - #ALEF)
		out[#out + 1] = core
	end
	-- Sound feminine plurals: جماعات to جماعة.
	if endswith(core, AT) and n > 4 then
		out[#out + 1] = string.sub(core, 1, #core - #AT) .. TAA_MARBUTA
	else
		out[#out + 1] = unsuffixed(core)
	end
	-- Texts often drop the hamza of an initial alef (ادارة); headwords keep it.
	if startswith(core, ALEF) and n > 2 then
		local tail = string.sub(core, #ALEF + 1)
		out[#out + 1] = utf8.char(0x625) .. tail -- إ
		out[#out + 1] = utf8.char(0x623) .. tail -- أ
	end
	return out
end

return {
	code = "ar",
	model_prefixes = { "xlmr_ar" },
	tokenizer = {
		-- Split و ف ب ل ك س and pronoun suffixes into the syntactic words PADT-trained models tag.
		arabic_clitics = true,
	},
	wiktionary = {
		code = "ar",
		sections = { "Arabic" },
		-- Headwords carry no vowel marks or definite article; one-letter proclitics may precede the article.
		forms = function(word)
			local out = {}
			local chars = {}
			for _, ch in ipairs(utf8.chars(word)) do
				if not TASHKEEL[ch] then
					chars[#chars + 1] = ch
				end
			end
			local bare = table.concat(chars)
			if bare ~= word then
				out[#out + 1] = bare
			end
			local rest
			if startswith(bare, LL) and #bare > #LL then
				rest = string.sub(bare, #LL + 1)
			elseif startswith(bare, AL) and #bare > #AL then
				rest = string.sub(bare, #AL + 1)
			else
				for _, cp in ipairs(PROCLITICS) do
					local pre = utf8.char(cp) .. AL
					if startswith(bare, pre) and #bare > #pre then
						rest = string.sub(bare, #pre + 1)
						break
					end
				end
			end
			if rest then
				out[#out + 1] = rest
			end
			for _, f in ipairs(stem_forms(rest or bare, string.find(word, FATHATAN, 1, true) ~= nil)) do
				out[#out + 1] = f
			end
			return out
		end,
	},
}
