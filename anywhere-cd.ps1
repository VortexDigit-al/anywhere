# anywhere-cd.ps1 — PowerShell wrapper for anywhere that enables cd-on-exit.
#
# Usage
# -----
#   .\anywhere-cd.ps1          # run anywhere; Ctrl+D changes your current directory
#
# Shell integration (add to $PROFILE so it works from anywhere)
# -------------------------------------------------------------
#   function any {
#       & "C:\path\to\anywhere-cd.ps1" @args
#   }
#
# How it works
# ------------
# anywhere.exe is launched with --cd-file pointing to a temp file.
# When you press Ctrl+D on a result, anywhere writes that directory to the file
# and exits.  This script reads the file and calls Set-Location.
# Without the wrapper, Ctrl+D opens a new terminal window at the directory
# instead — useful, but can't change *this* shell's directory.

param([Parameter(ValueFromRemainingArguments)] $passThru)

$tmp = [System.IO.Path]::GetTempFileName()
try {
    $anywhere = Join-Path $PSScriptRoot "anywhere.exe"
    & $anywhere --cd-file $tmp @passThru
    if (Test-Path $tmp) {
        $dir = (Get-Content $tmp -Raw -ErrorAction SilentlyContinue)?.Trim()
        if ($dir -and (Test-Path $dir -PathType Container)) {
            Set-Location $dir
        }
    }
} finally {
    if (Test-Path $tmp) {
        Remove-Item $tmp -Force -ErrorAction SilentlyContinue
    }
}
