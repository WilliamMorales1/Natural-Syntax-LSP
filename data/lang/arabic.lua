local TASHKEEL = {} -- U+064B..U+0652 and the dagger alef U+0670
for cp = 0x64B, 0x652 do
	TASHKEEL[utf8.char(cp)] = true
end
TASHKEEL[utf8.char(0x670)] = true

local AL = utf8.char(0x627) .. utf8.char(0x644) -- ال
local LL = utf8.char(0x644) .. utf8.char(0x644) -- لل: ل + ال with the alef dropped
local PROCLITICS = { 0x648, 0x641, 0x628, 0x643, 0x644 } -- و ف ب ك ل

local function startswith(s, prefix)
	return string.sub(s, 1, #prefix) == prefix
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
			return out
		end,
	},
}
