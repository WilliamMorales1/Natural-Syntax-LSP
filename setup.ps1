#Requires -Version 5
<#
.SYNOPSIS
    Build natural-syntax-ls: export ONNX model, copy ORT runtime, compile binary.
#>
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$root = $PSScriptRoot

function Step($label) { Write-Host "`n==> $label" -ForegroundColor Cyan }
function Ok($msg)   { Write-Host "    $msg" -ForegroundColor Green }
function Fail($msg) { Write-Host "ERROR: $msg" -ForegroundColor Red; exit 1 }

# ── 1. Check prerequisites ─────────────────────────────────────────────────

Step "Checking prerequisites"

$python = $null
foreach ($cmd in @("python", "python3", "py")) {
    if (Get-Command $cmd -ErrorAction SilentlyContinue) { $python = $cmd; break }
}
if (-not $python) { Fail "Python not found. Install Python 3 and add it to PATH." }
Ok "Python: $(& $python --version 2>&1)"

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Fail "Go not found. Install Go and add it to PATH."
}
Ok "Go: $(go version)"

# ── 2. Python deps ─────────────────────────────────────────────────────────

Step "Installing Python dependencies (transformers, torch, onnx, onnxscript, requests)"
& $python -m pip install --quiet transformers torch onnx onnxscript requests
if ($LASTEXITCODE -ne 0) { Fail "pip install failed." }
Ok "Python deps ready."

# ── 3. Export ONNX model + vocab ───────────────────────────────────────────

$modelFile = Join-Path $root "mobilebert_pos.onnx"
$vocabFile  = Join-Path $root "vocab.txt"

if ((Test-Path $modelFile) -and (Test-Path $vocabFile)) {
    Ok "mobilebert_pos.onnx and vocab.txt already exist, skipping export."
} else {
    Step "Exporting MobileBERT POS model to ONNX (this downloads ~100 MB)"
    & $python (Join-Path $root "export_model.py") $root
    if ($LASTEXITCODE -ne 0) { Fail "export_model.py failed." }
    Ok "Model exported."
}

# ── 4. Copy onnxruntime.dll from Go module cache ───────────────────────────

$ortDest = Join-Path $root "onnxruntime.dll"

if (Test-Path $ortDest) {
    Ok "onnxruntime.dll already present."
} else {
    Step "Locating onnxruntime.dll in Go module cache"
    $goPath = go env GOPATH
    $ortPattern = Join-Path $goPath "pkg\mod\github.com\yalue\onnxruntime_go@*\test_data\onnxruntime.dll"
    $ortSrc = Get-Item $ortPattern -ErrorAction SilentlyContinue | Select-Object -Last 1
    if (-not $ortSrc) {
        # Try fetching it via go get so the cache is populated.
        Step "onnxruntime_go not yet in module cache, running go mod download"
        Push-Location $root
        go mod download
        Pop-Location
        $ortSrc = Get-Item $ortPattern -ErrorAction SilentlyContinue | Select-Object -Last 1
    }
    if (-not $ortSrc) { Fail "Cannot find onnxruntime.dll in Go module cache. Run 'go mod download' manually." }
    Copy-Item $ortSrc.FullName $ortDest
    Ok "Copied $($ortSrc.FullName)"
}

# ── 5. Build Go binary ─────────────────────────────────────────────────────

Step "Building natural-syntax-ls.exe"
Push-Location $root
go build -o natural-syntax-ls.exe .
if ($LASTEXITCODE -ne 0) { Fail "go build failed." }
Pop-Location
Ok "Built: $(Join-Path $root 'natural-syntax-ls.exe')"

# ── Done ───────────────────────────────────────────────────────────────────

Write-Host ""
Write-Host "Done! Set naturalSyntaxLs.serverPath in VS Code to:" -ForegroundColor Yellow
Write-Host "  $root\natural-syntax-ls.exe" -ForegroundColor White
