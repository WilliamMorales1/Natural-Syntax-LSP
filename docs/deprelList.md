# UD Relation Reference

Universal Dependencies relation labels used in dependency mode, and the VS Code semantic token type each maps to (color comes from your theme). See [`internal/tokenmap/deprel_bits.go`](internal/tokenmap/deprel_bits.go).

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
