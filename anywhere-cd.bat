@echo off
:: anywhere-cd.bat — CMD wrapper for anywhere that enables cd-on-exit.
::
:: Usage
::   anywhere-cd         run anywhere; Ctrl+D changes your current directory
::
:: Shell integration (add a doskey macro so it works from anywhere):
::   doskey any="%~dp0anywhere-cd.bat" $*
::   (put that doskey line in your AutoRun registry key to persist it)
::
:: How it works
:: anywhere.exe is launched with --cd-file pointing to a temp file.
:: When you press Ctrl+D on a result, anywhere writes that directory to the
:: file and exits.  This script reads the file and does cd /d.

set "_cdtmp=%TEMP%\anywhere_cd_%RANDOM%.tmp"
"%~dp0anywhere.exe" --cd-file "%_cdtmp%" %*
if exist "%_cdtmp%" (
    set /p "_cddir="<"%_cdtmp%"
    del /f /q "%_cdtmp%" 2>nul
    if defined _cddir (
        if exist "%_cddir%\" cd /d "%_cddir%"
    )
)
set "_cdtmp="
set "_cddir="
