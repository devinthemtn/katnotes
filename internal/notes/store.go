// Package notes is the KatNotes core: it reads and writes Markdown notes with
// YAML frontmatter under a root directory with journal/ and pages/ subdirs.
// Every frontend (CLI, TUI, GUI) goes through Store.
package notes

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"text/template"
	"time"
	"unicode"
)

// Kind is the type of note, which decides its subdirectory.
type Kind string

const (
	KindPage    Kind = "page"
	KindJournal Kind = "journal"
)

// Kinds lists all kinds in display order.
var Kinds = []Kind{KindPage, KindJournal}

// Dir returns the subdirectory name for k.
func (k Kind) Dir() string {
	if k == KindJournal {
		return "journal"
	}
	return "pages"
}

// Valid reports whether k is a known kind.
func (k Kind) Valid() bool { return k == KindPage || k == KindJournal }

// ParseKind accepts "page", "pages", "journal", "j", "p".
func ParseKind(s string) (Kind, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "page", "pages", "p":
		return KindPage, nil
	case "journal", "journals", "j":
		return KindJournal, nil
	}
	return "", fmt.Errorf("unknown kind %q (want page or journal)", s)
}

// Key names a note across kinds as "kind/id", e.g. "page/ideas". The
// recent-notes list, pick lists and messages all use it.
func Key(kind Kind, id string) string { return string(kind) + "/" + id }

// Key is the note's "kind/id".
func (n *Note) Key() string { return Key(n.Kind, n.ID) }

// ParseKey splits a "kind/id" key.
func ParseKey(k string) (Kind, string, error) {
	kind, id, ok := strings.Cut(k, "/")
	if !ok || id == "" {
		return "", "", fmt.Errorf("%q is not kind/id", k)
	}
	pk, err := ParseKind(kind)
	return pk, id, err
}

// GetKey loads the note a "kind/id" key names.
func (s *Store) GetKey(k string) (*Note, error) {
	kind, id, err := ParseKey(k)
	if err != nil {
		return nil, err
	}
	return s.Get(kind, id)
}

// JournalIDLayout is the filename layout for journal entries.
const JournalIDLayout = "2006-01-02"

var (
	ErrNotFound  = errors.New("note not found")
	ErrExists    = errors.New("note already exists")
	ErrInvalidID = errors.New("invalid note id")
)

// Note is a single Markdown note.
type Note struct {
	Kind    Kind           `json:"kind"`
	ID      string         `json:"id"`
	Path    string         `json:"path"`
	Title   string         `json:"title"`
	Tags    []string       `json:"tags"`
	Created time.Time      `json:"created"`
	Updated time.Time      `json:"updated"`
	Extra   map[string]any `json:"extra,omitempty"`
	Body    string         `json:"body"`
	Snippet string         `json:"snippet,omitempty"`
	// Problem says why the file's frontmatter couldn't be read. The note is
	// still listed, with the whole file as its body, so nothing is lost.
	Problem string `json:"problem,omitempty"`
	// Stamp identifies the version of the file the note was read from or
	// written to (see FileStamp). Unlike Updated, which an updated: field
	// in the frontmatter overrides, it changes whenever the file does.
	Stamp string `json:"stamp,omitempty"`

	d *derived // links, tasks and lowercased body (see cache.go)
}

// FileStamp identifies one version of a file: its modification time and
// size.
func FileStamp(fi os.FileInfo) string {
	return fmt.Sprintf("%d:%d", fi.ModTime().UnixNano(), fi.Size())
}

// Stamp returns the FileStamp of a note's file, or "" when it is missing.
func (s *Store) Stamp(kind Kind, id string) string {
	fi, err := os.Stat(s.Path(kind, id))
	if err != nil {
		return ""
	}
	return FileStamp(fi)
}

// PinnedKey is the frontmatter field that pins a note to the top of lists.
const PinnedKey = "pinned"

// Pinned reports whether the note has pinned: true in its frontmatter.
func (n *Note) Pinned() bool {
	v, ok := n.Extra[PinnedKey].(bool)
	return ok && v
}

