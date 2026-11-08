# Examples

One file per language with a model in [models.md](../models.md#catalog). Each file is one paragraph, so semantic mode colors it as a whole. Line 1 is the same sentence in every language for comparison. The other lines target cases the tokenizer, POS model or parser has to get right. Across them they also use words for animals, food, technology and feelings, which semantic mode should give separate color families.

[test.txt](test.txt) is the quick-start file. [semantic-example.txt](semantic-example.txt) is built for semantic mode.

| File | Lines 2 onward show |
| ---- | ------------------- |
| [english.txt](english.txt) | Homographs that change POS (`saw`, `can`, `record` as verb then noun), PP attachment (`with the telescope`), double object (`fed the cat some cheese`), object relative clause with passive, concessive and causal clauses |
| [spanish.txt](spanish.txt) | Contractions `del` and `al`, enclitic pronouns (`dámelo`), clitic doubling (`Le di a mi hermana`), subjunctive after `ojalá` and `antes de que`, reflexive `se` |
| [french.txt](french.txt) | Elisions (`l'ami`, `qu'est-ce que`, `jusqu'à`, `aujourd'hui` kept whole), contractions `du` and `au`, subjunctive with expletive `ne`, participle agreement in a relative clause, `ne ... jamais` |
| [german.txt](german.txt) | Separable verbs (`räumte ... ein`, `mitgebracht`), V2 with a fronted adverb, verb-final subordinate clauses, `im`/`vom`, object-first order marked only by case (`Dem Mann gab die Frau`), long compounds |
| [italian.txt](italian.txt) | Articulated prepositions (`della`, `sul`, `nel`, `al`), clitic clusters (`Gliel'ho`, `se lo`), elision (`c'è`, `po'`), past infinitive, `essere` auxiliary, subjunctive after `benché` |
| [portuguese.txt](portuguese.txt) | Contractions (`do`, `no`, `pelo`, `num`, `na`, `da`), crase (`à`, `às`), enclitic `dá-lo`, hyphenated `quarta-feira` joined back into one word, future subjunctive (`quiser`) |
| [russian.txt](russian.txt) | Case-marked inflection for hover lemmas (`чаек`, `лодкой`, `бурей`), object-first order (`Книгу ... прочитала моя сестра`), genitive of negation with partitive `чаю`, dative experiencer (`Детям было скучно`), aspect pairs, reflexive `-ся` |
| [arabic.txt](arabic.txt) | Clitic splits: `و` + `ب` + `الكتاب`, future `س`, conjunction `ف`, `لل` as `ل` + `ال`, pronoun suffixes (`أصدقاءه`, `أرسلتها`); tanween alef (`جائعا`) and sound feminine plural (`الطالبات`) for hover lemmas |
| [japanese.txt](japanese.txt) | kagome segmentation of kana text, particles, `てしまう`, passive (`褒められた`), relative clauses, benefactive `てくれる`, negative conditional, `たかった` + `のに` |
| [chinese.txt](chinese.txt) | Word joining without spaces (`北京`, `大学`, `自然`, `语言`), `把` and `被` constructions, classifier (`一顿`), relative `的` clause, comparative `比`, resultative complements (`吃完`, `睡着`); Traditional characters use the same models |
| [mixed.txt](mixed.txt) | One line per language. Line-level detection drives hover definitions and tokenizer rules (French elisions, Arabic clitics on their own lines only). One model analyses the whole document, so POS and dependency tags are only good on the lines in the model's language; semantic mode with the multilingual model handles every line. The last paragraph has lines too short to detect on their own, which borrow the language of a nearby line |
