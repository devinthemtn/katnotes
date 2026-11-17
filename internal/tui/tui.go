// Package tui is the terminal frontend for KatNotes (bubbletea).
package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/devinthemtn/katnotes/internal/cli"
	"github.com/devinthemtn/katnotes/internal/editor"
	"github.com/devinthemtn/katnotes/internal/notes"
)

// Run starts the TUI.
func Run(app *cli.App) error {
	style := "light"
	if lipgloss.HasDarkBackground() {
		style = "dark"
	}
	m := newModel(app, style)
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

type tab int

const (
	tabPages tab = iota
	tabJournal
	tabTags
)

var tabNames = []string{"Pages", "Journal", "Tags"}

type mode int

const (
	modeNormal mode = iota
	modeInput
	modeConfirm
)

type inputPurpose int

const (
	inNew inputPurpose = iota
	inSearch
	inTag
	inRename
)

// list items

type noteItem struct{ n notes.Note }

func (i noteItem) Title() string { return i.n.Title }
func (i noteItem) Description() string {
	parts := []string{}
	if i.n.Kind == notes.KindPage {
		parts = append(parts, i.n.Updated.Format("Jan 02 2006"))
	} else {
		parts = append(parts, i.n.ID)
	}
	if len(i.n.Tags) > 0 {
		parts = append(parts, "#"+strings.Join(i.n.Tags, " #"))
	}
	if i.n.Snippet != "" {
		parts = append(parts, i.n.Snippet)
	}
	return strings.Join(parts, " · ")
}
func (i noteItem) FilterValue() string { return i.n.Title }

type tagItem struct{ t notes.TagCount }

func (i tagItem) Title() string       { return "#" + i.t.Tag }
func (i tagItem) Description() string { return fmt.Sprintf("%d note(s)", i.t.Count) }
func (i tagItem) FilterValue() string { return i.t.Tag }

// messages

type editedMsg struct {
	n      *notes.Note
	before time.Time
	err    error
}

// styles

var (
	activeTab   = lipgloss.NewStyle().Bold(true).Padding(0, 1).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#7D56F4"))
	inactiveTab = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("245"))
	filterStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#E0AF68"))
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#F7768E"))
	paneBorder  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238"))
)

type model struct {
	app   *cli.App
	style string

	tab    tab
	list   list.Model
	view   viewport.Model
	render *glamour.TermRenderer
	cache  map[string]string // path+updated -> rendered

	mode    mode
	purpose inputPurpose
	input   textinput.Model
	confirm *notes.Note

	tagFilter string
	query     string
	results   map[string]notes.Match // kind/id -> match while searching

	status  string
	isErr   bool
	width   int
	height  int
	listW   int
	preview string // key of note currently previewed
}

func newModel(app *cli.App, style string) model {
	d := list.NewDefaultDelegate()
	l := list.New(nil, d, 0, 0)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()

	ti := textinput.New()
	ti.CharLimit = 200

	m := model{app: app, style: style, list: l, view: viewport.New(0, 0), input: ti, cache: map[string]string{}}
	m.reload()
	return m
}

func (m model) Init() tea.Cmd { return nil }

func key(n *notes.Note) string { return string(n.Kind) + "/" + n.ID }

// reload rebuilds the list for the current tab, filters and search.
func (m *model) reload() {
	var sel string
	if it, ok := m.list.SelectedItem().(noteItem); ok {
		sel = key(&it.n)
	}
	var items []list.Item
	var err error
	switch m.tab {
	case tabTags:
		var tags []notes.TagCount
		tags, err = m.app.Store.SortedTags()
		for _, t := range tags {
			items = append(items, tagItem{t})
		}
	default:
		kind := notes.KindPage
		if m.tab == tabJournal {
			kind = notes.KindJournal
		}
		if m.query != "" {
			var res []notes.Match
			res, err = m.app.Store.Search(m.query, kind)
			m.results = map[string]notes.Match{}
			for _, r := range res {
				if m.tagFilter != "" && !r.Note.HasTag(m.tagFilter) {
					continue
				}
				m.results[key(&r.Note)] = r
				items = append(items, noteItem{r.Note})
			}
		} else {
			var ns []notes.Note
			ns, err = m.app.Store.List(kind, notes.ListOpts{Tag: m.tagFilter})
			for _, n := range ns {
				items = append(items, noteItem{n})
			}
		}
	}
	if err != nil {
		m.setErr(err)
	}
	m.list.SetItems(items)
	if sel != "" {
		for i, it := range items {
			if ni, ok := it.(noteItem); ok && key(&ni.n) == sel {
				m.list.Select(i)
				break
			}
		}
	}
	if m.list.Index() >= len(items) && len(items) > 0 {
		m.list.Select(len(items) - 1)
	}
	m.preview = ""
	m.updatePreview()
}

// selectKey moves the cursor to the note with key k, if listed.
func (m *model) selectKey(k string) {
	for i, it := range m.list.Items() {
		if ni, ok := it.(noteItem); ok && key(&ni.n) == k {
			m.list.Select(i)
			break
		}
	}
	m.updatePreview()
}

