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

// chunkResult holds per-chunk prediction results and the word spans used.
type chunkResult struct {
	wordTexts []string   // word texts for dirty detection
	words     []wordSpan // full spans for offset patching
	tokens    []POSToken
}

// documentStore tracks per-URI state.
type documentStore struct {
	queued         *textItem
	queuedWords    []wordSpan
	processing     bool
	doc            *document
	pendingReplies two[chan []uint32]
	pendingHover   *hoverRequest
	latestVersion  int32

	// incremental prediction state (valid while processing == true)
	processingText    string
	processingVersion int32
	chunkResults      []*chunkResult
	chunksTotal       int
	chunksDone        int
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
	msgPartialPredicted
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
	// for msgPartialPredicted
	chunkIdx int
	chunk    *chunkResult
}

// DocumentRegistry serialises all document state on a single goroutine.
type DocumentRegistry struct {
	ch           chan registryMsg
	model        Predictor
	tokenMap     TokenMap
	threshold    float64
	stores       map[string]*documentStore
	mu           sync.Mutex // protects nothing — registry is single-goroutine; mu for Send
	onDocReady   func(uri string, doc *document) // called after each chunk and on completion
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
			dr.handlePredicted(m.uri)
		case msgPartialPredicted:
			dr.handlePartialPredicted(m.uri, m.chunkIdx, m.chunk)
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
	words := basicTokenize(item.text)
	dr.scheduleProcessing(item, words, store)
}

func (dr *DocumentRegistry) handlePartialPredicted(uri string, chunkIdx int, cr *chunkResult) {
	store, ok := dr.stores[uri]
	if !ok || !store.processing {
		return
	}
	store.chunkResults[chunkIdx] = cr
	store.chunksDone++

	// Update doc so hover sees the latest partial data.
	partialDoc := &document{
		text:    store.processingText,
		tokens:  mergeChunkTokens(store.chunkResults),
		version: store.processingVersion,
	}
	store.doc = partialDoc

	if dr.onDocReady != nil {
		go dr.onDocReady(uri, partialDoc)
	}
}

