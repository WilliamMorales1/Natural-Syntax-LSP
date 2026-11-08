// Package lspserver implements the LSP JSON-RPC-over-stdio server: protocol handling, per-document state, and semantic-token/hover encoding.
package lspserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"natural-syntax-ls/internal/inference"
	"natural-syntax-ls/internal/lang"
	"natural-syntax-ls/internal/postag"
	"natural-syntax-ls/internal/tokenmap"
	"natural-syntax-ls/internal/wiktionary"
)

type jsonrpcMsg struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  any              `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

const (
	codeMethodNotFound = -32601
	codeInvalidRequest = -32600
	codeInvalidParams  = -32602
)

// replyTimeout bounds how long a request waits on the registry.
const replyTimeout = 30 * time.Second

// errExitWithoutShutdown is returned when the client sends exit before shutdown; the LSP spec requires exit code 1.
var errExitWithoutShutdown = errors.New("exit received without prior shutdown")

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message) }

// Mode selects what the server highlights by.
type Mode string

// Highlighting modes.
const (
	ModePOS        Mode = "pos"
	ModeSemantic   Mode = "semantic"
	ModeDependency Mode = "dependency"
)

// ParseMode validates a -mode flag value.
func ParseMode(s string) (Mode, error) {
	switch m := Mode(s); m {
	case ModePOS, ModeSemantic, ModeDependency:
		return m, nil
	}
	return "", fmt.Errorf("unknown mode %q (want pos, semantic, or dependency)", s)
}

// defaultWiktionaryLang looks words up in the language detected from their line, then English.
const defaultWiktionaryLang = "detect,en"

// Config configures the LSP server's model/mode.
type Config struct {
	ModelPath string // path to the mode's .onnx model
	VocabPath string // path to that same model's own _vocab.txt
	Mode      Mode
	Language  string          // Wiktionary code of the model's language ("" when English or multilingual)
	Forced    string          // language code that replaces detection for tokenizing and hover; "" detects
	Languages []lang.Language // language hooks for the tokenizer and Wiktionary
	// Switch names the installed model and vocab for a detected language; nil keeps one model for every text.
	Switch func(code string) (modelPath, vocabPath string)
	// ScriptDirs and LoadLanguages enable live reload: when a *.lua file in ScriptDirs changes, LoadLanguages is called again.
	ScriptDirs    []string
	LoadLanguages func() []lang.Language
}

// Run starts the LSP server over stdin/stdout and blocks until the client disconnects or sends exit.
// It returns nil after shutdown+exit or EOF, and errExitWithoutShutdown when exit arrives without shutdown.
// Cancelling ctx aborts in-flight hover lookups.
func Run(ctx context.Context, cfg Config) error {
	level := slog.LevelWarn
	if os.Getenv("NLS_LOG") == "debug" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	srv := &lspServer{ctx: ctx, cfg: cfg, ready: make(chan struct{})}
	srv.langs.Store(&cfg.Languages)
	srv.wikt.Store(wiktionary.NewClient(lang.WiktionaryLangs(cfg.Languages)...))
	srv.wiktionary.Store(true) // on by default
	defer srv.closeModel()
	return srv.serve(os.Stdin, os.Stdout)
}

type lspServer struct {
	ctx        context.Context // nil means Background
	cfg        Config
	registry   atomic.Pointer[documentRegistry]
	ready      chan struct{} // closed when registry is set or load failed
	loadFailed atomic.Bool

	loadStarted atomic.Bool
	predictor   predictor // owned by the server; set before ready closes
	switcher    *switcher // nil unless cfg.Switch is set
	shutdown    atomic.Bool
	writeErr    atomic.Pointer[error] // first transport write failure

	wikt           atomic.Pointer[wiktionary.Client] // nil disables lookups
	langs          atomic.Pointer[[]lang.Language]   // current language scripts
	wiktionary     atomic.Bool                       // true = show Wiktionary defs (default)
	wiktionaryLang atomic.Pointer[string]            // comma list of codes, "detect", "model" or "auto"; unset = defaultWiktionaryLang

	pendingMu sync.Mutex
	pending   []*textItem // didOpen/didChange before model ready

	documents textDocuments

	writerMu  sync.Mutex
	writer    io.Writer // set in serve(); used to push server→client requests
	nextReqID atomic.Int64
}

// closeModel releases the predictor once any in-flight load has finished.
func (s *lspServer) closeModel() {
	if !s.loadStarted.Load() {
		return
	}
	<-s.ready
	if reg := s.registry.Load(); reg != nil {
		reg.Close()
	}
	if s.switcher != nil {
		s.switcher.close()
	}
	if s.predictor != nil {
		if err := s.predictor.Close(); err != nil {
			slog.Warn("close model", "err", err)
		}
	}
}

func (s *lspServer) context() context.Context {
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}

func (s *lspServer) serve(r io.Reader, w io.Writer) error {
	s.writerMu.Lock()
	s.writer = w
	s.writerMu.Unlock()
	br := bufio.NewReader(r)
	for {
		if werr := s.writeErr.Load(); werr != nil {
			return *werr
		}
		msg, err := readMessage(br)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if msg.Method == "exit" {
			if s.shutdown.Load() {
				return nil
			}
			return errExitWithoutShutdown
		}
		// Messages without a method are responses to our server→client requests; ignore them.
		if msg.Method != "" {
			// Document notifications must be handled in wire order so incremental changes
			// are applied to the document version they follow.
			if msg.Method == "textDocument/didOpen" || msg.Method == "textDocument/didChange" || msg.Method == "textDocument/didClose" {
				s.handle(msg)
			} else {
				// Dispatch slow handlers in a goroutine so they don't block the read loop.
				go s.handle(msg)
			}
		}
	}
}

func (s *lspServer) handle(msg jsonrpcMsg) {
	if s.shutdown.Load() && msg.ID != nil {
		s.writeResponse(msg.ID, nil, &rpcError{Code: codeInvalidRequest, Message: "server is shut down"})
		return
	}

	var result any
	var rpcErr *rpcError
	if h, ok := routes[msg.Method]; ok {
		result, rpcErr = h(s, msg.Params)
	} else if msg.ID != nil {
		rpcErr = &rpcError{Code: codeMethodNotFound, Message: "method not found: " + msg.Method}
	}
	if msg.ID != nil {
		s.writeResponse(msg.ID, result, rpcErr)
	}
}

type handlerFunc func(s *lspServer, raw json.RawMessage) (any, *rpcError)

// routes maps LSP methods to handlers; unknown notifications are ignored.
var routes = map[string]handlerFunc{
	"initialize":                       (*lspServer).handleInitialize,
	"initialized":                      func(*lspServer, json.RawMessage) (any, *rpcError) { return nil, nil },
	"shutdown":                         func(s *lspServer, _ json.RawMessage) (any, *rpcError) { s.shutdown.Store(true); return nil, nil },
	"workspace/didChangeConfiguration": typed((*lspServer).handleDidChangeConfiguration),
	"textDocument/didOpen":             typed((*lspServer).handleDidOpen),
	"textDocument/didChange":           typed((*lspServer).handleDidChange),
	"textDocument/didClose":            typed((*lspServer).handleDidClose),
	"textDocument/hover":               typed((*lspServer).handleHover),
	"textDocument/semanticTokens/full": typed((*lspServer).handleSemanticTokensFull),
}

// typed decodes params into P once, so handlers take typed arguments.
func typed[P any](fn func(*lspServer, P) (any, *rpcError)) handlerFunc {
	return func(s *lspServer, raw json.RawMessage) (any, *rpcError) {
		var p P
		if err := json.Unmarshal(raw, &p); err != nil {
			slog.Warn("bad params", "err", err)
			return nil, &rpcError{Code: codeInvalidParams, Message: err.Error()}
		}
		return fn(s, p)
	}
}

// --- workspace/didChangeConfiguration ---

type didChangeConfigurationParams struct {
	Settings struct {
		WiktionaryLanguage *string `json:"wiktionary_language"`
	} `json:"settings"`
}

// handleDidChangeConfiguration switches the hover definition language without a restart.
func (s *lspServer) handleDidChangeConfiguration(p didChangeConfigurationParams) (any, *rpcError) {
	if l := p.Settings.WiktionaryLanguage; l != nil {
		lang := strings.ToLower(strings.TrimSpace(*l))
		s.wiktionaryLang.Store(&lang)
	}
	return nil, nil
}

// --- initialize ---

type initializeParams struct {
	InitializationOptions *json.RawMessage `json:"initializationOptions"`
}

type initOptions struct {
	TokenMapUpdate        map[string]json.RawMessage `json:"token_map_update"`
	ScoreThreshold        *float64                   `json:"score_threshold"`
	WiktionaryDefinitions *bool                      `json:"wiktionary_definitions"`
	WiktionaryLanguage    *string                    `json:"wiktionary_language"`
	SemanticLightness     *float64                   `json:"semantic_lightness"`
	SemanticChroma        *float64                   `json:"semantic_chroma"`
}

type initializeResult struct {
	Capabilities serverCapabilities `json:"capabilities"`
}

type serverCapabilities struct {
	TextDocumentSync       int                    `json:"textDocumentSync"`
	HoverProvider          bool                   `json:"hoverProvider"`
	SemanticTokensProvider semanticTokensProvider `json:"semanticTokensProvider"`
}

type semanticTokensProvider struct {
	Legend semanticTokensLegend `json:"legend"`
	Full   bool                 `json:"full"`
}

type semanticTokensLegend struct {
	TokenTypes     []string `json:"tokenTypes"`
	TokenModifiers []string `json:"tokenModifiers"`
}

func (s *lspServer) handleInitialize(rawParams json.RawMessage) (any, *rpcError) {
	var params initializeParams
	if err := json.Unmarshal(rawParams, &params); err != nil {
		slog.Warn("initialize params", "err", err)
	}

	// Load model in background so initialize responds immediately.
	s.loadStarted.Store(true)
	go s.loadModel(params.InitializationOptions)

	return initializeResult{
		Capabilities: serverCapabilities{
			TextDocumentSync: 1, // Full
			HoverProvider:    true,
			SemanticTokensProvider: semanticTokensProvider{
				Legend: semanticTokensLegend{
					TokenTypes:     tokenmap.TypeNames[:],
					TokenModifiers: tokenmap.ModifierNames[:],
				},
				Full: true,
			},
		},
	}, nil
}

// loadModel loads the configured predictor, starts the registry, replays queued documents, and applies rawOpts.
func (s *lspServer) loadModel(rawOpts *json.RawMessage) {
	if s.cfg.Switch != nil && s.cfg.Forced == "" {
		s.startSwitcher()
	}
	var pred predictor
	var err error
	tokOpts := lang.TokenizerOptions(*s.langs.Load(), s.cfg.Forced)
	switch s.cfg.Mode {
	case ModeSemantic:
		pred, err = inference.NewEmbeddingModel(s.cfg.ModelPath, s.cfg.VocabPath, tokOpts...)
	case ModeDependency:
		pred, err = inference.NewDependencyModel(s.cfg.ModelPath, s.cfg.VocabPath, tokOpts...)
	default:
		pred, err = inference.NewPOSModel(s.cfg.ModelPath, s.cfg.VocabPath, tokOpts...)
	}
	if err != nil {
		slog.Error("load model", "err", err)
		s.loadFailed.Store(true)
		close(s.ready)
		return
	}
	s.predictor = pred
	reg := newDocumentRegistry(pred)
	if s.cfg.Forced != "" {
		forced := s.cfg.Forced
		reg.detect = func(string) string { return forced }
	}
	if sw := s.switcher; sw != nil {
		reg.pick = func(text string, cur predictor) predictor { return sw.pick(text, cur, pred) }
	}
	if s.cfg.Mode == ModeSemantic {
		// O → nil so encodeSemanticTokens emits nothing; colors come via semanticColors push instead.
		reg.extendTokenMap(map[postag.PartOfSpeech]*tokenmap.Override{postag.O: nil})
		reg.onDocReady = s.sendSemanticColors
	}
	s.registry.Store(reg)

	// Replay pending didOpen/didChange BEFORE signaling ready, so they lead any queued semantic-token requests.
	s.pendingMu.Lock()
	queued := s.pending
	s.pending = nil
	s.pendingMu.Unlock()
	for _, item := range queued {
		reg.update(item)
	}
	// With switching, the open documents may all be in other languages; their models should not sit beside unused English weights.
	if u, ok := pred.(unloader); ok && s.switcher != nil && !reg.uses(pred) {
		u.Unload()
	}
	close(s.ready)
	// Tell VS Code to re-request semantic tokens for all open documents.
	s.sendRequest("workspace/semanticTokens/refresh")

	if s.cfg.LoadLanguages != nil {
		go s.watchScripts()
	}
	if rawOpts == nil {
		return
	}
	var opts initOptions
	if err := json.Unmarshal(*rawOpts, &opts); err != nil {
		slog.Warn("initializationOptions", "err", err)
		return
	}
	s.applyOptions(reg, opts)
}

func (s *lspServer) applyOptions(reg *documentRegistry, opts initOptions) {
	if opts.TokenMapUpdate != nil {
		update := make(map[postag.PartOfSpeech]*tokenmap.Override)
		for tag, val := range opts.TokenMapUpdate {
			pos, ok := postag.ParsePartOfSpeech(tag)
			if !ok {
				continue
			}
			var o *tokenmap.Override // JSON null leaves it nil, disabling the tag
			if err := json.Unmarshal(val, &o); err == nil {
				update[pos] = o
			}
		}
		reg.extendTokenMap(update)
	}
	if opts.ScoreThreshold != nil {
		reg.setThreshold(*opts.ScoreThreshold)
	}
	if opts.WiktionaryDefinitions != nil {
		s.wiktionary.Store(*opts.WiktionaryDefinitions)
	}
	if opts.WiktionaryLanguage != nil {
		lang := strings.ToLower(strings.TrimSpace(*opts.WiktionaryLanguage))
		s.wiktionaryLang.Store(&lang)
	}
	if opts.SemanticLightness != nil || opts.SemanticChroma != nil {
		l, c := inference.SemanticColorParams()
		if opts.SemanticLightness != nil {
			l = *opts.SemanticLightness
		}
		if opts.SemanticChroma != nil {
			c = *opts.SemanticChroma
		}
		inference.SetSemanticColorParams(l, c)
	}
}

// --- didOpen / didChange / didClose ---

type didOpenParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Text    string `json:"text"`
		Version int32  `json:"version"`
	} `json:"textDocument"`
}

type didChangeParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version int32  `json:"version"`
	} `json:"textDocument"`
	ContentChanges []textDocumentContentChange `json:"contentChanges"`
}

type documentPosition struct {
	Line      uint32 `json:"line"`
	Character uint32 `json:"character"`
}

type textDocumentContentChange struct {
	Range *contentChangeRange `json:"range"`
	Text  string              `json:"text"`
}

type contentChangeRange struct {
	Start documentPosition `json:"start"`
	End   documentPosition `json:"end"`
}

type trackedDocument struct {
	text    string
	version int32
}

// textDocuments tracks the client's current text per URI; its zero value is ready to use.
type textDocuments struct {
	mu sync.Mutex
	m  map[string]trackedDocument
}

func (d *textDocuments) open(uri, text string, version int32) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.m == nil {
		d.m = make(map[string]trackedDocument)
	}
	d.m[uri] = trackedDocument{text: text, version: version}
}

func (d *textDocuments) get(uri string) trackedDocument {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.m[uri]
}

func (d *textDocuments) close(uri string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.m, uri)
}

// change applies changes in order and returns the new text; ok is false for stale versions.
func (d *textDocuments) change(uri string, version int32, changes []textDocumentContentChange) (text string, ok bool, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.m == nil {
		d.m = make(map[string]trackedDocument)
	}
	current, known := d.m[uri]
	if known && current.version >= version {
		return "", false, nil
	}
	runes := []rune(current.text)
	for _, change := range changes {
		if change.Range == nil {
			runes = []rune(change.Text)
			continue
		}
		if runes, err = applyRuneChange(runes, *change.Range, change.Text); err != nil {
			return "", false, err
		}
	}
	text = string(runes)
	d.m[uri] = trackedDocument{text: text, version: version}
	return text, true, nil
}

type didCloseParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

// writeBytes writes a pre-marshaled JSON body as a framed LSP message, under writerMu.
func (s *lspServer) writeBytes(body []byte) {
	s.writerMu.Lock()
	defer s.writerMu.Unlock()
	if s.writer == nil {
		return
	}
	if _, err := fmt.Fprintf(s.writer, "Content-Length: %d\r\n\r\n%s", len(body), body); err != nil {
		slog.Error("write", "err", err)
		s.writeErr.CompareAndSwap(nil, &err)
	}
}

// writeMsg serializes msg and writes it atomically under writerMu.
func (s *lspServer) writeMsg(msg jsonrpcMsg) {
	body, err := json.Marshal(msg)
	if err != nil {
		slog.Error("marshal", "method", msg.Method, "err", err)
		return
	}
	s.writeBytes(body)
}

// startSwitcher sets up per-language models and prefetches for documents opened so far; later ones prefetch in update.
func (s *lspServer) startSwitcher() {
	sw := newSwitcher(s.cfg)
	sw.languages = func() []lang.Language { return *s.langs.Load() }
	// A model that finishes before the registry exists is picked up by the replay in loadModel.
	sw.onReady = func() {
		if reg := s.registry.Load(); reg != nil {
			reg.refresh()
			s.sendRequest("workspace/semanticTokens/refresh")
		}
	}
	s.pendingMu.Lock()
	s.switcher = sw
	queued := slices.Clone(s.pending)
	s.pendingMu.Unlock()
	for _, item := range queued {
		sw.prefetch(item.text)
	}
}

// sendRequest sends a server→client request (has an id; client must reply); replies are ignored in the default handler.
func (s *lspServer) sendRequest(method string) {
	id := s.nextReqID.Add(1)
	raw := json.RawMessage(strconv.FormatInt(id, 10))
	s.writeMsg(jsonrpcMsg{JSONRPC: "2.0", ID: &raw, Method: method})
}

// update forwards item to the registry, or queues it until the model has loaded.
func (s *lspServer) update(item *textItem) {
	if reg := s.registry.Load(); reg != nil {
		reg.update(item)
		return
	}
	s.pendingMu.Lock()
	// Check again under lock in case model finished between the Load and Lock.
	if reg := s.registry.Load(); reg != nil {
		s.pendingMu.Unlock()
		reg.update(item)
		return
	}
	s.pending = append(s.pending, item)
	sw := s.switcher
	s.pendingMu.Unlock()
	if sw != nil {
		sw.prefetch(item.text)
	}
}

func (s *lspServer) handleDidOpen(p didOpenParams) (any, *rpcError) {
	s.documents.open(p.TextDocument.URI, p.TextDocument.Text, p.TextDocument.Version)
	s.update(&textItem{uri: p.TextDocument.URI, text: p.TextDocument.Text, version: p.TextDocument.Version})
	return nil, nil
}

func (s *lspServer) handleDidChange(p didChangeParams) (any, *rpcError) {
	if len(p.ContentChanges) == 0 {
		return nil, nil
	}
	text, ok, err := s.documents.change(p.TextDocument.URI, p.TextDocument.Version, p.ContentChanges)
	if err != nil {
		slog.Warn("didChange apply", "uri", p.TextDocument.URI, "err", err)
		return nil, nil
	}
	if ok {
		s.update(&textItem{uri: p.TextDocument.URI, text: text, version: p.TextDocument.Version})
	}
	return nil, nil
}

func (s *lspServer) handleDidClose(p didCloseParams) (any, *rpcError) {
	s.documents.close(p.TextDocument.URI)
	if reg := s.registry.Load(); reg != nil {
		reg.discard(p.TextDocument.URI)
	}
	return nil, nil
}

// --- hover ---

type textDocumentIdentifier struct {
	URI string `json:"uri"`
}

type hoverParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Position     documentPosition       `json:"position"`
}

type hoverResult struct {
	Contents markupContent `json:"contents"`
}

type markupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

func formatHoverContent(tok *postag.Token, dependents []dependent, wiktDef, wiktURL string) string {
	var header string
	if tok.HasHead || len(dependents) > 0 {
		lines := []string{fmt.Sprintf("%s: %s", tok.Word, tok.Deprel)}
		if len(dependents) > 0 {
			// Nesting under a scalar is invalid YAML, which highlighters render as one multi-line string.
			lines = append(lines, "dependents:")
		}
		for _, d := range dependents {
			line := fmt.Sprintf("  %s: %s", d.tok.Word, d.tok.Deprel)
			if d.hasDeps {
				line += "  # has dependents"
			}
			lines = append(lines, line)
		}
		header = fmt.Sprintf("```yaml\n%s\n```", strings.Join(lines, "\n"))
	} else {
		label := tok.Tag.Description()
		if tok.Description != "" {
			label = tok.Description
		}
		var body string
		if tok.Color != "" {
			body = fmt.Sprintf("%s: %s", tok.Word, label)
		} else {
			body = fmt.Sprintf("%s: %s  # %.2f", tok.Word, label, tok.Score)
		}
		header = fmt.Sprintf("```yaml\n%s\n```", body)
	}
	if wiktDef == "" {
		return header
	}
	return fmt.Sprintf("%s\n\n%s\n\n[Wiktionary](%s)", header, wiktDef, wiktURL)
}

func (s *lspServer) handleHover(p hoverParams) (any, *rpcError) {
	reg := s.registry.Load()
	if reg == nil {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(s.context(), replyTimeout)
	defer cancel()
	res, err := reg.hover(ctx, p.TextDocument.URI, p.Position.Line, p.Position.Character)
	if err != nil {
		slog.Debug("hover", "err", err)
		return nil, nil
	}
	if res == nil || res.tok == nil {
		return nil, nil
	}
	tok := res.tok
	var wiktDef, wiktURL string
	if wikt := s.wikt.Load(); s.wiktionary.Load() && wikt != nil {
		spec := defaultWiktionaryLang
		if p := s.wiktionaryLang.Load(); p != nil {
			spec = *p
		}
		modelLang := s.cfg.Language
		deprel := tok.Deprel
		if !res.hasDeprel {
			deprel = postag.NoDeprel
		}
		// A comma list is a priority order: the first language with a definition wins.
		for lang := range strings.SplitSeq(spec, ",") {
			lang = strings.TrimSpace(lang)
			switch lang {
			case "detect":
				if lang = res.language; lang == "" {
					continue // unsure: let the next entry decide
				}
			case "model":
				lang = modelLang
			}
			if lang == "" {
				lang = "en"
			}
			if def, url, ok := wikt.FetchDef(s.context(), tok.Word, lang, modelLang, tok.Tag, deprel); ok {
				wiktDef, wiktURL = def, url
				break
			}
		}
	}
	text := formatHoverContent(tok, res.dependents, wiktDef, wiktURL)
	return hoverResult{Contents: markupContent{Kind: "markdown", Value: text}}, nil
}

// --- semanticTokens/full ---

type semanticTokensParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type semanticTokensResult struct {
	Data []uint32 `json:"data"`
}

func (s *lspServer) handleSemanticTokensFull(p semanticTokensParams) (any, *rpcError) {
	empty := semanticTokensResult{Data: []uint32{}}
	reg := s.registry.Load()
	if reg == nil {
		return empty, nil
	}
	ctx, cancel := context.WithTimeout(s.context(), replyTimeout)
	defer cancel()
	data, err := reg.semanticTokens(ctx, p.TextDocument.URI)
	if err != nil {
		slog.Debug("semantic tokens", "err", err)
		return empty, nil
	}
	if data == nil {
		return empty, nil
	}
	return semanticTokensResult{Data: data}, nil
}

// --- $/nls/semanticColors push notification ---

type colorTokenJSON struct {
	Line      int    `json:"line"`
	Character int    `json:"character"`
	Length    int    `json:"length"`
	Color     string `json:"color"`
}

type semanticColorsParams struct {
	URI    string           `json:"uri"`
	Tokens []colorTokenJSON `json:"tokens"`
}

func (s *lspServer) sendSemanticColors(uri string, doc *document) {
	if doc == nil || len(doc.tokens) == 0 {
		return
	}
	lines := doc.lineIndex()
	tokens := make([]colorTokenJSON, 0, len(doc.tokens))
	for _, tok := range doc.tokens {
		if tok.Color == "" {
			continue
		}
		line, col := lines.position(int(tok.OffsetBegin))
		tokens = append(tokens, colorTokenJSON{
			Line:      line,
			Character: col,
			Length:    lines.utf16Len(int(tok.OffsetBegin), int(tok.OffsetEnd)),
			Color:     tok.Color,
		})
	}
	if len(tokens) == 0 {
		return
	}
	s.writeNotification("$/nls/semanticColors", semanticColorsParams{URI: uri, Tokens: tokens})
}

func (s *lspServer) writeNotification(method string, params any) {
	body, err := json.Marshal(params)
	if err != nil {
		slog.Error("marshal notification", "method", method, "err", err)
		return
	}
	s.writeMsg(jsonrpcMsg{JSONRPC: "2.0", Method: method, Params: body})
}

// --- JSON-RPC framing ---

func readMessage(r *bufio.Reader) (jsonrpcMsg, error) {
	contentLength := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return jsonrpcMsg{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if v, ok := strings.CutPrefix(line, "Content-Length:"); ok {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || n < 0 {
				return jsonrpcMsg{}, fmt.Errorf("bad Content-Length %q", v)
			}
			contentLength = n
		}
	}
	if contentLength < 0 {
		return jsonrpcMsg{}, errors.New("missing Content-Length")
	}
	buf := make([]byte, contentLength)
	if _, err := io.ReadFull(r, buf); err != nil {
		return jsonrpcMsg{}, err
	}
	var msg jsonrpcMsg
	if err := json.Unmarshal(buf, &msg); err != nil {
		return jsonrpcMsg{}, err
	}
	return msg, nil
}

func (s *lspServer) writeResponse(id *json.RawMessage, result any, rpcErr *rpcError) {
	// JSON-RPC 2.0 requires "result" field in success responses; don't use jsonrpcMsg here since its Result has omitempty.
	type successResp struct {
		JSONRPC string           `json:"jsonrpc"`
		ID      *json.RawMessage `json:"id"`
		Result  any              `json:"result"` // no omitempty — null must be present
	}
	type errorResp struct {
		JSONRPC string           `json:"jsonrpc"`
		ID      *json.RawMessage `json:"id"`
		Error   *rpcError        `json:"error"`
	}
	var body []byte
	var err error
	if rpcErr != nil {
		body, err = json.Marshal(errorResp{JSONRPC: "2.0", ID: id, Error: rpcErr})
	} else {
		body, err = json.Marshal(successResp{JSONRPC: "2.0", ID: id, Result: result})
	}
	if err != nil {
		slog.Error("marshal response", "err", err)
		return
	}
	s.writeBytes(body)
}
