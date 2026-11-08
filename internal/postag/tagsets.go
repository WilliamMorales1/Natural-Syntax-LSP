package postag

import "strings"

// universalOf maps a label from the Universal, EAGLES or CKIP tag inventory to its Universal POS name.
// EAGLES labels are all caps with at least two characters ("NCMS000"); CKIP labels have a lowercase letter, or are one of its bare capitals ("D", "P").
func universalOf(label string) (string, bool) {
	if _, ok := upos[label]; ok {
		return label, true
	}
	if label == "" {
		return "", false
	}
	if strings.ToUpper(label) == label && len(label) >= 2 && !ckipCaps(label) {
		return eagles(label)
	}
	return ckip(label)
}

// ckipCaps reports CKIP labels written entirely in capitals, which would otherwise read as EAGLES.
func ckipCaps(label string) bool {
	return label == "DE" || label == "SHI" || label == "FW" || strings.HasSuffix(label, "CATEGORY")
}

func eagles(label string) (string, bool) {
	switch label[0] {
	case 'A':
		return "ADJ", true
	case 'R':
		return "ADV", true
	case 'D':
		return "DET", true
	case 'N':
		switch label[1] {
		case 'P':
			return "PROPN", true
		case 'C':
			return "NOUN", true
		}
	case 'P':
		return "PRON", true
	case 'C':
		if label[1] == 'S' {
			return "SCONJ", true
		}
		return "CCONJ", true
	case 'S':
		return "ADP", true
	case 'V':
		if label[1] == 'A' {
			return "AUX", true
		}
		return "VERB", true
	case 'Z', 'W':
		return "NUM", true
	case 'I':
		return "INTJ", true
	case 'F':
		return "PUNCT", true
	}
	return "", false
}

func ckip(label string) (string, bool) {
	switch {
	case strings.HasSuffix(label, "CATEGORY"):
		return "PUNCT", true
	case label == "DE":
		return "PART", true
	case label == "SHI", label == "V_2", label[0] == 'V':
		return "VERB", true
	case label == "FW":
		return "X", true
	case label == "Di":
		return "PART", true
	case label == "Cba", label == "Cbb":
		return "SCONJ", true
	case label == "Neu":
		return "NUM", true
	case label == "Nh":
		return "PRON", true
	case label == "Nb":
		return "PROPN", true
	case label == "Nes", label == "Nep", strings.HasPrefix(label, "Neq"):
		return "DET", true
	}
	switch label[0] {
	case 'N':
		return "NOUN", true
	case 'A':
		return "ADJ", true
	case 'C':
		return "CCONJ", true
	case 'D':
		return "ADV", true
	case 'P':
		return "ADP", true
	case 'T':
		return "PART", true
	case 'I':
		return "INTJ", true
	case 'F': // mixed-case EAGLES such as "Fc", which CKIP never uses
		return "PUNCT", true
	case 'Z', 'W':
		return "NUM", true
	}
	return "", false
}
