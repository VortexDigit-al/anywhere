package main

import (
	"strings"
	"sync"
)

// Index is a thread-safe in-memory set of file paths with fast substring search.
//
// Internally it maintains three parallel slices (paths, lower, and a reverse
// lookup map) so that Add and Remove are O(1) amortized and Search is a single
// cache-friendly pass over the lowercased slice.
type Index struct {
	mu     sync.RWMutex
	paths  []string       // original cased paths
	lower  []string       // paths[i] lowercased — searched against
	byPath map[string]int // path → index in the slices
}

func newIndex() *Index {
	return &Index{byPath: make(map[string]int, 1<<17)} // pre-alloc for ~128k paths
}

func (idx *Index) Len() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.paths)
}

// Add inserts path.  No-op if path is already present.
func (idx *Index) Add(path string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.add(path)
}

func (idx *Index) add(path string) {
	if _, exists := idx.byPath[path]; exists {
		return
	}
	i := len(idx.paths)
	idx.paths = append(idx.paths, path)
	idx.lower = append(idx.lower, strings.ToLower(path))
	idx.byPath[path] = i
}

// Remove deletes path.  No-op if not present.
// Uses swap-with-last to keep slices contiguous without shifting.
func (idx *Index) Remove(path string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	i, ok := idx.byPath[path]
	if !ok {
		return
	}
	last := len(idx.paths) - 1
	if i != last {
		// Move the last element into slot i.
		idx.paths[i] = idx.paths[last]
		idx.lower[i] = idx.lower[last]
		idx.byPath[idx.paths[i]] = i
	}
	idx.paths = idx.paths[:last]
	idx.lower = idx.lower[:last]
	delete(idx.byPath, path)
}

// Search returns up to limit paths whose lowercased form contains query
// (case-insensitive).  The order of results is unspecified.
func (idx *Index) Search(query string, limit int) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" || limit <= 0 {
		return nil
	}

	out := make([]string, 0, min(limit, 64))
	for i, lp := range idx.lower {
		if strings.Contains(lp, q) {
			out = append(out, idx.paths[i])
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}
