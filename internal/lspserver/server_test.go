package lspserver

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"natural-syntax-ls/internal/langid"
	"slices"
	"strings"
	"testing"
	"time"

	"natural-syntax-ls/internal/postag"
	"natural-syntax-ls/internal/tokenmap"
)

func frame(body string) string {
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
}

func TestReadMessage(t *testing.T) {
	in := frame(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`) +
		"Content-Type: application/vscode-jsonrpc; charset=utf-8\r\n" + frame(`{"jsonrpc":"2.0","method":"initialized","params":{}}`)
	r := bufio.NewReader(strings.NewReader(in))
	for _, want := range []string{"initialize", "initialized"} {
		msg, err := readMessage(r)
		if err != nil || msg.Method != want {
			t.Fatalf("got %q, %v; want %q", msg.Method, err, want)
		}
	}
	if _, err := readMessage(r); !errors.Is(err, io.EOF) {
		t.Errorf("after last message: %v, want EOF", err)
	}

	if _, err := readMessage(bufio.NewReader(strings.NewReader("X-Other: 1\r\n\r\n{}"))); err == nil {
		t.Error("missing Content-Length accepted")
	}
	if _, err := readMessage(bufio.NewReader(strings.NewReader(frame(`{not json`)))); err == nil {
		t.Error("malformed JSON accepted")
	}
}

func TestWriteResponse(t *testing.T) {
	id := json.RawMessage(`7`)
	payload := func(result any, rpcErr *rpcError) map[string]json.RawMessage {
		var buf bytes.Buffer
		(&lspServer{writer: &buf}).writeResponse(&id, result, rpcErr)
		header, body, _ := strings.Cut(buf.String(), "\r\n\r\n")
		if header != fmt.Sprintf("Content-Length: %d", len(body)) {
			t.Errorf("header %q for %d-byte body", header, len(body))
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal([]byte(body), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	// JSON-RPC requires "result" on success even when null.
	if ok := payload(nil, nil); string(ok["result"]) != "null" || string(ok["id"]) != "7" {
		t.Errorf("success: %v", ok)
	}
	if bad := payload(nil, &rpcError{Code: codeMethodNotFound, Message: "nope"}); bad["error"] == nil || bad["result"] != nil {
		t.Errorf("error: %v", bad)
	}
}

func TestLineIndex(t *testing.T) {
	for _, tc := range []struct {
		text  string
		lines []int
	}{
		{"", []int{0}},
		{"abc", []int{0}},
		{"abc\n", []int{0, 4}},
		{"a\nb\n\nc", []int{0, 2, 4, 5}},
		{"é\nü", []int{0, 2}}, // rune offsets, not bytes
	} {
		ix := newLineIndex([]rune(tc.text))
		if !slices.Equal(ix.starts, tc.lines) {
			t.Errorf("starts(%q) = %v, want %v", tc.text, ix.starts, tc.lines)
		}
		for line, start := range tc.lines {
			if got := ix.lineOf(start); got != line {
				t.Errorf("%q: lineOf(%d) = %d, want %d", tc.text, start, got, line)
			}
			if got, ok := ix.offset(uint32(line), 0); !ok || got != start {
				t.Errorf("%q: offset(%d,0) = %d, %v, want %d", tc.text, line, got, ok, start)
			}
		}
		for off := range len([]rune(tc.text)) + 1 {
			line, col := ix.position(off)
			if got, ok := ix.offset(uint32(line), uint32(col)); !ok || got != off {
				t.Errorf("%q: round trip %d -> %d:%d -> %d, %v", tc.text, off, line, col, got, ok)
			}
		}
	}
	if _, ok := newLineIndex([]rune("a\nb")).offset(5, 0); ok {
		t.Error("offset past last line accepted")
	}
	// An astral rune takes two UTF-16 units.
	ix := newLineIndex([]rune("a😀b"))
	if _, col := ix.position(2); col != 3 {
		t.Errorf("column after astral rune = %d, want 3", col)
	}
	if n := ix.utf16Len(1, 2); n != 2 {
		t.Errorf("utf16Len = %d, want 2", n)
	}
}

func TestApplyContentChange(t *testing.T) {
	text := "one two\nthree four"
	var err error
	text, err = applyContentChange(text, contentChangeRange{
		Start: documentPosition{Line: 0, Character: 3},
		End:   documentPosition{Line: 0, Character: 7},
	}, "")
	if err != nil || text != "one\nthree four" {
		t.Fatalf("delete line text: %q, %v", text, err)
	}
	text, err = applyContentChange(text, contentChangeRange{
		Start: documentPosition{Line: 1, Character: 0},
		End:   documentPosition{Line: 1, Character: 5},
	}, "four three")
	if err != nil || text != "one\nfour three four" {
		t.Fatalf("move/replacement text: %q, %v", text, err)
	}

	text, err = applyContentChange("é😀z", contentChangeRange{
		Start: documentPosition{Line: 0, Character: 1},
		End:   documentPosition{Line: 0, Character: 3},
	}, "x")
	if err != nil || text != "éxz" {
		t.Fatalf("UTF-16 text: %q, %v", text, err)
	}
	if _, err := applyContentChange("é😀z", contentChangeRange{
		Start: documentPosition{Line: 0, Character: 2},
		End:   documentPosition{Line: 0, Character: 2},
	}, "x"); err == nil {
		t.Error("accepted position inside a UTF-16 surrogate pair")
	}
}

func TestDidChangeAppliesIncrementalEdits(t *testing.T) {
	s := &lspServer{}
	s.handle(jsonrpcMsg{Method: "textDocument/didOpen", Params: json.RawMessage(`{"textDocument":{"uri":"file:///edit.txt","version":1,"text":"one two\nthree four"}}`)})
	s.handle(jsonrpcMsg{Method: "textDocument/didChange", Params: json.RawMessage(`{"textDocument":{"uri":"file:///edit.txt","version":2},"contentChanges":[{"range":{"start":{"line":0,"character":3},"end":{"line":0,"character":7}},"text":""}]}`)})
	s.handle(jsonrpcMsg{Method: "textDocument/didChange", Params: json.RawMessage(`{"textDocument":{"uri":"file:///edit.txt","version":3},"contentChanges":[{"range":{"start":{"line":1,"character":0},"end":{"line":1,"character":5}},"text":"four three"}]}`)})

	got := s.documents.get("file:///edit.txt").text
	if got != "one\nfour three four" {
		t.Fatalf("incremental edits produced %q", got)
	}
}

func TestEncodeSemanticTokens(t *testing.T) {
	text := "the dog\nran é far"
	doc := &document{text: text, tokens: []postag.Token{
		{Word: "the", Tag: postag.DT, OffsetBegin: 0, OffsetEnd: 3},
		{Word: "dog", Tag: postag.NN, OffsetBegin: 4, OffsetEnd: 7},
		{Word: "ran", Tag: postag.VBD, OffsetBegin: 8, OffsetEnd: 11},
		{Word: "far", Tag: postag.RB, OffsetBegin: 14, OffsetEnd: 17},
	}}
	tm := tokenmap.NewDefault()
	bits := func(p postag.PartOfSpeech) (uint32, uint32) {
		b := tm.Get(p)
		return b.TokenType, b.TokenModifierBitset
	}
	var want []uint32
	for _, row := range [][3]uint32{{0, 0, 3}, {0, 4, 3}, {1, 0, 3}, {0, 6, 3}} {
		want = append(want, row[:]...)
	}
	for i, p := range []postag.PartOfSpeech{postag.DT, postag.NN, postag.VBD, postag.RB} {
		ty, mod := bits(p)
		want = slices.Insert(want, i*5+3, ty, mod)
	}
	if got := encodeSemanticTokens(doc, &tm, false); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	// A disabled tag is skipped and the next token's delta spans the gap.
	tm.Extend(map[postag.PartOfSpeech]*tokenmap.Override{postag.NN: nil, postag.VBD: nil})
	got := encodeSemanticTokens(doc, &tm, false)
	ty, mod := bits(postag.RB)
	if len(got) != 10 || !slices.Equal(got[5:], []uint32{1, 6, 3, ty, mod}) {
		t.Errorf("with NN/VBD disabled: %v", got)
	}
}

func TestFormatHoverContent(t *testing.T) {
	pos := &postag.Token{Word: "dog", Tag: postag.NN, Score: 0.912}
	if got := formatHoverContent(pos, nil, "", ""); !strings.Contains(got, "```yaml\ndog: ") || !strings.Contains(got, "# 0.91") {
		t.Errorf("POS hover: %q", got)
	}
	sem := &postag.Token{Word: "dog", Color: "#112233", Description: "Semantic color #112233"}
	if got := formatHoverContent(sem, nil, "", ""); strings.Contains(got, "#") && strings.Contains(got, "# 0.") {
		t.Errorf("semantic hover shows a score: %q", got)
	}
	head := &postag.Token{Word: "ran", HasHead: true, Deprel: postag.DepRoot}
	deps := []dependent{{postag.Token{Word: "dog", Deprel: postag.DepNsubj}, true}, {postag.Token{Word: "far", Deprel: postag.DepAdvmod}, false}}
	got := formatHoverContent(head, deps, "to move quickly", "https://en.wiktionary.org/wiki/run")
	for _, want := range []string{"```yaml\nran: root\ndependents:\n", "  dog: nsubj  # has dependents\n", "  far: advmod\n```", "to move quickly", "[Wiktionary](https://en.wiktionary.org/wiki/run)"} {
		if !strings.Contains(got, want) {
			t.Errorf("dependency hover missing %q in %q", want, got)
		}
	}
}

// TestServerRoundTrip drives the JSON-RPC loop with a fake model: open a document, then request semantic tokens and a hover.
func TestServerRoundTrip(t *testing.T) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	s := &lspServer{ready: make(chan struct{})}
	s.wiktionary.Store(false)
	s.registry.Store(newDocumentRegistry(contextModel{}))
	close(s.ready)
	go s.serve(inR, outW)

	send := func(body string) {
		if _, err := io.WriteString(inW, frame(body)); err != nil {
			t.Fatal(err)
		}
	}
	responses := make(chan jsonrpcMsg)
	go func() {
		r := bufio.NewReader(outR)
		for {
			msg, err := readMessage(r)
			if err != nil {
				close(responses)
				return
			}
			responses <- msg
		}
	}()
	await := func(id string) json.RawMessage {
		t.Helper()
		timeout := time.After(10 * time.Second)
		for {
			select {
			case msg := <-responses:
				if msg.ID != nil && string(*msg.ID) == id {
					b, _ := json.Marshal(msg.Result)
					return b
				}
			case <-timeout:
				t.Fatalf("no response to id %s", id)
			}
		}
	}

	send(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"file:///a.txt","version":1,"text":"the dog ran\nfar away."}}}`)
	send(`{"jsonrpc":"2.0","id":1,"method":"textDocument/semanticTokens/full","params":{"textDocument":{"uri":"file:///a.txt"}}}`)
	var sem struct{ Data []uint32 }
	json.Unmarshal(await("1"), &sem)
	if len(sem.Data) != 5*5 { // five words; FilterToken drops the period
		t.Errorf("semantic tokens: %d ints, want 25: %v", len(sem.Data), sem.Data)
	}

	send(`{"jsonrpc":"2.0","id":2,"method":"textDocument/hover","params":{"textDocument":{"uri":"file:///a.txt"},"position":{"line":1,"character":1}}}`)
	var hover struct{ Contents markupContent }
	json.Unmarshal(await("2"), &hover)
	if hover.Contents.Kind != "markdown" || !strings.Contains(hover.Contents.Value, "far: ") {
		t.Errorf("hover: %+v", hover.Contents)
	}

	send(`{"jsonrpc":"2.0","id":3,"method":"no/such/method"}`)
	timeout := time.After(10 * time.Second)
	for {
		select {
		case msg := <-responses:
			if msg.ID != nil && string(*msg.ID) == "3" {
				if msg.Error == nil || msg.Error.Code != codeMethodNotFound {
					t.Errorf("unknown method: %+v", msg.Error)
				}
				inW.Close()
				return
			}
		case <-timeout:
			t.Fatal("no response to unknown method")
		}
	}
}

func TestTokenAtDetectsLineLanguage(t *testing.T) {
	text := "The quick brown fox jumps over the lazy dog.\nLe renard brun saute par-dessus le chien paresseux."
	doc := &document{text: text, tokens: []postag.Token{
		{Word: "quick", OffsetBegin: 4, OffsetEnd: 9},
		{Word: "renard", OffsetBegin: 48, OffsetEnd: 54},
	}}
	for line, want := range map[uint32]string{0: "en", 1: "fr"} {
		col := uint32(5)
		if line == 1 {
			col = 3
		}
		res := tokenAt(doc, line, col, langid.Detect)
		if res == nil || res.language != want {
			t.Errorf("line %d: language = %+v, want %q", line, res, want)
		}
	}
}
