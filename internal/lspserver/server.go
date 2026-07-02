// Package lspserver implements the LSP JSON-RPC-over-stdio server: protocol handling, per-document state, and semantic-token/hover encoding.
package lspserver

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"natural-syntax-ls/internal/inference"
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

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Config configures the LSP server's model/mode.
type Config struct {
	ModelPath       string // path to the mode's .onnx model
	VocabPath       string // path to that same model's own _vocab.txt
	Mode            string // "pos", "semantic", or "dependency"
	EmbedHiddenSize int    // hidden dimension of embedding model (384 or 768); semantic mode only
}

// Run starts the LSP server over stdin/stdout and blocks until the client disconnects.
func Run(cfg Config) error {
	srv := &lspServer{cfg: cfg, ready: make(chan struct{})}
	srv.wiktionary.Store(true) // on by default
	return srv.serve(os.Stdin, os.Stdout)
}

type lspServer struct {
	cfg        Config
	registry   atomic.Pointer[documentRegistry]
	ready      chan struct{} // closed when registry is set or load failed
	loadFailed atomic.Bool

	wiktionary atomic.Bool // true = show Wiktionary defs (default)

	pendingMu sync.Mutex
	pending   []registryMsg // didOpen/didChange before model ready

	writerMu  sync.Mutex
	writer    io.Writer // set in serve(); used to push server→client requests
	nextReqID atomic.Int64
}

func (s *lspServer) serve(r io.Reader, w io.Writer) error {
	s.writerMu.Lock()
	s.writer = w
	s.writerMu.Unlock()
	br := bufio.NewReader(r)
	for {
		msg, err := readMessage(br)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		switch {
		case msg.Method != "":
			// Dispatch in a goroutine so slow handlers don't block the read loop.
			go s.handle(msg)
		case msg.Method == "" && msg.ID != nil:
			// response to one of our server→client requests; ignore
		}
	}
}

func (s *lspServer) handle(msg jsonrpcMsg) {
	var result any
	var rpcErr *rpcError

	switch msg.Method {
	case "initialize":
		result, rpcErr = s.handleInitialize(msg.Params)
	case "initialized":
		// no-op notification
	case "shutdown":
		result = nil
	case "exit":
		os.Exit(0)
	case "textDocument/didOpen":
		s.handleDidOpen(msg.Params)
	case "textDocument/didChange":
		s.handleDidChange(msg.Params)
	case "textDocument/didClose":
		s.handleDidClose(msg.Params)
	case "textDocument/hover":
		result, rpcErr = s.handleHover(msg.Params)
	case "textDocument/semanticTokens/full":
		result, rpcErr = s.handleSemanticTokensFull(msg.Params)
	default:
		if msg.ID != nil {
			rpcErr = &rpcError{Code: -32601, Message: "method not found: " + msg.Method}
		}
	}

	if msg.ID != nil {
		s.writeResponse(msg.ID, result, rpcErr)
	}
}

// --- initialize ---

type initializeParams struct {
	InitializationOptions *json.RawMessage `json:"initializationOptions"`
}

