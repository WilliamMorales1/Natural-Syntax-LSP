package lspserver

import (
	"math"
	"slices"
	"strings"
	"sync"

	"natural-syntax-ls/internal/inference"
	"natural-syntax-ls/internal/postag"
	"natural-syntax-ls/internal/tokenizer"
	"natural-syntax-ls/internal/tokenmap"
)

const defaultScoreThreshold = 1.0 / 3.0

// replyQueue holds at most two pending semantic-token replies: one for the in-flight run and one for a queued run.
type replyQueue struct {
	items [2]chan []uint32
	n     int
}

// push adds c, evicting and returning the oldest reply when full.
func (q *replyQueue) push(c chan []uint32) chan []uint32 {
	if q.n < 2 {
		q.items[q.n] = c
		q.n++
		return nil
	}
	evicted := q.items[0]
	q.items = [2]chan []uint32{q.items[1], c}
	return evicted
}

// take returns the older reply when a run is queued (the newer one waits for it), otherwise the newest reply, clearing the queue.
func (q *replyQueue) take(queued bool) chan []uint32 {
	if queued {
		if q.n < 2 {
			return nil
		}
		c := q.items[0]
		q.items = [2]chan []uint32{q.items[1], nil}
		q.n--
		return c
	}
	if q.n == 0 {
		return nil
	}
	c := q.items[q.n-1]
	*q = replyQueue{}
	return c
}

// document holds a processed document.
type document struct {
	text    string
	tokens  []postag.Token
	version int32
}

type hoverRequest struct {
	line, char uint32
	reply      chan *hoverQueryResult
}

// hoverQueryResult is the hovered token plus (in dependency mode) the tokens whose head is this token.
type hoverQueryResult struct {
	tok        *postag.Token
	dependents []postag.Token
	// dependentsHaveDeps[i] is true when dependents[i] itself has dependents (tokens headed on it).
	dependentsHaveDeps []bool
}

// chunkResult holds per-chunk prediction results and the word spans used.
type chunkResult struct {
	key    string               // chunkKey of words, for reuse detection
	words  []tokenizer.WordSpan // full spans for offset patching
	tokens []postag.Token
	embeds [][]float32 // unit embedding per token; semantic mode only
}

// documentStore tracks per-URI state.
type documentStore struct {
	queued         *textItem
	queuedWords    []tokenizer.WordSpan
	processing     bool
	doc            *document
	pendingReplies replyQueue
	pendingHover   *hoverRequest
	latestVersion  int32

	// incremental prediction state (valid while processing == true)
	processingText    string
	processingVersion int32
	chunkResults      []*chunkResult

	plane inference.SemanticPlane // semantic mode: color plane carried across versions so hues stay stable
}

// textItem is a document text from the client.
type textItem struct {
	uri     string
	text    string
	version int32
}

// documentRegistry serialises all document state on a single goroutine; its methods queue work onto it.
type documentRegistry struct {
	ch         chan func()
	model      inference.Predictor
	embedder   inference.Embedder // non-nil in semantic mode: tokens are recolored per document
	tokenMap   tokenmap.Map
	useDeprel  bool // true when model is *inference.DependencyModel: color tokens by deprel category, not POS tag
	threshold  float64
	workers    int // parallel chunk predictions per document
	stores     map[string]*documentStore
	onDocReady func(uri string, doc *document) // called after each chunk and on completion
}

func newDocumentRegistry(model inference.Predictor) *documentRegistry {
	_, isDependency := model.(*inference.DependencyModel)
	embedder, _ := model.(inference.Embedder)
	_, workers := inference.Concurrency()
	dr := &documentRegistry{
		workers:   workers,
		ch:        make(chan func(), 64),
		model:     model,
		embedder:  embedder,
		tokenMap:  tokenmap.NewDefault(),
		useDeprel: isDependency,
		threshold: defaultScoreThreshold,
		stores:    make(map[string]*documentStore),
	}
	go dr.loop()
	return dr
}

func (dr *documentRegistry) loop() {
	for f := range dr.ch {
		f()
	}
}

// do runs f on the registry goroutine.
func (dr *documentRegistry) do(f func()) {
	dr.ch <- f
}

// update queues a new document version for prediction.
func (dr *documentRegistry) update(item *textItem) {
	dr.do(func() { dr.handleItem(item) })
}

// discard drops all state for uri.
func (dr *documentRegistry) discard(uri string) {
	dr.do(func() { delete(dr.stores, uri) })
}

