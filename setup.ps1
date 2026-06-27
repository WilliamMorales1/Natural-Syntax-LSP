#Requires -Version 5
<#
.SYNOPSIS
    Wrapper — delegates to setup.sh via Git Bash.
.PARAMETER Model
    Which POS model to export: mobilebert (default), bert-base, or all.
#>
param(
    [ValidateSet("mobilebert","bert-base","all")]
    [string]$Model = "mobilebert"
)

$sh = Join-Path $PSScriptRoot "setup.sh"
$bash = (Get-Command bash -ErrorAction SilentlyContinue)?.Source
if (-not $bash) { $bash = "C:\Program Files\Git\bin\bash.exe" }
if (-not (Test-Path $bash)) {
    Write-Error "bash not found. Install Git for Windows or add bash to PATH."
    exit 1
}

& $bash $sh --model $Model
exit $LASTEXITCODE
