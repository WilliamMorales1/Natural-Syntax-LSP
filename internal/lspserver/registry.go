package lspserver

import (
	"strings"
	"sync"

	"natural-syntax-ls/internal/inference"
	"natural-syntax-ls/internal/postag"
	"natural-syntax-ls/internal/tokenizer"
	"natural-syntax-ls/internal/tokenmap"
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

// takeQueuedOrLatest returns the older queued reply if one is pending, otherwise takes and clears the single/latest reply.
func (tw *two[T]) takeQueuedOrLatest(queued bool) *T {
	if queued {
		return tw.takeOlder()
	}
	return tw.takeNewerAndClear()
}

// document holds a processed document.
type document struct {
	text    string
	tokens  []postag.POSToken
	version int32
}

type hoverRequest struct {
	line, char uint32
	reply      chan *hoverQueryResult
}

// hoverQueryResult is the hovered token plus (in dependency mode) the tokens whose head is this token.
type hoverQueryResult struct {
	tok        *postag.POSToken
	dependents []postag.POSToken
	// dependentsHaveDeps[i] is true when dependents[i] itself has dependents (tokens headed on it).
	dependentsHaveDeps []bool
}

// chunkResult holds per-chunk prediction results and the word spans used.
type chunkResult struct {
	wordTexts []string             // word texts for dirty detection
	words     []tokenizer.WordSpan // full spans for offset patching
	tokens    []postag.POSToken
}

// documentStore tracks per-URI state.
type documentStore struct {
	queued         *textItem
	queuedWords    []tokenizer.WordSpan
	processing     bool
	doc            *document
	pendingReplies two[chan []uint32]
	pendingHover   *hoverRequest
	latestVersion  int32

	// incremental prediction state (valid while processing == true)
	processingText    string
	processingVersion int32
	chunkResults      []*chunkResult
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
	mapUpdate map[postag.PartOfSpeech]*tokenmap.Override
	threshold float64
	// for semanticTokens call
	semReply chan []uint32
	// for hover
	hoverLine      uint32
	hoverCharacter uint32
	hoverReply     chan *hoverQueryResult
	// for msgPartialPredicted
	chunkIdx int
	chunk    *chunkResult
}

// documentRegistry serialises all document state on a single goroutine.
type documentRegistry struct {
	ch         chan registryMsg
	model      inference.Predictor
	tokenMap   tokenmap.Map
	useDeprel  bool // true when model is *inference.DependencyModel: color tokens by deprel category, not POS tag
	threshold  float64
	workers    int // parallel chunk predictions per document
	stores     map[string]*documentStore
	onDocReady func(uri string, doc *document) // called after each chunk and on completion
}

func newDocumentRegistry(model inference.Predictor) *documentRegistry {
	_, isDependency := model.(*inference.DependencyModel)
	_, workers := inference.Concurrency()
	dr := &documentRegistry{
		workers:   workers,
		ch:        make(chan registryMsg, 64),
		model:     model,
		tokenMap:  tokenmap.NewDefault(),
		useDeprel: isDependency,
		threshold: defaultScoreThreshold,
		stores:    make(map[string]*documentStore),
	}
	go dr.loop()
	return dr
}

func (dr *documentRegistry) send(m registryMsg) {
	dr.ch <- m
}

func (dr *documentRegistry) loop() {
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
			dr.tokenMap.Extend(m.mapUpdate)
		case msgScoreThreshold:
			dr.threshold = m.threshold
		case msgSemanticTokensCall:
			dr.handleSemanticTokensCall(m.uri, m.semReply)
		case msgHoverQuery:
			dr.handleHoverQuery(m.uri, m.hoverLine, m.hoverCharacter, m.hoverReply)
		}
	}
}

func (dr *documentRegistry) handleItem(item *textItem) {
	store := dr.storeFor(item.uri)
	if store.latestVersion >= item.version {
		return
	}
	store.latestVersion = item.version
	words := tokenizer.BasicTokenize(item.text)
	dr.scheduleProcessing(item, words, store)
}

