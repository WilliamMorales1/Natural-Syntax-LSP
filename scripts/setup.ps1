#Requires -Version 5
<#
.SYNOPSIS
    Build natural-syntax-ls: export ONNX model, copy ORT runtime, compile binary.
.PARAMETER Model
    Which model to export: bert-base (default), mobilebert, pos-es, pos-fr, pos-ru, pos-ar, multilingual, dependency, dependency-es, dependency-fr, dependency-ru, dependency-ar, or all.
#>
param(
    [ValidateSet("bert-base","mobilebert","pos-es","pos-fr","pos-ru","pos-ar","multilingual","dependency","dependency-es","dependency-fr","dependency-ru","dependency-ar","all")]
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
#
# pip first, into a venv if a plain install fails (e.g. PEP 668 under pwsh on
# Linux). Without pip, fall back to uv, then conda, each into an env under the repo.

Step "Installing Python dependencies (transformers, torch, onnx, onnxscript, requests)"

$PKGS = @("transformers","torch","onnx","onnxscript","onnxruntime","requests")
if ($Model -like "dependency*" -or $Model -eq "all") { $PKGS += "diaparser" }

# Native stderr can throw under ErrorActionPreference=Stop in Windows PowerShell 5, so probes go through try/catch.
function Test-Native([scriptblock]$cmd) {
    try { & $cmd *> $null; return ($LASTEXITCODE -eq 0) } catch { return $false }
}

# EnvPython returns the interpreter path inside a venv/conda env, which differs between Windows and Unix pwsh.
function EnvPython($dir) {
    $win = Join-Path $dir "Scripts\python.exe"
    if (Test-Path $win) { return $win }
    $unixVenv = Join-Path $dir "bin/python"
    if (Test-Path $unixVenv) { return $unixVenv }
    return (Join-Path $dir "python.exe") # conda on Windows puts python.exe at the env root
}

function Install-Pip {
    & $PYTHON -m pip install --quiet @PKGS
    return ($LASTEXITCODE -eq 0)
}

if (Test-Native { & $PYTHON -m pip --version }) {
    if (-not (Install-Pip)) {
        Step "System pip install failed, falling back to a venv"
        $VENV_DIR = Join-Path $ROOT ".venv"
        if (-not (Test-Path $VENV_DIR)) { & $PYTHON -m venv $VENV_DIR }
        $PYTHON = EnvPython $VENV_DIR
        Ok "Using venv: $VENV_DIR"
        if (-not (Test-Native { & $PYTHON -m pip --version })) { & $PYTHON -m ensurepip --upgrade }
        if (-not (Install-Pip)) { Fail "Failed to install Python dependencies even inside a venv." }
    }
} elseif (Get-Command uv -ErrorAction SilentlyContinue) {
    Step "pip not found, using uv"
    $VENV_DIR = Join-Path $ROOT ".venv"
    if (-not (Test-Path $VENV_DIR)) { uv venv --quiet --python $PYTHON $VENV_DIR }
    $PYTHON = EnvPython $VENV_DIR
    Ok "Using venv: $VENV_DIR"
    uv pip install --quiet --python $PYTHON @PKGS
    if ($LASTEXITCODE -ne 0) { Fail "Failed to install Python dependencies with uv." }
} elseif (Get-Command conda -ErrorAction SilentlyContinue) {
    Step "pip and uv not found, using conda"
    $CONDA_ENV = Join-Path $ROOT ".conda-env"
    if (-not (Test-Path $CONDA_ENV)) {
        conda create --quiet --yes --prefix $CONDA_ENV python=3.12 pip
        if ($LASTEXITCODE -ne 0) { Fail "Failed to create conda env at $CONDA_ENV." }
    }
    $PYTHON = EnvPython $CONDA_ENV
    Ok "Using conda env: $CONDA_ENV"
    if (-not (Install-Pip)) { Fail "Failed to install Python dependencies inside the conda env." }
} else {
    Fail "No pip, uv, or conda found. Install one of them and re-run."
}
Ok "Python deps ready."

# ── 4. Export ONNX model(s) + vocab + labels ──────────────────────────────────

function Get-Slug($name) { return ($name -replace "[-./]", "_") }

# SentencePiece models ship _tokenizer.json instead of _vocab.txt.
function Test-Tokenizer($slug) {
    return (Test-Path (Join-Path $DATA_DIR "${slug}_vocab.txt")) -or (Test-Path (Join-Path $DATA_DIR "${slug}_tokenizer.json"))
}

function Invoke-Export($modelName, $label, $extra) {
    Step "Exporting $label to ONNX"
    & $PYTHON (Join-Path $ROOT "scripts\export_model.py") $DATA_DIR --model $modelName @extra
    if ($LASTEXITCODE -ne 0) { Fail "Export failed for $label." }
    Ok "$label exported."
}

function Export-POSModel($modelName, $label, $extra = @()) {
    $slug = Get-Slug $modelName
    if ((Test-Path (Join-Path $DATA_DIR "$slug.onnx")) -and (Test-Path (Join-Path $DATA_DIR "${slug}_labels.json")) -and (Test-Tokenizer $slug)) {
        Ok "$slug.onnx already exists, skipping export."
    } else {
        Invoke-Export $modelName $label $extra
    }
}

function Export-SemanticModel($modelName, $label, $extra = @()) {
    $slug = Get-Slug $modelName
    if ((Test-Path (Join-Path $DATA_DIR "$slug.onnx")) -and (Test-Tokenizer $slug)) {
        Ok "$slug.onnx already exists, skipping export."
    } else {
        Invoke-Export $modelName $label $extra
    }
}

function Export-DependencyModel($modelName, $label, $extra = @()) {
    $slug = (Get-Slug $modelName) + "_dependency"
    if ((Test-Path (Join-Path $DATA_DIR "$slug.onnx")) -and (Test-Path (Join-Path $DATA_DIR "${slug}_rels.json")) -and (Test-Tokenizer $slug)) {
        Ok "$slug.onnx already exists, skipping export."
    } else {
        Invoke-Export $modelName "$label dependency parser" $extra
    }
}

# The Spanish parser stays fp32: int8 drops its UAS from 91 to 42. Everything else is int8.
function Export-POSEs  { Export-POSModel "xlmr-es" "Spanish POS (XLM-R, int8, ~280 MB)" @("--int8") }
function Export-POSFr  { Export-POSModel "xlmr-fr" "French POS (XLM-R, int8, ~280 MB)" @("--int8") }
function Export-POSRu  { Export-POSModel "xlmr-ru" "Russian POS (XLM-R, int8, ~280 MB)" @("--int8") }
function Export-POSAr  { Export-POSModel "xlmr-ar" "Arabic POS (XLM-R, int8, ~280 MB)" @("--int8") }
function Export-Multilingual { Export-SemanticModel "multilingual-minilm" "multilingual MiniLM semantic (~470 MB)" }
function Export-DepEn  { Export-DependencyModel "en_ewt.electra-base" "English (~470 MB)" }
function Export-DepEs  { Export-DependencyModel "es_ancora.mbert" "Spanish (fp32, ~750 MB)" }
function Export-DepFr  { Export-DependencyModel "fr_sequoia.camembert-large" "French (int8, ~350 MB)" @("--int8") }
function Export-DepRu  { Export-DependencyModel "ru_syntagrus.DeepPavlov" "Russian (fp32, ~750 MB)" }
function Export-DepAr  { Export-DependencyModel "ar_padt.asafaya" "Arabic (int8, ~330 MB)" @("--int8") }

switch ($Model) {
    "bert-base"     { Export-POSModel "bert-base"  "BERT-base POS (~400 MB)" }
    "mobilebert"    { Export-POSModel "mobilebert" "MobileBERT POS (~100 MB)" }
    "pos-es"        { Export-POSEs }
    "pos-fr"        { Export-POSFr }
    "pos-ru"        { Export-POSRu }
    "pos-ar"        { Export-POSAr }
    "multilingual"  { Export-Multilingual }
    "dependency"    { Export-DepEn }
    "dependency-es" { Export-DepEs }
    "dependency-fr" { Export-DepFr }
    "dependency-ru" { Export-DepRu }
    "dependency-ar" { Export-DepAr }
    "all"           {
        Export-POSModel "bert-base"  "BERT-base POS (~400 MB)"
        Export-POSModel "mobilebert" "MobileBERT POS (~100 MB)"
        Export-POSEs
        Export-POSFr
        Export-POSRu
        Export-POSAr
        Export-Multilingual
        Export-DepEn
        Export-DepEs
        Export-DepFr
        Export-DepRu
        Export-DepAr
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
$exePath = Join-Path $ROOT "bin\natural-syntax-ls.exe"
Remove-Item $exePath,"${exePath}~" -ErrorAction SilentlyContinue
Push-Location $ROOT
go build -o $exePath ./cmd/natural-syntax-ls/
if ($LASTEXITCODE -ne 0) { Fail "go build failed." }
Pop-Location
Ok "Built: $exePath"

# ── 7. Install binary to user bin directory ───────────────────────────────────

$installDir = Join-Path $env:LOCALAPPDATA "Programs\natural-syntax-ls"
Step "Installing to $installDir"
New-Item -ItemType Directory -Force -Path $installDir | Out-Null
$installedExe = Join-Path $installDir "natural-syntax-ls.exe"
Copy-Item $exePath $installedExe -Force
Ok "Installed: $installedExe"
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (($userPath -split ";") -notcontains $installDir) {
    [Environment]::SetEnvironmentVariable("Path", ($userPath.TrimEnd(";") + ";" + $installDir).TrimStart(";"), "User")
    Ok "Added $installDir to user PATH (restart your terminal/editor to pick it up)."
}

# ── Done ──────────────────────────────────────────────────────────────────────

Write-Host
Write-Host "Done!"
Write-Host
Write-Host "Server binary: $installedExe"
Write-Host "(Set naturalSyntaxLs.serverPath in VS Code to this to use it instead of the bundled one.)"
Write-Host
Write-Host "Model files are in: $DATA_DIR"
Write-Host "The extension finds them automatically."
