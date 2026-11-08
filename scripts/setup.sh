#!/usr/bin/env bash
# Build natural-syntax-ls: export ONNX model, copy ORT runtime, compile binary.
#
# Usage: ./scripts/setup.sh [--model NAME|all] [--list]
#   --model  A model name from models.json (default: bert-base), or all
#   --list   Print the models in models.json and exit

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MODEL="bert-base"
LIST=0

while [[ $# -gt 0 ]]; do
    case "$1" in
    --model)
        MODEL="$2"
        shift 2
        ;;
    --list)
        LIST=1
        shift
        ;;
    *)
        echo "Unknown arg: $1"
        exit 1
        ;;
    esac
done

step() {
    echo
    echo "==> $1"
}
ok() { echo "    $1"; }
fail() {
    echo "ERROR: $1" >&2
    exit 1
}

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

# catalog prints one line per models.json entry: name, mode, int8 (0/1), description, tab separated.
CATALOG="$ROOT/data/models.json"
[[ -f "$CATALOG" ]] || fail "models.json not found at $CATALOG"
catalog() {
    "$PYTHON" -c '
import json, sys
for m in json.load(open(sys.argv[1], encoding="utf-8"))["models"]:
    print("\t".join([m["name"], m["mode"], "1" if m.get("int8") else "0", m.get("description", "")]))
' "$CATALOG"
}
if [[ "$LIST" == 1 ]]; then
    catalog | while IFS=$'\t' read -r name mode int8 desc; do printf '%-30s %-10s %s\n' "$name" "$mode" "$desc"; done
    exit 0
fi
if [[ "$MODEL" != "all" ]] && ! catalog | cut -f1 | grep -qxF -- "$MODEL"; then
    fail "Unknown model '$MODEL'. Use one of: $(catalog | cut -f1 | tr '\n' ' ')or all."
fi

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
MODELS_DIR="$DATA_DIR/models"
LIB_DIR="$DATA_DIR/lib"
mkdir -p "$DATA_DIR" "$MODELS_DIR" "$LIB_DIR"
ok "Data directory: $DATA_DIR"

# ── 3. Python deps ─────────────────────────────────────────────────────────
#
# pip first, into a venv if PEP 668 ("externally managed environment") blocks
# a plain install, as on Arch, Debian 12+, Fedora, and other recent distros.
# Without pip, fall back to uv, then conda, each into an env under the repo.

step "Installing Python dependencies (transformers, torch, onnx, onnxscript, onnxruntime, requests)"

PKGS=(transformers torch onnx onnxscript onnxruntime requests)
if catalog | awk -F'\t' -v m="$MODEL" '($1 == m || m == "all") && $2 == "dependency" {f=1} END {exit !f}'; then
    PKGS+=(diaparser)
fi
if [[ "$MODEL" == "all" || "$MODEL" == "ja_gsd.mbert" ]]; then
    PKGS+=(fugashi unidic-lite) # the Japanese parser's tokenizer
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
    uv pip install --quiet --python "$PYTHON" "${PKGS[@]}" ||
        fail "Failed to install Python dependencies with uv."
elif command -v conda &>/dev/null; then
    step "pip and uv not found, using conda"
    CONDA_ENV="$ROOT/.conda-env"
    [[ -d "$CONDA_ENV" ]] || conda create --quiet --yes --prefix "$CONDA_ENV" python=3.12 pip ||
        fail "Failed to create conda env at $CONDA_ENV."
    PYTHON="$(env_python "$CONDA_ENV")"
    ok "Using conda env: $CONDA_ENV"
    pip_install || fail "Failed to install Python dependencies inside the conda env."
else
    fail "No pip, uv, or conda found. Install one of them (or python3-pip) and re-run."
fi
ok "Python deps ready."

# ── 4. Export ONNX model(s) + vocab + labels ──────────────────────────────

slugify() { printf '%s' "$1" | tr './-' '___'; }

# has_tokenizer SLUG: WordPiece models ship _vocab.txt, SentencePiece ones only _tokenizer.json.
has_tokenizer() {
    [[ -f "$MODELS_DIR/${1}/${1}_vocab.txt" || -f "$MODELS_DIR/${1}/${1}_tokenizer.json" ]]
}

