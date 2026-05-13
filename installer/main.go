// anywhere installer — installs the anywhere TUI and its shell wrappers.
//
// Built by build.ps1, which copies the compiled anywhere.exe and scripts into
// the assets/ directory before running `go build` here.
//
// WinGet runs this with:   anywhere-installer.exe /S
// Manual uninstall runs:   anywhere-installer.exe /uninstall /S
package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// assets are populated by build.ps1 before this binary is compiled.
//
//go:embed assets
var assets embed.FS

// appVersion is stamped in by build.ps1 via -ldflags "-X main.appVersion=x.y.z"
var appVersion = "0.0.0"

const (
	appName     = "anywhere"
	displayName = "anywhere — Everything Search TUI"
	publisher   = "vortexdigital"

	// Install under the current user; no elevation required.
	installSubdir = `Programs\anywhere`

	// Marker lines written around the any block in the PS profile.
	profileBegin = "# --- anywhere begin ---"
	profileEnd   = "# --- anywhere end ---"

	// Registry key for Add/Remove Programs (current user).
	uninstallRegKey = `HKCU:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\anywhere`
)

// ── Entry point ───────────────────────────────────────────────────────────────

func main() {
	silent, uninstall := false, false
	for _, a := range os.Args[1:] {
		switch strings.ToLower(a) {
		case "/s", "-s", "--silent", "-silent":
			silent = true
		case "/uninstall", "-uninstall", "--uninstall":
			uninstall = true
		}
	}

	var err error
	if uninstall {
		err = doUninstall(silent)
	} else {
		err = doInstall(silent)
	}
	if err != nil {
		if !silent {
			fmt.Fprintf(os.Stderr, "\n✗ %v\n", err)
		}
		os.Exit(1)
	}
}

// ── Install ───────────────────────────────────────────────────────────────────

func doInstall(silent bool) error {
	dir := installDir()
	const n = 5

	header(silent, "installer", appVersion)
	if !silent {
		fmt.Printf("Installing to: %s\n\n", dir)
	}

	// 1 — Check / install Everything
	say(silent, 1, n, "Checking for Everything Search")
	if !everythingPresent() {
		inline(silent, " — not found, installing via winget")
		if err := installEverything(silent); err != nil {
			warn(silent, "Could not install Everything automatically: "+err.Error())
			warn(silent, "Install manually from https://voidtools.com and re-run this installer.")
			// Non-fatal: anywhere will show a helpful error on launch.
		}
	}
	ok(silent)

	// 2 — Extract files
	say(silent, 2, n, "Extracting files")
	if err := extractAssets(dir); err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	ok(silent)

	// 3 — PATH
	say(silent, 3, n, "Adding to user PATH")
	if err := addToPath(dir); err != nil {
		return fmt.Errorf("PATH: %w", err)
	}
	ok(silent)

	// 4 — PowerShell profile
	say(silent, 4, n, "Adding 'any' function to PowerShell profile")
	if err := setupProfile(dir); err != nil {
		warn(silent, "Profile setup: "+err.Error())
		// Non-fatal; user can run anywhere-cd.ps1 directly.
	} else {
		ok(silent)
	}

	// 5 — Uninstall registry entry
	say(silent, 5, n, "Registering uninstaller")
	if err := registerUninstaller(dir); err != nil {
		warn(silent, "Uninstall registry: "+err.Error())
	} else {
		ok(silent)
	}

	if !silent {
		fmt.Printf("\n✓ anywhere v%s installed successfully!\n\n", appVersion)
		fmt.Println("  Open a new terminal, then:")
		fmt.Println("    any         — launch anywhere (Ctrl+D changes your current directory)")
		fmt.Println("    anywhere   — launch standalone (no cd integration)")
		fmt.Println()
		fmt.Println("  To uninstall:")
		fmt.Println("    Settings > Apps > Installed apps > anywhere  (recommended)")
		fmt.Printf("    %s\\anywhere-installer.exe /uninstall\n", installDir())
	}
	return nil
}

// ── Uninstall ─────────────────────────────────────────────────────────────────

