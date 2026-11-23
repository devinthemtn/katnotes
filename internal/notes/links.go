package notes

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
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

// bodyLines calls fn for each body line outside fenced code, with the line
// and a copy whose inline code spans are blanked out (same byte offsets).
func bodyLines(n *Note, fn func(i int, line, masked string)) {
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
		if fence != "" {
			continue
		}
		fn(i, line, maskCode(line))
	}
}

// ParseLinks returns the links in a note's body to other notes, skipping
// fenced code, inline code and images.
func ParseLinks(n *Note) []Link {
	var out []Link
	bodyLines(n, func(i int, line, masked string) {
		if !strings.Contains(line, "](") {
			return
		}
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
	})
	return out
}

// BodyLink is any link in a note's body: to another note (Note set, with
// its Kind and ID; the note may not exist yet), a URL, or another file.
type BodyLink struct {
	Line int    // 0-based body line
	Text string // link text, unescaped (a bare URL is its own text)
	Dest string // destination as written, without <>
	Kind Kind
	ID   string
	Note bool
	// Byte offsets of the link text in the line, and of the destination
	// (with any <>) for an inline link; DestEnd is 0 for a bare URL.
	TextStart, TextEnd int
	DestStart, DestEnd int
}

// bareURL matches a URL written as plain text.
var bareURL = regexp.MustCompile(`https?://[^\s<>()\[\]]+`)

// BodyLinks returns every link in a note's body in reading order: inline
// [text](dest) links of any kind and bare http(s) URLs. It skips images,
// code and same-page #anchor links.
func BodyLinks(n *Note) []BodyLink {
	var out []BodyLink
	bodyLines(n, func(i int, line, masked string) {
		var spans [][2]int // inline links, so bare URLs inside them are skipped
		var row []BodyLink
		for _, m := range inlineLink.FindAllStringSubmatchIndex(masked, -1) {
			spans = append(spans, [2]int{m[0], m[1]})
			if m[3] > m[2] { // image
				continue
			}
			dest := strings.TrimSuffix(strings.TrimPrefix(line[m[6]:m[7]], "<"), ">")
			if dest == "" || strings.HasPrefix(dest, "#") {
				continue
			}
			l := BodyLink{Line: i, Text: unescapeLinkText(line[m[4]:m[5]]), Dest: dest,
				TextStart: m[4], TextEnd: m[5], DestStart: m[6], DestEnd: m[7]}
			l.Kind, l.ID, l.Note = resolveLink(n.Kind, dest)
			row = append(row, l)
		}
		for _, m := range bareURL.FindAllStringIndex(masked, -1) {
			inside := false
			for _, sp := range spans {
				if m[0] >= sp[0] && m[0] < sp[1] {
					inside = true
				}
			}
			if inside {
				continue
			}
			u := strings.TrimRight(line[m[0]:m[1]], ".,;:!?'\"")
			row = append(row, BodyLink{Line: i, Text: u, Dest: u, TextStart: m[0], TextEnd: m[0] + len(u)})
		}
		slices.SortFunc(row, func(a, b BodyLink) int { return a.TextStart - b.TextStart })
		out = append(out, row...)
	})
	return out
}

// PlainLinks replaces each inline link in s with its text, for showing a
// line without Markdown link syntax. Images are left as they are.
func PlainLinks(s string) string {
	return inlineLink.ReplaceAllStringFunc(s, func(m string) string {
		if strings.HasPrefix(m, "!") {
			return m
		}
		return unescapeLinkText(inlineLink.FindStringSubmatch(m)[2])
	})
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
	parseAllLinks(ns)
	out := []Backlink{}
	for _, n := range ns {
		if n.Kind == kind && n.ID == id {
			continue
		}
		var lines []LineMatch
		var body []string
		last := -1
		for _, l := range n.links() {
			if l.Kind == kind && l.ID == id && l.Line != last {
				if body == nil {
					body = strings.Split(n.Body, "\n")
				}
				lines = append(lines, LineMatch{Line: l.Line + 1, Text: strings.TrimSpace(body[l.Line])})
				last = l.Line
			}
		}
		if lines != nil {
			n.Body = ""
			out = append(out, Backlink{Note: n, Lines: lines})
		}
	}
	sortByIndex(out, func(i, j int) int { return out[j].Note.Updated.Compare(out[i].Note.Updated) })
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
	parseAllLinks(ns)
	var changed []string
	for i := range ns {
		n := &ns[i]
		links := n.links()
		if !slices.ContainsFunc(links, func(l Link) bool { return l.Kind == kind && l.ID == oldID }) {
			continue
		}
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
	if err := s.writeSynced(n, true); err != nil {
		return nil, false, err
	}
	n.Snippet = snippet(n.Body)
	return n, true, nil
}

// LinksHeading is the section AddLink puts links under.
const LinksHeading = "## Links"

// AddLink adds a link to to/toID, as a list item under the "## Links"
// section of the note from/fromID, creating the section at the end of the
// body when there isn't one. text is the link text. It returns the saved
// note and whether it changed (false when the note already links there).
func (s *Store) AddLink(from Kind, fromID string, to Kind, toID, text string) (*Note, bool, error) {
	if from == to && fromID == toID {
		return nil, false, errors.New("link: a note can't link to itself")
	}
	if err := s.check(to, toID); err != nil {
		return nil, false, err
	}
	n, err := s.Get(from, fromID)
	if err != nil {
		return nil, false, err
	}
	for _, l := range ParseLinks(n) {
		if l.Kind == to && l.ID == toID {
			return n, false, nil
		}
	}
	n.Body = addLinkLine(n.Body, "- "+LinkMarkdown(to, toID, text))
	if err := s.Save(n); err != nil {
		return nil, false, err
	}
	return n, true, nil
}

// addLinkLine inserts item at the end of body's "## Links" section, or
// appends the section with item when body has none.
func addLinkLine(body, item string) string {
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	start, fence := -1, ""
	end := len(lines)
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			if f := t[:3]; fence == "" {
				fence = f
			} else if f == fence {
				fence = ""
			}
			continue
		}
		if fence != "" || !strings.HasPrefix(t, "#") {
			continue
		}
		if start < 0 {
			if strings.EqualFold(strings.TrimRight(t, " #"), LinksHeading) {
				start = i
			}
		} else if headingLevel(t) <= 2 {
			end = i
			break
		}
	}
	if start < 0 {
		out := strings.Join(lines, "\n")
		if out != "" {
			out += "\n\n"
		}
		return out + LinksHeading + "\n\n" + item + "\n"
	}
	// After the section's last non-blank line.
	at := end
	for at > start+1 && strings.TrimSpace(lines[at-1]) == "" {
		at--
	}
	ins := []string{item}
	if at == start+1 {
		ins = []string{"", item} // blank line after the heading
	}
	if at < len(lines) {
		ins = append(ins, "") // keep a blank line before the next heading
		for at < end && strings.TrimSpace(lines[at]) == "" {
			lines = append(lines[:at], lines[at+1:]...)
			end--
		}
	}
	lines = append(lines[:at], append(ins, lines[at:]...)...)
	return strings.Join(lines, "\n") + "\n"
}

// headingLevel is the number of leading #s of an ATX heading (7 when t isn't one).
func headingLevel(t string) int {
	n := 0
	for n < len(t) && t[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || (n < len(t) && t[n] != ' ') {
		return 7
	}
	return n
}
