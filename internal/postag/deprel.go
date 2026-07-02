package postag

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Deprel is a Universal Dependencies relation label (https://universaldependencies.org/u/dep/).
type Deprel int

const (
	DEP_ACL        Deprel = 0
	DEP_ADVCL      Deprel = 1
	DEP_ADVMOD     Deprel = 2
	DEP_AMOD       Deprel = 3
	DEP_APPOS      Deprel = 4
	DEP_AUX        Deprel = 5
	DEP_CASE       Deprel = 6
	DEP_CC         Deprel = 7
	DEP_CCOMP      Deprel = 8
	DEP_CLF        Deprel = 9
	DEP_COMPOUND   Deprel = 10
	DEP_CONJ       Deprel = 11
	DEP_COP        Deprel = 12
	DEP_CSUBJ      Deprel = 13
	DEP_DEP        Deprel = 14
	DEP_DET        Deprel = 15
	DEP_DISCOURSE  Deprel = 16
	DEP_DISLOCATED Deprel = 17
	DEP_EXPL       Deprel = 18
	DEP_FIXED      Deprel = 19
	DEP_FLAT       Deprel = 20
	DEP_GOESWITH   Deprel = 21
	DEP_IOBJ       Deprel = 22
	DEP_LIST       Deprel = 23
	DEP_MARK       Deprel = 24
	DEP_NMOD       Deprel = 25
	DEP_NSUBJ      Deprel = 26
	DEP_NUMMOD     Deprel = 27
	DEP_OBJ        Deprel = 28
	DEP_OBL        Deprel = 29
	DEP_ORPHAN     Deprel = 30
	DEP_PARATAXIS  Deprel = 31
	DEP_PUNCT      Deprel = 32
	DEP_REPARANDUM Deprel = 33
	DEP_ROOT       Deprel = 34
	DEP_VOCATIVE   Deprel = 35
	DEP_XCOMP      Deprel = 36

	N_DEPREL = 37
)

var deprelToString = [N_DEPREL]string{
	"acl", "advcl", "advmod", "amod", "appos", "aux", "case", "cc",
	"ccomp", "clf", "compound", "conj", "cop", "csubj", "dep", "det",
	"discourse", "dislocated", "expl", "fixed", "flat", "goeswith", "iobj",
	"list", "mark", "nmod", "nsubj", "nummod", "obj", "obl", "orphan",
	"parataxis", "punct", "reparandum", "root", "vocative", "xcomp",
}

var deprelFromString = func() map[string]Deprel {
	m := make(map[string]Deprel, N_DEPREL)
	for i, s := range deprelToString {
		m[s] = Deprel(i)
	}
	return m
}()

func (d Deprel) String() string {
	if d >= 0 && int(d) < N_DEPREL {
		return deprelToString[d]
	}
	return fmt.Sprintf("dep(%d)", int(d))
}

func (d Deprel) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Deprel) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v, ok := deprelFromString[strings.ToLower(s)]
	if !ok {
		return fmt.Errorf("unknown deprel tag: %s", s)
	}
	*d = v
	return nil
}

// ParseDeprel maps a UD relation string to a Deprel, stripping any ":subtype" suffix (e.g. "nsubj:pass" -> DEP_NSUBJ).
func ParseDeprel(s string) (Deprel, bool) {
	base, _, _ := strings.Cut(s, ":")
	rel, ok := deprelFromString[strings.ToLower(base)]
	return rel, ok
}
