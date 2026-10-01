#!/usr/bin/env bash
# Build natural-syntax-ls: export ONNX model, copy ORT runtime, compile binary.
#
# Usage: ./scripts/setup.sh [--model bert-base|mobilebert|minilm|mpnet|dependency|all]
#   --model  Which model to export (default: bert-base)

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MODEL="bert-base"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --model) MODEL="$2"; shift 2 ;;
        *) echo "Unknown arg: $1"; exit 1 ;;
    esac
done

# Validate early.
case "$MODEL" in
    bert-base|mobilebert|all|minilm|mpnet|dependency) ;;
    *) echo "ERROR: Unknown model '$MODEL'. Use bert-base, mobilebert, minilm, mpnet, dependency, or all." >&2; exit 1 ;;
esac

step() { echo; echo "==> $1"; }
ok()   { echo "    $1"; }
fail() { echo "ERROR: $1" >&2; exit 1; }

# ── 1. Check prerequisites ─────────────────────────────────────────────────

step "Checking prerequisites"

PYTHON=""
for cmd in python python3 py; do
    if command -v "$cmd" &>/dev/null && "$cmd" --version 2>&1 | grep -q "Python 3"; then
        PYTHON="$cmd"
        break
    fi
done
[[ -n "$PYTHON" ]] || fail "Python 3 not found. Install it and add to PATH."
ok "Python: $($PYTHON --version 2>&1)"

command -v go &>/dev/null || fail "Go not found. Install Go and add to PATH."
ok "Go: $(go version)"

# ── 2. Determine data directory ────────────────────────────────────────────

DATA_DIR="$("$PYTHON" -c "
import os, sys
if sys.platform == 'win32' or os.name == 'nt':
    base = os.environ.get('APPDATA', os.path.join(os.path.expanduser('~'), 'AppData', 'Roaming'))
else:
    base = os.environ.get('XDG_CONFIG_HOME', os.path.join(os.path.expanduser('~'), '.config'))
print(os.path.join(base, 'natural-syntax-ls'))
")"
mkdir -p "$DATA_DIR"
ok "Data directory: $DATA_DIR"

# ── 3. Python deps ─────────────────────────────────────────────────────────
#
# pip first, into a venv if PEP 668 ("externally managed environment") blocks
# a plain install, as on Arch, Debian 12+, Fedora, and other recent distros.
# Without pip, fall back to uv, then conda, each into an env under the repo.

step "Installing Python dependencies (transformers, torch, onnx, onnxscript, requests)"

PKGS=(transformers torch onnx onnxscript requests)
if [[ "$MODEL" == "dependency" || "$MODEL" == "all" ]]; then
    PKGS+=(diaparser)
fi

# env_python prints the interpreter path inside a venv/conda env, which differs on Windows.
env_python() {
    if [[ -x "$1/bin/python" ]]; then
        echo "$1/bin/python"
    else
        echo "$1/Scripts/python.exe"
    fi
}

pip_install() {
    "$PYTHON" -m pip install --quiet "${PKGS[@]}"
}

if "$PYTHON" -m pip --version &>/dev/null; then
    if ! pip_install; then
        step "System pip install failed (externally-managed-environment?), falling back to a venv"
        VENV_DIR="$ROOT/.venv"
        [[ -d "$VENV_DIR" ]] || "$PYTHON" -m venv "$VENV_DIR"
        PYTHON="$(env_python "$VENV_DIR")"
        ok "Using venv: $VENV_DIR"
        "$PYTHON" -m pip --version &>/dev/null || "$PYTHON" -m ensurepip --upgrade
        pip_install || fail "Failed to install Python dependencies even inside a venv."
    fi
elif command -v uv &>/dev/null; then
    step "pip not found, using uv"
    VENV_DIR="$ROOT/.venv"
    [[ -d "$VENV_DIR" ]] || uv venv --quiet --python "$PYTHON" "$VENV_DIR"
    PYTHON="$(env_python "$VENV_DIR")"
    ok "Using venv: $VENV_DIR"
    uv pip install --quiet --python "$PYTHON" "${PKGS[@]}" \
        || fail "Failed to install Python dependencies with uv."
