# Go idiom review

Scope: all non-test Go under `cmd/` and `internal/` on branch `multilingual`. The code is already modern (range-over-int, `slices`, `strings.SplitSeq`, `wg.Go`, `min`/`max`, `atomic.Pointer`). Findings below are the remaining places where it departs from common Go patterns, ordered by payoff. Each item lists the location, the current pattern, the idiomatic replacement, and why.

`golangci-lint` was rebuilt against the current toolchain and `golangci-lint run ./...` reports 0 issues. Tests exempt `errcheck` and `noctx`; `Close`/`Destroy` returns are exempt everywhere.

## Status

Updated after the first implementation pass. `go vet ./...` and `go test -short -race ./...` pass.

Done: 2, 3, 4, 7 (env refcount only), 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 21, 22 (except typed 404 and negative cache), 23 (except negative cache), 24.

Partial:
- 1: registry has `Close()`, and hover and semantic-token waits share one timeout helper (`awaitReply`). Actor design kept.
- 5: `intraOpThreads` is atomic. The color projection is an atomic pointer built by `NewEmbeddingModel` and lazily by `EmbeddingToColor`, so `InitSemantic` is no longer required. The `init()` default for color params is gone. Still global: `SetORTLibPath` (wraps a global in the ort package), the wiktionary cache/client/`fetchPayload`, the Japanese segmenter and lemmatizer.
- 25: `slices` argmax, in-place softmax, shared `textDocumentIdentifier`, `[]dependent` replaces the parallel hover slices. Open: `chunkKey` digest, `replyQueue`, `pending` queue.
- 26: fuzz targets, plus subtests and `t.Parallel` in `postag_test`. `t.Cleanup` and `testing.Short` gating still open elsewhere.

Open, deliberately:
- 6: the `jaseg` blank import stays. It is the build-tag switch (`nojaseg`, `english`) and the package registers itself; explicit wiring would need the tokenizer to take options through `inference` constructors for little gain.
- 20: `HasHead` plus `HeadOffsetBegin` stays. Starting `Deprel` at `iota + 1` touches every table keyed by relation index and the model's relation sidecar; `HasHead` already guards the field.
- 5 (rest): a `wiktionary.Client` type means threading a receiver through `wikitext.go`, `simplify.go` and every test that swaps `fetchPayload`.

New this pass:
- `internal/lspserver/predictor.go` holds the consumer-side `predictor`, `textTokenizer`, `embedder` and `deprelModel` interfaces. `Close()` returns `error`.
- Method table `routes` with generic `typed[P]`: params decode once. Bad params get `-32602`. `rpcError` implements `error`.
- `lines.go`: `lineIndex` converts rune offsets and UTF-16 positions in both directions. Semantic token columns, lengths, hover lookup and `$/nls/semanticColors` are now UTF-16 correct for astral characters. Cached once per `document`. Edits apply on one rune slice per `didChange`.
- `tokenizer.Encoding` replaces the five-value return of `EncodeWords`.

Behavior changes worth knowing:
- `exit` without prior `shutdown` makes the server return an error, so the process exits 1. After `shutdown`, requests get `InvalidRequest`.
- Unknown `-mode` is rejected at startup.
- Logs go to stderr via `log/slog`. Default level is warn, `NLS_LOG=debug` enables debug.
- `Content-Length: 0` is no longer treated as missing. A missing or malformed header is an error.
- Transport write errors end the session at the next read.

## High value

### 1. Hand-rolled actor loop with `chan func()` and unbuffered replies

`internal/lspserver/registry.go` (`documentRegistry.ch`, `do`, `loop`), `server.go` (`handleHover`, `handleSemanticTokensFull`).

Pattern: every registry method wraps its body in a closure sent on a 64-slot channel; callers wait on a `reply` channel. Cancellation is encoded as "close the reply channel" or "send nil".

Problems:
- `do` blocks forever once the channel fills and the loop is wedged. No way to stop `loop`, so the goroutine leaks and tests cannot shut it down.
- `handleSemanticTokensFull` does `data := <-reply` with no timeout or cancel; `handleHover` uses `time.After(30s)` inline. Inconsistent.
- `nil` result means three different things (superseded, no token, timeout).