// extendTokenMap applies per-POS token-map overrides.
func (dr *documentRegistry) extendTokenMap(update map[postag.PartOfSpeech]*tokenmap.Override) {
	dr.do(func() { dr.tokenMap.Extend(update) })
}

// setThreshold sets the score below which POS tokens are dropped from future predictions.
func (dr *documentRegistry) setThreshold(t float64) {
	dr.do(func() { dr.threshold = t })
}

// semanticTokens sends uri's encoded tokens on reply once its latest version is predicted; reply is closed if a newer request supersedes it.
func (dr *documentRegistry) semanticTokens(uri string, reply chan []uint32) {
	dr.do(func() { dr.handleSemanticTokensCall(uri, reply) })
}

// hover sends the token at line/character in uri on reply, or nil if there is none.
func (dr *documentRegistry) hover(uri string, line, character uint32, reply chan *hoverQueryResult) {
	dr.do(func() { dr.handleHoverQuery(uri, line, character, reply) })
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
		tokens:  dr.docTokens(store, store.chunkResults, false),
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
	dr.finalize(uri, store)
}

// finalize publishes the completed doc, answers pending requests, and starts any queued run.
func (dr *documentRegistry) finalize(uri string, store *documentStore) {
	store.processing = false
	doc := &document{
		text:    store.processingText,
		tokens:  dr.docTokens(store, store.chunkResults, true),
		version: store.processingVersion,
	}
	store.doc = doc

	if reply := store.pendingReplies.take(store.queued != nil); reply != nil {
		reply <- encodeSemanticTokens(doc, &dr.tokenMap, dr.useDeprel)
	}
	if ph := store.pendingHover; ph != nil {
		store.pendingHover = nil
		dr.handleHoverQuery(uri, ph.line, ph.char, ph.reply)
	}
	if dr.onDocReady != nil {
		go dr.onDocReady(uri, doc)
	}

	if queued := store.queued; queued != nil {
		words := store.queuedWords
		store.queued, store.queuedWords = nil, nil
		dr.scheduleProcessing(queued, words, store)
	}
}

func (dr *documentRegistry) handleSemanticTokensCall(uri string, reply chan []uint32) {
	store := dr.storeFor(uri)
	if !store.processing && store.doc != nil {
		reply <- encodeSemanticTokens(store.doc, &dr.tokenMap, dr.useDeprel)
		return
	}
	if evicted := store.pendingReplies.push(reply); evicted != nil {
		close(evicted) // signal caller that this reply was dropped
	}
}

func (dr *documentRegistry) handleHoverQuery(uri string, line, character uint32, reply chan *hoverQueryResult) {
	store, ok := dr.stores[uri]
	if ok && store.doc == nil && store.processing {
		if store.pendingHover != nil {
			store.pendingHover.reply <- nil
		}
		store.pendingHover = &hoverRequest{line: line, char: character, reply: reply}
		return
	}
	if !ok || store.doc == nil {
		reply <- nil
		return
	}
	reply <- tokenAt(store.doc, line, character)
}

// tokenAt returns the token at the position and its direct dependents, or nil if none.
func tokenAt(doc *document, line, character uint32) *hoverQueryResult {
	lineStart := lineToCharOffset(doc.text, int(line))
	if lineStart < 0 {
		return nil
	}
	offset := uint32(lineStart) + character
	i := slices.IndexFunc(doc.tokens, func(t postag.Token) bool {
		return t.OffsetBegin <= offset && offset < t.OffsetEnd
	})
	if i < 0 {
		return nil
	}
	tok := doc.tokens[i]
	res := &hoverQueryResult{tok: &tok}
	for _, d := range doc.tokens {
		if d.HasHead && d.HeadOffsetBegin == tok.OffsetBegin {
			res.dependents = append(res.dependents, d)
			res.dependentsHaveDeps = append(res.dependentsHaveDeps, hasDependents(doc.tokens, d.OffsetBegin))
		}
	}
	return res
}

func hasDependents(tokens []postag.Token, headBegin uint32) bool {
	return slices.ContainsFunc(tokens, func(t postag.Token) bool {
		return t.HasHead && t.HeadOffsetBegin == headBegin
	})
}

