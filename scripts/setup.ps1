#Requires -Version 5
<#
.SYNOPSIS
    Build natural-syntax-ls: export ONNX model, copy ORT runtime, compile binary.
.PARAMETER Model
    Which model to export: bert-base (default), mobilebert, all, or semantic.
#>
param(
    [ValidateSet("bert-base","mobilebert","all","semantic")]
    [string]$Model = "bert-base"
)

$ErrorActionPreference = "Stop"
$ROOT = Split-Path $PSScriptRoot -Parent

function Step($msg) { Write-Host; Write-Host "==> $msg" }
function Ok($msg)   { Write-Host "    $msg" }
function Fail($msg) { Write-Host "ERROR: $msg" -ForegroundColor Red; exit 1 }

# ── 1. Check prerequisites ────────────────────────────────────────────────────

Step "Checking prerequisites"

$PYTHON = $null
foreach ($cmd in @("python","python3","py")) {
    try {
        $v = & $cmd --version 2>&1
        if ($v -match "Python 3") { $PYTHON = $cmd; break }
    } catch {}
}
if (-not $PYTHON) { Fail "Python 3 not found. Install it and add to PATH." }
Ok "Python: $(& $PYTHON --version 2>&1)"

if (-not (Get-Command go -ErrorAction SilentlyContinue)) { Fail "Go not found. Install Go and add to PATH." }
Ok "Go: $(go version)"

# ── 2. Determine data directory ───────────────────────────────────────────────

$base = if ($env:APPDATA) { $env:APPDATA } else { Join-Path $HOME "AppData\Roaming" }
$DATA_DIR = Join-Path $base "natural-syntax-ls"
New-Item -ItemType Directory -Force $DATA_DIR | Out-Null
Ok "Data directory: $DATA_DIR"

# ── 3. Python deps ────────────────────────────────────────────────────────────

Step "Installing Python dependencies (transformers, torch, onnx, onnxscript, requests)"
& $PYTHON -m pip install --quiet transformers torch onnx onnxscript requests
if ($LASTEXITCODE -ne 0) { Fail "pip install failed." }
Ok "Python deps ready."

# ── 4. Export ONNX model(s) ───────────────────────────────────────────────────

function Export-POSModel($modelName, $label, $onnxFile, $vocabFile, $labelsFile) {
    $o = Join-Path $DATA_DIR $onnxFile
    $v = Join-Path $DATA_DIR $vocabFile
    $l = Join-Path $DATA_DIR $labelsFile
    if ((Test-Path $o) -and (Test-Path $v) -and (Test-Path $l)) {
        Ok "$onnxFile, $vocabFile, and $labelsFile already exist, skipping export."
    } else {
        Step "Exporting $label to ONNX"
        & $PYTHON (Join-Path $ROOT "scripts\export_model.py") $DATA_DIR --model $modelName
        if ($LASTEXITCODE -ne 0) { Fail "Export failed for $label." }
        Ok "$label exported."
    }
}

function Export-EmbeddingModel {
    $onnx = Join-Path $DATA_DIR "minilm_embed.onnx"
    $vocab = Join-Path $DATA_DIR "minilm_vocab.txt"
    if ((Test-Path $onnx) -and (Test-Path $vocab)) {
        Ok "minilm_embed.onnx and minilm_vocab.txt already exist, skipping export."
    } else {
        Step "Exporting all-MiniLM-L6-v2 embedding model (~22 MB)"
        & $PYTHON (Join-Path $ROOT "scripts\export_embedding_model.py") $DATA_DIR
        if ($LASTEXITCODE -ne 0) { Fail "Embedding model export failed." }
        Ok "Embedding model exported."
    }
}

switch ($Model) {
    "bert-base"  { Export-POSModel "bert-base"  "BERT-base POS (~400 MB)"  "bert_base_pos.onnx"  "bert_base_vocab.txt"  "bert_base_labels.json" }
    "mobilebert" { Export-POSModel "mobilebert" "MobileBERT POS (~100 MB)" "mobilebert_pos.onnx" "mobilebert_vocab.txt" "mobilebert_labels.json" }
    "semantic"   { Export-EmbeddingModel }
    "all"        {
        Export-POSModel "bert-base"  "BERT-base POS (~400 MB)"  "bert_base_pos.onnx"  "bert_base_vocab.txt"  "bert_base_labels.json"
        Export-POSModel "mobilebert" "MobileBERT POS (~100 MB)" "mobilebert_pos.onnx" "mobilebert_vocab.txt" "mobilebert_labels.json"
        Export-EmbeddingModel
    }
}

# ── 5. Copy onnxruntime.dll ───────────────────────────────────────────────────

$ORT_DEST = Join-Path $DATA_DIR "onnxruntime.dll"
if (Test-Path $ORT_DEST) {
    Ok "onnxruntime.dll already present in data directory."
} else {
    Step "Locating onnxruntime.dll in Go module cache"
    $GOPATH = (go env GOPATH)
    $pattern = Join-Path $GOPATH "pkg\mod\github.com\yalue\onnxruntime_go@*\test_data\onnxruntime.dll"
    $ORT_SRC = Get-Item $pattern -ErrorAction SilentlyContinue | Select-Object -Last 1
    if (-not $ORT_SRC) {
        Step "onnxruntime.dll not in module cache, running go mod download"
        Push-Location $ROOT
        go mod download
        Pop-Location
        $ORT_SRC = Get-Item $pattern -ErrorAction SilentlyContinue | Select-Object -Last 1
    }
    if (-not $ORT_SRC) { Fail "Cannot find onnxruntime.dll in Go module cache. Run 'go mod download' manually." }
    Copy-Item $ORT_SRC $ORT_DEST
    Ok "Copied onnxruntime.dll to data directory."
}

# ── 6. Build Go binary ────────────────────────────────────────────────────────

Step "Building natural-syntax-ls"
$exePath = Join-Path $ROOT "natural-syntax-ls.exe"
Remove-Item "$exePath","${exePath}~" -ErrorAction SilentlyContinue
Push-Location $ROOT
go build -o natural-syntax-ls.exe ./cmd/natural-syntax-ls/
if ($LASTEXITCODE -ne 0) { Fail "go build failed." }
Pop-Location
Ok "Built: $exePath"

# ── Done ──────────────────────────────────────────────────────────────────────

Write-Host
Write-Host "Done!"
Write-Host
Write-Host "Set naturalSyntaxLs.serverPath in VS Code to:"
Write-Host "  $exePath"
Write-Host
Write-Host "Model files are in: $DATA_DIR"
Write-Host "The extension finds them automatically."
