# TODO

## Attention-weighted subword pooling

**File:** `cmd/natural-syntax-ls/embedding_inference.go`, `EmbeddingModel.embedChunk()`

Currently mean-pools subword hidden states per word (lines ~160–192). This ignores attention: padding tokens and rare subwords get equal weight.

**What to do:**

- The ONNX session outputs `last_hidden_state` (shape `[1, seqLen, 384]`). The model also has an `attention_mask` input already populated in `m.attMask`.
- Weight each subword's hidden state by its attention mask value before averaging. Mask is 1 for real tokens, 0 for padding — already computed and available in `maskBuf`.
- Alternatively, export the model with `output_attentions=True` and use the last-layer attention scores as weights. This requires a new ONNX export (edit `scripts/export_embedding_model.py`) and a new output tensor in the session.
- Simple first step: use the attention mask weights (already available, no new export needed). This eliminates padding-token contamination at chunk boundaries.

---

## Incremental / streaming updates

**Files:** `cmd/natural-syntax-ls/document_registry.go`, `cmd/natural-syntax-ls/embedding_inference.go` (or `predictor.go`)

The whole document is re-predicted on every change (`scheduleProcessing` in `document_registry.go`). Large docs feel slow (~10s for BERT-base on 500 words).

**What to do:**

- Split `Predict(text)` into `PredictChunk(words []wordSpan) ([]POSToken, error)` — both models already chunk internally (`chunkSize` in `embedding_inference.go`; `main.go` tokenizer logic).
- In `DocumentRegistry`, track which char ranges are dirty (set on `didChange`). On each change, only re-predict dirty chunks.
- Push partial results via a new `msgPartialPredicted` message kind so the registry can update `store.doc.tokens` incrementally and call `onDocReady` after each chunk.
- For semantic mode, also push `$/nls/semanticColors` after each chunk so colors appear progressively.
- Coordinate with the existing `pendingReplies` queue: `semanticTokens/full` requests should wait for at least one full pass before responding.

---

## Bundle binary in VSIX

**Files:** `vscode-extension/package.json`, `vscode-extension/extension.js`, `.github/workflows/` (new)

Users must build `natural-syntax-ls.exe` themselves. Ideal: bundle prebuilt platform binaries inside the VSIX.

**What to do:**

1. Add a GitHub Actions workflow (`.github/workflows/release.yml`) that cross-compiles for `windows/amd64`, `linux/amd64`, `darwin/amd64`, `darwin/arm64` using `GOOS`/`GOARCH`.
2. Store binaries under `vscode-extension/bin/<platform>/natural-syntax-ls[.exe]`.
3. In `extension.js`, replace the `serverPath` lookup with a platform-keyed path into `context.extensionPath + '/bin/<platform>/natural-syntax-ls'`. Keep the `naturalSyntaxLs.serverPath` override for users who want to use their own build.
4. In `package.json`, add `"bin"` entries and update `.vscodeignore` so the binaries are included in the packaged VSIX.
5. The ORT shared library (`onnxruntime.dll` / `libonnxruntime.so`) must also be bundled — copy it alongside the binary during the CI build step (`go mod download`, then glob `$GOPATH/pkg/mod/github.com/yalue/onnxruntime_go@*/test_data/`).
