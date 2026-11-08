package lspserver

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"math"
	"slices"
	"sync"
	"sync/atomic"

	"natural-syntax-ls/internal/inference"
	"natural-syntax-ls/internal/langid"
	"natural-syntax-ls/internal/postag"
	"natural-syntax-ls/internal/tokenizer"
	"natural-syntax-ls/internal/tokenmap"
)

const defaultScoreThreshold = 1.0 / 3.0

var (
	// errSuperseded reports that the document was closed before the awaited run finished.
	errSuperseded = errors.New("lspserver: document closed before run finished")
	// errClosed reports that the registry was closed.
	errClosed = errors.New("lspserver: registry closed")
)

// run is one prediction pass; done closes when it finishes, with doc set unless the document was discarded first.
type run struct {
	done chan struct{}
	doc  *document
}

// document holds a processed document.
type document struct {
	text     string
	tokens   []postag.Token
	version  int32
	lines    atomic.Pointer[lineIndex] // built on first use
	langOnce sync.Once
	lang     string // language of the whole text, detected on first use
}

// language returns the detected language of the whole text, or "" when unsure.
func (d *document) language(detect func(string) string) string {
	d.langOnce.Do(func() { d.lang = detect(d.text[:min(len(d.text), docSample)]) })
	return d.lang
}

// docSample caps how much of a text is read to guess its language.
const docSample = 4000

// lineN returns the text of line n.
func (d *document) lineN(n int) string {
	ix := d.lineIndex()
	end := len(ix.runes)
	if n+1 < len(ix.starts) {
		end = ix.starts[n+1]
	}
	return string(ix.runes[ix.starts[n]:end])
}

// lineLanguage names the language of the line holding offset. A line too short to tell borrows the nearest earlier line that can, within lookback lines, so a one-word line in a French paragraph stays French.
func (d *document) lineLanguage(offset int, detect func(string) string) string {
	n := d.lineIndex().lineOf(offset)
	for i := n; i >= 0 && i >= n-lookback; i-- {
		if l := detect(d.lineN(i)); l != "" {
			return l
		}
	}
	return ""
}

// lookback bounds how many earlier lines a short line may borrow its language from.
const lookback = 5

// lineIndex returns the document's line index, building it once per document.
func (d *document) lineIndex() *lineIndex {
	if ix := d.lines.Load(); ix != nil {
		return ix
	}
	d.lines.CompareAndSwap(nil, newLineIndex([]rune(d.text)))
	return d.lines.Load()
}

// dependent is a token headed on the hovered token.
type dependent struct {
	tok     postag.Token
	hasDeps bool // some token is headed on this one
}

// hoverQueryResult is the hovered token plus (in dependency mode) the tokens whose head is this token.
type hoverQueryResult struct {
	tok        *postag.Token
	dependents []dependent
	language   string // detected language of the token's line, else of the whole document; "" when unsure
}

// chunkResult holds per-chunk prediction results and the word spans used.
type chunkResult struct {
	key    chunkDigest          // chunkKey of words, for reuse detection
	words  []tokenizer.WordSpan // full spans for offset patching
	tokens []postag.Token
	embeds [][]float32 // unit embedding per token; semantic mode only
}

