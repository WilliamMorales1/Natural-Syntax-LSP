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

step "Installing Python dependencies (transformers, torch, onnx, onnxscript, requests)"
"$PYTHON" -m pip install --quiet transformers torch onnx onnxscript requests
if [[ "$MODEL" == "dependency" || "$MODEL" == "all" ]]; then
    "$PYTHON" -m pip install --quiet diaparser
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

# ── 5. Copy onnxruntime DLL/SO to data directory ──────────────────────────

GOPATH="$(go env GOPATH)"

if [[ "$OSTYPE" == "msys" || "$OSTYPE" == "cygwin" || "$OSTYPE" == "win32" || -n "${WINDIR:-}" ]]; then
    ORT_DEST="$DATA_DIR/onnxruntime.dll"
    ORT_GLOB="$GOPATH/pkg/mod/github.com/yalue/onnxruntime_go@*/test_data/onnxruntime.dll"
    ORT_NAME="onnxruntime.dll"
else
    ORT_DEST="$DATA_DIR/libonnxruntime.so"
    ORT_GLOB="$GOPATH/pkg/mod/github.com/yalue/onnxruntime_go@*/test_data/libonnxruntime.so"
    ORT_NAME="libonnxruntime.so"
fi

if [[ -f "$ORT_DEST" ]]; then
    ok "$ORT_NAME already present in data directory."
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

step "Building natural-syntax-ls"
rm -f "$ROOT/natural-syntax-ls.exe" "$ROOT/natural-syntax-ls.exe~" "$ROOT/natural-syntax-ls"
(cd "$ROOT" && go build -o bin/natural-syntax-ls.exe ./cmd/natural-syntax-ls/)
ok "Built: $ROOT/bin/natural-syntax-ls.exe"

# ── Done ───────────────────────────────────────────────────────────────────

echo
echo "Done!"
echo
echo "Set naturalSyntaxLs.serverPath in VS Code to:"
echo "  $ROOT/bin/natural-syntax-ls.exe"
echo
echo "Model files are in: $DATA_DIR"
echo "The extension finds them automatically."
