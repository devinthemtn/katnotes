package notes

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Links between notes are ordinary relative Markdown links, so they work in
// any viewer that follows .md links (katdown, GitHub):
//
//	[Groceries](../pages/groceries.md)
//	[2026-11-17](../journal/2026-11-17.md)
//
// KatNotes writes the "../<dir>/<id>.md" form, which is valid from both
// folders, but any relative link that resolves to a note counts.

// Link is one link from a note body to another note.
type Link struct {
	Line int    `json:"line"` // 0-based body line
	Text string `json:"text"` // link text, unescaped
	Dest string `json:"dest"` // destination as written
	Kind Kind   `json:"kind"`
	ID   string `json:"id"`
	// Byte offsets in the line of the text and destination (for rewriting).
	textStart, textEnd int
	destStart, destEnd int
}

// LinkRef is an outgoing link with what it points at.
type LinkRef struct {
	Link
	Exists bool   `json:"exists"`
	Title  string `json:"title"` // target title, or the link text when missing
}

// Backlink is a note that links to another, with the linking lines.
type Backlink struct {
	Note  Note        `json:"note"`
	Lines []LineMatch `json:"lines"`
}

// inlineLink matches [text](dest) and [text](<dest> "title"). Group 1 is "!"
// for images.
var inlineLink = regexp.MustCompile(`(!?)\[((?:\\.|[^\]\\])*)\]\(\s*(<[^<>\n]*>|[^\s()<>]+)(?:\s+"[^"]*")?\s*\)`)

// LinkDest is the destination KatNotes writes for a link to a note.
func LinkDest(kind Kind, id string) string {
	return "../" + kind.Dir() + "/" + id + ".md"
}

// LinkMarkdown is a complete link to a note, e.g. "[Groceries](../pages/groceries.md)".
func LinkMarkdown(kind Kind, id, text string) string {
	text = strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`).Replace(text)
	return "[" + text + "](" + LinkDest(kind, id) + ")"
}

// maskCode blanks out inline code spans so links inside them are ignored,
// keeping byte offsets intact.
func maskCode(line string) string {
	if !strings.Contains(line, "`") {
		return line
	}
	b := []byte(line)
	for i := 0; i < len(b); {
		if b[i] != '`' {
			i++
			continue
		}
		n := 0
		for i+n < len(b) && b[i+n] == '`' {
			n++
		}
		fence := strings.Repeat("`", n)
		end := strings.Index(string(b[i+n:]), fence)
		if end < 0 {
			break // unmatched: not a code span
		}
		for j := i; j < i+n+end+n; j++ {
			b[j] = ' '
		}
		i += n + end + n
	}
	return string(b)
}

