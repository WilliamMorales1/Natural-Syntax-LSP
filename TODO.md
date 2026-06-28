# TODO / Known Issues

## Semantic mode

- **Wiktionary not showing on first hover** — hover before the embedding model finishes predicting returns no token (doc not yet indexed). The model initializes in ~5s. Hover again after colors appear. If still missing, Wiktionary HTTP request may be timing out (increased to 8s but network/firewall can still block it).

- **No perceptual uniformity** — the 2D projection + atan2 hue approach distributes hues evenly over the embedding space but does not account for perceptual lightness differences between hues (yellow looks brighter than blue at the same HSL lightness). Could switch to OKLCH or use a perceptually uniform color space. There shouldn't even be a 2D projection to begin with, the hash should be directly from the embedding matrix itself.

- **Color contrast vs dark/light themes** — current HSL(h, 75%, 62%) is tuned for dark themes. On light themes, text may be hard to read. Should expose a lightness setting or auto-detect theme type.

- **Subword mean-pooling ignores attention** — embeddings are computed by mean-pooling over subword hidden states. A weighted pool (using attention scores) or using the CLS-based sentence embedding as context might produce better per-word vectors.

- **No color legend / sidebar** — there is no UI to see what colors are assigned or to inspect semantic neighborhoods. A webview panel showing word–color pairs would help users understand the highlighting.

## POS mode

- **Theme dependence** — colors come entirely from the VS Code theme's semantic token colors. On some themes (especially light ones), several POS categories may get the same color or an unreadable one. No way to override per-tag color without editing theme JSON.

- **`tokenMapUpdate` is JSON-only and unintuitive** — overriding POS → color requires writing raw JSON with LSP token type names. A settings UI with dropdowns would be better.

- **Proper nouns not distinguished from class names visually** — `NNP` maps to `class` which on many themes is the same color as `NNP`. Consider remapping to a less-used token type.

## General

- **Binary not bundled in VSIX** — users must build `natural-syntax-ls.exe` themselves and either put it on PATH or set `naturalSyntaxLs.serverPath`. Ideal fix: bundle prebuilt binaries per platform inside the VSIX.

- **No streaming / incremental updates** — the entire document is re-predicted on every change. Large documents feel slow. Could predict in chunks and update incrementally.

- **Windows-only tested** — scripts and binary have been tested on Windows. Linux/macOS paths in `setup.sh` are implemented but untested.

- **No sentence-level context for POS** — BERT processes a fixed window of tokens. Very long documents are chunked, potentially breaking sentence context at chunk boundaries.

- **`setup.sh` requires Git Bash on Windows** — users who only have PowerShell must use `setup.ps1` instead, but `setup.ps1` lags behind `setup.sh` in features.