Replace with:
- `context.Context` on `semanticTokens` and `hover`, `select` on `ctx.Done()`. Wire the context from the LSP `$/cancelRequest` (see item 3).
- A `Close()` / `Stop()` on the registry that closes `ch` (or a `done` channel) so `loop` exits.
- Reply types that carry an explicit state: `type tokensReply struct{ data []uint32; superseded bool }` or return `(data, error)` with `errSuperseded`.

Alternative: replace actor with a `sync.Mutex` guarding `stores`. The actor buys serialization of state changes, but the code already holds results in one struct per URI; a mutex per `documentStore` plus `sync.Cond` or per-version `chan struct{}` for "ready" is simpler and removes the closure indirection. Pick one; do not keep both styles.

### 2. `context.Context` is absent from the whole call path

No non-test file imports `context`. Affected:
- `wiktionary.fetch` uses `http.NewRequest` + `client.Do`. Use `http.NewRequestWithContext(ctx, ...)` so hover cancellation aborts the HTTP request (today a cancelled hover still holds a goroutine for up to 8 s).
- `wiktionary.FetchDef`, `fetchDefUncached`, `fetchPayload` need a `ctx` first parameter.
- `Predictor.PredictChunk` cannot be cancelled; at minimum check `ctx.Err()` between blocks in `POSModel.PredictChunk` / `EmbeddingModel.embedChunk` so a superseded document stops early.
- `lspserver.Run(cfg)` should be `Run(ctx, cfg)` and `main` should pass `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)`.

### 3. `os.Exit(0)` inside the request handler

`server.go:113` (`case "exit": os.Exit(0)`).

Skips deferred cleanup, so `Predictor.Close()` (which calls `ort.DestroyEnvironment`) never runs, and the test suite cannot exercise the `exit` path. Replace with a `done` channel or cancel func that makes `serve` return `nil`; `main` then runs `defer predictor.Close()` and exits normally. Per LSP spec, `exit` after `shutdown` is code 0, otherwise 1; return a typed error and let `main` map it.

Also: `shutdown` is a no-op. It should set a `shuttingDown` flag and reject later requests with `InvalidRequest`.

### 4. Predictor is never closed on the server path

`server.go:loadModel` creates the predictor and hands it to `newDocumentRegistry`, but nothing calls `Predictor.Close()`. Own it in the server (`defer`) or give the registry a `Close` that stops the loop and closes the model. Constructor-owns-what-it-creates is the idiom; document ownership in the `newDocumentRegistry` comment.

### 5. Package-level mutable configuration

- `inference/concurrency.go`: `var intraOpThreads int` set by `SetIntraOpThreads`, read from `Concurrency()` (unsynchronized; race if called after load, and the comment says "must be called before").
- `inference/semantic_color.go`: `colorProjX`, `colorProjY` set by `InitSemantic(dim)` ("must be called once before any EmbeddingToColor call").
- `inference/shared.go`: `SetORTLibPath` wraps a process-global in the ort package.
- `tokenizer/unigram.go`: `japaneseSegmenter atomic.Pointer`, registered from `jaseg`'s `init()` via blank import in `main`.
- `wiktionary`: package `cache`, `client`, and `fetchPayload` function variable for test injection.

Replace with explicit dependencies:
- `type Options struct{ IntraOpThreads int; ORTLibPath string }` passed into `NewPOSModel` etc. (functional options or a config struct).
- `type ColorProjection struct{ x, y []float32 }` created by `NewColorProjection(dim)` and held by `EmbeddingModel`; `EmbeddingToColor` becomes a method. This also removes the "forgot to call InitSemantic" nil-deref hazard (`dot(v, nil)` silently yields zeros).
- `type wiktionary.Client struct{ HTTP *http.Client; cache sync.Map; fetch func(...) }` with `NewClient()`. Tests construct a `Client` with a fake `fetch` instead of mutating a package var (which also blocks `t.Parallel`).
- Japanese segmenter: pass as an option on `tokenizer.Open(path, tokenizer.WithJapaneseSegmenter(jaseg.Segment))` instead of `init()` side effects. Keeps the build-tag split but removes hidden registration.

### 6. `init()` and blank imports for registration

