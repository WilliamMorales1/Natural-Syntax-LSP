package postag

import (
	"fmt"
	"strings"
)

// Deprel is a Universal Dependencies relation label (https://universaldependencies.org/u/dep/).
type Deprel int

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

func (d Deprel) String() string {
	if d >= 0 && d < NumDeprels {
		return deprelNames[d]
	}
	return fmt.Sprintf("dep(%d)", int(d))
}

func (d Deprel) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

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
