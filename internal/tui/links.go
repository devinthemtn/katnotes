package tui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/devinthemtn/katnotes/internal/editor"
	"github.com/devinthemtn/katnotes/internal/notes"
)

// Following links from the focused preview: n / N highlight the next or
// previous link in the rendered note, then the notes in its "Linked from"
// footer, enter opens it, backspace goes back.
//
// glamour renders the Markdown, so where a link lands on screen isn't
// known. Instead the selected link's text is wrapped in two private-use
// runes before rendering; afterwards they mark its line and are swapped
// for visible markers.

const (
	linkOpen  = ""
	linkClose = ""
	maxBack   = 50 // notes remembered for backspace
)

// openExternal opens a URL or file in the system's default app (a var so
// tests can stub it).
var openExternal = func(target string) error {
	return editor.StartDetached(editor.SystemOpenCommand(target))
}

// syncLinks refreshes bodyLinks when the previewed note changes.
func (m *model) syncLinks(n *notes.Note) {
	key := n.Key() + n.Updated.String()
	if key == m.linksKey {
		return
	}
	m.linksKey, m.bodyLinks, m.linkIdx = key, nil, -1
	if full, err := m.app.Store.Get(n.Kind, n.ID); err == nil {
		m.bodyLinks = notes.BodyLinks(full)
	}
}

// syncLinkedBy refreshes linkedBy, the notes linking to n (read on every
// preview update, since a change to another note can alter them).
func (m *model) syncLinkedBy(n *notes.Note) {
	m.linkedBy = nil
	if bl, err := m.app.Store.Backlinks(n.Kind, n.ID); err == nil {
		m.linkedBy = bl
	}
	if m.linkIdx >= len(m.bodyLinks)+len(m.linkedBy) {
		m.linkIdx = -1
	}
}

// previewBody is body as the preview renders it. Links to notes point at
// "#", which glamour shows as their text alone instead of text plus a
// ../pages/x.md path (web links keep their URL). The link at index sel, if
// any, has its text wrapped in the sentinel runes, and so does each
// {{ … }} (markHighlights). links are body's links.
func previewBody(body string, links []notes.BodyLink, sel int) string {
	type edit struct {
		start, end int
		with       string
	}
	byLine := map[int][]edit{}
	for i, l := range links {
		if l.Note && l.DestEnd > l.DestStart {
			byLine[l.Line] = append(byLine[l.Line], edit{l.DestStart, l.DestEnd, "#"})
		}
		if i == sel {
			byLine[l.Line] = append(byLine[l.Line],
				edit{l.TextEnd, l.TextEnd, linkClose}, edit{l.TextStart, l.TextStart, linkOpen})
		}
	}
	lines := strings.Split(body, "\n")
	for n, edits := range byLine {
		if n >= len(lines) {
			continue
		}
		// Right to left, so earlier offsets stay valid.
		slices.SortStableFunc(edits, func(a, b edit) int { return b.start - a.start })
		s := lines[n]
		for _, e := range edits {
			if e.start < 0 || e.end > len(s) || e.start > e.end {
				continue
			}
			s = s[:e.start] + e.with + s[e.end:]
		}
		lines[n] = s
	}
	return markHighlights(strings.Join(lines, "\n"))
}

// highlightLink swaps the sentinels in rendered text for visible markers
// and returns the line the link starts on (-1 if it went missing).
func highlightLink(out string) (string, int) {
	line := -1
	if i := strings.Index(out, linkOpen); i >= 0 {
		line = strings.Count(out[:i], "\n")
	}
	mark := lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	out = strings.ReplaceAll(out, linkOpen, mark.Render("▶"))
	out = strings.ReplaceAll(out, linkClose, mark.Render("◀"))
	return out, line
}

// renderWithLink renders n with the selected link highlighted, returning
// the preview text and the line the link is on.
func (m *model) renderWithLink(n *notes.Note) (string, int) {
	full, err := m.app.Store.Get(n.Kind, n.ID)
	if err != nil {
		return errStyle.Render(err.Error()), -1
	}
	body := previewBody(full.Body, m.bodyLinks, m.linkIdx)
	out := body
	if m.render != nil {
		if r, err := m.render.Render(body); err == nil {
			out = r
		}
	}
	out, line := highlightLink(applyHighlights(out))
	fm := m.frontmatterView(full)
	if line >= 0 {
		line += strings.Count(fm, "\n")
	}
	return fm + out, line
}

