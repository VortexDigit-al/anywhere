//go:build linux

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// locateCmd is "plocate" or "locate", set by platformInit.
var locateCmd string

// daemonAvailable is true when anywhere-daemon is reachable at startup.
var daemonAvailable bool

// ── Platform init ─────────────────────────────────────────────────────────────

func platformInit() (backendFound, sdkActive bool) {
	if probeDaemon() {
		daemonAvailable = true
		return true, true
	}
	for _, cmd := range []string{"plocate", "locate"} {
		if _, err := exec.LookPath(cmd); err == nil {
			locateCmd = cmd
			return true, false
		}
	}
	return false, false
}

// probeDaemon tries a quick connection to the daemon socket to see if it's up.
func probeDaemon() bool {
	conn, err := net.DialTimeout("unix", daemonSocketPath(), 100*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func daemonSocketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "anywhere.sock")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "anywhere", "anywhere.sock")
}

// ── Platform search ───────────────────────────────────────────────────────────

func platformSearch(ctx context.Context, query string, gen int) tea.Cmd {
	if daemonAvailable {
		return daemonSearch(query, gen)
	}
	return locateSearch(ctx, query, gen)
}

// daemonSearch sends one query to anywhere-daemon over the Unix socket and
// returns results immediately.  No debounce needed — the daemon answers in RAM.
func daemonSearch(query string, gen int) tea.Cmd {
	return func() tea.Msg {
		q := strings.TrimSpace(query)
		if q == "" {
			return searchDoneMsg{query: query, gen: gen}
		}

		conn, err := net.DialTimeout("unix", daemonSocketPath(), 200*time.Millisecond)
		if err != nil {
			// Daemon went away — fall back to locate for this query.
			return locateOnce(q, query, gen)
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(2 * time.Second))

		req := struct {
			Q   string `json:"q"`
			Max int    `json:"max"`
		}{Q: q, Max: maxResults}
		if err := json.NewEncoder(conn).Encode(req); err != nil {
			return errMsg{fmt.Errorf("daemon write: %w", err)}
		}

		var resp struct {
			Results []string `json:"results"`
		}
		if err := json.NewDecoder(conn).Decode(&resp); err != nil {
			return errMsg{fmt.Errorf("daemon read: %w", err)}
		}

		items := make([]fileItem, 0, len(resp.Results))
		for _, p := range resp.Results {
			items = append(items, fileItem{path: p, name: filepath.Base(p), dir: filepath.Dir(p)})
		}
		return searchDoneMsg{results: items, query: query, gen: gen}
	}
}

func locateSearch(ctx context.Context, query string, gen int) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-time.After(150 * time.Millisecond):
		case <-ctx.Done():
			return nil
		}
		return locateOnce(strings.TrimSpace(query), query, gen)
	}
}

func locateOnce(q, rawQuery string, gen int) tea.Msg {
	if q == "" {
		return searchDoneMsg{query: rawQuery, gen: gen}
	}
	if locateCmd == "" {
		return searchDoneMsg{query: rawQuery, gen: gen}
	}
	out, err := exec.Command(locateCmd, "-i", "--", q).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return errMsg{fmt.Errorf("%s", strings.TrimSpace(string(ee.Stderr)))}
		}
		return searchDoneMsg{query: rawQuery, gen: gen}
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	items := make([]fileItem, 0, len(lines))
	for _, l := range lines {
		if l == "" {
			continue
		}
		items = append(items, fileItem{path: l, name: filepath.Base(l), dir: filepath.Dir(l)})
		if len(items) >= maxResults {
			break
		}
	}
	return searchDoneMsg{results: items, query: rawQuery, gen: gen}
}

// ── Platform actions ──────────────────────────────────────────────────────────

func platformOpenItem(path string, _ bool) {
	exec.Command("xdg-open", path).Start() //nolint:errcheck
}

func platformRevealItem(path string) {
	// Try nautilus --select (GNOME Files); fall back to opening the parent dir.
	if exec.Command("nautilus", "--select", path).Start() != nil {
		exec.Command("xdg-open", filepath.Dir(path)).Start() //nolint:errcheck
	}
}

func platformOpenTerminal(dir string) {
	shCmd := fmt.Sprintf(`cd %q && exec "${SHELL:-bash}"`, dir)

	type entry struct {
		cmd  string
		args []string
	}
	attempts := []entry{
		{"gnome-terminal", []string{"--working-directory=" + dir}},
		{"xfce4-terminal", []string{"--working-directory=" + dir}},
		{"konsole", []string{"--workdir", dir}},
		{"kitty", []string{"--directory", dir}},
		{"alacritty", []string{"--working-directory", dir}},
		{"wezterm", []string{"start", "--cwd", dir}},
		{"foot", []string{"--working-directory=" + dir}},
		{"lxterminal", []string{"--working-directory=" + dir}},
		{"tilix", []string{"--working-directory=" + dir}},
		{"xterm", []string{"-e", "bash", "-c", shCmd}},
		{"urxvt", []string{"-e", "bash", "-c", shCmd}},
		{"st", []string{"-e", "bash", "-c", shCmd}},
	}
	if term := os.Getenv("TERMINAL"); term != "" {
		attempts = append([]entry{{term, nil}}, attempts...)
	}
	for _, a := range attempts {
		if exec.Command(a.cmd, a.args...).Start() == nil {
			return
		}
	}
}

func platformBackendErrorMsg() string {
	return "Start anywhere-daemon, or install plocate: sudo apt install plocate"
}

func platformFastLabel() string {
	return "daemon ✓"
}