`cmd/.../main.go` imports `_ "natural-syntax-ls/internal/jaseg"` purely for the `init()` side effect, and `semantic_color.go` uses `init()` to seed a default atomic. Explicit wiring (item 5) is easier to read and test. If kept, the default atomic can be initialized in the declaration: `var p = func() *atomic.Pointer[...]{...}()` or store default lazily in `Load`.

## Medium value

### 7. Constructor and error-path cleanup via manual `ort.DestroyEnvironment()` calls

`inference/shared.go`, `pos_model.go`, `dependency_model.go`: each error branch calls `ort.DestroyEnvironment()` by hand. `NewDependencyModel` duplicates `newBaseModel` (own session, own `Close`).

Idiom: named return plus a deferred cleanup, or a small cleanup stack.

```go
func newBaseModel(...) (m *baseModel, err error) {
	if err := ort.InitializeEnvironment(...); err != nil { return nil, err }
	defer func() { if err != nil { ort.DestroyEnvironment() } }()
	...
}
```

Also factor `DependencyModel` onto `baseModel` (parameterize inputs/outputs) so `Close` exists once. Note `InitializeEnvironment` is process-global and ref-counted nowhere; two models in one process would double-destroy. Hide behind a package-level `sync.Once`/refcount helper (`acquireEnv()` / `releaseEnv()`).

### 8. `runWords` returns five values

`inference/shared.go:runWords` returns `(out []float32, seqLen int, swWordIdx []int, swIsFirst []bool, err error)`. `tokenizer.EncodeWords` also returns five slices. Group into a struct:

```go
type Encoding struct {
	IDs, Mask, TypeIDs []int64
	WordIdx            []int
	IsFirst            []bool
}
```

`buildPoolMatrix` returns five values too. Structs make call sites self-documenting and let new fields be added without touching every caller.

### 9. Tensor creation boilerplate in `runWords`

Four near-identical `ort.NewTensor` + `defer Destroy` blocks with distinct wrapped error strings. Collect in a loop over a small slice of `(name, data)` and keep a `[]ort.Value` plus one `defer func(){ for _, v := range vals { v.Destroy() } }()`. Cuts ~25 lines and cannot forget a Destroy.

### 10. `interface` placement and size

`inference/predictor.go` defines `Predictor` (producer side) alongside `TextTokenizer` and `Embedder`. Go proverb: accept interfaces at the consumer. The only consumer is `lspserver`; define the small interfaces there:

```go
// package lspserver
type predictor interface { PredictChunk([]tokenizer.WordSpan) ([]postag.Token, error); Close() }
```

and drop the producer-side interface, returning concrete `*POSModel` etc. from constructors (already done). Then the `_, isDependency := model.(*inference.DependencyModel)` type assertion in `newDocumentRegistry` can become a method on the interface (`UsesDeprel() bool`) or a field set by `loadModel`, which already knows the mode.

Also `Predictor.Close()` should return `error` (`io.Closer`) so it composes with `defer` helpers and `errors.Join`.

### 11. Stringly-typed `Mode`

`lspserver.Config.Mode string` ("pos", "semantic", "dependency"), switched in `loadModel`, compared with `== "semantic"` in three places. Define `type Mode string` with constants, parse once in `main` via `flag.Func` or `UnmarshalText`, and reject unknown values (today an unknown `-mode` silently falls through to POS).

### 12. JSON-RPC layer re-implemented by hand

`server.go` contains framing (`readMessage`, `writeBytes`), response encoding with two throwaway local types in `writeResponse`, and a `jsonrpcMsg` struct that mixes request/response/notification fields. Options:
- Adopt `go.lsp.dev/jsonrpc2` / `golang.org/x/tools/internal/jsonrpc2` pattern (dependency trade-off; the project currently has a single dependency).
- If staying dependency-free, split into `type request`, `type response{Result json.RawMessage}` with `Result` marshaled eagerly, and a separate `transport` type owning `writerMu`/`writer`/`nextReqID`. That also makes `writer` a constructor argument instead of being assigned inside `serve` under a mutex.
- Move to its own file (`jsonrpc.go`); `server.go` is 689 lines mixing protocol types, text sync, hover, and encoding.

