package notes

import (
	"os"
	"strings"
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
	mu sync.RWMutex
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
	c.mu.RLock()
	e, ok := c.m[path]
	c.mu.RUnlock()
	if !ok || !e.mod.Equal(fi.ModTime()) || e.size != fi.Size() {
		return nil, false
	}
	n := e.note.clone() // entries are never changed in place, so no lock needed
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

// derived holds what KatNotes works out from a note's body: its links, its
// tasks and a lowercased copy for text search. A note read through the cache
// shares one derived with every copy of it, built the first time each part
// is asked for, so whole-folder features (backlinks, the task views, text
// search) don't re-parse thousands of unchanged bodies on every call.
//
// The parts are shared read-only: callers must not change the returned
// slices or the tasks' Meta maps.
type derived struct {
	body string // the body the parts were worked out from

	linksOnce, tasksOnce, lowerOnce sync.Once
	links                           []Link
	tasks                           []Task
	lower                           string
}

// derive returns n's derived data, or a fresh, unshared one when n's body is
// no longer the one the shared data was built from (a caller edited it).
func (n *Note) derive() *derived {
	if d := n.d; d != nil && d.body == n.Body {
		return d
	}
	return &derived{body: n.Body}
}

// links is ParseLinks(n), worked out once per version of the file.
func (n *Note) links() []Link {
	d := n.derive()
	d.linksOnce.Do(func() { d.links = ParseLinks(&Note{Kind: n.Kind, Body: d.body}) })
	return d.links
}

// parseAllLinks works out the links of every note in ns on a few goroutines,
// so the first whole-folder link scan after a start isn't one long serial
// parse. Afterwards (and when the cache already has them) n.links() is free.
func parseAllLinks(ns []Note) { parallel(len(ns), func(i int) { ns[i].links() }) }

// tasks is ParseTasks(n.Body), worked out once per version of the file.
func (n *Note) tasks() []Task {
	d := n.derive()
	d.tasksOnce.Do(func() { d.tasks = ParseTasks(d.body) })
	return d.tasks
}

// lowerBody is strings.ToLower(n.Body), worked out once per version of the
// file.
func (n *Note) lowerBody() string {
	d := n.derive()
	d.lowerOnce.Do(func() { d.lower = strings.ToLower(d.body) })
	return d.lower
}
