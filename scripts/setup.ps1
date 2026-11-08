#Requires -Version 5
<#
.SYNOPSIS
    Build natural-syntax-ls: export ONNX model, copy ORT runtime, compile binary.
.PARAMETER Model
    A model name from models.json (default bert-base), or all.
.PARAMETER List
    Print the models in models.json and exit.
#>
param(
    [string]$Model = "bert-base",
    [switch]$List
)

$ErrorActionPreference = "Stop"
$ROOT = Split-Path $PSScriptRoot -Parent

function Step($msg) { Write-Host; Write-Host "==> $msg" }
function Ok($msg)   { Write-Host "    $msg" }
function Fail($msg) { Write-Host "ERROR: $msg" -ForegroundColor Red; exit 1 }

$catalogPath = Join-Path $ROOT "data/models.json"
if (-not (Test-Path $catalogPath)) { Fail "models.json not found at $catalogPath" }
$CATALOG = (Get-Content $catalogPath -Raw -Encoding UTF8 | ConvertFrom-Json).models
if ($List) { $CATALOG | ForEach-Object { "{0,-30} {1,-10} {2}" -f $_.name, $_.mode, $_.description }; exit 0 }
if ($Model -ne "all" -and -not ($CATALOG | Where-Object { $_.name -eq $Model })) {
    Fail "Unknown model '$Model'. Use one of: $(($CATALOG | ForEach-Object { $_.name }) -join ' ') or all."
}
$SELECTED = @($CATALOG | Where-Object { $Model -eq "all" -or $_.name -eq $Model })

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
if ($SELECTED | Where-Object { $_.mode -eq "dependency" }) { $PKGS += "diaparser" }

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

# Export-Model skips the export when the files for the model already exist.
function Export-Model($m) {
    $slug = Get-Slug $m.name
    $hasFiles = switch ($m.mode) {
        "pos"        { (Test-Path (Join-Path $DATA_DIR "$slug.onnx")) -and (Test-Path (Join-Path $DATA_DIR "${slug}_labels.json")) -and (Test-Tokenizer $slug) }
        "semantic"   { (Test-Path (Join-Path $DATA_DIR "$slug.onnx")) -and (Test-Tokenizer $slug) }
        "dependency" { $slug = "${slug}_dependency"; (Test-Path (Join-Path $DATA_DIR "$slug.onnx")) -and (Test-Path (Join-Path $DATA_DIR "${slug}_rels.json")) -and (Test-Tokenizer $slug) }
        default      { Fail "Unknown mode '$($m.mode)' for $($m.name) in models.json" }
    }
    if ($hasFiles) { Ok "$slug.onnx already exists, skipping export."; return }
    $extra = if ($m.int8) { @("--int8") } else { @() }
    Invoke-Export $m.name "$($m.name) ($($m.mode): $($m.description))" $extra
}

foreach ($m in $SELECTED) { Export-Model $m }

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

# Language scripts: copied per file, never overwriting a user-edited one.
Step "Installing language scripts"
$langDest = Join-Path $DATA_DIR "lang"
New-Item -ItemType Directory -Force $langDest | Out-Null
Get-ChildItem (Join-Path $ROOT "data/lang") -Filter *.lua | ForEach-Object {
    $dest = Join-Path $langDest $_.Name
    if (-not (Test-Path $dest)) { Copy-Item $_.FullName $dest }
}
Ok "Language scripts: $langDest (add entries in $(Join-Path $DATA_DIR "models.json") to extend the bundled catalog)"

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