Concrete bugs in this layer:
- `readMessage` treats `Content-Length: 0` as "missing"; a zero-length body is legal if useless.
- `writeMsg`/`writeNotification` discard `json.Marshal` errors (`body, _ :=`).
- `writeBytes` ignores the `fmt.Fprintf` error; a broken pipe goes unnoticed. Return the error and make `serve` exit on it.
- Dropped `json.Unmarshal` errors in `handleInitialize` and notification handlers; at least log to stderr (item 14).

### 13. Large stringly `switch msg.Method`

Replace with a `map[string]func(*lspServer, jsonrpcMsg) (any, *rpcError)` registered once, or generic helper `func handle[P, R any](s *lspServer, fn func(P) (R, *rpcError)) handler` that unmarshals params once. That removes the repeated `var p X; if err := json.Unmarshal(raw, &p); err != nil { return ... }` block in each handler (appears 5 times).

Also `rpcError` should implement `error` (`func (e *rpcError) Error() string`) and handlers should return `error` with `errors.As` at the dispatch site, instead of a `*rpcError` out-parameter in every signature. Beware the typed-nil-in-interface trap when doing this.

### 14. No structured logging

Diagnostics are `fmt.Fprintf(os.Stderr, "natural-syntax-ls: ...")` in four places; failures inside handlers are swallowed with bare `return`. Use `log/slog` with a `*slog.Logger` on the server (stderr handler, level via flag or `NLS_LOG`). Then add `slog.Warn` at each silent `return` in `handleDidOpen`, `handleDidChange`, `applyContentChange`.

### 15. `atomic.Value` where `atomic.Pointer`/`atomic.Bool` fits

`lspServer.wiktionaryLang atomic.Value` is type-asserted at read (`lang, _ := ...Load().(string)`) and stores a string. Use `atomic.Pointer[string]`, or group `wiktionary bool` + `wiktionaryLang string` into one immutable `wiktionaryOpts` struct behind a single `atomic.Pointer`. Same for `SemanticLightness/Chroma` handling in `applyOptions`, which reads the current pair then stores a new one (a lost-update race when two `applyOptions` interleave; low risk but trivial to fix with `CompareAndSwap` or by storing only from one goroutine).

### 16. Lazy map initialization under lock, duplicated

`handleDidOpen` and `handleDidChange` both do `if s.documents == nil { s.documents = make(...) }` under `documentsMu`. Initialize in `Run` (`documents: map[string]trackedDocument{}`) and delete both checks. Better: a small `documentStoreText` type with `Open/Change/Close` methods that owns the mutex and the map, and move `applyContentChange` / `positionToRuneOffset` into it. Also replace manual `Unlock()` on each early return with a helper function that uses `defer`.

## Lower value / polish

### 17. Add lint config and CI gate

No `.golangci.yml`; `.github/workflows` has only `release.yml` (no test or vet job). Add a `ci.yml` running `go vet ./...`, `go test -race ./...` (with `-short` to skip model-dependent tests), and `golangci-lint` with at least: `errcheck`, `govet`, `staticcheck`, `revive` (exported doc comments, receiver names), `gocritic`, `copyloopvar`, `modernize`, `nilerr`, `errorlint`, `bodyclose`, `noctx` (would flag item 2).

### 18. Exported identifiers without doc comments, and `go vet` hygiene

Missing or non-conforming doc comments on exported names: all `postag.PartOfSpeech` and `Deprel` constants, `PartOfSpeech.String`, `MarshalText`, `UnmarshalText`, `Deprel.String`, `MarshalText`, `UnmarshalText`, `tokenmap.Type`/`Modifier.UnmarshalText`, `inference.POSModel.PredictChunk` (has), `EmbeddingModel.embedBlock` (unexported but undocumented). Use group comments for const blocks. Run `revive -enable-rule exported`.

### 19. Enum sentinel via `iota` in the same block

`NumPartsOfSpeech = iota`, `NumDeprels = iota`, `NumTypes = iota`, `NumModifiers = iota`: each is an untyped int constant at the end of a typed enum block, which works but is not typed. Write `numPartsOfSpeech` as `PartOfSpeech(iota)` or keep as is but add `//go:generate stringer`-style tests. More important: `PartOfSpeech.valid()` allows negative values because the type is `int`. Use `uint8` underlying type: it removes the `p >= 0` checks, shrinks `Token` (currently 112+ bytes with two strings, a string color, and a Deprel `int`), and makes `Map [NumPartsOfSpeech]*Bits` index checks one-sided.

