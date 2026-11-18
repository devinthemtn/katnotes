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
	"sort"
	"strings"
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
}

// HasTag reports whether the note carries tag (case-insensitive).
func (n *Note) HasTag(tag string) bool {
	tag = strings.ToLower(strings.TrimPrefix(tag, "#"))
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
	// JournalTemplate is a text/template for new journal bodies. Fields:
	// .Date (formatted with DateFormat), .ID, .Weekday, .Time.
	JournalTemplate string
	// DateFormat is the Go time layout used for journal titles and .Date.
	DateFormat string
	// Now is the clock; tests can override it.
	Now func() time.Time
	// NoDoneDate stops SetTask from stamping done:<date> into task metadata.
	NoDoneDate bool
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
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s/%s", ErrNotFound, kind, id)
	}
	if err != nil {
		return nil, err
	}
	n := &Note{Kind: kind, ID: id, Path: path}
	if err := parseNote(data, n); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if n.Title == "" {
		n.Title = titleFromBody(n.Body)
	}
	if n.Title == "" {
		n.Title = id
	}
	if n.Created.IsZero() || n.Updated.IsZero() {
		if fi, err := os.Stat(path); err == nil {
			if n.Created.IsZero() {
				n.Created = fi.ModTime()
			}
			if n.Updated.IsZero() {
				n.Updated = fi.ModTime()
			}
		}
	}
	if n.Tags == nil {
		n.Tags = []string{}
	}
	n.Snippet = snippet(n.Body)
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
	SortID      SortBy = "id" // journal default: newest date first
)

// ListOpts filters and orders List results.
type ListOpts struct {
	Tag  string
	Sort SortBy
}

// List returns all notes of a kind. An empty kind lists every kind.
func (s *Store) List(kind Kind, opts ListOpts) ([]Note, error) {
	kinds := Kinds
	if kind != "" {
		kinds = []Kind{kind}
	}
	var out []Note
	for _, k := range kinds {
		entries, err := os.ReadDir(filepath.Join(s.Root, k.Dir()))
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
			n, err := s.load(k, id, filepath.Join(s.Root, k.Dir(), name))
			if err != nil {
				return nil, err
			}
			if opts.Tag != "" && !n.HasTag(opts.Tag) {
				continue
			}
			out = append(out, *n)
		}
	}
	sortNotes(out, kind, opts.Sort)
	return out, nil
}

func sortNotes(ns []Note, kind Kind, by SortBy) {
	if by == "" {
		by = SortUpdated
		if kind == KindJournal {
			by = SortID
		}
	}
	sort.SliceStable(ns, func(i, j int) bool {
		a, b := ns[i], ns[j]
		switch by {
		case SortTitle:
			return strings.ToLower(a.Title) < strings.ToLower(b.Title)
		case SortID:
			return a.ID > b.ID
		default:
			return a.Updated.After(b.Updated)
		}
	})
}

// Find returns every note whose ID is id, across all kinds.
func (s *Store) Find(id string) ([]*Note, error) {
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
// journal entries title must be a YYYY-MM-DD date (or empty for today).
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
			d, err = time.ParseInLocation(JournalIDLayout, strings.TrimSpace(title), time.Local)
			if err != nil {
				return nil, fmt.Errorf("%w: journal entries are named YYYY-MM-DD", ErrInvalidID)
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
	layout := s.DateFormat
	if layout == "" {
		layout = JournalIDLayout
	}
	now := s.now()
	n := &Note{Kind: KindJournal, ID: id, Path: s.Path(KindJournal, id), Title: d.Format(layout),
		Tags: NormalizeTags(tags), Created: now, Updated: now, Body: body}
	if n.Body == "" {
		b, err := s.renderJournal(d)
		if err != nil {
			return nil, err
		}
		n.Body = b
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
	tmpl, err := template.New("journal").Parse(src)
	if err != nil {
		return "", fmt.Errorf("journal template: %w", err)
	}
	layout := s.DateFormat
	if layout == "" {
		layout = JournalIDLayout
	}
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
	return os.Rename(tmp.Name(), n.Path)
}
