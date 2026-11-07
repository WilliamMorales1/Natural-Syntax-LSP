package wiktionary

import (
	"testing"
)

var testSections = map[string]string{"Chinese": "zh"}

func TestParseWikitext(t *testing.T) {
	text := "==Chinese==\n{{zh-forms}}\n===Etymology 1===\n====Definitions====\n{{head|zh|hanzi}}\n\n# to [[stop]]; to [[finish]]\n#* {{zh-x|a|b}}\n# {{lb|zh|literary}} {{l|en|already}}\n\n===Verb===\n# bad\n"
	got := parseWikitext(text, testSections)["zh"]
	if len(got) != 2 || got[1].PartOfSpeech != "Verb" || got[1].Definitions[0].Definition != "bad" {
		t.Fatalf("heading handling: %+v", got)
	}
	text = "==Chinese==\n===Adverb===\n{{head|zh|adverb}}\n\n# to [[stop]]; to [[finish]]\n#* {{zh-x|a|b}}\n# {{lb|zh|literary}} {{l|en|already}}\n"
	got = parseWikitext(text, testSections)["zh"]
	if len(got) != 1 || len(got[0].Definitions) != 2 || got[0].Definitions[0].Definition != "to stop; to finish" || got[0].Definitions[1].Definition != "(literary) already" {
		t.Fatalf("%+v", got)
	}
}

func TestParseHanziDefinitions(t *testing.T) {
	text := "==Chinese==\n===Etymology 1===\n====Definitions====\n{{head|zh|hanzi}}\n\n# to [[stop]]\n\n===Etymology 2===\n====Definitions====\n{{head|zh|adverb}}\n\n# already\n"
	got := parseWikitext(text, testSections)["zh"]
	if len(got) != 2 || got[0].PartOfSpeech != "Hanzi" || got[1].PartOfSpeech != "Adverb" {
		t.Fatalf("%+v", got)
	}
}
