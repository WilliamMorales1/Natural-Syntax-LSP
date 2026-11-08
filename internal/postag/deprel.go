package postag

import (
	"fmt"
	"strings"
)

// Deprel is a Universal Dependencies relation label (https://universaldependencies.org/u/dep/).
type Deprel uint8

// UD relation labels.
const (
	DepAcl Deprel = iota
	DepAdvcl
	DepAdvmod
	DepAmod
	DepAppos
	DepAux
	DepCase
	DepCc
	DepCcomp
	DepClf
	DepCompound
	DepConj
	DepCop
	DepCsubj
	DepDep
	DepDet
	DepDiscourse
	DepDislocated
	DepExpl
	DepFixed
	DepFlat
	DepGoeswith
	DepIobj
	DepList
	DepMark
	DepNmod
	DepNsubj
	DepNummod
	DepObj
	DepObl
	DepOrphan
	DepParataxis
	DepPunct
	DepReparandum
	DepRoot
	DepVocative
	DepXcomp

	NumDeprels = iota
)

// NoDeprel marks a token with no dependency relation (POS and semantic modes), unlike the zero value DepAcl.
const NoDeprel Deprel = NumDeprels

var deprelNames = [NumDeprels]string{
	"acl", "advcl", "advmod", "amod", "appos", "aux", "case", "cc",
	"ccomp", "clf", "compound", "conj", "cop", "csubj", "dep", "det",
	"discourse", "dislocated", "expl", "fixed", "flat", "goeswith", "iobj",
	"list", "mark", "nmod", "nsubj", "nummod", "obj", "obl", "orphan",
	"parataxis", "punct", "reparandum", "root", "vocative", "xcomp",
}

var deprelByName = func() map[string]Deprel {
	m := make(map[string]Deprel, NumDeprels)
	for i, s := range deprelNames {
		m[s] = Deprel(i)
	}
	return m
}()

// String returns the UD relation name, "" for NoDeprel, or dep(n) when out of range.
func (d Deprel) String() string {
	if d < NumDeprels {
		return deprelNames[d]
	}
	if d == NoDeprel {
		return ""
	}
	return fmt.Sprintf("dep(%d)", int(d))
}

// MarshalText encodes d as its relation name.
func (d Deprel) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

// UnmarshalText decodes a UD relation name, case-insensitively.
func (d *Deprel) UnmarshalText(text []byte) error {
	v, ok := deprelByName[strings.ToLower(string(text))]
	if !ok {
		return fmt.Errorf("unknown deprel tag: %s", text)
	}
	*d = v
	return nil
}

// ParseDeprel maps a UD relation string to a Deprel, stripping any ":subtype" suffix (e.g. "nsubj:pass" -> DepNsubj).
func ParseDeprel(s string) (Deprel, bool) {
	base, _, _ := strings.Cut(s, ":")
	rel, ok := deprelByName[strings.ToLower(base)]
	return rel, ok
}