func (dr *documentRegistry) scheduleProcessing(item *textItem, words []tokenizer.WordSpan, store *documentStore) {
	if store.processing {
		store.queued = item
		store.queuedWords = words
		return
	}
	store.processing = true
	store.queued, store.queuedWords = nil, nil
	store.processingText = item.text
	store.processingVersion = item.version

	chunks := inference.SplitChunks(item.text, words)

	// Reuse any previous chunk with identical words, wherever it moved; inference only sees a chunk's own words, so this matches a fresh run exactly.
	cache := make(map[string]*chunkResult, len(store.chunkResults))
	for _, old := range store.chunkResults {
		if old != nil {
			cache[old.key] = old
		}
	}
	results := make([]*chunkResult, len(chunks))
	var dirty []int
	for i, chunk := range chunks {
		if old, ok := cache[chunkKey(chunk)]; ok {
			results[i] = patchChunkOffsets(old, chunk)
		} else {
			dirty = append(dirty, i)
		}
	}
	store.chunkResults = results

	if len(dirty) == 0 {
		dr.finalize(item.uri, store)
		return
	}

	// Immediately push colors for clean chunks.
	if len(dirty) < len(chunks) && dr.onDocReady != nil {
		partialDoc := &document{
			text:    item.text,
			tokens:  dr.docTokens(store, results, false),
			version: item.version,
		}
		store.doc = partialDoc
		go dr.onDocReady(item.uri, partialDoc)
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
					cr := dr.predictChunk(chunks[i], threshold)
					dr.do(func() { dr.handlePartialPredicted(uri, i, cr) })
				}
			})
		}
		for _, i := range dirty {
			jobs <- i
		}
		close(jobs)
		wg.Wait()
		dr.do(func() { dr.handlePredicted(uri) })
	}()
}

// predictChunk runs the model on one chunk; errors yield an empty result so the chunk still completes.
func (dr *documentRegistry) predictChunk(words []tokenizer.WordSpan, threshold float64) *chunkResult {
	var tokens []postag.Token
	var embeds [][]float32
	var err error
	if dr.embedder != nil {
		tokens, embeds, err = dr.embedder.EmbedChunk(words)
	} else {
		tokens, err = dr.model.PredictChunk(words)
	}
	if err != nil {
		tokens, embeds = nil, nil
	}
	// Dependency mode's Score is arc-softmax confidence, not a classification-confidence gate; thresholding it would hide valid roots/arcs.
	if !dr.useDeprel {
		kept := tokens[:0]
		var keptEmbeds [][]float32
		for i, t := range tokens {
			if !t.Keep(threshold) {
				continue
			}
			kept = append(kept, t)
			if embeds != nil {
				keptEmbeds = append(keptEmbeds, embeds[i])
			}
		}
		tokens, embeds = kept, keptEmbeds
	}
	return &chunkResult{key: chunkKey(words), words: words, tokens: tokens, embeds: embeds}
}

// docTokens merges chunk tokens and, in semantic mode, recolors them from store's plane, refitting it first when refit is set or it was never fit.
func (dr *documentRegistry) docTokens(store *documentStore, results []*chunkResult, refit bool) []postag.Token {
	tokens := mergeChunkTokens(results)
	if dr.embedder == nil {
		return tokens
	}
	var embeds [][]float32
	for _, cr := range results {
		if cr != nil {
			embeds = append(embeds, cr.embeds...)
		}
	}
	if refit || !store.plane.Fitted() {
		store.plane.Fit(embeds)
	}
	// mergeChunkTokens copied the tokens, so cached chunk results keep their own colors.
	for i, e := range embeds {
		c := store.plane.Color(e)
		tokens[i].Color = c
		tokens[i].Description = "Semantic color " + c
	}
	return tokens
}

func (dr *documentRegistry) storeFor(uri string) *documentStore {
	s, ok := dr.stores[uri]
	if !ok {
		s = &documentStore{latestVersion: math.MinInt32}
		dr.stores[uri] = s
	}
	return s
}

// chunkKey identifies a chunk by its word texts.
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

	patched := slices.Clone(old.tokens)
	for i, t := range old.tokens {
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

	return &chunkResult{key: old.key, words: newWords, tokens: patched, embeds: old.embeds}
}

// mergeChunkTokens concatenates chunk token slices in order (chunks are contiguous, so no sort is needed).
func mergeChunkTokens(results []*chunkResult) []postag.Token {
	var tokens []postag.Token
	for _, cr := range results {
		if cr != nil {
			tokens = append(tokens, cr.tokens...)
		}
	}
	return tokens
}

// lineToCharOffset returns the unicode char index of the start of line (0-indexed), or -1 if out of range.
func lineToCharOffset(text string, line int) int {
	if line == 0 {
		return 0
	}
	n := 0
	for _, r := range text {
		n++
		if r == '\n' {
			if line--; line == 0 {
				return n
			}
		}
	}
	return -1
}
