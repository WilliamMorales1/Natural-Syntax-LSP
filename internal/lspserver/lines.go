package lspserver

import (
	"errors"
	"fmt"
	"slices"
)

// lineIndex maps between rune offsets and LSP line/UTF-16 positions for one text.
type lineIndex struct {
	runes  []rune
	starts []int // rune offset of each line start; a final newline opens an empty last line
}

func newLineIndex(runes []rune) *lineIndex {
	starts := []int{0}
	for i, r := range runes {
		if r == '\n' {
			starts = append(starts, i+1)
		}
	}
	return &lineIndex{runes: runes, starts: starts}
}

func utf16Units(r rune) int {
	if r > 0xffff {
		return 2
	}
	return 1
}

// lineOf returns the line containing the rune offset.
func (ix *lineIndex) lineOf(offset int) int {
	i, found := slices.BinarySearch(ix.starts, offset)
	if found {
		return i
	}
	return i - 1
}

// position converts a rune offset to a line and UTF-16 column.
func (ix *lineIndex) position(offset int) (line, col int) {
	line = ix.lineOf(offset)
	for _, r := range ix.runes[ix.starts[line]:min(offset, len(ix.runes))] {
		col += utf16Units(r)
	}
	return line, col
}

// utf16Len returns the UTF-16 length of the rune range [begin, end).
func (ix *lineIndex) utf16Len(begin, end int) int {
	n := 0
	for _, r := range ix.runes[min(begin, len(ix.runes)):min(end, len(ix.runes))] {
		n += utf16Units(r)
	}
	return n
}

// offset converts a line and UTF-16 column to a rune offset; ok is false when the position is past the line end or splits a surrogate pair.
func (ix *lineIndex) offset(line, col uint32) (int, bool) {
	if int(line) >= len(ix.starts) {
		return 0, false
	}
	start := ix.starts[line]
	end := len(ix.runes)
	if int(line)+1 < len(ix.starts) {
		end = ix.starts[line+1] - 1
	}
	units := uint32(0)
	for i := start; ; i++ {
		if units == col {
			return i, true
		}
		if i == end {
			return 0, false
		}
		units += uint32(utf16Units(ix.runes[i]))
		if units > col {
			return 0, false
		}
	}
}

// errBadPosition marks a content-change range that does not fit the document.
var errBadPosition = errors.New("invalid content change position")

// applyContentChange replaces the range r of text with replacement.
func applyContentChange(text string, r contentChangeRange, replacement string) (string, error) {
	runes, err := applyRuneChange([]rune(text), r, replacement)
	return string(runes), err
}

func applyRuneChange(runes []rune, r contentChangeRange, replacement string) ([]rune, error) {
	ix := newLineIndex(runes)
	start, ok := ix.offset(r.Start.Line, r.Start.Character)
	if !ok {
		return nil, fmt.Errorf("%w: start", errBadPosition)
	}
	end, ok := ix.offset(r.End.Line, r.End.Character)
	if !ok || end < start {
		return nil, fmt.Errorf("%w: end", errBadPosition)
	}
	return slices.Concat(runes[:start:start], []rune(replacement), runes[end:]), nil
}
