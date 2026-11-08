package luaload

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	lua "github.com/yuin/gopher-lua"

	"natural-syntax-ls/internal/lang"
	"natural-syntax-ls/internal/postag"
)

// Parse runs a script and converts the table it returns; errors name the script and field.
func Parse(name, src string) (lang.Language, error) {
	sb := newSandbox(name)
	v, err := sb.run(src)
	if err != nil {
		return lang.Language{}, fmt.Errorf("%s: %w", name, err)
	}
	t, ok := v.(*lua.LTable)
	if !ok {
		return lang.Language{}, fmt.Errorf("%s: script must return a table, got %s", name, v.Type())
	}
	l, err := convert(sb, t)
	if err != nil {
		return lang.Language{}, fmt.Errorf("%s: %w", name, err)
	}
	return l, nil
}

func convert(sb *sandbox, t *lua.LTable) (lang.Language, error) {
	var l lang.Language
	if err := checkKeys(t, "", "code", "model_prefixes", "tokenizer", "wiktionary"); err != nil {
		return l, err
	}
	var err error
	if l.Code, err = str(t, "code"); err != nil || l.Code == "" {
		return l, errors.New("field code: must be a non-empty string")
	}
	if l.ModelPrefixes, err = strList(t, "model_prefixes"); err != nil {
		return l, err
	}
	for i, p := range l.ModelPrefixes {
		l.ModelPrefixes[i] = strings.ToLower(p)
	}
	if tt, ok := t.RawGetString("tokenizer").(*lua.LTable); ok {
		if l.Tokenizer, err = tokenizerOptions(tt); err != nil {
			return l, err
		}
	} else if t.RawGetString("tokenizer") != lua.LNil {
		return l, errors.New("field tokenizer: must be a table")
	}
	switch wt := t.RawGetString("wiktionary").(type) {
	case *lua.LTable:
		if l.Wiktionary, err = wiktionary(sb, wt); err != nil {
			return l, err
		}
	case *lua.LNilType:
	default:
		return l, errors.New("field wiktionary: must be a table")
	}
	return l, nil
}

func wiktionary(sb *sandbox, t *lua.LTable) (*lang.Wiktionary, error) {
	if err := checkKeys(t, "wiktionary.", "code", "sections", "rewrite", "forms", "wikitext", "script"); err != nil {
		return nil, err
	}
	w := &lang.Wiktionary{}
	var err error
	if w.Code, err = str(t, "code"); err != nil || w.Code == "" {
		return nil, fmt.Errorf("field wiktionary.code: must be a non-empty string")
	}
	if w.Sections, err = strList(t, "sections"); err != nil {
		return nil, err
	}
	if b, ok := t.RawGetString("wikitext").(lua.LBool); ok {
		w.Wikitext = bool(b)
	}
	if name, ok := t.RawGetString("script").(lua.LString); ok {
		tab := unicode.Scripts[string(name)]
		if tab == nil {
			return nil, fmt.Errorf("field wiktionary.script: unknown Unicode script %q", string(name))
		}
		w.Script = tab
	}
	switch v := t.RawGetString("rewrite").(type) {
	case *lua.LFunction:
		w.Rewrite = func(word string, pos postag.PartOfSpeech, deprel postag.Deprel) string {
			res, ok := sb.call(v, lua.LString(word), lua.LString(pos.String()), lua.LString(deprel.String()))
			if s, isStr := res.(lua.LString); ok && isStr {
				return string(s)
			}
			return ""
		}
	case *lua.LNilType:
	default:
		return nil, fmt.Errorf("field wiktionary.rewrite: must be a function")
	}
	switch v := t.RawGetString("forms").(type) {
	case *lua.LFunction:
		w.Forms = func(word string) []string {
			res, ok := sb.call(v, lua.LString(word))
			tab, isTab := res.(*lua.LTable)
			if !ok || !isTab {
				return nil
			}
			var out []string
			tab.ForEach(func(_, val lua.LValue) {
				if s, isStr := val.(lua.LString); isStr && s != "" {
					out = append(out, string(s))
				}
			})
			return out
		}
	case *lua.LNilType:
	default:
		return nil, fmt.Errorf("field wiktionary.forms: must be a function")
	}
	return w, nil
}

func checkKeys(t *lua.LTable, prefix string, allowed ...string) error {
	var bad error
	t.ForEach(func(k, _ lua.LValue) {
		if ks, ok := k.(lua.LString); !ok || !slices.Contains(allowed, string(ks)) {
			bad = fmt.Errorf("unknown field %s%v (allowed: %s)", prefix, k, strings.Join(allowed, ", "))
		}
	})
	return bad
}

func str(t *lua.LTable, key string) (string, error) {
	switch v := t.RawGetString(key).(type) {
	case lua.LString:
		return string(v), nil
	case *lua.LNilType:
		return "", nil
	default:
		return "", fmt.Errorf("field %s: must be a string", key)
	}
}

func strList(t *lua.LTable, key string) ([]string, error) {
	switch v := t.RawGetString(key).(type) {
	case *lua.LNilType:
		return nil, nil
	case *lua.LTable:
		var out []string
		for i := 1; i <= v.Len(); i++ {
			s, ok := v.RawGetInt(i).(lua.LString)
			if !ok {
				return nil, fmt.Errorf("field %s[%d]: must be a string", key, i)
			}
			out = append(out, string(s))
		}
		return out, nil
	default:
		return nil, fmt.Errorf("field %s: must be a list of strings", key)
	}
}

// ParseFS loads every *.lua file directly under dir of fsys, returning the languages that loaded and one error per script that did not.
func ParseFS(fsys fs.FS, dir string) ([]lang.Language, []error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, []error{err}
	}
	var langs []lang.Language
	var errs []error
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".lua" {
			continue
		}
		path := e.Name()
		if dir != "." {
			path = dir + "/" + path
		}
		src, err := fs.ReadFile(fsys, path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		l, err := Parse(e.Name(), string(src))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		langs = append(langs, l)
	}
	return langs, errs
}

// LoadDir loads the *.lua files of an on-disk directory, logging scripts that fail; a missing directory yields nothing.
func LoadDir(dir string) []lang.Language {
	if dir == "" {
		return nil
	}
	langs, errs := ParseFS(os.DirFS(dir), ".")
	for _, err := range errs {
		slog.Error("language script", "dir", dir, "err", err)
	}
	for _, l := range langs {
		slog.Info("language script loaded", "code", l.Code, "dir", dir)
	}
	return langs
}

// Merge returns the union of the lists; a later language replaces an earlier one with the same code.
func Merge(lists ...[]lang.Language) []lang.Language {
	var out []lang.Language
	for _, list := range lists {
		for _, l := range list {
			if i := slices.IndexFunc(out, func(o lang.Language) bool { return o.Code == l.Code }); i >= 0 {
				out[i] = l
			} else {
				out = append(out, l)
			}
		}
	}
	return out
}
