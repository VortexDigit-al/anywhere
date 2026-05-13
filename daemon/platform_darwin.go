//go:build darwin

// anywhere-daemon on macOS uses FSEvents (via fsnotify/kqueue) to monitor the
// filesystem and answers search queries over a Unix domain socket.
//
// Default socket:  ~/Library/Application Support/anywhere/anywhere.sock
// Default roots:   $HOME
// Default exclude: /System,/Volumes,/private/var,/private/tmp,/cores
//
// ── Running as a launchd user agent ──────────────────────────────────────────
//
// Create ~/Library/LaunchAgents/com.anywhere.daemon.plist:
//
//	<?xml version="1.0" encoding="UTF-8"?>
//	<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
//	  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
//	<plist version="1.0">
//	<dict>
//	  <key>Label</key>
//	  <string>com.anywhere.daemon</string>
//	  <key>ProgramArguments</key>
//	  <array>
//	    <string>/usr/local/bin/anywhere-daemon</string>
//	  </array>
//	  <key>RunAtLoad</key>
//	  <true/>
//	  <key>KeepAlive</key>
//	  <true/>
//	  <key>SoftResourceLimits</key>
//	  <dict>
//	    <key>NumberOfFiles</key>
//	    <integer>65536</integer>
//	  </dict>
//	  <key>HardResourceLimits</key>
//	  <dict>
//	    <key>NumberOfFiles</key>
//	    <integer>65536</integer>
//	  </dict>
//	  <key>StandardOutPath</key>
//	  <string>/tmp/anywhere-daemon.log</string>
//	  <key>StandardErrorPath</key>
//	  <string>/tmp/anywhere-daemon.log</string>
//	</dict>
//	</plist>
//
// Then load it:
//
//	launchctl load ~/Library/LaunchAgents/com.anywhere.daemon.plist
//
// To start/stop manually:
//
//	launchctl start com.anywhere.daemon
//	launchctl stop  com.anywhere.daemon
//
// To unload:
//
//	launchctl unload ~/Library/LaunchAgents/com.anywhere.daemon.plist

package main

import (
	"os"
	"path/filepath"
)

func defaultSocketPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "anywhere", "anywhere.sock")
}

func defaultExcludeList() string {
	return "/System,/Volumes,/private/var,/private/tmp,/cores"
}
