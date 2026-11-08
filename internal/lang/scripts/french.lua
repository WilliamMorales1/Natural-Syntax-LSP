return {
  code = "fr",
  model_prefixes = { "xlmr_fr", "fr_sequoia" },
  -- UD treats elided clitics as their own words.
  tokenizer = {
    elisions = {
      clitics = { "l", "d", "j", "m", "t", "s", "n", "c", "qu", "jusqu", "lorsqu", "puisqu", "quoiqu" },
      words = { "aujourd'hui" },
    },
  },
  wiktionary = { code = "fr", sections = { "French" } },
}
