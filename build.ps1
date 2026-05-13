#Requires -Version 5.1
# build.ps1 — build anywhere and produce a WinGet-ready installer EXE.
#
# Usage:
#   .\build.ps1                   # version = "0.0.0-dev"
#   .\build.ps1 -Version 1.2.3   # stamp a release version
#
# Outputs:
#   dist\anywhere-installer.exe      (WinGet installer)
#   dist\anywhere-portable.zip       (manual install fallback)

param(
    [string]$Version = "0.0.0-dev"
)

$ErrorActionPreference = "Stop"
$root    = $PSScriptRoot
$assets  = Join-Path $root "installer\assets"
$dist    = Join-Path $root "dist"

function Step([string]$msg) { Write-Host "  $msg..." -NoNewline -ForegroundColor Cyan }
function Done { Write-Host " done" -ForegroundColor Green }
function Fail([string]$msg) { Write-Host " FAILED" -ForegroundColor Red; throw $msg }

Write-Host ""
Write-Host "⚡ anywhere build  v$Version" -ForegroundColor Yellow
Write-Host ("─" * 50)

# Create output directories
New-Item -ItemType Directory -Force -Path $assets, $dist | Out-Null

# ── 1. Build main TUI binary ──────────────────────────────────────────────────
Step "Building anywhere.exe"
$anywhereOut = Join-Path $assets "anywhere.exe"
& go build -trimpath -buildvcs=false `
           -ldflags "-s -w -X main.version=$Version" `
           -o $anywhereOut `
           $root
if ($LASTEXITCODE -ne 0) { Fail "go build (anywhere) failed" }
Done

# ── 2. Copy wrapper scripts ───────────────────────────────────────────────────
Step "Copying shell wrappers"
Copy-Item (Join-Path $root "anywhere-cd.ps1") $assets -Force
Copy-Item (Join-Path $root "anywhere-cd.bat") $assets -Force
Done

# ── 3. Build installer EXE ───────────────────────────────────────────────────
Step "Building anywhere-installer.exe"
$installerOut = Join-Path $dist "anywhere-installer.exe"
Push-Location (Join-Path $root "installer")
try {
    & go build -trimpath -buildvcs=false `
               -ldflags "-s -w -X main.appVersion=$Version" `
               -o $installerOut `
               .
    if ($LASTEXITCODE -ne 0) { Fail "go build (installer) failed" }
} finally {
    Pop-Location
}
Done

# ── 4. Create portable zip ───────────────────────────────────────────────────
Step "Creating portable zip"
$zipOut = Join-Path $dist "anywhere-$Version-windows-amd64.zip"
$zipItems = @(
    (Join-Path $assets "anywhere.exe"),
    (Join-Path $root   "anywhere-cd.ps1"),
    (Join-Path $root   "anywhere-cd.bat")
)
Compress-Archive -Path $zipItems -DestinationPath $zipOut -Force
Done

# ── Summary ───────────────────────────────────────────────────────────────────
$sha256 = (Get-FileHash $installerOut -Algorithm SHA256).Hash.ToLower()
$size   = [math]::Round((Get-Item $installerOut).Length / 1MB, 1)

Write-Host ""
Write-Host "✓ Build complete" -ForegroundColor Green
Write-Host ""
Write-Host "  Installer : dist\anywhere-installer.exe  ($size MB)"
Write-Host "  Portable  : dist\anywhere-$Version-windows-amd64.zip"
Write-Host ""
Write-Host "  SHA256 (installer): $sha256" -ForegroundColor DarkCyan
Write-Host ""
Write-Host "  Update manifests\t\vortexdigital\anywhere\$Version\vortexdigital.anywhere.installer.yaml"
Write-Host "  with the SHA256 above and the GitHub release URL before publishing." -ForegroundColor DarkYellow