# export_model NAME MODE INT8 DESC: skips the export when the files for NAME already exist.
export_model() {
    local name="$1" mode="$2" int8="$3" desc="$4" slug extra=() done_check
    slug="$(slugify "$name")"
    [[ "$int8" == 1 ]] && extra=(--int8)
    case "$mode" in
    pos) done_check=$([[ -f "$MODELS_DIR/${slug}/${slug}.onnx" && -f "$MODELS_DIR/${slug}/${slug}_labels.json" ]] && has_tokenizer "$slug" && echo y || true) ;;
    semantic) done_check=$([[ -f "$MODELS_DIR/${slug}/${slug}.onnx" ]] && has_tokenizer "$slug" && echo y || true) ;;
    dependency)
        slug="${slug}_dependency"
        done_check=$([[ -f "$MODELS_DIR/${slug}/${slug}.onnx" && -f "$MODELS_DIR/${slug}/${slug}_rels.json" ]] && has_tokenizer "$slug" && echo y || true)
        ;;
    *) fail "Unknown mode '$mode' for $name in models.json" ;;
    esac
    if [[ -n "$done_check" ]]; then
        ok "${slug}.onnx already exists, skipping export."
    else
        step "Exporting $name ($mode: $desc) to ONNX"
        "$PYTHON" "$ROOT/scripts/export_model.py" "$MODELS_DIR" --model "$name" ${extra[@]+"${extra[@]}"}
        ok "$name exported."
    fi
}

while IFS=$'\t' read -r name mode int8 desc; do
    if [[ "$MODEL" == "all" || "$MODEL" == "$name" ]]; then
        export_model "$name" "$mode" "$int8" "$desc"
    fi
done < <(catalog)

# ── 5. Copy onnxruntime DLL/SO/dylib to data directory ────────────────────
#
# The yalue/onnxruntime_go module only bundles Windows x64 and ARM64
# Linux/macOS test binaries — Linux x64 and macOS x64 aren't in the module
# cache at all, so those fall back to downloading the official prebuilt
# release from Microsoft.

GOPATH="$(go env GOPATH)"
ARCH="$(uname -m)"

if [[ "$OSTYPE" == "msys" || "$OSTYPE" == "cygwin" || "$OSTYPE" == "win32" || -n "${WINDIR:-}" ]]; then
    ORT_DEST="$LIB_DIR/onnxruntime.dll"
    ORT_GLOB="$GOPATH/pkg/mod/github.com/yalue/onnxruntime_go@*/test_data/onnxruntime.dll"
    ORT_NAME="onnxruntime.dll"
    ORT_RELEASE_ARCHIVE=""
elif [[ "$OSTYPE" == "darwin"* ]]; then
    ORT_DEST="$LIB_DIR/libonnxruntime.dylib"
    ORT_NAME="libonnxruntime.dylib"
    if [[ "$ARCH" == "arm64" ]]; then
        ORT_GLOB="$GOPATH/pkg/mod/github.com/yalue/onnxruntime_go@*/test_data/onnxruntime_arm64.dylib"
        ORT_RELEASE_ARCHIVE=""
    else
        ORT_GLOB=""
        ORT_RELEASE_ARCHIVE="onnxruntime-osx-x86_64-1.26.0.tgz"
    fi
else
    ORT_DEST="$LIB_DIR/libonnxruntime.so"
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
        "https://github.com/microsoft/onnxruntime/releases/download/v1.26.0/$ORT_RELEASE_ARCHIVE" ||
        fail "Failed to download ONNX Runtime release. Download $ORT_RELEASE_ARCHIVE manually from https://github.com/microsoft/onnxruntime/releases and place the shared library at $ORT_DEST"
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

# Language scripts: copied per file, never overwriting a user-edited one.
step "Installing language scripts"
mkdir -p "$DATA_DIR/lang"
for f in "$ROOT"/data/lang/*.lua; do
    [[ -f "$DATA_DIR/lang/$(basename "$f")" ]] || cp "$f" "$DATA_DIR/lang/"
done
ok "Language scripts: $DATA_DIR/lang (add entries in $DATA_DIR/models.json to extend the bundled catalog)"

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