// resolveLink maps a destination written in a note of kind `from` to the note
// it points at.
func resolveLink(from Kind, dest string) (Kind, string, bool) {
	dest = strings.TrimSuffix(strings.TrimPrefix(dest, "<"), ">")
	if i := strings.IndexAny(dest, "#?"); i >= 0 {
		dest = dest[:i]
	}
	if dest == "" || strings.Contains(dest, ":") || strings.HasPrefix(dest, "/") || strings.HasPrefix(dest, `\`) {
		return "", "", false // external, absolute or same-page
	}
	if d, err := url.PathUnescape(dest); err == nil {
		dest = d
	}
	p := filepath.ToSlash(filepath.Clean(filepath.Join(from.Dir(), filepath.FromSlash(dest))))
	dir, file, ok := strings.Cut(p, "/")
	if !ok || strings.Contains(file, "/") || !strings.HasSuffix(file, ".md") {
		return "", "", false
	}
	id := strings.TrimSuffix(file, ".md")
	for _, k := range Kinds {
		if k.Dir() == dir && validID(id) {
			if (&Store{}).check(k, id) != nil {
				return "", "", false
			}
			return k, id, true
		}
	}
	return "", "", false
}

// ParseLinks returns the links in a note's body to other notes, skipping
// fenced code, inline code and images.
func ParseLinks(n *Note) []Link {
	var out []Link
	fence := ""
	for i, line := range strings.Split(n.Body, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			f := t[:3]
			if fence == "" {
				fence = f
			} else if f == fence {
				fence = ""
			}
			continue
		}
		if fence != "" || !strings.Contains(line, "](") {
			continue
		}
		masked := maskCode(line)
		for _, m := range inlineLink.FindAllStringSubmatchIndex(masked, -1) {
			if m[3] > m[2] { // image
				continue
			}
			dest := line[m[6]:m[7]]
			k, id, ok := resolveLink(n.Kind, dest)
			if !ok {
				continue
			}
			ds, de := m[6], m[7]
			if strings.HasPrefix(dest, "<") {
				ds, de = ds+1, de-1
			}
			out = append(out, Link{
				Line: i, Text: unescapeLinkText(line[m[4]:m[5]]), Dest: line[ds:de], Kind: k, ID: id,
				textStart: m[4], textEnd: m[5], destStart: ds, destEnd: de,
			})
		}
	}
	return out
}

var linkEscape = regexp.MustCompile(`\\([\\\[\]])`)

func unescapeLinkText(s string) string { return linkEscape.ReplaceAllString(s, "$1") }

// Links lists the note's outgoing links to other notes, in order.
func (s *Store) Links(kind Kind, id string) ([]LinkRef, error) {
	n, err := s.Get(kind, id)
	if err != nil {
		return nil, err
	}
	out := []LinkRef{}
	for _, l := range ParseLinks(n) {
		r := LinkRef{Link: l, Title: l.Text}
		if t, err := s.Get(l.Kind, l.ID); err == nil {
			r.Exists, r.Title = true, t.Title
		}
		out = append(out, r)
	}
	return out, nil
}

// Backlinks lists the notes that link to kind/id, most recently updated
// first, each with its linking lines (1-based, like search matches).
func (s *Store) Backlinks(kind Kind, id string) ([]Backlink, error) {
	if err := s.check(kind, id); err != nil {
		return nil, err
	}
	ns, err := s.List("", ListOpts{})
	if err != nil {
		return nil, err
	}
	out := []Backlink{}
	for _, n := range ns {
		if n.Kind == kind && n.ID == id {
			continue
		}
		var lines []LineMatch
		last := -1
		body := strings.Split(n.Body, "\n")
		for _, l := range ParseLinks(&n) {
			if l.Kind == kind && l.ID == id && l.Line != last {
				lines = append(lines, LineMatch{Line: l.Line + 1, Text: strings.TrimSpace(body[l.Line])})
				last = l.Line
			}
		}
		if lines != nil {
			n.Body = ""
			out = append(out, Backlink{Note: n, Lines: lines})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Note.Updated.After(out[j].Note.Updated) })
	return out, nil
}

// rewriteLinks points every link to kind/oldID at newID instead, and renames
// link text that was the old title. It returns the "kind/id" of each note it
// changed.
func (s *Store) rewriteLinks(kind Kind, oldID, newID, oldTitle, newTitle string) ([]string, error) {
	ns, err := s.List("", ListOpts{})
	if err != nil {
		return nil, err
	}
	var changed []string
	for i := range ns {
		n := &ns[i]
		links := ParseLinks(n)
		lines := strings.Split(n.Body, "\n")
		touched := false
		// Right to left, so earlier offsets on the same line stay valid.
		for j := len(links) - 1; j >= 0; j-- {
			l := links[j]
			if l.Kind != kind || l.ID != oldID {
				continue
			}
			line := lines[l.Line]
			line = line[:l.destStart] + retarget(l.Dest, newID) + line[l.destEnd:]
			if l.Text == oldTitle {
				line = line[:l.textStart] + strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`).Replace(newTitle) + line[l.textEnd:]
			}
			lines[l.Line] = line
			touched = true
		}
		if !touched {
			continue
		}
		n.Body = strings.Join(lines, "\n")
		if err := s.Save(n); err != nil {
			return changed, fmt.Errorf("%s/%s: %w", n.Kind, n.ID, err)
		}
		changed = append(changed, n.Key())
	}
	return changed, nil
}

// retarget swaps the file name in a link destination for newID.md, keeping
// its directory prefix and any #fragment.
func retarget(dest, newID string) string {
	path, rest := dest, ""
	if i := strings.IndexAny(dest, "#?"); i >= 0 {
		path, rest = dest[:i], dest[i:]
	}
	dir := ""
	if i := strings.LastIndex(path, "/"); i >= 0 {
		dir = path[:i+1]
	}
	return dir + newID + ".md" + rest
}

// Ensure returns the note kind/id, creating it when it doesn't exist yet so
// a link to a missing note can be followed. A new page gets title (or its id
// when title is empty) and keeps exactly this id, so the link stays valid.
func (s *Store) Ensure(kind Kind, id, title string) (n *Note, created bool, err error) {
	if err := s.check(kind, id); err != nil {
		return nil, false, err
	}
	n, err = s.Get(kind, id)
	if err == nil || !errors.Is(err, ErrNotFound) {
		return n, false, err
	}
	if kind == KindJournal {
		d, _ := time.ParseInLocation(JournalIDLayout, id, time.Local)
		return s.Journal(d)
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = id
	}
	now := s.now()
	n = &Note{Kind: kind, ID: id, Path: s.Path(kind, id), Title: title, Created: now, Updated: now,
		Body: "# " + title + "\n\n"}
	if err := s.write(n, true); err != nil {
		return nil, false, err
	}
	n.Snippet = snippet(n.Body)
	return n, true, nil
}
