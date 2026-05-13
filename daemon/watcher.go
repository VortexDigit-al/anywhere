package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

// Watcher wraps an fsnotify watcher and keeps the Index in sync with the
// filesystem.  On startup, call WatchTree for each root directory; it will
// recursively add inotify watches and populate the index.  Afterwards, Run
// processes events until Stop is called.
type Watcher struct {
	fw   *fsnotify.Watcher
	idx  *Index
	skip map[string]struct{} // directories to never descend into
}

func newWatcher(idx *Index, skip map[string]struct{}) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{fw: fw, idx: idx, skip: skip}, nil
}

// WatchTree adds an inotify watch for dir, indexes its contents, then recurses
// into every subdirectory.  It is safe to call concurrently with Run.
//
// The correct startup order is:
//  1. Call WatchTree for each root.
//  2. Start Run in a goroutine.
//
// WatchTree adds the watch before reading directory contents, so any files
// created after the watch is added but before the directory is read will
// generate Create events that Run will process — duplicates are harmless
// because Index.Add is idempotent.
func (w *Watcher) WatchTree(dir string) {
	if _, excluded := w.skip[dir]; excluded {
		return
	}

	if err := w.fw.Add(dir); err != nil {
		// Usually ENOSPC (inotify watch limit) or a permission error.
		// Log once at the first failure, then continue with partial coverage.
		log.Printf("watch limit reached or permission denied for %s: %v", dir, err)
		// Still index the directory's contents even though we won't get events.
		w.indexDir(dir)
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		w.idx.Add(path)
		// IsDir returns false for symlinks, so we never follow them.
		if e.IsDir() {
			w.WatchTree(path)
		}
	}
}

// indexDir adds all direct children of dir to the index without adding
// inotify watches.  Used as a fallback when we exceed the watch limit.
func (w *Watcher) indexDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		w.idx.Add(filepath.Join(dir, e.Name()))
	}
}

// Run processes filesystem events until the watcher is stopped.
func (w *Watcher) Run() {
	for {
		select {
		case event, ok := <-w.fw.Events:
			if !ok {
				return
			}
			w.handle(event)
		case err, ok := <-w.fw.Errors:
			if !ok {
				return
			}
			if err != nil {
				log.Printf("watcher error: %v", err)
			}
		}
	}
}

func (w *Watcher) Stop() {
	w.fw.Close()
}

func (w *Watcher) handle(event fsnotify.Event) {
	switch {
	case event.Has(fsnotify.Create):
		fi, err := os.Lstat(event.Name)
		if err != nil {
			return
		}
		if fi.IsDir() {
			// New directory: watch its tree and index current contents.
			// (A rename target also fires Create, so this handles mv too.)
			w.WatchTree(event.Name)
		} else if fi.Mode().IsRegular() {
			w.idx.Add(event.Name)
		}

	case event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename):
		// Rename fires on the source path; the destination gets a Create.
		w.idx.Remove(event.Name)
	}
}
