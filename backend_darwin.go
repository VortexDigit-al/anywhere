//go:build darwin

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

// daemonAvailable is true when anywhere-daemon is reachable at startup.
var daemonAvailable bool

// ── Platform init ─────────────────────────────────────────────────────────────

func platformInit() (backendFound, sdkActive bool) {
	if probeDaemon() {
		daemonAvailable = true
		return true, true
	}
	if _, err := exec.LookPath("mdfind"); err == nil {
		return true, false
	}
	return false, false
}

func probeDaemon() bool {
	conn, err := net.DialTimeout("unix", daemonSocketPath(), 100*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func daemonSocketPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "anywhere", "anywhere.sock")
}

// ── Platform search ───────────────────────────────────────────────────────────

func platformSearch(ctx context.Context, query string, gen int) tea.Cmd {
	if daemonAvailable {
		return daemonSearch(query, gen)
	}
	return mdfindSearch(ctx, query, gen)
}

// daemonSearch sends one query to anywhere-daemon over the Unix socket.
func daemonSearch(query string, gen int) tea.Cmd {
	return func() tea.Msg {
		q := strings.TrimSpace(query)
		if q == "" {
			return searchDoneMsg{query: query, gen: gen}
		}

		conn, err := net.DialTimeout("unix", daemonSocketPath(), 200*time.Millisecond)
		if err != nil {
			// Daemon went away — fall back to mdfind for this query.
			return mdfindOnce(q, query, gen)
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

func mdfindSearch(ctx context.Context, query string, gen int) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-time.After(150 * time.Millisecond):
		case <-ctx.Done():
			return nil
		}
		return mdfindOnce(strings.TrimSpace(query), query, gen)
	}
}

func mdfindOnce(q, rawQuery string, gen int) tea.Msg {
	if q == "" {
		return searchDoneMsg{query: rawQuery, gen: gen}
	}
	out, err := exec.Command("mdfind", "-name", q).Output()
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
	exec.Command("open", path).Start() //nolint:errcheck
}

func platformRevealItem(path string) {
	exec.Command("open", "-R", path).Start() //nolint:errcheck
}

func platformOpenTerminal(dir string) {
	// Try iTerm2 first.
	itermScript := fmt.Sprintf(
		`tell application "iTerm2" to create window with default profile command "cd %s"`,
		shellQuote(dir),
	)
	if exec.Command("osascript", "-e", itermScript).Run() == nil {
		return
	}
	// Fall back to Terminal.app.
	termScript := fmt.Sprintf(
		`tell application "Terminal" to do script "cd %s" activate`,
		shellQuote(dir),
	)
	exec.Command("osascript", "-e", termScript).Start() //nolint:errcheck
}

// shellQuote wraps path in single quotes and escapes any embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func platformBackendErrorMsg() string {
	return "Start anywhere-daemon, or ensure Spotlight indexing is enabled"
}

func platformFastLabel() string {
	return "daemon ✓"
}