func doUninstall(silent bool) error {
	dir := installDir()

	header(silent, "uninstaller", appVersion)
	if !silent {
		fmt.Printf("Removing from: %s\n\n", dir)
	}

	// PATH
	_ = runPS(fmt.Sprintf(`
$d = '%s'
$old = [Environment]::GetEnvironmentVariable("Path","User")
$new = ($old -split ';' | Where-Object {$_ -and $_ -ne $d}) -join ';'
[Environment]::SetEnvironmentVariable("Path",$new,"User")
`, dir))

	// PowerShell profile block
	_ = runPS(fmt.Sprintf(`
$p = $PROFILE
if (Test-Path $p) {
    $c = Get-Content $p -Raw
    $c = $c -replace '(?s)\r?\n%s.*?%s', ''
    Set-Content $p $c.TrimEnd()
}
`, profileBegin, profileEnd))

	// Registry
	_ = runPS(fmt.Sprintf(`Remove-Item -Path '%s' -Force -ErrorAction SilentlyContinue`, uninstallRegKey))

	// Files
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove directory: %w", err)
	}

	if !silent {
		fmt.Println("\n✓ anywhere uninstalled.")
	}
	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func installDir() string {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		local = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
	}
	return filepath.Join(local, installSubdir)
}

// everythingPresent returns true if the Everything service appears to be
// installed (checks known paths and es.exe in PATH).
func everythingPresent() bool {
	if _, err := exec.LookPath("es"); err == nil {
		return true
	}
	knownPaths := []string{
		`C:\Program Files\Everything\Everything.exe`,
		`C:\Program Files (x86)\Everything\Everything.exe`,
	}
	for _, p := range knownPaths {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	// Check winget list
	out, err := exec.Command("winget", "list", "--id", "voidtools.Everything",
		"--exact", "--accept-source-agreements").Output()
	return err == nil && strings.Contains(string(out), "voidtools.Everything")
}

// installEverything installs voidtools.Everything and the CLI via winget.
func installEverything(silent bool) error {
	flags := []string{
		"install",
		"--accept-package-agreements",
		"--accept-source-agreements",
	}
	if silent {
		flags = append(flags, "--silent")
	}

	// Main application (search service + tray)
	cmd := exec.Command("winget", append(flags, "--id", "voidtools.Everything")...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("winget voidtools.Everything: %w", err)
	}

	// CLI tool (es.exe) — non-fatal if unavailable
	cmd2 := exec.Command("winget", append(flags, "--id", "voidtools.Everything.Cli")...)
	cmd2.Stdout, cmd2.Stderr = os.Stdout, os.Stderr
	_ = cmd2.Run()

	return nil
}

// extractAssets copies all embedded assets to dir, then copies the running
// installer binary itself into dir so the uninstaller entry works.
func extractAssets(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Embedded files (anywhere.exe, anywhere-cd.ps1, anywhere-cd.bat)
	if err := fs.WalkDir(assets, "assets", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() == ".gitkeep" {
			return err
		}
		data, err := assets.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		return os.WriteFile(filepath.Join(dir, d.Name()), data, 0755)
	}); err != nil {
		return err
	}

	// Copy the running installer itself so the uninstall registry entry works.
	if err := copyInstallerSelf(dir); err != nil {
		// Non-fatal: log it so we know what went wrong during testing/CI.
		fmt.Fprintf(os.Stderr, "warning: could not copy installer to install dir: %v\n", err)
		// Fallback: write a minimal PS1 uninstaller so the ARP entry still works.
		_ = writeUninstallScript(dir)
	}
	return nil
}

// copyInstallerSelf copies the running executable into dir as anywhere-installer.exe.
// This is necessary so the UninstallString in the ARP registry entry resolves.
func copyInstallerSelf(dir string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("os.Executable: %w", err)
	}
	// Resolve symlinks (some launchers wrap the exe).
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return fmt.Errorf("EvalSymlinks: %w", err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		return fmt.Errorf("read self %s: %w", self, err)
	}
	return os.WriteFile(filepath.Join(dir, "anywhere-installer.exe"), data, 0755)
}