type initOptions struct {
	TokenMapUpdate        map[string]json.RawMessage `json:"token_map_update"`
	ScoreThreshold        *float64                   `json:"score_threshold"`
	WiktionaryDefinitions *bool                      `json:"wiktionary_definitions"`
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
	_ = json.Unmarshal(rawParams, &params)

	// Load model in background so initialize responds immediately.
	go func() {
		var predictor inference.Predictor
		var err error
		switch s.cfg.Mode {
		case "semantic":
			predictor, err = inference.NewEmbeddingModel(s.cfg.ModelPath, s.cfg.VocabPath, s.cfg.EmbedHiddenSize)
		case "dependency":
			predictor, err = inference.NewDependencyModel(s.cfg.ModelPath, s.cfg.VocabPath)
		default:
			predictor, err = inference.NewPOSModel(s.cfg.ModelPath, s.cfg.VocabPath)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "natural-syntax-ls: load model: %v\n", err)
			s.loadFailed.Store(true)
			close(s.ready)
			return
		}
		reg := newDocumentRegistry(predictor)
		if s.cfg.Mode == "semantic" {
			// POS_O → nil so encodeSemanticTokens emits nothing; colors come via semanticColors push instead.
			reg.send(registryMsg{kind: msgTokenMapUpdate, mapUpdate: map[postag.PartOfSpeech]*tokenmap.Override{
				postag.POS_O: nil,
			}})
			reg.onDocReady = func(uri string, doc *document) {
				s.sendSemanticColors(uri, doc)
			}
		}
		s.registry.Store(reg)

		// Replay pending didOpen/didChange BEFORE signaling ready, so they lead any queued semantic-token requests.
		s.pendingMu.Lock()
		queued := s.pending
		s.pending = nil
		s.pendingMu.Unlock()
		for _, m := range queued {
			reg.send(m)
		}
		close(s.ready)
		// Tell VS Code to re-request semantic tokens for all open documents.
		s.sendRequest("workspace/semanticTokens/refresh")

		// Apply initializationOptions if provided.
		if params.InitializationOptions != nil {
			var opts initOptions
			if err := json.Unmarshal(*params.InitializationOptions, &opts); err == nil {
				if opts.TokenMapUpdate != nil {
					update := make(map[postag.PartOfSpeech]*tokenmap.Override)
					for tag, val := range opts.TokenMapUpdate {
						pos, ok := postag.FromString[tag]
						if !ok {
							continue
						}
						if string(val) == "null" {
							update[pos] = nil
						} else {
							var o tokenmap.Override
							if err := json.Unmarshal(val, &o); err == nil {
								update[pos] = &o
							}
						}
					}
					reg.send(registryMsg{kind: msgTokenMapUpdate, mapUpdate: update})
				}
				if opts.ScoreThreshold != nil {
					reg.send(registryMsg{kind: msgScoreThreshold, threshold: *opts.ScoreThreshold})
				}
				if opts.WiktionaryDefinitions != nil {
					s.wiktionary.Store(*opts.WiktionaryDefinitions)
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
		}
	}()

	types := make([]string, tokenmap.N_TOKEN_TYPES)
	copy(types, tokenmap.TypeNames[:])
	mods := make([]string, tokenmap.N_TOKEN_MODIFIERS)
	copy(mods, tokenmap.ModifierNames[:])

	return initializeResult{
		Capabilities: serverCapabilities{
			TextDocumentSync: 1, // Full
			HoverProvider:    true,
			SemanticTokensProvider: semanticTokensProvider{
				Legend: semanticTokensLegend{
					TokenTypes:     types,
					TokenModifiers: mods,
				},
				Full: true,
			},
		},
	}, nil
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
	ContentChanges []json.RawMessage `json:"contentChanges"`
}

type didCloseParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
}

// writeBytes writes a pre-marshaled JSON body as a framed LSP message, under writerMu.
func (s *lspServer) writeBytes(body []byte) {
	s.writerMu.Lock()
	if s.writer != nil {
		fmt.Fprintf(s.writer, "Content-Length: %d\r\n\r\n", len(body))
		s.writer.Write(body)
	}
	s.writerMu.Unlock()
}

// writeMsg serializes msg and writes it atomically under writerMu.
func (s *lspServer) writeMsg(msg jsonrpcMsg) {
	body, _ := json.Marshal(msg)
	s.writeBytes(body)
}

// sendRequest sends a server→client request (has an id; client must reply); replies are ignored in the default handler.
func (s *lspServer) sendRequest(method string) {
	id := s.nextReqID.Add(1)
	raw, _ := json.Marshal(id)
	rm := json.RawMessage(raw)
	s.writeMsg(jsonrpcMsg{JSONRPC: "2.0", ID: &rm, Method: method})
}

func (s *lspServer) queueOrSend(m registryMsg) {
	reg := s.registry.Load()
	if reg != nil {
		reg.send(m)
		return
	}
	s.pendingMu.Lock()
	// Check again under lock in case model finished between the Load and Lock.
	reg = s.registry.Load()
	if reg != nil {
		s.pendingMu.Unlock()
		reg.send(m)
		return
	}
	s.pending = append(s.pending, m)
	s.pendingMu.Unlock()
}

func (s *lspServer) handleDidOpen(raw json.RawMessage) {
	var p didOpenParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return
	}
	s.queueOrSend(registryMsg{
		kind: msgItem,
		item: &textItem{uri: p.TextDocument.URI, text: p.TextDocument.Text, version: p.TextDocument.Version},
	})
}

func (s *lspServer) handleDidChange(raw json.RawMessage) {
	var p didChangeParams
	if err := json.Unmarshal(raw, &p); err != nil || len(p.ContentChanges) == 0 {
		return
	}
	// Full sync — last change is the whole document.
	var change struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(p.ContentChanges[len(p.ContentChanges)-1], &change); err != nil {
		return
	}
	s.queueOrSend(registryMsg{
		kind: msgItem,
		item: &textItem{uri: p.TextDocument.URI, text: change.Text, version: p.TextDocument.Version},
	})
}

func (s *lspServer) handleDidClose(raw json.RawMessage) {
	reg := s.registry.Load()
	var p didCloseParams
	if err := json.Unmarshal(raw, &p); err != nil || reg == nil {
		return
	}
	reg.send(registryMsg{kind: msgDiscard, uri: p.TextDocument.URI})
}

// --- hover ---

type hoverParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Position struct {
		Line      uint32 `json:"line"`
		Character uint32 `json:"character"`
	} `json:"position"`
}

