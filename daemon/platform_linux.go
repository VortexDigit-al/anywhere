//go:build linux

package main

import (
	"os"
	"path/filepath"
)

func defaultSocketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "anywhere.sock")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "anywhere", "anywhere.sock")
}

func defaultExcludeList() string {
	return "/proc,/sys,/dev,/run,/tmp"
}
