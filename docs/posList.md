# POS Tag Reference

Penn Treebank part-of-speech tags used in POS mode, and the VS Code semantic token type each maps to (color comes from your theme). See [`internal/tokenmap/pos_bits.go`](internal/tokenmap/pos_bits.go).

| # | Tag | Meaning | Example | Token Type |
|---|---|---|---|---|
| 1 | CC | Coordinating conjunction | and | `keyword` |
| 2 | CD | Cardinal number | one | `number` |
| 3 | DT | Determiner | the | `macro` |
| 4 | EX | Existential there | there is | `keyword` (abstract) |
| 5 | FW | Foreign word | おはようございます | `string` |
| 6 | IN | Preposition, subordinating conj. | in, that | `operator` |
| 7 | JJ | Adjective | big | `type` |
| 8 | JJR | Adjective, comparative | more | `struct` |
| 9 | JJS | Adjective, superlative | most | `interface` |
| 10 | LS | List item marker | Example: 1) | `decorator` |
| 11 | MD | Modal | should | `modifier` |
| 12 | NN | Noun, singular or mass | thing, water | `variable` |
| 13 | NNS | Noun, plural | things | `variable` (modification) |
| 14 | NNP | Proper noun, singular | California | `namespace` |
| 15 | NNPS | Proper noun, plural | The Obamas | `typeParameter` |
| 16 | PDT | Predeterminer | all the boys | `macro` (definition) |
| 17 | POS | Possessive ending | Mary's dog | `operator` (definition) |
| 18 | PRP | Personal pronoun | you | `keyword` (declaration) |
| 19 | PRP$ | Possessive pronoun | your | `property` |
| 20 | RB | Adverb | quickly | `enumMember` |
| 21 | RBR | Adverb, comparative | faster | `enumMember` (async) |
| 22 | RBS | Adverb, superlative | most carefully | `enumMember` (defaultLibrary) |
| 23 | RP | Particle | give up | `operator` (modification) |
| 24 | SYM | Symbol [filtered out] | % | `operator` (documentation) |
| 25 | TO | to | to | `keyword` (static) |
| 26 | UH | Interjection | uh | `string` (declaration) |
| 27 | VB | Verb, base form | be | `function` |
| 28 | VBD | Verb, past tense | was | `function` (modification) |
| 29 | VBG | Verb, gerund, present participle | being | `function` (async) |
| 30 | VBN | Verb, past participle | been | `method` (defaultLibrary) |
| 31 | VBP | Verb, non-3rd person sing. pres. | are | `function` (readonly) |
| 32 | VBZ | Verb, 3rd person singular present | is | `method` (static) |
| 33 | WDT | Wh-determiner | what thing | `macro` (modification) |
| 34 | WP | Wh-pronoun | what | `regexp` |
| 35 | WP$ | Possessive wh-pronoun | whose | `property` (declaration) |
| 36 | WRB | Wh-adverb | when | `enumMember` (modification) |

Everything else (unrecognized tags) falls back to `comment` (deprecated), same as O (other/punctuation).