func (m *model) setErr(err error)   { m.status, m.isErr = err.Error(), true }
func (m *model) setStatus(s string) { m.status, m.isErr = s, false }

func (m *model) selected() *notes.Note {
	if it, ok := m.list.SelectedItem().(noteItem); ok {
		n := it.n
		return &n
	}
	return nil
}

func (m *model) layout() {
	if m.width == 0 {
		return
	}
	listW := m.width * 2 / 5
	if listW < 28 {
		listW = 28
	}
	bodyH := m.height - 4 // tabs + filter line + status + help
	if bodyH < 3 {
		bodyH = 3
	}
	m.list.SetSize(listW, bodyH)
	m.listW = listW
	pw := m.width - listW - 3
	if pw < 10 {
		pw = 10
	}
	m.view.Width, m.view.Height = pw-2, bodyH-2
	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle(m.style), glamour.WithWordWrap(m.view.Width-2))
	if err == nil {
		m.render = r
		m.cache = map[string]string{}
	}
	m.preview = ""
	m.updatePreview()
}

func (m *model) updatePreview() {
	var content, k string
	if n := m.selected(); n != nil {
		k = n.Path + n.Updated.String()
		if k == m.preview {
			return
		}
		out, ok := m.cache[k]
		if !ok {
			full, err := m.app.Store.Get(n.Kind, n.ID)
			if err != nil {
				out = errStyle.Render(err.Error())
			} else {
				out = full.Body
				if m.render != nil {
					if r, err := m.render.Render(full.Body); err == nil {
						out = r
					}
				}
				if len(full.Tags) > 0 {
					out = filterStyle.Render("  #"+strings.Join(full.Tags, " #")) + "\n" + out
				}
			}
			m.cache[k] = out
		}
		content = out
	} else if it, ok := m.list.SelectedItem().(tagItem); ok {
		k = "tag:" + it.t.Tag
		ns, _ := m.app.Store.List("", notes.ListOpts{Tag: it.t.Tag})
		var b strings.Builder
		fmt.Fprintf(&b, "\n  Notes tagged #%s\n\n", it.t.Tag)
		for _, n := range ns {
			fmt.Fprintf(&b, "  • %s  (%s/%s)\n", n.Title, n.Kind, n.ID)
		}
		b.WriteString("\n  enter: filter Pages/Journal by this tag\n")
		content = b.String()
	} else {
		content = statusStyle.Render("\n  Nothing here yet. Press n to create a note, T for today's journal.")
	}
	m.preview = k
	m.view.SetContent(content)
	m.view.GotoTop()
}

func (m *model) startInput(p inputPurpose, prompt, value string) tea.Cmd {
	m.mode, m.purpose = modeInput, p
	m.input.Prompt = prompt
	m.input.SetValue(value)
	m.input.CursorEnd()
	return m.input.Focus()
}

