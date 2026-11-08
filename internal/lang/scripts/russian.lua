local STRESS = "\204\129" -- U+0301 combining acute
local MAX_YO = 3 -- caps extra requests for words with many е

return {
  code = "ru",
  model_prefixes = { "xlmr_ru", "ru_syntagrus" },
  wiktionary = {
    code = "ru",
    sections = { "Russian" },
    -- Headwords carry no stress marks, and ё words are filed under ё though texts write е.
    forms = function(word)
      local out = {}
      if string.find(word, STRESS, 1, true) then
        local stripped = string.gsub(word, STRESS, "")
        word = stripped
        out[#out + 1] = word
      end
      if string.find(word, "ё", 1, true) then
        return out
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
      return out
    end,
  },
}
