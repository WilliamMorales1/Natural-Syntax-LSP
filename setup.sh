#!/usr/bin/env bash
# Build natural-syntax-ls: export ONNX model, copy ORT runtime, compile binary.
#
# Usage: ./setup.sh [--model mobilebert|bert-base|all]
#   --model  Which POS model to export (default: mobilebert)

set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
MODEL="mobilebert"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --model) MODEL="$2"; shift 2 ;;
        *) echo "Unknown arg: $1"; exit 1 ;;
    esac
done

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

# ── 2. Python deps ─────────────────────────────────────────────────────────

step "Installing Python dependencies (transformers, torch, onnx, onnxscript, requests)"
"$PYTHON" -m pip install --quiet transformers torch onnx onnxscript requests
ok "Python deps ready."

# ── 3. Export ONNX model(s) + vocab ────────────────────────────────────────

export_model() {
    local model_name="$1" label="$2" onnx_file="$3" vocab_file="$4" labels_file="$5"
    if [[ -f "$ROOT/$onnx_file" && -f "$ROOT/$vocab_file" && -f "$ROOT/$labels_file" ]]; then
        ok "$onnx_file, $vocab_file, and $labels_file already exist, skipping export."
    else
        step "Exporting $label to ONNX"
        "$PYTHON" "$ROOT/export_model.py" "$ROOT" --model "$model_name"
        ok "$label exported."
    fi
}

case "$MODEL" in
    mobilebert) export_model mobilebert "MobileBERT POS (~100 MB)"  mobilebert_pos.onnx  mobilebert_vocab.txt  mobilebert_labels.json ;;
    bert-base)  export_model bert-base  "BERT-base POS (~400 MB)"   bert_base_pos.onnx   bert_base_vocab.txt   bert_base_labels.json  ;;
    all)
        export_model mobilebert "MobileBERT POS (~100 MB)"  mobilebert_pos.onnx  mobilebert_vocab.txt  mobilebert_labels.json
        export_model bert-base  "BERT-base POS (~400 MB)"   bert_base_pos.onnx   bert_base_vocab.txt   bert_base_labels.json
        ;;
    *) fail "Unknown model '$MODEL'. Use mobilebert, bert-base, or all." ;;
esac

# ── 4. Copy onnxruntime DLL/SO from Go module cache ───────────────────────

GOPATH="$(go env GOPATH)"

if [[ "$OSTYPE" == "msys" || "$OSTYPE" == "cygwin" || "$OSTYPE" == "win32" ]]; then
    ORT_DEST="$ROOT/onnxruntime.dll"
    ORT_GLOB="$GOPATH/pkg/mod/github.com/yalue/onnxruntime_go@*/test_data/onnxruntime.dll"
    ORT_NAME="onnxruntime.dll"
else
    ORT_DEST="$ROOT/libonnxruntime.so"
    ORT_GLOB="$GOPATH/pkg/mod/github.com/yalue/onnxruntime_go@*/test_data/libonnxruntime.so"
    ORT_NAME="libonnxruntime.so"
fi

if [[ -f "$ORT_DEST" ]]; then
    ok "$ORT_NAME already present."
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
    ok "Copied $ORT_SRC"
fi

# ── 5. Build Go binary ─────────────────────────────────────────────────────

step "Building natural-syntax-ls"
rm -f "$ROOT/natural-syntax-ls.exe" "$ROOT/natural-syntax-ls.exe~"
(cd "$ROOT" && go build -o natural-syntax-ls.exe .)
ok "Built: $ROOT/natural-syntax-ls.exe"

# ── Done ───────────────────────────────────────────────────────────────────

echo
echo "Done! Set naturalSyntaxLs.serverPath in VS Code to:"
echo "  $ROOT/natural-syntax-ls.exe"
echo
echo "Set naturalSyntaxLs.model to 'mobilebert' or 'bert-base' in VS Code settings."
