return {
	code = "ar",
	model_prefixes = { "xlmr_ar", "ar_padt" },
	wiktionary = {
		code = "ar",
		sections = { "Arabic" },
		-- Headwords omit tashkeel (U+064B-065F, U+0670) and tatweel (U+0640).
		forms = function(word)
			local keep = {}
			for _, ch in ipairs(utf8.chars(word)) do
				local c = utf8.codepoint(ch)
				if not (c == 0x640 or (c >= 0x64B and c <= 0x65F) or c == 0x670) then
					keep[#keep + 1] = ch
				end
			end
			local plain = table.concat(keep)
			if plain ~= word then
				return { plain }
			end
		end,
	},
}