// scrollToLine scrolls the preview so line shows, a third of the way down,
// when it is out of view.
func (m *model) scrollToLine(line int) {
	if line >= m.view.YOffset && line < m.view.YOffset+m.view.Height {
		return
	}
	m.view.SetYOffset(max(line-m.view.Height/3, 0))
}

// cycleLink highlights the next (d = 1) or previous (d = -1) link.
func (m *model) cycleLink(d int) {
	n := len(m.bodyLinks) + len(m.linkedBy)
	if n == 0 {
		m.setStatus("no links in this note")
		return
	}
	if m.linkIdx < 0 {
		if d > 0 {
			m.linkIdx = 0
		} else {
			m.linkIdx = n - 1
		}
	} else {
		m.linkIdx = (m.linkIdx + d + n) % n
	}
	if i := m.linkIdx - len(m.bodyLinks); i >= 0 {
		x := m.linkedBy[i].Note
		m.setStatus(fmt.Sprintf("link %d/%d: linked from %s (%s) · enter opens", m.linkIdx+1, n, x.Title, x.Key()))
		m.updatePreview()
		return
	}
	l := m.bodyLinks[m.linkIdx]
	to := l.Dest
	if l.Note {
		to = notes.Key(l.Kind, l.ID)
	}
	m.setStatus(fmt.Sprintf("link %d/%d: %s → %s · enter opens", m.linkIdx+1, n, l.Text, to))
	m.updatePreview()
}

// openSelectedLink opens the highlighted link: a note is shown here (and a
// missing one created and opened in the editor), anything else opens in
// the system's default app.
func (m *model) openSelectedLink() tea.Cmd {
	from := m.selected()
	if i := m.linkIdx - len(m.bodyLinks); i >= 0 && i < len(m.linkedBy) {
		n, err := m.app.Store.Get(m.linkedBy[i].Note.Kind, m.linkedBy[i].Note.ID)
		if err != nil {
			m.setErr(err)
			return nil
		}
		m.pushBack(from)
		m.touchRecent(n)
		m.showNote(n)
		m.setStatus("opened " + n.Key() + " (b back)")
		return nil
	}
	if m.linkIdx < 0 || m.linkIdx >= len(m.bodyLinks) {
		return nil
	}
	l := m.bodyLinks[m.linkIdx]
	if !l.Note {
		target := l.Dest
		if !strings.Contains(target, ":") && from != nil {
			target = filepath.Join(filepath.Dir(from.Path), filepath.FromSlash(target))
		}
		if err := openExternal(target); err != nil {
			m.setErr(fmt.Errorf("open %s: %w", l.Dest, err))
		} else {
			m.setStatus("opened " + l.Dest)
		}
		return nil
	}
	n, err := m.app.Store.Get(l.Kind, l.ID)
	created := false
	if err != nil {
		if n, created, err = m.app.Store.Ensure(l.Kind, l.ID, l.Text); err != nil {
			m.setErr(err)
			return nil
		}
	}
	m.pushBack(from)
	m.touchRecent(n)
	m.showNote(n)
	if created {
		m.setStatus("created " + n.Key() + " (b back)")
		return m.editCmd(n)
	}
	m.setStatus("opened " + n.Key() + " (b back)")
	return nil
}

// pushBack remembers from (if any) for b / backspace.
func (m *model) pushBack(from *notes.Note) {
	if from == nil {
		return
	}
	m.back = append(m.back, from.Key())
	if len(m.back) > maxBack {
		m.back = m.back[1:]
	}
}

// goBack shows the note a link was followed from.
func (m *model) goBack() {
	for len(m.back) > 0 {
		k := m.back[len(m.back)-1]
		m.back = m.back[:len(m.back)-1]
		if n := m.noteByKey(k); n != nil {
			m.showNote(n)
			m.setStatus("back to " + k)
			return
		}
	}
	m.setStatus("no previous note")
}