elif command -v conda &>/dev/null; then
    step "pip and uv not found, using conda"
    CONDA_ENV="$ROOT/.conda-env"
    [[ -d "$CONDA_ENV" ]] || conda create --quiet --yes --prefix "$CONDA_ENV" python=3.12 pip \
        || fail "Failed to create conda env at $CONDA_ENV."
    PYTHON="$(env_python "$CONDA_ENV")"
    ok "Using conda env: $CONDA_ENV"
    pip_install || fail "Failed to install Python dependencies inside the conda env."
else
    fail "No pip, uv, or conda found. Install one of them (or python3-pip) and re-run."
fi
ok "Python deps ready."

# ── 4. Export ONNX model(s) + vocab + labels ──────────────────────────────

export_pos_model() {
    local model_name="$1" label="$2"
    local slug="${model_name//-/_}"
    if [[ -f "$DATA_DIR/${slug}.onnx" && -f "$DATA_DIR/${slug}_vocab.txt" && -f "$DATA_DIR/${slug}_labels.json" ]]; then
        ok "${slug}.onnx already exists, skipping export."
    else
        step "Exporting $label to ONNX"
        "$PYTHON" "$ROOT/scripts/export_model.py" "$DATA_DIR" --model "$model_name"
        ok "$label exported."
    fi
}

export_semantic_model() {
    local model_name="$1" label="$2"
    local slug="${model_name//-/_}"
    if [[ -f "$DATA_DIR/${slug}.onnx" && -f "$DATA_DIR/${slug}_vocab.txt" ]]; then
        ok "${slug}.onnx already exists, skipping export."
    else
        step "Exporting $label to ONNX"
        "$PYTHON" "$ROOT/scripts/export_model.py" "$DATA_DIR" --model "$model_name"
        ok "$label exported."
    fi
}

export_dependency_model() {
    step "Exporting en_ewt.electra-base dependency parser to ONNX"
    "$PYTHON" "$ROOT/scripts/export_model.py" "$DATA_DIR" --model en_ewt.electra-base
    ok "Dependency model exported."
}

case "$MODEL" in
    bert-base)   export_pos_model  bert-base  "BERT-base POS (~400 MB)" ;;
    mobilebert)  export_pos_model  mobilebert "MobileBERT POS (~100 MB)" ;;
    minilm)      export_semantic_model minilm "all-MiniLM-L6-v2 (~22 MB)" ;;
    mpnet)       export_semantic_model mpnet  "all-mpnet-base-v2 (~110 MB)" ;;
    dependency)  export_dependency_model ;;
    all)
        export_pos_model  bert-base  "BERT-base POS (~400 MB)"
        export_pos_model  mobilebert "MobileBERT POS (~100 MB)"
        export_semantic_model minilm "all-MiniLM-L6-v2 (~22 MB)"
        export_semantic_model mpnet  "all-mpnet-base-v2 (~110 MB)"
        export_dependency_model
        ;;
esac

# ── 5. Copy onnxruntime DLL/SO/dylib to data directory ────────────────────
#
# The yalue/onnxruntime_go module only bundles Windows x64 and ARM64
# Linux/macOS test binaries — Linux x64 and macOS x64 aren't in the module
# cache at all, so those fall back to downloading the official prebuilt
# release from Microsoft.

GOPATH="$(go env GOPATH)"
ARCH="$(uname -m)"

if [[ "$OSTYPE" == "msys" || "$OSTYPE" == "cygwin" || "$OSTYPE" == "win32" || -n "${WINDIR:-}" ]]; then
    ORT_DEST="$DATA_DIR/onnxruntime.dll"
    ORT_GLOB="$GOPATH/pkg/mod/github.com/yalue/onnxruntime_go@*/test_data/onnxruntime.dll"
    ORT_NAME="onnxruntime.dll"
    ORT_RELEASE_ARCHIVE=""
elif [[ "$OSTYPE" == "darwin"* ]]; then
    ORT_DEST="$DATA_DIR/libonnxruntime.dylib"
    ORT_NAME="libonnxruntime.dylib"
    if [[ "$ARCH" == "arm64" ]]; then
        ORT_GLOB="$GOPATH/pkg/mod/github.com/yalue/onnxruntime_go@*/test_data/onnxruntime_arm64.dylib"
        ORT_RELEASE_ARCHIVE=""
    else
        ORT_GLOB=""
        ORT_RELEASE_ARCHIVE="onnxruntime-osx-x86_64-1.26.0.tgz"
    fi
