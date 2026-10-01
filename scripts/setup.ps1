#Requires -Version 5
<#
.SYNOPSIS
    Build natural-syntax-ls: export ONNX model, copy ORT runtime, compile binary.
.PARAMETER Model
    Which model to export: bert-base (default), mobilebert, minilm, mpnet, dependency, or all.
#>
param(
    [ValidateSet("bert-base","mobilebert","minilm","mpnet","dependency","all")]
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

$PKGS = @("transformers","torch","onnx","onnxscript","requests")
if ($Model -eq "dependency" -or $Model -eq "all") { $PKGS += "diaparser" }

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

function Export-POSModel($modelName, $label) {
    $slug = $modelName -replace "-", "_"
    $o = Join-Path $DATA_DIR "$slug.onnx"
    $v = Join-Path $DATA_DIR "${slug}_vocab.txt"
    $l = Join-Path $DATA_DIR "${slug}_labels.json"
    if ((Test-Path $o) -and (Test-Path $v) -and (Test-Path $l)) {
        Ok "$slug.onnx already exists, skipping export."
    } else {
        Step "Exporting $label to ONNX"
        & $PYTHON (Join-Path $ROOT "scripts\export_model.py") $DATA_DIR --model $modelName
        if ($LASTEXITCODE -ne 0) { Fail "Export failed for $label." }
        Ok "$label exported."
    }
}

function Export-SemanticModel($modelName, $label) {
    $slug = $modelName -replace "-", "_"
    $o = Join-Path $DATA_DIR "$slug.onnx"
    $v = Join-Path $DATA_DIR "${slug}_vocab.txt"
    if ((Test-Path $o) -and (Test-Path $v)) {
        Ok "$slug.onnx already exists, skipping export."
    } else {
        Step "Exporting $label to ONNX"
        & $PYTHON (Join-Path $ROOT "scripts\export_model.py") $DATA_DIR --model $modelName
        if ($LASTEXITCODE -ne 0) { Fail "Export failed for $label." }
        Ok "$label exported."
    }
}

function Export-DependencyModel {
    Step "Exporting en_ewt.electra-base dependency parser to ONNX"
    & $PYTHON (Join-Path $ROOT "scripts\export_model.py") $DATA_DIR --model en_ewt.electra-base
    if ($LASTEXITCODE -ne 0) { Fail "Dependency model export failed." }
    Ok "Dependency model exported."
}

switch ($Model) {
    "bert-base"  { Export-POSModel "bert-base"  "BERT-base POS (~400 MB)" }
    "mobilebert" { Export-POSModel "mobilebert" "MobileBERT POS (~100 MB)" }
    "minilm"     { Export-SemanticModel "minilm" "all-MiniLM-L6-v2 (~22 MB)" }
    "mpnet"      { Export-SemanticModel "mpnet"  "all-mpnet-base-v2 (~110 MB)" }
    "dependency" { Export-DependencyModel }
    "all"        {
        Export-POSModel "bert-base"  "BERT-base POS (~400 MB)"
        Export-POSModel "mobilebert" "MobileBERT POS (~100 MB)"
        Export-SemanticModel "minilm" "all-MiniLM-L6-v2 (~22 MB)"
        Export-SemanticModel "mpnet"  "all-mpnet-base-v2 (~110 MB)"
        Export-DependencyModel
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