// HasTag reports whether the note carries tag (case-insensitive).
func (n *Note) HasTag(tag string) bool {
	norm := NormalizeTags([]string{tag})
	if len(norm) == 0 {
		return false
	}
	tag = norm[0]
	for _, t := range n.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// Store reads and writes notes under Root.
type Store struct {
	Root string
	// JournalTemplate is a text/template for new journal entries. Fields:
	// .Date (formatted with DateFormat), .ID, .Weekday, .Time. It may start
	// with frontmatter, like a page template.
	JournalTemplate string
	// DateFormat is the Go time layout used for journal titles and .Date.
	DateFormat string
	// Now is the clock; tests can override it.
	Now func() time.Time
	// NoDoneDate stops SetTask from stamping done:<date> into task metadata.
	NoDoneDate bool

	cache noteCache
}

// New returns a store rooted at root with default settings.
func New(root string) *Store {
	return &Store{Root: root, JournalTemplate: "# {{.Date}}\n\n", DateFormat: JournalIDLayout, Now: time.Now}
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().Truncate(time.Second)
	}
	return time.Now().Truncate(time.Second)
}

// Init creates the root, journal/ and pages/ directories.
func (s *Store) Init() error {
	for _, k := range Kinds {
		if err := os.MkdirAll(filepath.Join(s.Root, k.Dir()), 0o755); err != nil {
			return err
		}
	}
	return nil
}

// Path returns the file path for a note.
func (s *Store) Path(kind Kind, id string) string {
	return filepath.Join(s.Root, kind.Dir(), id+".md")
}

