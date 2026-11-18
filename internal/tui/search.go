package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/devinthemtn/katnotes/internal/notes"
	"github.com/devinthemtn/katnotes/internal/viewstate"
)

// Finding notes in a big collection, as in the GUI: a live search bar with
// Name / Tags / Text modes ("/", tab switches), quick open (ctrl+p), a sort
// order per tab ("s", shared with the GUI through view.json), month
// headers on date-sorted lists, and saved searches on the Tags tab.

var (
	chipOn  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#7D56F4")).Padding(0, 1)
	chipOff = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Padding(0, 1)
	groupHd = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#9D7DF8"))
)

var searchHints = map[notes.SearchMode]string{
	notes.SearchName: "fuzzy title or filename",
	notes.SearchTags: "tags, all must match (-tag excludes)",
	notes.SearchText: `words or "a phrase" in the text`,
}

func newSearchInput() textinput.Model {
	ti := textinput.New()
	ti.Prompt = "/"
	ti.CharLimit = 200
	return ti
}

func (m *model) openSearch() tea.Cmd {
	m.mode = modeSearch
	if m.searchMode == "" {
		m.searchMode = notes.SearchName
	}
	m.searchInput.SetValue(m.query)
	m.searchInput.CursorEnd()
	return m.searchInput.Focus()
}

func (m model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = modeNormal
		m.searchInput.Blur()
		m.query = ""
		m.list.Select(0)
		m.reload()
		m.setStatus("search cleared")
		return m, nil
	case "enter":
		m.mode = modeNormal
		m.searchInput.Blur()
		if m.query != "" {
			m.setStatus(fmt.Sprintf("search %s %q · / edits · esc clears", m.searchMode, m.query))
		}
		return m, nil
	case "tab", "shift+tab":
		d := 1
		if msg.String() == "shift+tab" {
			d = -1
		}
		m.searchMode = notes.NextSearchMode(m.searchMode, d)
		m.list.Select(0)
		m.reload()
		return m, nil
	case "up", "down", "ctrl+n", "ctrl+p", "pgup", "pgdown":
		// Move through the results without leaving the search bar.
		k := map[string]string{"ctrl+n": "down", "ctrl+p": "up"}[msg.String()]
		if k != "" {
			msg = tea.KeyMsg{Type: map[string]tea.KeyType{"down": tea.KeyDown, "up": tea.KeyUp}[k]}
		}
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		m.skipHeaders(msg.String())
		m.updatePreview()
		return m, cmd
	}
	before := m.searchInput.Value()
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	if v := m.searchInput.Value(); v != before {
		m.query = v
		m.list.Select(0)
		m.reload()
	}
	return m, cmd
}

// searchBarView is the bottom line while searching: mode chips and input.
func (m model) searchBarView() string {
	var chips []string
	for _, md := range notes.SearchModes {
		label := strings.ToUpper(string(md[:1])) + string(md[1:])
		if md == m.searchMode {
			chips = append(chips, chipOn.Render(label))
		} else {
			chips = append(chips, chipOff.Render(label))
		}
	}
	return strings.Join(chips, "") + " " + m.searchInput.View() + "  " + cmdGhost.Render(searchHints[m.searchMode]+" · tab mode · enter keep · esc clear")
}

// ---- sort order per tab ----

// sortCycle is the order "s" steps through for each kind.
var sortCycle = map[notes.Kind][]notes.SortBy{
	notes.KindPage:    {notes.SortUpdated, notes.SortCreated, notes.SortTitle},
	notes.KindJournal: {notes.SortID, notes.SortUpdated, notes.SortTitle},
}

var sortNames = map[notes.SortBy]string{
	notes.SortUpdated: "modified", notes.SortCreated: "created", notes.SortTitle: "title A–Z", notes.SortID: "date",
}

func (m *model) sortFor(k notes.Kind) notes.SortBy {
	if s, ok := m.sorts[k]; ok && s != "" {
		return s
	}
	return sortCycle[k][0]
}

func (m *model) cycleSort(k notes.Kind) {
	cyc := sortCycle[k]
	cur := m.sortFor(k)
	next := cyc[0]
	for i, s := range cyc {
		if s == cur {
			next = cyc[(i+1)%len(cyc)]
		}
	}
	m.sorts[k] = next
	if m.viewPath != "" {
		viewstate.SetSort(m.viewPath, string(k), string(next)) //nolint:errcheck // a convenience
	}
	m.list.Select(0)
	m.reload()
	m.setStatus("sort: " + sortNames[next])
}

// ---- month headers ----

// headerItem is a group label in the list ("November 2026"); the cursor
// skips it.
type headerItem struct{ label string }

func (i headerItem) Title() string       { return groupHd.Render("── " + i.label + " ──") }
func (i headerItem) Description() string { return "" }
func (i headerItem) FilterValue() string { return "" }

// withMonthHeaders inserts headers into a date-sorted list of notes:
// "Pinned" above pinned notes, then one per month of the sort date.
func withMonthHeaders(items []list.Item, by notes.SortBy) []list.Item {
	if by == notes.SortTitle || len(items) == 0 {
		return items
	}
	out := make([]list.Item, 0, len(items)+12)
	last := ""
	for _, it := range items {
		ni, ok := it.(noteItem)
		if !ok {
			return items
		}
		label := "Pinned"
		if !ni.n.Pinned() {
			t := ni.n.Updated
			switch by {
			case notes.SortCreated:
				t = ni.n.Created
			case notes.SortID:
				if d, err := parseDay(ni.n.ID); err == nil {
					t = d
				}
			}
			label = t.Format("January 2006")
		}
		if label != last {
			out = append(out, headerItem{label})
			last = label
		}
		out = append(out, it)
	}
	return out
}

// skipHeaders moves the selection off a header, in the direction of the key
// that landed on it.
func (m *model) skipHeaders(key string) {
	items := m.list.Items()
	if len(items) == 0 {
		return
	}
	d := 1
	switch key {
	case "up", "k", "pgup", "ctrl+u", "home", "g":
		d = -1
	}
	i := m.list.Index()
	for step := 0; step < 2; step++ {
		for i >= 0 && i < len(items) {
			if _, ok := items[i].(headerItem); !ok {
				m.list.Select(i)
				return
			}
			i += d
		}
		d, i = -d, m.list.Index()
	}
}

func parseDay(id string) (time.Time, error) {
	return time.ParseInLocation(notes.JournalIDLayout, id, time.Local)
}

// ---- saved searches (shown on the Tags tab) ----

type savedItem struct{ s viewstate.Search }

func (i savedItem) Title() string       { return "🔎 " + i.s.Name }
func (i savedItem) Description() string { return i.s.Mode + ": " + i.s.Text }
func (i savedItem) FilterValue() string { return i.s.Name }

// applySearch shows a search (a saved one, or from ":search"). It runs on
// the Pages or Journal tab you are on, or switches to Pages.
func (m *model) applySearch(mode, text string) {
	md, err := notes.ParseSearchMode(mode)
	if err != nil {
		m.setErr(err)
		return
	}
	m.searchMode, m.query = md, text
	m.clearViews()
	if m.tab != tabPages && m.tab != tabJournal {
		m.tab = tabPages
	}
	m.list.Select(0)
	m.reload()
	m.setStatus(fmt.Sprintf("search %s %q: %d note(s) · esc clears", md, text, len(m.noteItems())))
}

func (m *model) noteItems() []list.Item {
	var out []list.Item
	for _, it := range m.list.Items() {
		if _, ok := it.(noteItem); ok {
			out = append(out, it)
		}
	}
	return out
}
