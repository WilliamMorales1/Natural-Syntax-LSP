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

-- Noun and verb endings to dictionary forms, longest first; the first matching ending wins per table.
local NOUN = {
	{ "остями", { "ость" } }, { "остях", { "ость" } }, { "остям", { "ость" } }, { "остью", { "ость" } }, { "остей", { "ость" } }, { "ости", { "ость" } },
	{ "ениями", { "ение" } }, { "ениях", { "ение" } }, { "ением", { "ение" } }, { "ения", { "ение" } }, { "ению", { "ение" } }, { "ении", { "ение" } }, { "ений", { "ение" } },
	{ "иями", { "ия", "ие" } }, { "ия", { "ий", "ие" } }, { "иях", { "ия", "ие" } }, { "ией", { "ия" } }, { "ием", { "ие" } }, { "ию", { "ия", "ие" } }, { "ии", { "ия", "ие" } }, { "ий", { "ия", "ие" } },
	{ "ами", { "", "а", "о" } }, { "ями", { "ь", "я", "е" } }, { "ах", { "", "а", "о" } }, { "ях", { "ь", "я", "е" } },
	{ "ам", { "", "а" } }, { "ям", { "ь", "я" } }, { "ов", { "", "о" } }, { "ев", { "ь", "й" } }, { "ей", { "ь", "я" } },
	{ "ом", { "", "о" } }, { "ой", { "а" } }, { "ою", { "а" } }, { "ем", { "ь", "й", "е" } }, { "ью", { "ь" } },
	{ "ы", { "", "а" } }, { "у", { "", "а" } }, { "а", { "", "о" } }, { "и", { "а", "ь", "я" } }, { "е", { "", "а", "о" } }, { "ю", { "ь", "я" } }, { "я", { "ь", "е" } },
}
local VERB = {
	{ "ывает", { "ывать" } }, { "ивает", { "ивать" } }, { "ывают", { "ывать" } }, { "ивают", { "ивать" } },
	{ "ает", { "ать" } }, { "ают", { "ать" } }, { "аешь", { "ать" } }, { "аем", { "ать" } }, { "аете", { "ать" } }, { "аю", { "ать" } },
	{ "яет", { "ять" } }, { "яют", { "ять" } }, { "яю", { "ять" } }, { "еет", { "еть" } }, { "еют", { "еть" } },
	{ "ует", { "овать" } }, { "уют", { "овать" } }, { "ую", { "овать" } },
	{ "ала", { "ать" } }, { "али", { "ать" } }, { "ало", { "ать" } }, { "ял", { "ять" } }, { "яла", { "ять" } }, { "яли", { "ять" } },
	{ "ела", { "еть" } }, { "ели", { "еть" } }, { "ила", { "ить" } }, { "или", { "ить" } }, { "ило", { "ить" } },
	{ "ишь", { "ить" } }, { "ите", { "ить" } }, { "ит", { "ить", "еть" } }, { "ят", { "ить", "еть" } }, { "ал", { "ать" } }, { "ил", { "ить" } }, { "ел", { "еть" } },
}
local MAX_LEMMAS = 4 -- each candidate costs a request when the word itself misses

local function by_endings(word, rules, min_stem, wrap)
	local out = {}
	for _, rule in ipairs(rules) do
		local ending = rule[1]
		if endswith(word, ending) then
			local stem = string.sub(word, 1, #word - #ending)
			-- One-letter endings need a longer stem: все is not a form of вс.
			if utf8.len(stem) >= (utf8.len(ending) == 1 and min_stem or 2) then
				for _, suffix in ipairs(rule[2]) do
					out[#out + 1] = wrap(stem .. suffix)
				end
			end
			break
		end
	end
	return out
end

-- Reflexive verbs keep -ся on their headword: учится, учился to учиться.
local function verb_lemmas(word)
	for _, refl in ipairs({ "ся", "сь" }) do
		if endswith(word, refl) then
			return by_endings(string.sub(word, 1, #word - #refl), VERB, 2, function(v)
				return v .. "ся"
			end)
		end
	end
	return by_endings(word, VERB, 2, function(v)
		return v
	end)
end

local function noun_lemmas(word)
	if endswith(word, "ся") or endswith(word, "сь") then
		return {}
	end
	return by_endings(word, NOUN, MIN_STEM, function(v)
		return v
	end)
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
				local base = lowerfirst(word) or word
				local groups = { adjective_lemmas(base), verb_lemmas(base), noun_lemmas(base) }
				if base ~= word then -- proper nouns keep their capital: Париже to Париж
					table.insert(groups, 3, noun_lemmas(word))
				end
				local added, seen = 0, {}
				-- Round-robin, since the hook has no POS to say which reading is right.
				for i = 1, 3 do
					for _, lemmas in ipairs(groups) do
						local lemma = lemmas[i]
						if lemma and added < MAX_LEMMAS and lemma ~= base and not seen[lemma] then
							seen[lemma] = true
							added = added + 1
							out[#out + 1] = lemma
						end
					end
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