func validID(id string) bool {
	return id != "" && id != "." && id != ".." && !strings.ContainsAny(id, `/\`) && !strings.HasPrefix(id, ".")
}

func (s *Store) check(kind Kind, id string) error {
	if !kind.Valid() {
		return fmt.Errorf("unknown kind %q", kind)
	}
	if !validID(id) {
		return fmt.Errorf("%w: %q", ErrInvalidID, id)
	}
	if kind == KindJournal {
		if _, err := time.Parse(JournalIDLayout, id); err != nil {
			return fmt.Errorf("%w: journal ids are YYYY-MM-DD, got %q", ErrInvalidID, id)
		}
	}
	return nil
}

// Get loads a single note.
func (s *Store) Get(kind Kind, id string) (*Note, error) {
	if err := s.check(kind, id); err != nil {
		return nil, err
	}
	return s.load(kind, id, s.Path(kind, id))
}

func (s *Store) load(kind Kind, id, path string) (*Note, error) {
	fi, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s/%s", ErrNotFound, kind, id)
	}
	if err != nil {
		return nil, err
	}
	if n, ok := s.cache.get(path, fi); ok {
		return n, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s/%s", ErrNotFound, kind, id)
	}
	if err != nil {
		return nil, err
	}
	n := &Note{Kind: kind, ID: id, Path: path}
	if err := parseNote(data, n); err != nil {
		// Broken frontmatter (common in imported files): keep the note
		// visible with the whole file as its body, and say what's wrong.
		*n = Note{Kind: kind, ID: id, Path: path, Body: string(data), Problem: err.Error()}
		for _, line := range strings.Split(n.Body, "\n") {
			if l := strings.TrimSpace(line); strings.HasPrefix(l, "# ") {
				n.Title = strings.TrimSpace(l[2:]) // first heading, past the broken block
				break
			}
		}
	}
	if n.Title == "" {
		n.Title = titleFromBody(n.Body)
	}
	if n.Title == "" {
		n.Title = id
	}
	if n.Created.IsZero() {
		n.Created = fi.ModTime()
	}
	if n.Updated.IsZero() {
		n.Updated = fi.ModTime()
	}
	if n.Tags == nil {
		n.Tags = []string{}
	}
	n.Snippet = snippet(n.Body)
	n.Stamp = FileStamp(fi)
	n.d = &derived{body: n.Body}
	s.cache.put(path, fi, n)
	return n, nil
}

func titleFromBody(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(line[2:])
		}
		if line != "" {
			return ""
		}
	}
	return ""
}

func snippet(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		r := []rune(line)
		if len(r) > 120 {
			return string(r[:117]) + "..."
		}
		return line
	}
	return ""
}

// SortBy selects List ordering.
type SortBy string

const (
	SortUpdated SortBy = "updated" // newest first (default for pages)
	SortTitle   SortBy = "title"
	SortID      SortBy = "id"      // journal default: newest date first
	SortCreated SortBy = "created" // newest first
)

// ParseSort reads a sort name ("" is the kind's default).
func ParseSort(s string) (SortBy, error) {
	switch b := SortBy(strings.ToLower(strings.TrimSpace(s))); b {
	case "", SortUpdated, SortTitle, SortID, SortCreated:
		return b, nil
	case "modified":
		return SortUpdated, nil
	case "date":
		return SortID, nil
	}
	return "", fmt.Errorf("unknown sort %q (want updated, created, title or id)", s)
}

// ListOpts filters and orders List results.
type ListOpts struct {
	Tag  string
	Sort SortBy
}

// List returns all notes of a kind. An empty kind lists every kind. A file
// deleted while the folder is being read is left out, not an error.
func (s *Store) List(kind Kind, opts ListOpts) ([]Note, error) {
	kinds := Kinds
	if kind != "" {
		kinds = []Kind{kind}
	}
	type file struct {
		kind     Kind
		id, path string
	}
	var files []file
	for _, k := range kinds {
		dir := filepath.Join(s.Root, k.Dir())
		entries, err := os.ReadDir(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".md") || strings.HasPrefix(name, ".") {
				continue
			}
			id := strings.TrimSuffix(name, ".md")
			if k == KindJournal {
				if _, err := time.Parse(JournalIDLayout, id); err != nil {
					continue
				}
			}
			files = append(files, file{k, id, filepath.Join(dir, name)})
		}
	}

	// Load (stat, and parse what the cache doesn't have) in parallel: with
	// thousands of notes the file system calls dominate.
	loaded := make([]*Note, len(files))
	errs := make([]error, len(files))
	parallel(len(files), func(i int) {
		f := files[i]
		loaded[i], errs[i] = s.load(f.kind, f.id, f.path)
	})
	out := make([]Note, 0, len(files))
	for i, n := range loaded {
		if errors.Is(errs[i], ErrNotFound) {
			continue // deleted since ReadDir
		}
		if errs[i] != nil {
			return nil, errs[i]
		}
		if opts.Tag != "" && !n.HasTag(opts.Tag) {
			continue
		}
		out = append(out, *n)
	}
	sortNotes(out, kind, opts.Sort)
	return out, nil
}

// parallel runs f(0) … f(n-1) on a few goroutines and waits for them all.
// Small jobs run inline.
func parallel(n int, f func(i int)) {
	workers := min(runtime.NumCPU(), 8, n/64)
	if workers < 2 {
		for i := range n {
			f(i)
		}
		return
	}
	var next atomic.Int64
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for {
				i := int(next.Add(1)) - 1
				if i >= n {
					return
				}
				f(i)
			}
		})
	}
	wg.Wait()
}

func sortNotes(ns []Note, kind Kind, by SortBy) {
	if by == "" {
		by = SortUpdated
		if kind == KindJournal {
			by = SortID
		}
	}
	var titles []string
	if by == SortTitle {
		titles = make([]string, len(ns))
		for i := range ns {
			titles[i] = strings.ToLower(ns[i].Title)
		}
	}
	pinned := make([]bool, len(ns))
	for i := range ns {
		pinned[i] = ns[i].Pinned()
	}
	sortByIndex(ns, func(i, j int) int {
		if pinned[i] != pinned[j] {
			if pinned[i] {
				return -1 // pinned notes first, whatever the order
			}
			return 1
		}
		a, b := &ns[i], &ns[j]
		switch by {
		case SortTitle:
			return strings.Compare(titles[i], titles[j])
		case SortID:
			return strings.Compare(b.ID, a.ID)
		case SortCreated:
			return b.Created.Compare(a.Created)
		default:
			return b.Updated.Compare(a.Updated)
		}
	})
}

// sortByIndex stably sorts s by cmp, which compares the elements at two
// indexes of s as it was before sorting. It moves each element once instead
// of swapping large structs (notes) many times over.
func sortByIndex[T any](s []T, cmp func(i, j int) int) {
	idx := make([]int, len(s))
	for i := range idx {
		idx[i] = i
	}
	slices.SortFunc(idx, func(i, j int) int {
		if c := cmp(i, j); c != 0 {
			return c
		}
		return i - j // ties keep their order
	})
	sorted := make([]T, len(s))
	for to, from := range idx {
		sorted[to] = s[from]
	}
	copy(s, sorted)
}

// FindByID returns every note whose ID is id, across all kinds.
func (s *Store) FindByID(id string) ([]*Note, error) {
	var out []*Note
	for _, k := range Kinds {
		if s.check(k, id) != nil {
			continue
		}
		n, err := s.Get(k, id)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return out, nil
}

// Slugify turns a title into a filename-safe id: lowercase letters and digits
// (Unicode letters are kept) joined by single dashes.
func Slugify(title string) string {
	var out strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(r)
			dash = false
		} else if !dash && out.Len() > 0 {
			out.WriteRune('-')
			dash = true
		}
	}
	res := strings.Trim(out.String(), "-")
	if res == "" {
		return "untitled"
	}
	return res
}

// Create makes a new note. For pages the id is the slugified title; for
// journal entries title must be a date, YYYY-MM-DD or in the store's
// DateFormat (or empty for today).
func (s *Store) Create(kind Kind, title string, tags []string, body string) (*Note, error) {
	now := s.now()
	var id string
	switch kind {
	case KindPage:
		if strings.TrimSpace(title) == "" {
			return nil, fmt.Errorf("%w: a page needs a title", ErrInvalidID)
		}
		id = Slugify(title)
	case KindJournal:
		d := now
		if strings.TrimSpace(title) != "" {
			var err error
			d, err = ParseDate(s.DateFormat, title, time.Local)
			if err != nil {
				return nil, fmt.Errorf("%w: journal entries are named by date: %v", ErrInvalidID, err)
			}
		}
		return s.createJournal(d, tags, body)
	default:
		return nil, fmt.Errorf("unknown kind %q", kind)
	}
	if err := s.check(kind, id); err != nil {
		return nil, err
	}
	n := &Note{Kind: kind, ID: id, Path: s.Path(kind, id), Title: strings.TrimSpace(title),
		Tags: NormalizeTags(tags), Created: now, Updated: now, Body: body}
	if n.Body == "" {
		n.Body = "# " + n.Title + "\n\n"
	}
	if err := s.write(n, true); err != nil {
		return nil, err
	}
	n.Snippet = snippet(n.Body)
	return n, nil
}

func (s *Store) createJournal(d time.Time, tags []string, body string) (*Note, error) {
	id := d.Format(JournalIDLayout)
	layout := s.dateLayout()
	now := s.now()
	n := &Note{Kind: KindJournal, ID: id, Path: s.Path(KindJournal, id), Title: d.Format(layout),
		Tags: NormalizeTags(tags), Created: now, Updated: now, Body: body}
	if n.Body == "" {
		b, err := s.renderJournal(d)
		if err != nil {
			return nil, err
		}
		// Frontmatter in the template: its tags join the entry's and other
		// keys are copied, as for page templates. The entry's own title,
		// created and updated win.
		var t Note
		if err := parseNote([]byte(b), &t); err != nil {
			return nil, fmt.Errorf("journal template: %w", err)
		}
		n.Body = t.Body
		n.Tags = NormalizeTags(append(n.Tags, t.Tags...))
		for _, k := range []string{"created", "updated", TaskKey} {
			delete(t.Extra, k)
		}
		n.Extra = t.Extra
	}
	if err := s.write(n, true); err != nil {
		return nil, err
	}
	n.Snippet = snippet(n.Body)
	return n, nil
}

func (s *Store) renderJournal(d time.Time) (string, error) {
	src := s.JournalTemplate
	if src == "" {
		return "", nil
	}
	// missingkey=zero: a misspelled field renders empty, as in page templates.
	tmpl, err := template.New("journal").Option("missingkey=zero").Parse(src)
	if err != nil {
		return "", fmt.Errorf("journal template: %w", err)
	}
	layout := s.dateLayout()
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, map[string]string{
		"Date":    d.Format(layout),
		"ID":      d.Format(JournalIDLayout),
		"Weekday": d.Weekday().String(),
		"Time":    s.now().Format("15:04"),
	})
	return buf.String(), err
}

// Journal returns the journal entry for date d, creating it if needed.
func (s *Store) Journal(d time.Time) (n *Note, created bool, err error) {
	id := d.Format(JournalIDLayout)
	n, err = s.Get(KindJournal, id)
	if err == nil {
		return n, false, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, false, err
	}
	n, err = s.createJournal(d, nil, "")
	return n, err == nil, err
}

// Today returns today's journal entry, creating it if needed.
func (s *Store) Today() (*Note, bool, error) { return s.Journal(s.now()) }

// Save writes n back to disk, bumping Updated.
func (s *Store) Save(n *Note) error {
	if err := s.check(n.Kind, n.ID); err != nil {
		return err
	}
	n.Path = s.Path(n.Kind, n.ID)
	n.Tags = NormalizeTags(n.Tags)
	now := s.now()
	if n.Created.IsZero() {
		n.Created = now
	}
	n.Updated = now
	if err := s.write(n, false); err != nil {
		return err
	}
	n.Snippet = snippet(n.Body)
	return nil
}

// Rename changes a note's title. Pages also move to the new slug, and links
// to them in other notes are rewritten (link text that was the old title
// becomes the new one); journal ids are dates and never change. The second
// result lists the "kind/id" of each note whose links were updated.
func (s *Store) Rename(kind Kind, id, newTitle string) (*Note, []string, error) {
	newTitle = strings.TrimSpace(newTitle)
	if newTitle == "" {
		return nil, nil, fmt.Errorf("%w: empty title", ErrInvalidID)
	}
	n, err := s.Get(kind, id)
	if err != nil {
		return nil, nil, err
	}
	oldTitle := n.Title
	n.Title = newTitle
	if h := "# " + oldTitle + "\n"; strings.HasPrefix(n.Body, h) {
		n.Body = "# " + newTitle + "\n" + strings.TrimPrefix(n.Body, h)
	}
	if kind == KindPage {
		newID := Slugify(newTitle)
		if newID != id {
			if _, err := os.Stat(s.Path(kind, newID)); err == nil {
				return nil, nil, fmt.Errorf("%w: %s/%s", ErrExists, kind, newID)
			}
			old := n.Path
			n.ID = newID
			if err := s.Save(n); err != nil {
				return nil, nil, err
			}
			s.cache.drop(old)
			if err := os.Remove(old); err != nil {
				return nil, nil, err
			}
			changed, err := s.rewriteLinks(kind, id, newID, oldTitle, newTitle)
			return n, changed, err
		}
	}
	return n, nil, s.Save(n)
}

// Delete removes a note.
func (s *Store) Delete(kind Kind, id string) error {
	if err := s.check(kind, id); err != nil {
		return err
	}
	s.cache.drop(s.Path(kind, id))
	err := os.Remove(s.Path(kind, id))
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %s/%s", ErrNotFound, kind, id)
	}
	return err
}

// ModTime returns the file modification time of a note (zero if missing).
func (s *Store) ModTime(kind Kind, id string) time.Time {
	fi, err := os.Stat(s.Path(kind, id))
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}

// write atomically writes n. With exclusive set it fails if the file exists.
func (s *Store) write(n *Note, exclusive bool) error {
	if n.Body != "" && !strings.HasSuffix(n.Body, "\n") {
		n.Body += "\n"
	}
	dir := filepath.Dir(n.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if exclusive {
		if _, err := os.Stat(n.Path); err == nil {
			return fmt.Errorf("%w: %s/%s", ErrExists, n.Kind, n.ID)
		}
	}
	syncTaskFlag(n)
	data, err := encodeNote(n)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".katnotes-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	s.cache.drop(n.Path)
	if exclusive {
		err = placeNew(tmp.Name(), n.Path)
	} else {
		err = os.Rename(tmp.Name(), n.Path)
	}
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%w: %s/%s", ErrExists, n.Kind, n.ID)
	}
	if err != nil {
		return err
	}
	if fi, err := os.Stat(n.Path); err == nil {
		n.Stamp = FileStamp(fi)
	}
	return nil
}

// placeNew moves the finished temp file to path only if nothing is there
// yet. A hard link does the check and the move in one step, so two apps
// creating the same note at once can't overwrite each other. Filesystems
// without hard links fall back to a check followed by a rename.
func placeNew(tmp, path string) error {
	err := os.Link(tmp, path)
	if err == nil || errors.Is(err, os.ErrExist) {
		return err
	}
	if _, serr := os.Stat(path); serr == nil {
		return os.ErrExist
	}
	return os.Rename(tmp, path)
}