### 20. `Token` struct layout and `HasHead` + offset pair

`postag.Token` carries `HasHead bool` plus `HeadOffsetBegin uint32` ("valid only if HasHead"). Idiom is a pointer or sentinel: `Head int32` with `-1` meaning none, or `Head *uint32`. Also `Deprel` has no zero value meaning "none" (zero is `DepAcl`), so `Deprel` on a POS-mode token is a live-looking `acl`. Start the enum at `iota + 1` with `DepNone = 0`, or keep `HasHead` as the guard but drop the comment "meaningless unless".

### 21. `[]rune(text)` conversions on every operation

`applyContentChange`, `positionToRuneOffset`, `sendSemanticColors` (`buildLineStarts([]rune(doc.text))`), `SplitChunks`, `basicTokenize` each convert the full document to `[]rune`. For large files that is an allocation per keystroke. Hold the rune slice (or a precomputed line index) on `document` / `trackedDocument` once per version. LSP wants UTF-16 offsets anyway: put one `type lineIndex` in its own file with `OffsetToPosition` / `PositionToOffset`; today the UTF-16 logic is split between `positionToRuneOffset` (incoming) and `lineCursor`/`lineToCharOffset` (outgoing), and `lineToCharOffset` treats `character` as a rune count, which disagrees with `positionToRuneOffset` for astral characters.

### 22. Error values and wrapping

- Errors built with `fmt.Errorf("invalid content change start position")` (no verbs) should be package sentinels: `var errBadPosition = errors.New("...")`, so callers can `errors.Is`.
- `tokenizer.Load` returns raw `os`/`json` errors with no path context. Wrap: `fmt.Errorf("load vocab %s: %w", path, err)`.
- `wiktionary.fetch`'s status error should be a typed error (`*StatusError`) or sentinel so a 404 ("no entry") can be distinguished from a transient failure and cached negatively. Today failed lookups are never cached, so a missing word re-hits the network on every hover.
- `NewPOSModel` fallback logic uses `errors.Is(err, fs.ErrNotExist)` correctly; keep and mirror in `NewDependencyModel`.

### 23. `wiktionary` details

- `sort.Strings(rest)` → `slices.Sorted(maps.Keys(payload))` (removes the `rest` build loop and the `sort` import).
- `order := []string{}` → `var order []string`.
- `cache sync.Map` with a `cacheEntry` type assertion → `sync.Map` is for write-once/read-many; with a bounded map under `sync.RWMutex` (or an LRU) you get type safety and a size cap. The unbounded cache grows with every hovered word for the life of the editor.
- Compiled regexes at package level are fine. The `htmlEntities` map plus regex could be `html.UnescapeString` from stdlib (handles all named and numeric entities, replacing ~15 lines).
- `append(ordered(normal), ordered(lowPri)...)` can alias `normal`'s backing array; use `slices.Concat`.
- `User-Agent` hardcodes a stale repo URL (`wsm5224/NLSyntaxHighlighting-Go`); take from a package `var`/build-time `-ldflags -X` version string.

### 24. `main.go`

- `configDir()` reimplements `os.UserConfigDir()` with an XDG tweak for macOS; ignore-errors on `os.UserHomeDir()`. Fine, but return `(string, error)` and handle.
- Candidate-path list is a 10-line literal repeated for dataDir/exeDir. Build it by looping `for _, dir := range []string{dataDir, exeDir} { for _, name := range modelNames { ... } }`. The model order also appears inconsistent (bert_base/mobilebert across both dirs before minilm variants).
- `fmt.Fprintln(os.Stderr, ...); os.Exit(1)` in `main` is fine, but move logic into `func run() error` so `main` is `if err := run(); err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }` and all defers run.
- `flag.Bool("stdio", ...)` result discarded; fine, but use `_ = flag.Bool` to signal intent.
- `modelLanguage` ranges a map literal that is rebuilt per call; make it a package-level slice of `{prefix, lang}` structs (also gives deterministic order).

### 25. Small idioms

