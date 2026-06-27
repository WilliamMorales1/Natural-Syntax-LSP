package main

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
)

// LSP JSON-RPC over stdio.

type jsonrpcMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func runLSP(modelPath, vocabPath string) error {
	srv := &lspServer{modelPath: modelPath, vocabPath: vocabPath, ready: make(chan struct{})}
	srv.wiktionary.Store(true) // on by default
	return srv.serve(os.Stdin, os.Stdout)
}

type lspServer struct {
	modelPath  string
	vocabPath  string
	registry   atomic.Pointer[DocumentRegistry]
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
			s.handle(w, msg)
		case msg.Method == "" && msg.ID != nil:
			// response to one of our server→client requests; ignore
		}
	}
}

func (s *lspServer) handle(w io.Writer, msg jsonrpcMsg) {
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
		writeResponse(w, msg.ID, result, rpcErr)
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
		model, err := newPOSModel(s.modelPath, s.vocabPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "natural-syntax-ls: load model: %v\n", err)
			s.loadFailed.Store(true)
			close(s.ready)
			return
		}
		reg := newDocumentRegistry(model)
		s.registry.Store(reg)

		// Replay pending didOpen/didChange BEFORE signaling ready, so they
		// are ahead of any semantic-token requests in the registry queue.
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
					update := make(map[PartOfSpeech]*TokenTypeNModifiers)
					for tag, val := range opts.TokenMapUpdate {
						pos, ok := posFromString[tag]
						if !ok {
							continue
						}
						if string(val) == "null" {
							update[pos] = nil
						} else {
							var tnm TokenTypeNModifiers
							if err := json.Unmarshal(val, &tnm); err == nil {
								update[pos] = &tnm
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
			}
		}
	}()

	types := make([]string, N_TOKEN_TYPES)
	copy(types, tokenTypeNames[:])
	mods := make([]string, N_TOKEN_MODIFIERS)
	copy(mods, tokenModifierNames[:])

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

// sendRequest sends a server→client request (has an id; client must reply).
// Replies from the client arrive as messages with no Method; we ignore them
// in the default handler, which is fine for fire-and-forget requests.
func (s *lspServer) sendRequest(method string) {
	id := s.nextReqID.Add(1)
	raw, _ := json.Marshal(id)
	rm := json.RawMessage(raw)
	s.writerMu.Lock()
	w := s.writer
	s.writerMu.Unlock()
	if w == nil {
		return
	}
	writeMessage(w, jsonrpcMsg{JSONRPC: "2.0", ID: &rm, Method: method})
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

func (s *lspServer) handleHover(raw json.RawMessage) (any, *rpcError) {
	reg := s.registry.Load()
	var p hoverParams
	if err := json.Unmarshal(raw, &p); err != nil || reg == nil {
		return nil, nil
	}
	reply := make(chan *POSToken, 1)
	reg.send(registryMsg{
		kind:           msgHoverQuery,
		uri:            p.TextDocument.URI,
		hoverLine:      p.Position.Line,
		hoverCharacter: p.Position.Character,
		hoverReply:     reply,
	})
	tok := <-reply
	if tok == nil {
		return nil, nil
	}
	text := fmt.Sprintf("**%s** (%s) · confidence: %.2f", tok.Word, posDescription(tok.Tag), tok.Score)
	if s.wiktionary.Load() {
		if def, url, ok := fetchWiktionaryDef(tok.Word, tok.Tag); ok {
			text += fmt.Sprintf("\n\n%s\n\n[Wiktionary](%s)", def, url)
		}
	}
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

func writeResponse(w io.Writer, id *json.RawMessage, result any, rpcErr *rpcError) {
	type response struct {
		JSONRPC string           `json:"jsonrpc"`
		ID      *json.RawMessage `json:"id"`
		Result  any              `json:"result"`
		Error   *rpcError        `json:"error,omitempty"`
	}
	resp := response{JSONRPC: "2.0", ID: id, Result: result}
	if rpcErr != nil {
		resp.Result = nil
		resp.Error = rpcErr
	}
	body, _ := json.Marshal(resp)
	fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(body))
	w.Write(body)
}

func writeMessage(w io.Writer, msg jsonrpcMsg) {
	body, _ := json.Marshal(msg)
	fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(body))
	w.Write(body)
}