func (dr *DocumentRegistry) handlePredicted(uri string) {
	store, ok := dr.stores[uri]
	if !ok || !store.processing {
		return
	}
	store.processing = false

	doc := &document{
		text:    store.processingText,
		tokens:  mergeChunkTokens(store.chunkResults),
		version: store.processingVersion,
	}
	store.doc = doc

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
	if ph := store.pendingHover; ph != nil {
		store.pendingHover = nil
		dr.handleHoverQuery(uri, ph.line, ph.char, ph.reply)
	}
	if dr.onDocReady != nil {
		go dr.onDocReady(uri, doc)
	}

	if queued := store.queued; queued != nil {
		queuedWords := store.queuedWords
		store.queued = nil
		store.queuedWords = nil
		dr.scheduleProcessing(queued, queuedWords, store)
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
			if store.pendingHover != nil {
				store.pendingHover.reply <- nil
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

func (dr *DocumentRegistry) scheduleProcessing(item *textItem, words []wordSpan, store *documentStore) {
	if store.processing {
		store.queued = item
		store.queuedWords = words
		return
	}
	store.processing = true
	store.queued = nil
	store.queuedWords = nil
	store.processingText = item.text
	store.processingVersion = item.version

	numChunks := (len(words) + chunkSize - 1) / chunkSize
	if len(words) == 0 {
		numChunks = 0
	}

	// Determine dirty chunks; pre-populate clean ones with offset-patched results.
	newChunkResults := make([]*chunkResult, numChunks)
	dirty := make([]bool, numChunks)

	if len(store.chunkResults) == numChunks {
		for i := range numChunks {
			start := i * chunkSize
			end := min(start+chunkSize, len(words))
			if old := store.chunkResults[i]; old != nil && chunksEqual(old.wordTexts, words[start:end]) {
				newChunkResults[i] = patchChunkOffsets(old, words[start:end])
			} else {
				dirty[i] = true
			}
		}
	} else {
		for i := range dirty {
			dirty[i] = true
		}
	}

	cleanCount := 0
	for _, d := range dirty {
		if !d {
			cleanCount++
		}
	}

	store.chunkResults = newChunkResults
	store.chunksTotal = numChunks
	store.chunksDone = cleanCount

	// Immediately push colors for clean chunks.
	if cleanCount > 0 && dr.onDocReady != nil {
		partialDoc := &document{
			text:    item.text,
			tokens:  mergeChunkTokens(newChunkResults),
			version: item.version,
		}
		store.doc = partialDoc
		go dr.onDocReady(item.uri, partialDoc)
	}

	// If everything is clean, finalize immediately without spawning a goroutine.
	if cleanCount == numChunks {
		// Reuse handlePredicted logic inline by sending the signal synchronously.
		store.processing = false
		doc := &document{
			text:    item.text,
			tokens:  mergeChunkTokens(newChunkResults),
			version: item.version,
		}
		store.doc = doc

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
		if ph := store.pendingHover; ph != nil {
			store.pendingHover = nil
			dr.handleHoverQuery(item.uri, ph.line, ph.char, ph.reply)
		}
		if dr.onDocReady != nil {
			go dr.onDocReady(item.uri, doc)
		}
		return
	}

	threshold := dr.threshold
	uri := item.uri
	go func() {
		for i := range numChunks {
			if !dirty[i] {
				continue
			}
			start := i * chunkSize
			end := min(start+chunkSize, len(words))
			chunkWords := words[start:end]
			tokens, err := dr.model.PredictChunk(chunkWords)
			if err != nil {
				tokens = nil
			}
			filtered := tokens[:0]
			for _, t := range tokens {
				if filterToken(t, threshold) {
					filtered = append(filtered, t)
				}
			}
			wordTexts := make([]string, len(chunkWords))
			for j, w := range chunkWords {
				wordTexts[j] = w.text
			}
			cr := &chunkResult{wordTexts: wordTexts, words: chunkWords, tokens: filtered}
			dr.send(registryMsg{kind: msgPartialPredicted, uri: uri, chunkIdx: i, chunk: cr})
		}
		dr.send(registryMsg{kind: msgPredicted, uri: uri})
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

// chunksEqual reports whether old word texts match the new word spans.
func chunksEqual(oldTexts []string, newWords []wordSpan) bool {
	if len(oldTexts) != len(newWords) {
		return false
	}
	for i, w := range newWords {
		if oldTexts[i] != w.text {
			return false
		}
	}
	return true
}

// patchChunkOffsets returns a new chunkResult with offsets updated from newWords.
// Word texts are assumed equal (same order), so position j in old maps to j in new.
func patchChunkOffsets(old *chunkResult, newWords []wordSpan) *chunkResult {
	// Map old begin offset → word index within chunk.
	oldBeginToIdx := make(map[uint32]int, len(old.words))
	for j, w := range old.words {
		oldBeginToIdx[w.begin] = j
	}

	patched := make([]POSToken, len(old.tokens))
	for i, t := range old.tokens {
		patched[i] = t
		if j, ok := oldBeginToIdx[t.OffsetBegin]; ok && j < len(newWords) {
			patched[i].OffsetBegin = newWords[j].begin
			patched[i].OffsetEnd = newWords[j].end
		}
	}

	wordTexts := make([]string, len(newWords))
	for j, w := range newWords {
		wordTexts[j] = w.text
	}
	return &chunkResult{wordTexts: wordTexts, words: newWords, tokens: patched}
}

// mergeChunkTokens concatenates chunk token slices in order.
// Chunks are contiguous document regions so no sort is needed.
func mergeChunkTokens(results []*chunkResult) []POSToken {
	var tokens []POSToken
	for _, cr := range results {
		if cr != nil {
			tokens = append(tokens, cr.tokens...)
		}
	}
	return tokens
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
