// anywhere-daemon — indexes the filesystem and answers search queries from the
// anywhere TUI over a Unix domain socket.
//
// Usage:
//
//	anywhere-daemon [--socket PATH] [--roots DIR,...] [--exclude DIR,...]
//
// Defaults:
//
//	--socket   $XDG_RUNTIME_DIR/anywhere.sock  (or ~/.local/share/anywhere/anywhere.sock)
//	--roots    $HOME
//	--exclude  /proc,/sys,/dev,/run,/tmp
//
// The daemon indexes the roots at startup (takes a few seconds for large trees),
// then keeps the index current via inotify.  The TUI connects automatically when
// the daemon is running; otherwise it falls back to plocate/locate.
//
// To raise the inotify watch limit (needed for large trees):
//
//	echo 1048576 | sudo tee /proc/sys/fs/inotify/max_user_watches
//	# persist across reboots:
//	echo 'fs.inotify.max_user_watches = 1048576' | sudo tee /etc/sysctl.d/50-inotify.conf
//
// To run as a systemd user service, create
// ~/.config/systemd/user/anywhere-daemon.service:
//
//	[Unit]
//	Description=anywhere file-index daemon
//	After=default.target
//
//	[Service]
//	ExecStart=/usr/local/bin/anywhere-daemon
//	Restart=on-failure
//
//	[Install]
//	WantedBy=default.target
//
// Then: systemctl --user enable --now anywhere-daemon
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

func main() {
	var (
		socketFlag  string
		rootsFlag   string
		excludeFlag string
	)
	flag.StringVar(&socketFlag, "socket", defaultSocketPath(), "Unix socket path")
	flag.StringVar(&rootsFlag, "roots", defaultRoots(), "Comma-separated directories to index")
	flag.StringVar(&excludeFlag, "exclude", "/proc,/sys,/dev,/run,/tmp", "Comma-separated directories to exclude")
	flag.Parse()

	roots := splitPaths(rootsFlag)
	skip := make(map[string]struct{})
	for _, p := range splitPaths(excludeFlag) {
		skip[p] = struct{}{}
	}

	log.SetFlags(log.Ltime)

	idx := newIndex()

	watcher, err := newWatcher(idx, skip)
	if err != nil {
		log.Fatalf("watcher: %v", err)
	}

	log.Printf("building index from %v...", roots)
	for _, root := range roots {
		watcher.WatchTree(root)
	}
	log.Printf("indexed %d paths", idx.Len())

	// Start processing filesystem events now that the initial walk is done.
	go watcher.Run()

	srv, err := newServer(socketFlag, idx)
	if err != nil {
		log.Fatalf("server: %v", err)
	}
	log.Printf("listening on %s", socketFlag)
	go srv.Run()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Println("shutting down")
	srv.Stop()
	watcher.Stop()
	os.Remove(socketFlag)
}

func defaultSocketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "anywhere.sock")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "anywhere", "anywhere.sock")
}

func defaultRoots() string {
	home, _ := os.UserHomeDir()
	return home
}

func splitPaths(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
