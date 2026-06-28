package main

import (
	"sync"
)

const defaultScoreThreshold = 1.0 / 3.0

type two[T any] struct {
	items [2]*T
	n     int
}

func (tw *two[T]) push(v *T) *T {
	if tw.n < 2 {
		tw.items[tw.n] = v
		tw.n++
		return nil
	}
	discarded := tw.items[0]
	tw.items[0] = tw.items[1]
	tw.items[1] = v
	return discarded
}

func (tw *two[T]) takeOlder() *T {
	if tw.n < 2 {
		return nil
	}
	v := tw.items[0]
	tw.items[0] = tw.items[1]
	tw.items[1] = nil
	tw.n--
	return v
}

func (tw *two[T]) takeNewerAndClear() *T {
	if tw.n == 0 {
		return nil
	}
	v := tw.items[tw.n-1]
	tw.items[0] = nil
	tw.items[1] = nil
	tw.n = 0
	return v
}

// document holds a processed document.
type document struct {
	text    string
	tokens  []POSToken
	version int32
}

type hoverRequest struct {
	line, char uint32
	reply      chan *POSToken
}

// documentStore tracks per-URI state.
type documentStore struct {
	queued         *textItem
	processing     bool
	doc            *document
	pendingReplies two[chan []uint32]
	pendingHover   *hoverRequest
	latestVersion  int32
}

// textItem is a document text from the client.
type textItem struct {
	uri     string
	text    string
	version int32
}

// msgKind is the discriminant for registry messages.
type msgKind int

const (
	msgItem msgKind = iota
	msgPredicted
	msgDiscard
	msgTokenMapUpdate
	msgScoreThreshold
	msgSemanticTokensCall
	msgHoverQuery
)

type registryMsg struct {
	kind      msgKind
	item      *textItem
	uri       string
	doc       *document
	mapUpdate map[PartOfSpeech]*TokenTypeNModifiers
	threshold float64
	// for semanticTokens call
	semReply chan []uint32
	// for hover
	hoverLine      uint32
	hoverCharacter uint32
	hoverReply     chan *POSToken
}

// DocumentRegistry serialises all document state on a single goroutine.
type DocumentRegistry struct {
	ch           chan registryMsg
	model        Predictor
	tokenMap     TokenMap
	threshold    float64
	stores       map[string]*documentStore
	mu           sync.Mutex // protects nothing — registry is single-goroutine; mu for Send
	onDocReady   func(uri string, doc *document) // called after each document is processed
}

func newDocumentRegistry(model Predictor) *DocumentRegistry {
	dr := &DocumentRegistry{
		ch:        make(chan registryMsg, 64),
		model:     model,
		tokenMap:  defaultTokenMap(),
		threshold: defaultScoreThreshold,
		stores:    make(map[string]*documentStore),
	}
	go dr.loop()
	return dr
}

func (dr *DocumentRegistry) send(m registryMsg) {
	dr.ch <- m
}

func (dr *DocumentRegistry) loop() {
	for m := range dr.ch {
		switch m.kind {
		case msgItem:
			dr.handleItem(m.item)
		case msgPredicted:
			dr.handlePredicted(m.uri, m.doc)
		case msgDiscard:
			delete(dr.stores, m.uri)
		case msgTokenMapUpdate:
			dr.tokenMap.extend(m.mapUpdate)
		case msgScoreThreshold:
			dr.threshold = m.threshold
		case msgSemanticTokensCall:
			dr.handleSemanticTokensCall(m.uri, m.semReply)
		case msgHoverQuery:
			dr.handleHoverQuery(m.uri, m.hoverLine, m.hoverCharacter, m.hoverReply)
		}
	}
}

func (dr *DocumentRegistry) handleItem(item *textItem) {
	store := dr.storeFor(item.uri)
	if store.latestVersion >= item.version {
		return
	}
	store.latestVersion = item.version
	dr.scheduleProcessing(item, store)
}

func (dr *DocumentRegistry) handlePredicted(uri string, doc *document) {
	store, ok := dr.stores[uri]
	if !ok {
		return
	}
	store.processing = false

	var reply *chan []uint32
	if store.queued != nil {
		reply = store.pendingReplies.takeOlder()
	} else {
		reply = store.pendingReplies.takeNewerAndClear()
	}
	if reply != nil {
		tokens := encodeSemanticTokens(doc, &dr.tokenMap)
		*reply <- tokens
	}
	store.doc = doc
	if ph := store.pendingHover; ph != nil {
		store.pendingHover = nil
		dr.handleHoverQuery(uri, ph.line, ph.char, ph.reply)
	}
	if dr.onDocReady != nil {
		go dr.onDocReady(uri, doc)
	}

	if queued := store.queued; queued != nil {
		store.queued = nil
		dr.scheduleProcessing(queued, store)
	}
}

func (dr *DocumentRegistry) handleSemanticTokensCall(uri string, reply chan []uint32) {
	store := dr.storeFor(uri)
	if !store.processing && store.doc != nil {
		tokens := encodeSemanticTokens(store.doc, &dr.tokenMap)
		reply <- tokens
		return
	}
	discarded := store.pendingReplies.push(&reply)
	if discarded != nil {
		close(*discarded) // signal caller that this reply was dropped
	}
}

func (dr *DocumentRegistry) handleHoverQuery(uri string, line, character uint32, reply chan *POSToken) {
	store, ok := dr.stores[uri]
	if !ok || store.doc == nil {
		if ok && store.processing {
			// Doc not ready yet — stash hover; handlePredicted will resolve it.
			if store.pendingHover != nil {
				store.pendingHover.reply <- nil // cancel previous waiting hover
			}
			store.pendingHover = &hoverRequest{line: line, char: character, reply: reply}
			return
		}
		reply <- nil
		return
	}
	doc := store.doc
	lineStart := lineToCharOffset(doc.text, int(line))
	if lineStart < 0 {
		reply <- nil
		return
	}
	charOffset := uint32(lineStart) + character
	for i := range doc.tokens {
		t := &doc.tokens[i]
		if t.OffsetBegin <= charOffset && charOffset < t.OffsetEnd {
			cp := *t
			reply <- &cp
			return
		}
	}
	reply <- nil
}

func (dr *DocumentRegistry) scheduleProcessing(item *textItem, store *documentStore) {
	if store.processing {
		store.queued = item
		return
	}
	store.processing = true
	store.queued = nil
	threshold := dr.threshold
	go func() {
		tokens, err := dr.model.Predict(item.text)
		if err != nil {
			tokens = nil
		}
		// Filter tokens.
		filtered := tokens[:0]
		for _, t := range tokens {
			if filterToken(t, threshold) {
				filtered = append(filtered, t)
			}
		}
		doc := &document{
			text:    item.text,
			tokens:  filtered,
			version: item.version,
		}
		dr.send(registryMsg{kind: msgPredicted, uri: item.uri, doc: doc})
	}()
}

func (dr *DocumentRegistry) storeFor(uri string) *documentStore {
	s, ok := dr.stores[uri]
	if !ok {
		s = &documentStore{latestVersion: -1 << 31}
		dr.stores[uri] = s
	}
	return s
}

// lineToCharOffset returns the unicode char index of the start of the given line
// (0-indexed). Returns -1 if line is out of range.
func lineToCharOffset(text string, line int) int {
	cur := 0
	runes := []rune(text)
	n := len(runes)
	for range line {
		for cur < n && runes[cur] != '\n' {
			cur++
		}
		if cur >= n {
			return -1
		}
		cur++ // skip '\n'
	}
	return cur
}