- `argmax` hand-written in `pos_model.go` and `argmaxFloat32ExceptSelf` in `dependency_model.go`: for the first, `slices.Index(scores, slices.Max(scores))`.
- `softmax` allocates a new slice per subword (`pos_model.go`); do it in place or reuse a scratch buffer. It runs once per first-subword per chunk.
- `chunkKey` uses `strings.Builder` with NUL separators; a `hash/maphash` or `crypto/sha1` digest would shrink map keys from kilobytes to 8-20 bytes with equal practical safety, or use `[]string` + `slices.Equal` with a hash prefix.
- `replyQueue` is a fixed two-slot structure with two methods and ~40 lines of index juggling. A `[]chan []uint32` with `slices.Delete` is simpler; or drop the queue by always superseding older requests (LSP allows cancelling stale full-token requests).
- `lspServer.pending []*textItem` + `pendingMu` + double-checked `registry.Load()` in `update` can be replaced by having `update` send to the registry's channel from construction time, with the registry buffering until the model arrives; or `sync.OnceValue`/a `ready` channel receive (`<-s.ready` already exists).
- `hoverParams` and `hoverResult` use anonymous nested structs for `TextDocument`/`Position`; define `textDocumentIdentifier` and `position` once and reuse (currently `documentPosition` duplicates `Position`).
- `formatHoverContent` has a 5-argument signature taking two parallel slices (`dependents`, `dependentsHaveDeps`); fold into `[]dependent{tok postag.Token; hasDeps bool}`. Parallel slices are a recurring smell: also `chunkResult.tokens`/`embeds`, `POSModel.labels`/`cont`, and `conts []bool` in `predictBlock`. Prefer a slice of struct.
- Prefer `any` over `interface{}`: already done. `strconv.FormatInt` in `sendRequest` can be `json.RawMessage(strconv.AppendInt(nil, id, 10))` or just use an `int64` ID field.
- Remove the doc comment "Run starts ... and blocks until the client disconnects" mismatch: with item 3 fixed it should also document the exit code semantics.

### 26. Tests

Positives: `t.Helper` used, `rand.New(rand.NewPCG(...))` seeded, table-driven loops in `postag_test`.

Gaps:
- Few `t.Run` subtests (grep: 5 in `tokenizer_test`, 3 in `registry_test`). Table-driven cases using a `[]struct{name string; ...}` with `t.Run(tc.name, ...)` give per-case failures and `-run` filtering. `TestDeprel` loops over a map literal with no names.
- No `t.Parallel()` anywhere; blocked in `wiktionary` by package globals (item 5) and in `inference` by the process-global ORT environment.
- No `t.Cleanup`/`t.TempDir()` usage; use `t.Cleanup(m.Close)` over `defer m.Close()` in helpers so it runs after subtests.
- Model-dependent tests (`models_test.go`, `eval_test.go`, `multilingual_test.go`) should gate with `testing.Short()` and a missing-model `t.Skip`, not rely on being run selectively.
- No fuzz targets. `positionToRuneOffset`/`applyContentChange`, `Tokenize`, `parseCPUList`, and `stripHTML` are pure and cheap to fuzz (`func FuzzParseCPUList(f *testing.F)`).
- `-race` is the main check missing for the registry/server concurrency; add to CI (item 17).
- `contextModel.Close()` in test code is fine, but if `Predictor` moves to the consumer (item 10) the fake can live in an `export_test.go`-style helper.

## Suggested order of work

1. Items 3, 4, 12 (bug-class fixes: exit path, leaked model, ignored write/marshal errors). Small and safe.
2. Items 1, 2 (context + registry lifecycle). Largest behavior change; do with `-race` tests in place (item 26).
3. Items 5, 6 (remove package-level mutable state); unlocks parallel tests.
4. Items 7-11, 13-16 (structure and types).
5. Items 17-26 (lint, docs, polish, fuzz).

## Things already idiomatic (leave alone)

- Package layout (`cmd/` + `internal/`), small focused packages, one dependency.
- `//go:build` split for `jaseg` with a stub sharing the package name.
- `MarshalText`/`UnmarshalText` on enum types so JSON uses names.
- `%w` wrapping in the inference constructors.
- `defer` for `opts.Destroy()` and `resp.Body.Close()`.
- Use of `slices`, `maps`-style helpers, `min`/`max`, `range` over int and iterators, `wg.Go`.
- Doc comments on most exported symbols, starting with the name.
- Generics used sparingly and appropriately (`decodeEnumList`, `indexByName`).
