local STRESS = "\204\129" -- U+0301 combining acute
local MAX_YO = 3 -- caps extra requests for words with many е

-- Adjective endings by the dictionary form they come from: -ый/-ой after hard stems, -ий after soft ones.
local HARD = { "ого", "ому", "ым", "ом", "ая", "ую", "ое", "ые", "ых", "ыми", "ой", "ою" }
local SOFT = { "его", "ему", "им", "ем", "яя", "юю", "ее", "ие", "их", "ими", "ей", "ею" }
local MIN_STEM = 3 -- characters
local VELAR = { ["г"] = true, ["к"] = true, ["х"] = true, ["ж"] = true, ["ш"] = true, ["ч"] = true, ["щ"] = true }
local HARD_GROUP = { HARD, { "ый", "ой" } }

local function endswith(s, suffix)
	return #s > #suffix and string.sub(s, -#suffix) == suffix
end

local function lowerfirst(word)
	local chars = utf8.chars(word)
	local cp = utf8.codepoint(chars[1])
	if cp >= 0x410 and cp <= 0x42F then
		chars[1] = utf8.char(cp + 0x20)
	elseif cp == 0x401 then
		chars[1] = utf8.char(0x451)
	else
		return nil
	end
	return table.concat(chars)
end

-- Inflected adjectives often lack their own page, so offer the nominative masculine singular.
local function adjective_lemmas(word)
	local out = {}
	for _, group in ipairs({ HARD_GROUP, { SOFT, { "ий" } } }) do
		for _, ending in ipairs(group[1]) do
			if endswith(word, ending) then
				local stem = string.sub(word, 1, #word - #ending)
				if utf8.len(stem) >= MIN_STEM then
					local chars = utf8.chars(stem)
					-- The spelling rule bans ы after г к х ж ш ч щ: русского, not русский with ы.
					local suffixes = VELAR[chars[#chars]] and group == HARD_GROUP and { "ий", "ой" } or group[2]
					for _, suffix in ipairs(suffixes) do
						out[#out + 1] = stem .. suffix
					end
				end
				break
			end
		end
	end
	return out
end

return {
	code = "ru",
	model_prefixes = { "xlmr_ru", "ru_syntagrus" },
	wiktionary = {
		code = "ru",
		sections = { "Russian" },
		-- Headwords carry no stress marks, and ё words are filed under ё though texts write е.
		forms = function(word)
			local function with_lemmas(out)
				for _, lemma in ipairs(adjective_lemmas(lowerfirst(word) or word)) do
					out[#out + 1] = lemma
				end
				return out
			end
			local out = {}
			if string.find(word, STRESS, 1, true) then
				local stripped = string.gsub(word, STRESS, "")
				word = stripped
				out[#out + 1] = word
			end
			if string.find(word, "ё", 1, true) then
				return with_lemmas(out)
			end
			local chars = utf8.chars(word)
			for i, ch in ipairs(chars) do
				if #out >= MAX_YO then
					break
				end
				local yo = (ch == "е" and "ё") or (ch == "Е" and "Ё") or nil
				if yo then
					local cand = { unpack(chars) }
					cand[i] = yo
					out[#out + 1] = table.concat(cand)
				end
			end
			return with_lemmas(out)
		end,
	},
}
