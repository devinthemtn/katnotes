package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/devinthemtn/katnotes/internal/commands"
	"github.com/devinthemtn/katnotes/internal/notes"
	"github.com/devinthemtn/katnotes/internal/recent"
)

// Behaviour kept in step with the GUI: the pick view (the GUI palette's pick
// mode), task filters (the GUI's Open/Done/All and Any date/Overdue/This
// week buttons), the frontmatter block (the GUI's table above the editor),
// the shared recent-notes list and the start_page option.

// pickView is a list of notes a command returned (:recent, :git status).
type pickView struct {
	title string
	items []commands.Choice
}

// clearViews leaves the links and pick views.
func (m *model) clearViews() {
	m.links = nil
	m.pick = nil
}

// showPick switches the list to a command's pick list.
func (m *model) showPick(title string, items []commands.Choice) {
	m.links = nil
	m.pick = &pickView{title: title, items: items}
	m.list.Select(0)
	m.reload()
	m.setStatus(fmt.Sprintf("%s: %d note(s) · enter opens · esc closes", title, len(m.list.Items())))
}

// What the all-tasks view shows (the GUI's Open / Done / All buttons).
const (
	showOpen = "open"
	showDone = "done"
	showAll  = "all"
)

// taskFilter is the all-tasks view's filter.
func (m *model) taskFilter() notes.TaskFilter {
	return notes.TaskFilter{
		IncludeDone: m.taskShow == showAll,
		OnlyDone:    m.taskShow == showDone,
		When:        m.taskWhen,
	}
}

func (m *model) taskFilterText() string {
	when := map[string]string{notes.WhenAny: "any date", notes.WhenOverdue: "overdue", notes.WhenWeek: "due this week"}[m.taskWhen]
	return m.taskShow + " · " + when
}

// setTaskFilters applies ":tasks open|done|all any|overdue|week".
func (m *model) setTaskFilters(args []string) {
	for _, a := range args {
		switch a = strings.ToLower(a); a {
		case showOpen, showDone, showAll:
			m.taskShow = a
		case "any":
			m.taskWhen = notes.WhenAny
		case "overdue", "week":
			m.taskWhen = a
		}
	}
	if len(args) > 0 {
		m.allTasks = true
	}
}

var fmKey = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Width(10)

// frontmatterView is the block above a note's preview: every field (like the
// GUI's table) when m.fmOpen, otherwise just the tags.
func (m *model) frontmatterView(n *notes.Note) string {
	if !m.fmOpen {
		if len(n.Tags) == 0 {
			return ""
		}
		return filterStyle.Render("  "+tagsText(n.Tags)) + "\n"
	}
	row := func(k, v string) string {
		if v == "" {
			v = "—"
		}
		return "  " + fmKey.Render(k) + v + "\n"
	}
	var b strings.Builder
	b.WriteString(row("title", n.Title))
	b.WriteString(row("tags", strings.Join(n.Tags, ", ")))
	b.WriteString(row("created", n.Created.Format("Jan 02 2006 15:04")))
	b.WriteString(row("updated", n.Updated.Format("Jan 02 2006 15:04")))
	keys := make([]string, 0, len(n.Extra))
	for k := range n.Extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(row(k, commands.FieldText(n, k)))
	}
	return statusStyle.Render(b.String()) + "\n"
}

// touchRecent records n as recently opened (shared with the GUI).
func (m *model) touchRecent(n *notes.Note) {
	if n != nil && m.recentPath != "" {
		recent.Touch(m.recentPath, n.Key()) //nolint:errcheck // a convenience
	}
}

// applyStartPage selects the note the start_page option names, as the GUI
// opens it on launch: "today", "last" (the most recent note in either app),
// or "page/<id>" / "journal/<id>". "none" and "" leave the list as it is.
func (m *model) applyStartPage() {
	start := strings.TrimSpace(m.app.Cfg.StartPage)
	switch start {
	case "", "none":
		return
	case "today":
		if n, _, err := m.app.Store.Today(); err == nil {
			m.showNote(n)
		} else {
			m.setErr(err)
		}
		return
	case "last":
		if m.recentPath == "" {
			return
		}
		for _, k := range recent.Load(m.recentPath) {
			if n := m.noteByKey(k); n != nil {
				m.showNote(n)
				return
			}
		}
		return
	}
	if n := m.noteByKey(start); n != nil {
		m.showNote(n)
	} else {
		m.setErr(fmt.Errorf("start page %s not found", start))
	}
}

func (m *model) noteByKey(k string) *notes.Note {
	n, err := m.app.Store.GetKey(k)
	if err != nil {
		return nil
	}
	return n
}
