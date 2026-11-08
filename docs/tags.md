# Tag Reference

Both lists map tags to VS Code semantic token types; color comes from your theme.

## POS tags

Penn Treebank tags used in POS mode. See [`internal/tokenmap/pos_bits.go`](../internal/tokenmap/pos_bits.go).

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

## UD relations

Universal Dependencies labels used in dependency mode. See [`internal/tokenmap/deprel_bits.go`](../internal/tokenmap/deprel_bits.go).

| # | Relation | Meaning | Example | Token Type |
|---|---|---|---|---|
| 1 | acl | Clausal modifier of noun (adjectival clause) | the man who I saw | `method` (modification) |
| 2 | advcl | Adverbial clause modifier | I left because she arrived | `function` (modification) |
| 3 | advmod | Adverbial modifier | she left quickly | `enumMember` |
| 4 | amod | Adjectival modifier | the big house | `type` |
| 5 | appos | Appositional modifier | my sister, a doctor, ... | `property` (definition) |
| 6 | aux | Auxiliary | she is running | `keyword` |
| 7 | case | Case marking | the office of the president | `operator` (modification) |
| 8 | cc | Coordinating conjunction | you and me | `keyword` (modification) |
| 9 | ccomp | Clausal complement | he says that you like him | `function` |
| 10 | clf | Classifier | three cup tea | `decorator` |
| 11 | compound | Compound | phone book | `namespace` |
| 12 | conj | Conjunct | Sam and Bob | `operator` (static) |
| 13 | cop | Copula | Bill is honest | `keyword` (static) |
| 14 | csubj | Clausal subject | what she said makes sense | `variable` (definition) |
| 15 | dep | Unspecified dependency | (fallback relation) | `modifier` |
| 16 | det | Determiner | the man | `macro` |
| 17 | discourse | Discourse element | Bob, hi! | `string` |
| 18 | dislocated | Dislocated elements | Bob, he was such a nice guy | `modifier` (definition) |
| 19 | expl | Expletive | it is raining | `keyword` (abstract) |
| 20 | fixed | Fixed multiword expression | as well as | `namespace` (readonly) |
| 21 | flat | Flat multiword expression | Boston University | `namespace` (definition) |
| 22 | goeswith | Goes with (word fragment) | more over ("moreover") | `namespace` (deprecated) |
| 23 | iobj | Indirect object | she gave me a raise | `variable` (readonly) |
| 24 | list | List | Tom, Dick, Harry | `decorator` (modification) |
| 25 | mark | Marker (subordinating conjunction) | she said that he was fine | `operator` |
| 26 | nmod | Nominal modifier | the office of the president | `property` |
| 27 | nsubj | Nominal subject | she runs | `variable` (declaration) |
| 28 | nummod | Numeric modifier | I have four cats | `number` |
| 29 | obj | Direct object | she gave me a raise | `variable` |
| 30 | obl | Oblique nominal | she gave me a raise on Tuesday | `parameter` |
| 31 | orphan | Orphan (in gapping) | Fred bought apples, Bob oranges | `modifier` (deprecated) |
| 32 | parataxis | Parataxis | the guy, John said, left early | `modifier` (modification) |
| 33 | punct | Punctuation [filtered out] | Done! | `comment` (deprecated) |
| 34 | reparandum | Overridden disfluency | she was, she went to school | `modifier` (deprecated, definition) |
| 35 | root | Root | (the sentence's main predicate) | `class` (declaration) |
| 36 | vocative | Vocative | Bill, are you OK? | `string` (declaration) |
| 37 | xcomp | Open clausal complement | she wants to leave | `function` (async) |