else
    ORT_DEST="$DATA_DIR/libonnxruntime.so"
    ORT_NAME="libonnxruntime.so"
    if [[ "$ARCH" == "aarch64" || "$ARCH" == "arm64" ]]; then
        ORT_GLOB="$GOPATH/pkg/mod/github.com/yalue/onnxruntime_go@*/test_data/onnxruntime_arm64.so"
        ORT_RELEASE_ARCHIVE=""
    else
        ORT_GLOB=""
        ORT_RELEASE_ARCHIVE="onnxruntime-linux-x64-1.26.0.tgz"
    fi
fi

if [[ -f "$ORT_DEST" ]]; then
    ok "$ORT_NAME already present in data directory."
elif [[ -n "$ORT_RELEASE_ARCHIVE" ]]; then
    step "Downloading official ONNX Runtime release ($ORT_RELEASE_ARCHIVE)"
    TMP_ORT="$(mktemp -d)"
    curl -fL --progress-bar -o "$TMP_ORT/ort.tgz" \
        "https://github.com/microsoft/onnxruntime/releases/download/v1.26.0/$ORT_RELEASE_ARCHIVE" \
        || fail "Failed to download ONNX Runtime release. Download $ORT_RELEASE_ARCHIVE manually from https://github.com/microsoft/onnxruntime/releases and place the shared library at $ORT_DEST"
    tar -xzf "$TMP_ORT/ort.tgz" -C "$TMP_ORT"
    ORT_SRC="$(find "$TMP_ORT" -name "$ORT_NAME" -o -name "${ORT_NAME}.*" | head -1)"
    [[ -n "$ORT_SRC" ]] || fail "Downloaded archive did not contain $ORT_NAME"
    cp "$ORT_SRC" "$ORT_DEST"
    rm -rf "$TMP_ORT"
    ok "Downloaded and installed $ORT_NAME."
else
    step "Locating $ORT_NAME in Go module cache"
    ORT_SRC="$(ls $ORT_GLOB 2>/dev/null | tail -1 || true)"
    if [[ -z "$ORT_SRC" ]]; then
        step "$ORT_NAME not in module cache, running go mod download"
        (cd "$ROOT" && go mod download)
        ORT_SRC="$(ls $ORT_GLOB 2>/dev/null | tail -1 || true)"
    fi
    [[ -n "$ORT_SRC" ]] || fail "Cannot find $ORT_NAME in Go module cache. Run 'go mod download' manually."
    cp "$ORT_SRC" "$ORT_DEST"
    ok "Copied $ORT_NAME to data directory."
fi

# ── 6. Build Go binary ─────────────────────────────────────────────────────

if [[ "$OSTYPE" == "msys" || "$OSTYPE" == "cygwin" || "$OSTYPE" == "win32" || -n "${WINDIR:-}" ]]; then
    BIN_NAME="natural-syntax-ls.exe"
else
    BIN_NAME="natural-syntax-ls"
fi

step "Building natural-syntax-ls"
rm -f "$ROOT/natural-syntax-ls.exe" "$ROOT/natural-syntax-ls.exe~" "$ROOT/natural-syntax-ls"
(cd "$ROOT" && go build -o "bin/$BIN_NAME" ./cmd/natural-syntax-ls/)
ok "Built: $ROOT/bin/$BIN_NAME"

# ── 7. Install binary to user bin directory ────────────────────────────────

INSTALL_DIR="${XDG_BIN_HOME:-$HOME/.local/bin}"
step "Installing to $INSTALL_DIR"
mkdir -p "$INSTALL_DIR"
cp "$ROOT/bin/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"
ok "Installed: $INSTALL_DIR/$BIN_NAME"
case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *) echo "    WARNING: $INSTALL_DIR is not on your PATH. Add it to use 'natural-syntax-ls' by name." ;;
esac

# ── Done ───────────────────────────────────────────────────────────────────

echo
echo "Done!"
echo
echo "Server binary: $INSTALL_DIR/$BIN_NAME"
echo "(Set naturalSyntaxLs.serverPath in VS Code to this to use it instead of the bundled one.)"
echo
echo "Model files are in: $DATA_DIR"
echo "The extension finds them automatically."
