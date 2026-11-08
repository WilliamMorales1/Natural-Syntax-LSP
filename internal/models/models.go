// Package models reads the model catalog, models.json, from the config directories. No model name lives in Go source.
package models

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Model is one catalog entry.
type Model struct {
	Name        string `json:"name"`
	Mode        string `json:"mode"`
	Source      string `json:"source"`
	Language    string `json:"language"`
	Description string `json:"description"`
	// Default marks models the server picks when none is given, in catalog order.
	Default bool `json:"default"`
	// Int8 marks models that keep their accuracy when quantized.
	Int8 bool `json:"int8"`
}

// Stem returns the model's file stem: the name with - . / turned into _, plus _dependency for parsers.
func (m Model) Stem() string {
	s := strings.NewReplacer("-", "_", ".", "_", "/", "_").Replace(m.Name)
	if m.Mode == "dependency" {
		s += "_dependency"
	}
	return s
}

// Catalog is the model list.
type Catalog struct {
	Models []Model `json:"models"`
}

// Load reads models.json from each dir in turn; a later file replaces earlier entries with the same name and appends new ones. Missing files are skipped.
func Load(dirs ...string) (Catalog, error) {
	var out Catalog
	for _, d := range dirs {
		b, err := os.ReadFile(filepath.Join(d, "models.json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return out, err
		}
		var c Catalog
		if err := json.Unmarshal(b, &c); err != nil {
			return out, &os.PathError{Op: "parse", Path: filepath.Join(d, "models.json"), Err: err}
		}
		for _, m := range c.Models {
			replaced := false
			for i := range out.Models {
				if out.Models[i].Name == m.Name {
					out.Models[i], replaced = m, true
				}
			}
			if !replaced {
				out.Models = append(out.Models, m)
			}
		}
	}
	return out, nil
}

// DefaultStems lists default model stems for mode in catalog order; an empty mode lists every default.
func (c Catalog) DefaultStems(mode string) []string {
	var out []string
	for _, m := range c.Models {
		if m.Default && (mode == "" || m.Mode == mode) {
			out = append(out, m.Stem())
		}
	}
	return out
}