type hoverResult struct {
	Contents markupContent `json:"contents"`
}

type markupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

func formatHoverContent(tok *postag.POSToken, dependents []postag.POSToken, wiktDef, wiktURL string) string {
	var header string
	if tok.HasHead || len(dependents) > 0 {
		// Rendered as an "nlsdep" fenced block using this extension's own grammar (syntaxes/nlsdep.tmLanguage.json), which colors by position not keyword matching.
		var lines []string
		if len(dependents) == 0 {
			lines = []string{fmt.Sprintf("head %s %s", tok.Word, tok.Deprel.String())}
		} else {
			lines = []string{fmt.Sprintf("head %s %s {", tok.Word, tok.Deprel.String())}
			for _, d := range dependents {
				lines = append(lines, fmt.Sprintf("    %s %s", d.Word, d.Deprel.String()))
			}
			lines = append(lines, "}")
		}
		header = fmt.Sprintf("```nlsdep\n%s\n```", strings.Join(lines, "\n"))
	} else {
		label := postag.Description(tok.Tag)
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

func (s *lspServer) handleHover(raw json.RawMessage) (any, *rpcError) {
	reg := s.registry.Load()
	var p hoverParams
	if err := json.Unmarshal(raw, &p); err != nil || reg == nil {
		return nil, nil
	}
	reply := make(chan *hoverQueryResult, 1)
	reg.send(registryMsg{
		kind:           msgHoverQuery,
		uri:            p.TextDocument.URI,
		hoverLine:      p.Position.Line,
		hoverCharacter: p.Position.Character,
		hoverReply:     reply,
	})
	var res *hoverQueryResult
	select {
	case res = <-reply:
	case <-time.After(30 * time.Second):
	}
	if res == nil || res.tok == nil {
		return nil, nil
	}
	tok := res.tok
	var wiktDef, wiktURL string
	if s.wiktionary.Load() {
		if def, url, ok := wiktionary.FetchDef(tok.Word, tok.Tag); ok {
			wiktDef, wiktURL = def, url
		}
	}
	text := formatHoverContent(tok, res.dependents, wiktDef, wiktURL)
	return hoverResult{Contents: markupContent{Kind: "markdown", Value: text}}, nil
}

// --- semanticTokens/full ---

type semanticTokensParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
}

type semanticTokensResult struct {
	Data []uint32 `json:"data"`
}

func (s *lspServer) handleSemanticTokensFull(raw json.RawMessage) (any, *rpcError) {
	reg := s.registry.Load()
	var p semanticTokensParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return semanticTokensResult{Data: []uint32{}}, nil
	}
	if reg == nil {
		return semanticTokensResult{Data: []uint32{}}, nil
	}
	reply := make(chan []uint32, 1)
	reg.send(registryMsg{
		kind:     msgSemanticTokensCall,
		uri:      p.TextDocument.URI,
		semReply: reply,
	})
	data, ok := <-reply
	if !ok || data == nil {
		return semanticTokensResult{Data: []uint32{}}, nil
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
	lineStarts := buildLineStarts([]rune(doc.text))
	tokens := make([]colorTokenJSON, 0, len(doc.tokens))
	for _, tok := range doc.tokens {
		if tok.Color == "" {
			continue
		}
		charIdx := int(tok.OffsetBegin)
		line := charOffsetToLine(lineStarts, charIdx)
		col := charIdx - lineStarts[line]
		tokens = append(tokens, colorTokenJSON{
			Line:      line,
			Character: col,
			Length:    int(tok.OffsetEnd - tok.OffsetBegin),
			Color:     tok.Color,
		})
	}
	if len(tokens) == 0 {
		return
	}
	s.writeNotification("$/nls/semanticColors", semanticColorsParams{URI: uri, Tokens: tokens})
}

func (s *lspServer) writeNotification(method string, params any) {
	body, _ := json.Marshal(params)
	rm := json.RawMessage(body)
	s.writeMsg(jsonrpcMsg{JSONRPC: "2.0", Method: method, Params: rm})
}

// --- JSON-RPC framing ---

func readMessage(r *bufio.Reader) (jsonrpcMsg, error) {
	var contentLength int
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return jsonrpcMsg{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			n, _ := strconv.Atoi(strings.TrimSpace(line[len("Content-Length:"):]))
			contentLength = n
		}
	}
	if contentLength == 0 {
		return jsonrpcMsg{}, fmt.Errorf("missing Content-Length")
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
	if rpcErr != nil {
		body, _ = json.Marshal(errorResp{JSONRPC: "2.0", ID: id, Error: rpcErr})
	} else {
		body, _ = json.Marshal(successResp{JSONRPC: "2.0", ID: id, Result: result})
	}
	s.writeBytes(body)
}
