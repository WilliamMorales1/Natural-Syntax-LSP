// Package models is the catalog of known model files: which to look for by default.
package models

// DefaultStems lists model file stems in search order: English POS models first, then embedding models.
var DefaultStems = [][]string{{"bert_base", "mobilebert"}, {"multilingual_minilm", "mpnet", "minilm"}}
