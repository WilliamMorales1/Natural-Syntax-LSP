//go:build nojaseg || english

// Package jaseg is empty in builds with -tags nojaseg or english.
package jaseg

// Segment always reports no segmentation.
func Segment(string) [][2]int { return nil }
