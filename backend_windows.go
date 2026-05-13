//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	tea "github.com/charmbracelet/bubbletea"
)

// ── Everything SDK ────────────────────────────────────────────────────────────

type everythingSDK struct {
	mu            sync.Mutex
	setSearchW    *syscall.Proc
	setMax        *syscall.Proc
	queryW        *syscall.Proc
	getNumResults *syscall.Proc
	getResultPath *syscall.Proc
	getLastError  *syscall.Proc
}

func loadSDK() *everythingSDK {
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	candidates := []string{
		filepath.Join(exeDir, "Everything64.dll"),
		`C:\Program Files\Everything\Everything64.dll`,
		`C:\Program Files (x86)\Everything\Everything64.dll`,
		"Everything64.dll",
	}
	var dll *syscall.DLL
	for _, c := range candidates {
		if d, err := syscall.LoadDLL(c); err == nil {
			dll = d
			break
		}
	}
	if dll == nil {
		tmpPath := filepath.Join(os.TempDir(), "anywhere-Everything64.dll")
		if _, err := os.Stat(tmpPath); err != nil {
			_ = os.WriteFile(tmpPath, embeddedDLL, 0644)
		}
		if d, err := syscall.LoadDLL(tmpPath); err == nil {
			dll = d
		}
	}
	if dll == nil {
		return nil
	}
	sdk := &everythingSDK{}
	procs := map[string]**syscall.Proc{
		"Everything_SetSearchW":             &sdk.setSearchW,
		"Everything_SetMax":                 &sdk.setMax,
		"Everything_QueryW":                 &sdk.queryW,
		"Everything_GetNumResults":          &sdk.getNumResults,
		"Everything_GetResultFullPathNameW": &sdk.getResultPath,
		"Everything_GetLastError":           &sdk.getLastError,
	}
	for name, ptr := range procs {
		p, err := dll.FindProc(name)
		if err != nil {
			return nil
		}
		*ptr = p
	}
	return sdk
}

func (e *everythingSDK) Search(query string, max int) ([]fileItem, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.setMax.Call(uintptr(max))
	wq, err := syscall.UTF16PtrFromString(query)
	if err != nil {
		return nil, err
	}
	e.setSearchW.Call(uintptr(unsafe.Pointer(wq)))
	r, _, _ := e.queryW.Call(1)
	if r == 0 {
		code, _, _ := e.getLastError.Call()
		if code == 2 {
			return nil, fmt.Errorf("Everything is not running")
		}
		return nil, fmt.Errorf("Everything IPC error %d", code)
	}
	count, _, _ := e.getNumResults.Call()
	n := int(count)
	if n > max {
		n = max
	}
	buf := make([]uint16, 32768)
	items := make([]fileItem, 0, n)
	for i := 0; i < n; i++ {
		e.getResultPath.Call(uintptr(i), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		path := syscall.UTF16ToString(buf)
		if path == "" {
			continue
		}
		items = append(items, fileItem{path: path, name: filepath.Base(path), dir: filepath.Dir(path)})
	}
	return items, nil
}

// ── Platform init ─────────────────────────────────────────────────────────────

var globalSDK *everythingSDK

func platformInit() (backendFound, sdkActive bool) {
	globalSDK = loadSDK()
	if globalSDK != nil {
		return true, true
	}
	_, err := exec.LookPath("es")
	return err == nil, false
}

// ── Platform search ───────────────────────────────────────────────────────────

func platformSearch(ctx context.Context, query string, gen int) tea.Cmd {
	if globalSDK != nil {
		return func() tea.Msg {
			if strings.TrimSpace(query) == "" {
				return searchDoneMsg{query: query, gen: gen}
			}
			results, err := globalSDK.Search(strings.TrimSpace(query), maxResults)
			if err != nil {
				return errMsg{err}
			}
			return searchDoneMsg{results: results, query: query, gen: gen}
		}
	}
	return esSearch(ctx, query, gen)
}

func esSearch(ctx context.Context, query string, gen int) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-time.After(150 * time.Millisecond):
		case <-ctx.Done():
			return nil
		}
		q := strings.TrimSpace(query)
		if q == "" {
			return searchDoneMsg{query: query, gen: gen}
		}
		cmd := exec.CommandContext(ctx, "es", q)
		out, err := cmd.Output()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
				return errMsg{fmt.Errorf("%s", strings.TrimSpace(string(ee.Stderr)))}
			}
			return searchDoneMsg{query: query, gen: gen}
		}
		raw := strings.ReplaceAll(string(out), "\r\n", "\n")
		lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
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
		return searchDoneMsg{results: items, query: query, gen: gen}
	}
}

// ── Platform actions ──────────────────────────────────────────────────────────

func platformOpenItem(path string, isDir bool) {
	if isDir {
		exec.Command("explorer", path).Start() //nolint:errcheck
	} else {
		exec.Command("cmd", "/c", "start", "", path).Start() //nolint:errcheck
	}
}

func platformRevealItem(path string) {
	exec.Command("explorer", "/select,"+path).Start() //nolint:errcheck
}

func platformOpenTerminal(dir string) {
	if exec.Command("wt", "-d", dir).Start() != nil {
		exec.Command("cmd", "/c", "start", "cmd", "/k",
			fmt.Sprintf(`cd /d "%s"`, dir)).Start() //nolint:errcheck
	}
}

func platformBackendErrorMsg() string {
	return "Install Everything from voidtools.com"
}

func platformFastLabel() string {
	return "SDK ✓"
}
