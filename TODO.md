# TODO

## Bundle binary in VSIX

**Files:** `vscode-extension/package.json`, `vscode-extension/extension.js`, `.github/workflows/` (new)

Users must build `natural-syntax-ls.exe` themselves. Ideal: bundle prebuilt platform binaries inside the VSIX.

**What to do:**

1. Add a GitHub Actions workflow (`.github/workflows/release.yml`) that cross-compiles for `windows/amd64`, `linux/amd64`, `darwin/amd64`, `darwin/arm64` using `GOOS`/`GOARCH`.
2. Store binaries under `vscode-extension/bin/<platform>/natural-syntax-ls[.exe]`.
3. In `extension.js`, replace the `serverPath` lookup with a platform-keyed path into `context.extensionPath + '/bin/<platform>/natural-syntax-ls'`. Keep the `naturalSyntaxLs.serverPath` override for users who want to use their own build.
4. In `package.json`, add `"bin"` entries and update `.vscodeignore` so the binaries are included in the packaged VSIX.
5. The ORT shared library (`onnxruntime.dll` / `libonnxruntime.so`) must also be bundled — copy it alongside the binary during the CI build step (`go mod download`, then glob `$GOPATH/pkg/mod/github.com/yalue/onnxruntime_go@*/test_data/`).
