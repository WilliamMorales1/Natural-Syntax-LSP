return {
	code = "en",
	wiktionary = {
		code = "en",
		sections = { "English", "Translingual" },
		-- The possessive clitic lives only on the "-'s" suffix page.
		rewrite = function(word, pos, deprel)
			if pos == "POS" or (deprel == "case" and string.lower(word) == "'s") then
				return "-'s"
			end
		end,
	},
}