func (m *model) editCmd(n *notes.Note) tea.Cmd {
	before := m.app.Store.ModTime(n.Kind, n.ID)
	c, err := editor.Command(m.app.Cfg.Editor, n.Path)
	if err != nil {
		m.setErr(err)
		return nil
	}
	return tea.ExecProcess(c, func(err error) tea.Msg { return editedMsg{n: n, before: before, err: err} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case editedMsg:
		if msg.err != nil {
			m.setErr(fmt.Errorf("editor: %w", msg.err))
		} else if n, err := m.app.AfterEdit(msg.n, msg.before); err != nil {
			m.setErr(err)
		} else {
			m.setStatus("saved " + key(n))
		}
		m.reload()
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case modeInput:
			return m.updateInput(msg)
		case modeConfirm:
			return m.updateConfirm(msg)
		}
		return m.updateNormal(msg)
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab", "right", "l":
		m.tab = (m.tab + 1) % 3
		m.list.Select(0)
		m.reload()
		return m, nil
	case "shift+tab", "left", "h":
		m.tab = (m.tab + 2) % 3
		m.list.Select(0)
		m.reload()
		return m, nil
	case "1", "2", "3":
		m.tab = tab(msg.String()[0] - '1')
		m.list.Select(0)
		m.reload()
		return m, nil
	case "esc":
		if m.query != "" || m.tagFilter != "" {
			m.query, m.tagFilter = "", ""
			m.setStatus("filters cleared")
			m.reload()
		}
		return m, nil
	case "n":
		if m.tab == tabJournal {
			return m, m.startInput(inNew, "New journal date (YYYY-MM-DD, empty = today): ", "")
		}
		return m, m.startInput(inNew, "New page title: ", "")
	case "/":
		return m, m.startInput(inSearch, "Search: ", m.query)
	case "t":
		return m, m.startInput(inTag, "Filter by tag (empty clears): ", m.tagFilter)
	case "T":
		n, created, err := m.app.Store.Today()
		if err != nil {
			m.setErr(err)
			return m, nil
		}
		if created {
			m.setStatus("created " + key(n))
		}
		m.tab = tabJournal
		m.reload()
		m.selectKey(key(n))
		return m, m.editCmd(n)
	case "r":
		if n := m.selected(); n != nil {
			return m, m.startInput(inRename, "Rename to: ", n.Title)
		}
	case "d", "delete":
		if n := m.selected(); n != nil {
			m.mode, m.confirm = modeConfirm, n
		}
		return m, nil
	case "e", "enter":
		if it, ok := m.list.SelectedItem().(tagItem); ok {
			m.tagFilter = it.t.Tag
			m.tab = tabPages
			m.list.Select(0)
			m.setStatus("filtering by #" + it.t.Tag + " (esc clears)")
			m.reload()
			return m, nil
		}
		if n := m.selected(); n != nil {
			return m, m.editCmd(n)
		}
		return m, nil
	case "R":
		m.reload()
		m.setStatus("reloaded")
		return m, nil
	case "pgdown", "ctrl+d", "J":
		m.view.HalfPageDown()
		return m, nil
	case "pgup", "ctrl+u", "K":
		m.view.HalfPageUp()
		return m, nil
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	m.updatePreview()
	return m, cmd
}

func (m model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.input.Blur()
		return m, nil
	case "enter":
		m.mode = modeNormal
		m.input.Blur()
		val := strings.TrimSpace(m.input.Value())
		return m.submit(val)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) submit(val string) (tea.Model, tea.Cmd) {
	switch m.purpose {
	case inNew:
		kind := notes.KindPage
		if m.tab == tabJournal {
			kind = notes.KindJournal
		} else if val == "" {
			return m, nil
		}
		var tags []string
		if m.tagFilter != "" {
			tags = []string{m.tagFilter}
		}
		n, err := m.app.Store.Create(kind, val, tags, "")
		if err != nil {
			m.setErr(err)
			return m, nil
		}
		m.setStatus("created " + key(n))
		m.reload()
		m.selectKey(key(n))
		return m, m.editCmd(n)
	case inSearch:
		m.query = val
		if val != "" {
			m.setStatus(fmt.Sprintf("search %q (esc clears)", val))
		}
		m.list.Select(0)
		m.reload()
	case inTag:
		m.tagFilter = strings.ToLower(strings.TrimPrefix(val, "#"))
		m.list.Select(0)
		m.reload()
	case inRename:
		if n := m.selected(); n != nil && val != "" {
			r, err := m.app.Store.Rename(n.Kind, n.ID, val)
			if err != nil {
				m.setErr(err)
				return m, nil
			}
			m.setStatus("renamed to " + key(r))
			m.reload()
			m.selectKey(key(r))
		}
	}
	return m, nil
}

func (m model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := m.confirm
	m.mode, m.confirm = modeNormal, nil
	if strings.ToLower(msg.String()) == "y" && n != nil {
		if err := m.app.Store.Delete(n.Kind, n.ID); err != nil {
			m.setErr(err)
		} else {
			m.setStatus("deleted " + key(n))
		}
		m.reload()
	} else {
		m.setStatus("delete cancelled")
	}
	return m, nil
}

func (m model) View() string {
	if m.width == 0 {
		return "loading…"
	}
	var tabs []string
	for i, name := range tabNames {
		label := fmt.Sprintf("%d %s", i+1, name)
		if tab(i) == m.tab {
			tabs = append(tabs, activeTab.Render(label))
		} else {
			tabs = append(tabs, inactiveTab.Render(label))
		}
	}
	header := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
	root := m.app.Store.Root
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(root, home) {
		root = "~" + strings.TrimPrefix(root, home)
	}
	if room := m.width - lipgloss.Width(header) - 2; room > 4 {
		if r := []rune(root); len(r) > room {
			root = "…" + string(r[len(r)-room+1:])
		}
		header += "  " + statusStyle.Render(root)
	}

	var filters []string
	if m.tagFilter != "" {
		filters = append(filters, "#"+m.tagFilter)
	}
	if m.query != "" {
		filters = append(filters, fmt.Sprintf("search %q", m.query))
	}
	filterLine := fmt.Sprintf(" %d item(s)", len(m.list.Items()))
	if len(filters) > 0 {
		filterLine += "  " + filterStyle.Render("filter: "+strings.Join(filters, " + ")+"  (esc clears)")
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(m.listW).Render(m.list.View()),
		" ",
		paneBorder.Width(m.view.Width).Height(m.view.Height).Render(m.view.View()),
	)

	var bottom string
	switch m.mode {
	case modeInput:
		bottom = m.input.View()
	case modeConfirm:
		bottom = errStyle.Render(fmt.Sprintf("Delete %s (%q)? y/N", key(m.confirm), m.confirm.Title))
	default:
		if m.status != "" {
			if m.isErr {
				bottom = errStyle.Render(m.status)
			} else {
				bottom = statusStyle.Render(m.status)
			}
		}
	}
	help := statusStyle.Render("n new · enter/e edit · r rename · d delete · T today · / search · t tag · tab switch · J/K scroll · q quit")
	return lipgloss.JoinVertical(lipgloss.Left, header, filterLine, body, bottom, help)
}