// writeUninstallScript is a fallback used when copyInstallerSelf fails.
// It writes a small PowerShell script to dir that performs the same cleanup
// as doUninstall, and updates the UninstallString in the registry to point at it.
func writeUninstallScript(dir string) error {
	script := fmt.Sprintf(`# anywhere uninstall script — generated by installer
$d = '%s'
$old = [Environment]::GetEnvironmentVariable("Path","User")
$new = ($old -split ';' | Where-Object {$_ -and $_ -ne $d}) -join ';'
[Environment]::SetEnvironmentVariable("Path",$new,"User")
$p = $PROFILE
if (Test-Path $p) {
    $c = Get-Content $p -Raw
    $c = $c -replace '(?s)\r?\n%s.*?%s', ''
    Set-Content $p $c.TrimEnd()
}
Remove-Item -Path '%s' -Force -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force $d -ErrorAction SilentlyContinue
`, dir, profileBegin, profileEnd, uninstallRegKey)

	scriptPath := filepath.Join(dir, "uninstall.ps1")
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		return err
	}

	// Update UninstallString to use the PS script.
	ps, err := exec.LookPath("pwsh")
	if err != nil {
		ps, _ = exec.LookPath("powershell")
	}
	if ps == "" {
		return fmt.Errorf("PowerShell not found")
	}
	uninstallCmd := fmt.Sprintf(`"%s" -ExecutionPolicy Bypass -File "%s"`, ps, scriptPath)
	return runPS(fmt.Sprintf(`Set-ItemProperty -Path '%s' -Name UninstallString -Value '%s'`,
		uninstallRegKey, strings.ReplaceAll(uninstallCmd, "'", "''")))
}

// addToPath appends dir to the current user's PATH if not already present.
func addToPath(dir string) error {
	return runPS(fmt.Sprintf(`
$d = '%s'
$old = [Environment]::GetEnvironmentVariable("Path","User")
if (($old -split ';') -notcontains $d) {
    [Environment]::SetEnvironmentVariable("Path","$old;$d","User")
}
`, dir))
}

// setupProfile adds an `any` wrapper function to the PowerShell profile.
func setupProfile(dir string) error {
	block := fmt.Sprintf(`
%s
function any {
    & '%s\anywhere-cd.ps1' @args
}
%s`, profileBegin, dir, profileEnd)

	// Escape single quotes in the block for embedding in the PS heredoc.
	escaped := strings.ReplaceAll(block, "'", "''")

	return runPS(fmt.Sprintf(`
$p = $PROFILE
if (-not (Test-Path (Split-Path $p))) {
    New-Item -ItemType Directory -Path (Split-Path $p) -Force | Out-Null
}
if (-not (Test-Path $p)) {
    New-Item -ItemType File -Path $p -Force | Out-Null
}
$c = Get-Content $p -Raw -ErrorAction SilentlyContinue
if ($c -notlike '*%s*') {
    Add-Content $p '%s'
}
`, profileBegin, escaped))
}

// registerUninstaller writes the standard Add/Remove Programs registry entry.
func registerUninstaller(dir string) error {
	exe := filepath.Join(dir, "anywhere-installer.exe")
	return runPS(fmt.Sprintf(`
$k = '%s'
New-Item -Path $k -Force | Out-Null
@{
    DisplayName            = '%s'
    DisplayVersion         = '%s'
    Publisher              = '%s'
    InstallLocation        = '%s'
    UninstallString        = '"%s" /uninstall'
    NoModify               = 1
    NoRepair               = 1
    WinGetPackageIdentifier = 'VortexDigital.Anywhere'
    WinGetSourceIdentifier  = 'wingetty'
}.GetEnumerator() | ForEach-Object {
    Set-ItemProperty -Path $k -Name $_.Key -Value $_.Value
}
`, uninstallRegKey, displayName, appVersion, publisher, dir, exe))
}

// runPS executes a PowerShell script block, preferring pwsh (v7) over
// Windows PowerShell 5.
func runPS(script string) error {
	ps, err := exec.LookPath("pwsh")
	if err != nil {
		ps, err = exec.LookPath("powershell")
		if err != nil {
			return fmt.Errorf("PowerShell not found")
		}
	}
	cmd := exec.Command(ps, "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ── Console output ────────────────────────────────────────────────────────────

func header(silent bool, mode, ver string) {
	if silent {
		return
	}
	fmt.Printf("\n⚡ anywhere %s v%s\n", mode, ver)
	fmt.Println(strings.Repeat("─", 50))
}

func say(silent bool, i, total int, msg string) {
	if !silent {
		fmt.Printf("[%d/%d] %s... ", i, total, msg)
	}
}

func inline(silent bool, msg string) {
	if !silent {
		fmt.Print(msg)
	}
}

func ok(silent bool) {
	if !silent {
		fmt.Println("done")
	}
}

func warn(silent bool, msg string) {
	if !silent {
		fmt.Printf("\n      ⚠  %s\n      ", msg)
	}
}