func (dr *documentRegistry) handlePartialPredicted(uri string, chunkIdx int, cr *chunkResult) {
	store, ok := dr.stores[uri]
	if !ok || !store.processing {
		return
	}
	store.chunkResults[chunkIdx] = cr

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

func (dr *documentRegistry) handlePredicted(uri string) {
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

	reply := store.pendingReplies.takeQueuedOrLatest(store.queued != nil)
	if reply != nil {
		tokens := encodeSemanticTokens(doc, &dr.tokenMap, dr.useDeprel)
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

func (dr *documentRegistry) handleSemanticTokensCall(uri string, reply chan []uint32) {
	store := dr.storeFor(uri)
	if !store.processing && store.doc != nil {
		tokens := encodeSemanticTokens(store.doc, &dr.tokenMap, dr.useDeprel)
		reply <- tokens
		return
	}
	discarded := store.pendingReplies.push(&reply)
	if discarded != nil {
		close(*discarded) // signal caller that this reply was dropped
	}
}

func (dr *documentRegistry) handleHoverQuery(uri string, line, character uint32, reply chan *hoverQueryResult) {
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
			var dependents []postag.POSToken
			var dependentsHaveDeps []bool
			for _, d := range doc.tokens {
				if d.HasHead && d.HeadOffsetBegin == t.OffsetBegin {
					dependents = append(dependents, d)
					hasSubdeps := false
					for _, dd := range doc.tokens {
						if dd.HasHead && dd.HeadOffsetBegin == d.OffsetBegin {
							hasSubdeps = true
							break
						}
					}
					dependentsHaveDeps = append(dependentsHaveDeps, hasSubdeps)
				}
			}
			reply <- &hoverQueryResult{tok: &cp, dependents: dependents, dependentsHaveDeps: dependentsHaveDeps}
			return
		}
	}
	reply <- nil
}

func (dr *documentRegistry) scheduleProcessing(item *textItem, words []tokenizer.WordSpan, store *documentStore) {
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

	chunks := inference.SplitChunks(item.text, words)
	numChunks := len(chunks)

	// Reuse any previous chunk with identical words, wherever it moved; inference only sees a chunk's own words, so this matches a fresh run exactly.
	cache := make(map[string]*chunkResult, len(store.chunkResults))
	for _, old := range store.chunkResults {
		if old != nil {
			cache[strings.Join(old.wordTexts, "\x00")] = old
		}
	}
	newChunkResults := make([]*chunkResult, numChunks)
	dirty := make([]bool, numChunks)
	for i, chunk := range chunks {
		if old, ok := cache[chunkKey(chunk)]; ok {
			newChunkResults[i] = patchChunkOffsets(old, chunk)
		} else {
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
			tokens := encodeSemanticTokens(doc, &dr.tokenMap, dr.useDeprel)
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
		// Dirty chunks are independent, so spare physical cores run them in parallel against the one shared session.
		jobs := make(chan int)
		var wg sync.WaitGroup
		for range dr.workers {
			wg.Go(func() {
				for i := range jobs {
					chunkWords := chunks[i]
					tokens, err := dr.model.PredictChunk(chunkWords)
					if err != nil {
						tokens = nil
					}
					var filtered []postag.POSToken
					if _, isDependency := dr.model.(*inference.DependencyModel); isDependency {
						// Dependency mode's Score is arc-softmax confidence, not a classification-confidence gate; thresholding it would hide valid roots/arcs.
						filtered = tokens
					} else {
						filtered = tokens[:0]
						for _, t := range tokens {
							if postag.FilterToken(t, threshold) {
								filtered = append(filtered, t)
							}
						}
					}
					wordTexts := make([]string, len(chunkWords))
					for j, w := range chunkWords {
						wordTexts[j] = w.Text
					}
					cr := &chunkResult{wordTexts: wordTexts, words: chunkWords, tokens: filtered}
					dr.send(registryMsg{kind: msgPartialPredicted, uri: uri, chunkIdx: i, chunk: cr})
				}
			})
		}
		for i := range numChunks {
			if dirty[i] {
				jobs <- i
			}
		}
		close(jobs)
		wg.Wait()
		dr.send(registryMsg{kind: msgPredicted, uri: uri})
	}()
}

func (dr *documentRegistry) storeFor(uri string) *documentStore {
	s, ok := dr.stores[uri]
	if !ok {
		s = &documentStore{latestVersion: -1 << 31}
		dr.stores[uri] = s
	}
	return s
}

// chunkKey identifies a chunk by its word texts, matching strings.Join(chunkResult.wordTexts, "\x00").
func chunkKey(words []tokenizer.WordSpan) string {
	var sb strings.Builder
	for i, w := range words {
		if i > 0 {
			sb.WriteByte(0)
		}
		sb.WriteString(w.Text)
	}
	return sb.String()
}

// patchChunkOffsets returns a new chunkResult with offsets updated from newWords (assumed same order/texts as old).
func patchChunkOffsets(old *chunkResult, newWords []tokenizer.WordSpan) *chunkResult {
	// Map old begin offset → word index within chunk.
	oldBeginToIdx := make(map[uint32]int, len(old.words))
	for j, w := range old.words {
		oldBeginToIdx[w.Begin] = j
	}

	patched := make([]postag.POSToken, len(old.tokens))
	for i, t := range old.tokens {
		patched[i] = t
		if j, ok := oldBeginToIdx[t.OffsetBegin]; ok && j < len(newWords) {
			patched[i].OffsetBegin = newWords[j].Begin
			patched[i].OffsetEnd = newWords[j].End
		}
		if t.HasHead {
			if j, ok := oldBeginToIdx[t.HeadOffsetBegin]; ok && j < len(newWords) {
				patched[i].HeadOffsetBegin = newWords[j].Begin
			}
		}
	}

	wordTexts := make([]string, len(newWords))
	for j, w := range newWords {
		wordTexts[j] = w.Text
	}
	return &chunkResult{wordTexts: wordTexts, words: newWords, tokens: patched}
}

// mergeChunkTokens concatenates chunk token slices in order (chunks are contiguous, so no sort is needed).
func mergeChunkTokens(results []*chunkResult) []postag.POSToken {
	var tokens []postag.POSToken
	for _, cr := range results {
		if cr != nil {
			tokens = append(tokens, cr.tokens...)
		}
	}
	return tokens
}

// lineToCharOffset returns the unicode char index of the start of line (0-indexed), or -1 if out of range.
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