// documentStore tracks per-URI state.
type documentStore struct {
	queued        *textItem
	queuedWords   []tokenizer.WordSpan
	queuedModel   predictor
	model         predictor // model of the pass in flight or last finished; nil before the first
	lastItem      *textItem // latest accepted text, for re-running under another model
	processing    bool
	doc           *document
	run           *run // in-flight pass while processing
	latestVersion int32

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

// documentRegistry guards all document state with one mutex; requests that need an in-flight pass wait on its run.
type documentRegistry struct {
	mu         sync.Mutex
	done       chan struct{} // closed by Close; releases waiters
	closeOnce  sync.Once
	model      predictor
	embedder   embedder // non-nil in semantic mode: tokens are recolored per document
	tokenMap   tokenmap.Map
	useDeprel  bool // true when model reports UsesDeprel: color tokens by deprel category, not POS tag
	threshold  float64
	workers    int // parallel chunk predictions per document
	stores     map[string]*documentStore
	onDocReady func(uri string, doc *document)            // called after each chunk and on completion
	detect     func(text string) string                   // names a text's language for hover
	pick       func(text string, cur predictor) predictor // chooses the model for a text; nil keeps model
}

// newDocumentRegistry starts a registry around model; the caller keeps ownership of model.
func newDocumentRegistry(model predictor) *documentRegistry {
	dm, _ := model.(deprelModel)
	isDependency := dm != nil && dm.UsesDeprel()
	emb, _ := model.(embedder)
	_, workers := inference.Concurrency()
	dr := &documentRegistry{
		workers:   workers,
		done:      make(chan struct{}),
		model:     model,
		embedder:  emb,
		tokenMap:  tokenmap.NewDefault(),
		useDeprel: isDependency,
		threshold: defaultScoreThreshold,
		detect:    langid.Detect,
		stores:    make(map[string]*documentStore),
	}
	return dr
}

// Close releases waiters; later work is dropped.
func (dr *documentRegistry) Close() {
	dr.closeOnce.Do(func() { close(dr.done) })
}

// do runs f under the registry lock and reports whether it ran; it does not once the registry is closed.
func (dr *documentRegistry) do(f func()) bool {
	select {
	case <-dr.done:
		return false
	default:
	}
	dr.mu.Lock()
	defer dr.mu.Unlock()
	f()
	return true
}

// update queues a new document version for prediction.
func (dr *documentRegistry) update(item *textItem) {
	m := dr.modelFor(item)
	words := tokenizeWith(m, item.text)
	dr.do(func() { dr.handleItem(item, words, m) })
}

// tokenizeWith splits text the way m dictates, else by the basic rules.
func tokenizeWith(m predictor, text string) []tokenizer.WordSpan {
	if tt, ok := m.(textTokenizer); ok {
		return tt.Tokenize(text)
	}
	return tokenizer.BasicTokenize(text)
}

// modelFor picks the model that should analyse item's text.
func (dr *documentRegistry) modelFor(item *textItem) predictor {
	m := dr.model
	dr.do(func() {
		if st := dr.stores[item.uri]; st != nil && st.model != nil {
			m = st.model
		}
		if dr.pick != nil {
			m = dr.pick(item.text, m)
		}
	})
	return m
}

// refresh re-runs every document whose preferred model changed, e.g. after a language's model finished loading.
func (dr *documentRegistry) refresh() {
	dr.do(func() {
		for _, st := range dr.stores {
			if st.lastItem == nil || st.model == nil || dr.pick == nil {
				continue
			}
			m := dr.pick(st.lastItem.text, st.model)
			if m == st.model && (st.queuedModel == nil || st.queuedModel == m) {
				continue
			}
			dr.scheduleProcessing(st.lastItem, tokenizeWith(m, st.lastItem.text), m, st)
		}
	})
}

// discard drops all state for uri and releases requests waiting on it.
func (dr *documentRegistry) discard(uri string) {
	dr.do(func() {
		if store := dr.stores[uri]; store != nil && store.run != nil {
			close(store.run.done)
			store.run = nil
		}
		delete(dr.stores, uri)
	})
}

// extendTokenMap applies per-POS token-map overrides.
func (dr *documentRegistry) extendTokenMap(update map[postag.PartOfSpeech]*tokenmap.Override) {
	dr.do(func() { dr.tokenMap.Extend(update) })
}

// setThreshold sets the score below which POS tokens are dropped from future predictions.
func (dr *documentRegistry) setThreshold(t float64) {
	dr.do(func() { dr.threshold = t })
}

// wait blocks until r finishes and returns its document.
func (dr *documentRegistry) wait(ctx context.Context, r *run) (*document, error) {
	select {
	case <-r.done:
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	case <-dr.done:
		return nil, errClosed
	}
	if r.doc == nil {
		return nil, errSuperseded
	}
	return r.doc, nil
}

// semanticTokens returns uri's encoded tokens once its latest version is predicted; an unknown uri yields nil.
func (dr *documentRegistry) semanticTokens(ctx context.Context, uri string) ([]uint32, error) {
	var r *run
	var data []uint32
	if !dr.do(func() {
		store := dr.stores[uri]
		switch {
		case store == nil:
		case !store.processing && store.doc != nil:
			data = encodeSemanticTokens(store.doc, &dr.tokenMap, dr.useDeprel)
		default:
			r = store.run
		}
	}) {
		return nil, errClosed
	}
	if r == nil {
		return data, nil
	}
	doc, err := dr.wait(ctx, r)
	if err != nil {
		return nil, err
	}
	dr.do(func() { data = encodeSemanticTokens(doc, &dr.tokenMap, dr.useDeprel) })
	return data, nil
}

// hover returns the token at line/character in uri, or nil if there is none.
func (dr *documentRegistry) hover(ctx context.Context, uri string, line, character uint32) (*hoverQueryResult, error) {
	var r *run
	var doc *document
	if !dr.do(func() {
		if store := dr.stores[uri]; store != nil {
			doc = store.doc
			if store.processing {
				r = store.run
			}
		}
	}) {
		return nil, errClosed
	}
	if doc != nil {
		if res := tokenAt(doc, line, character, dr.detect); res != nil {
			return res, nil
		}
	}
	// A partial doc lacks the chunks still being analysed, so a miss waits for the run to finish.
	if r != nil {
		var err error
		if doc, err = dr.wait(ctx, r); err != nil {
			return nil, err
		}
	}
	if doc == nil {
		return nil, nil
	}
	return tokenAt(doc, line, character, dr.detect), nil
}

func (dr *documentRegistry) handleItem(item *textItem, words []tokenizer.WordSpan, m predictor) {
	store := dr.storeFor(item.uri)
	if store.latestVersion >= item.version {
		return
	}
	store.latestVersion = item.version
	store.lastItem = item
	dr.scheduleProcessing(item, words, m, store)
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
	if r := store.run; r != nil {
		r.doc = doc
		close(r.done)
		store.run = nil
	}
	if dr.onDocReady != nil {
		go dr.onDocReady(uri, doc)
	}

	if queued := store.queued; queued != nil {
		words, m := store.queuedWords, store.queuedModel
		store.queued, store.queuedWords, store.queuedModel = nil, nil, nil
		dr.scheduleProcessing(queued, words, m, store)
	}
}

// tokenAt returns the token at the position and its direct dependents, or nil if none.
func tokenAt(doc *document, line, character uint32, detect func(string) string) *hoverQueryResult {
	off, ok := doc.lineIndex().offset(line, character)
	if !ok {
		return nil
	}
	offset := uint32(off)
	i := slices.IndexFunc(doc.tokens, func(t postag.Token) bool {
		return t.OffsetBegin <= offset && offset < t.OffsetEnd
	})
	if i < 0 {
		return nil
	}
	tok := doc.tokens[i]
	res := &hoverQueryResult{tok: &tok, language: doc.lineLanguage(off, detect)}
	if res.language == "" {
		res.language = doc.language(detect)
	}
	for _, d := range doc.tokens {
		if d.HasHead && d.HeadOffsetBegin == tok.OffsetBegin {
			res.dependents = append(res.dependents, dependent{d, hasDependents(doc.tokens, d.OffsetBegin)})
		}
	}
	return res
}

func hasDependents(tokens []postag.Token, headBegin uint32) bool {
	return slices.ContainsFunc(tokens, func(t postag.Token) bool {
		return t.HasHead && t.HeadOffsetBegin == headBegin
	})
}

func (dr *documentRegistry) scheduleProcessing(item *textItem, words []tokenizer.WordSpan, m predictor, store *documentStore) {
	if store.processing {
		store.queued = item
		store.queuedWords = words
		store.queuedModel = m
		return
	}
	// Chunks and the color plane belong to one model, so a switch starts clean.
	if store.model != m {
		store.chunkResults = nil
		store.plane = inference.SemanticPlane{}
		store.model = m
	}
	store.processing = true
	store.run = &run{done: make(chan struct{})}
	store.queued, store.queuedWords, store.queuedModel = nil, nil, nil
	store.processingText = item.text
	store.processingVersion = item.version

	chunks := inference.SplitChunks(item.text, words)

	// Reuse any previous chunk with identical words, wherever it moved; inference only sees a chunk's own words, so this matches a fresh run exactly.
	cache := make(map[chunkDigest]*chunkResult, len(store.chunkResults))
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
					cr := dr.predictChunk(m, chunks[i], threshold)
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
func (dr *documentRegistry) predictChunk(m predictor, words []tokenizer.WordSpan, threshold float64) *chunkResult {
	var tokens []postag.Token
	var embeds [][]float32
	var err error
	if emb, ok := m.(embedder); ok && dr.embedder != nil {
		tokens, embeds, err = emb.EmbedChunk(words)
	} else {
		tokens, err = m.PredictChunk(words)
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

// chunkDigest is a 128-bit hash of a chunk's words; collisions are negligible next to hardware error.
type chunkDigest [16]byte

// chunkKey identifies a chunk by its word texts and block starts.
func chunkKey(words []tokenizer.WordSpan) chunkDigest {
	h := sha256.New()
	for _, w := range words {
		// Block starts change sentence splits, so they're part of the key.
		if w.BlockStart {
			h.Write([]byte{1})
		}
		_, _ = io.WriteString(h, w.Text)
		h.Write([]byte{0})
	}
	var d chunkDigest
	copy(d[:], h.Sum(nil))
	return d
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
