package notes

import (
	"os"
	"sync"
	"time"
)

// The parse cache: a note file is read and parsed again only when its
// modification time or size changed, so listing, searching and completion
// over thousands of unchanged notes cost one directory read plus stats.

type cacheEntry struct {
	mod  time.Time
	size int64
	note Note
}

type noteCache struct {
	mu sync.Mutex
	m  map[string]cacheEntry
	// parses counts real parses (tests check the cache with it).
	parses int
}

// clone copies the parts of a note a caller might change in place.
func (n Note) clone() Note {
	c := n
	c.Tags = append([]string(nil), n.Tags...)
	if n.Tags != nil && c.Tags == nil {
		c.Tags = []string{}
	}
	if n.Extra != nil {
		c.Extra = make(map[string]any, len(n.Extra))
		for k, v := range n.Extra {
			c.Extra[k] = v
		}
	}
	return c
}

func (c *noteCache) get(path string, fi os.FileInfo) (*Note, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[path]
	if !ok || !e.mod.Equal(fi.ModTime()) || e.size != fi.Size() {
		return nil, false
	}
	n := e.note.clone()
	return &n, true
}

func (c *noteCache) put(path string, fi os.FileInfo, n *Note) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]cacheEntry{}
	}
	c.parses++
	c.m[path] = cacheEntry{mod: fi.ModTime(), size: fi.Size(), note: n.clone()}
}

func (c *noteCache) drop(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, path)
}

// Parses reports how many times a note file was actually parsed (for
// tests of the cache).
func (s *Store) Parses() int {
	s.cache.mu.Lock()
	defer s.cache.mu.Unlock()
	return s.cache.parses
}
