// Package tui is the terminal frontend for KatNotes (bubbletea).
package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/devinthemtn/katnotes/internal/cli"
	"github.com/devinthemtn/katnotes/internal/commands"
	"github.com/devinthemtn/katnotes/internal/editor"
	"github.com/devinthemtn/katnotes/internal/notes"
	"github.com/devinthemtn/katnotes/internal/recent"
	"github.com/devinthemtn/katnotes/internal/viewstate"
)

// Run starts the TUI.
func Run(app *cli.App) error {
	style := "light"
	if lipgloss.HasDarkBackground() {
		style = "dark"
	}
	env := commands.Env{}
	if dir := env.TemplatesPath(); dir != "" {
		notes.WriteExampleTemplate(dir) //nolint:errcheck // the example is a convenience
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
	tabTasks
)

var tabNames = []string{"Pages", "Journal", "Tags", "Tasks"}

type mode int

const (
	modeNormal mode = iota
	modeInput
	modeConfirm
	modeHelp
	modeCommand
	modeSearch // the live "/" search bar
)

type inputPurpose int

const (
	inNew inputPurpose = iota
	inSearch
	inTag
	inRename
	inDue
)

// list items

// noteItem is a note in the list. match is the search hit shown under it.
type noteItem struct {
	n     notes.Note
	match string
}

func (i noteItem) Title() string {
	t := i.n.Title
	if i.n.Pinned() {
		t = "📌 " + t
	}
	if i.n.Problem != "" {
		t = "⚠ " + t
	}
	return t
}

// Description is the lines under the title (the GUI sidebar shows the same):
// pages get "created │ m: modified" then tags; journal entries their date and
// tags on one line. A search hit adds its matching line.
func (i noteItem) Description() string {
	var lines []string
	if i.n.Kind == notes.KindPage {
		const day = "Jan 02 2006"
		lines = append(lines, i.n.Created.Format(day)+"  │  m: "+i.n.Updated.Format(day))
		lines = append(lines, tagsText(i.n.Tags))
	} else {
		line := i.n.ID
		if len(i.n.Tags) > 0 {
			line += " · " + tagsText(i.n.Tags)
		}
		lines = append(lines, line)
	}
	if i.match != "" {
		lines = append(lines, "…"+i.match)
	}
	return strings.Join(lines, "\n")
}

func tagsText(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	return "#" + strings.Join(tags, " #")
}
func (i noteItem) FilterValue() string { return i.n.Title }

type tagItem struct{ t notes.TagCount }

func (i tagItem) Title() string       { return "#" + i.t.Tag }
func (i tagItem) Description() string { return fmt.Sprintf("%d note(s)", i.t.Count) }
func (i tagItem) FilterValue() string { return i.t.Tag }

// taskNoteItem is a note that contains tasks (Tasks tab, notes view).
type taskNoteItem struct{ nt notes.NoteTasks }

func (i taskNoteItem) Title() string { return i.nt.Note.Title }
func (i taskNoteItem) Description() string {
	parts := []string{fmt.Sprintf("%d open · %d done", i.nt.Open, i.nt.Done)}
	if t, ok := firstOpen(i.nt.Tasks); ok {
		parts = append(parts, "☐ "+t.Text)
	} else {
		parts = append(parts, string(i.nt.Note.Kind)+"/"+i.nt.Note.ID)
	}
	return strings.Join(parts, " · ")
}
func (i taskNoteItem) FilterValue() string { return i.nt.Note.Title }

// taskItem is one task line (Tasks tab, all-tasks view).
type taskItem struct {
	n notes.Note
	t notes.Task
}

func (i taskItem) Title() string {
	box := "☐ "
	if i.t.Done {
		box = "☑ "
	}
	text := i.t.Text
	if text == "" {
		text = "(empty task)"
	}
	return strings.Repeat(" ", min(i.t.Indent, 8)) + box + priMark(i.t) + text
}
func (i taskItem) Description() string {
	parts := []string{}
	if d := dueLabel(i.t, time.Now()); d != "" {
		parts = append(parts, d)
	}
	if s, ok := i.t.Meta[notes.MetaStart]; ok && !i.t.Done && s > time.Now().Format(notes.DateLayout) {
		parts = append(parts, "starts "+s)
	}
	parts = append(parts, fmt.Sprintf("%s · %s/%s:%d", i.n.Title, i.n.Kind, i.n.ID, i.t.Line+1))
	return strings.Join(parts, " · ")
}

// priMark is a short priority prefix for a task title.
func priMark(t notes.Task) string {
	switch t.Meta[notes.MetaPri] {
	case "high":
		return priHigh.Render("!!!") + " "
	case "med":
		return priMed.Render("!!") + " "
	case "low":
		return priLow.Render("!") + " "
	}
	return ""
}

// dueLabel describes a task's due date relative to now, styled when it is
// overdue or due today.
func dueLabel(t notes.Task, now time.Time) string {
	d, ok := t.Meta[notes.MetaDue]
	if !ok {
		return ""
	}
	today := now.Format(notes.DateLayout)
	switch {
	case t.Done:
		return "due " + d
	case d < today:
		return errStyle.Render("overdue " + d)
	case d == today:
		return filterStyle.Render("due today")
	}
	return "due " + d
}
func (i taskItem) FilterValue() string { return i.t.Text }

// missingItem is a link target that doesn't exist yet (links view).
type missingItem struct{ l notes.LinkRef }

func (i missingItem) Title() string { return "(missing) " + i.l.Title }
func (i missingItem) Description() string {
	return fmt.Sprintf("%s/%s · enter creates it", i.l.Kind, i.l.ID)
}
func (i missingItem) FilterValue() string { return i.l.Title }

// linkView lists the notes linking to a note (back) or the notes it links to.
type linkView struct {
	back bool
	of   notes.Note
}

func firstOpen(ts []notes.Task) (notes.Task, bool) {
	for _, t := range ts {
		if !t.Done {
			return t, true
		}
	}
	return notes.Task{}, false
}

// nextPriority cycles none → high → med → low → none.
func nextPriority(p string) string {
	switch p {
	case "":
		return "high"
	case "high":
		return "med"
	case "med":
		return "low"
	}
	return ""
}

// itemKey identifies a list item across reloads.
func itemKey(it list.Item) string {
	switch v := it.(type) {
	case noteItem:
		return key(&v.n)
	case taskNoteItem:
		return key(&v.nt.Note)
	case taskItem:
		return fmt.Sprintf("%s#%d", key(&v.n), v.t.Line)
	case missingItem:
		return "missing:" + string(v.l.Kind) + "/" + v.l.ID
	}
	return ""
}

// taskMatches applies the tag filter and search to a note with tasks.
func (m *model) taskMatches(nt notes.NoteTasks, t *notes.Task) bool {
	if m.tagFilter != "" && !nt.Note.HasTag(m.tagFilter) {
		return false
	}
	q := strings.ToLower(m.query)
	if q == "" || strings.Contains(strings.ToLower(nt.Note.Title), q) {
		return true
	}
	if t != nil {
		return strings.Contains(strings.ToLower(t.Text), q)
	}
	for _, x := range nt.Tasks {
		if strings.Contains(strings.ToLower(x.Text), q) {
			return true
		}
	}
	return false
}

// messages

// previewedMsg reports that a terminal previewer exited.
type previewedMsg struct{ err error }

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
	priHigh     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F7768E"))
	priMed      = lipgloss.NewStyle().Foreground(lipgloss.Color("#E0AF68"))
	priLow      = lipgloss.NewStyle().Foreground(lipgloss.Color("#7AA2F7"))
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
	help    viewport.Model

	cmdInput    textinput.Model     // the ":" command window
	cmdSel      int                 // selected suggestion
	cmdMoved    bool                // the selection was moved with ↑↓ since the last edit
	cmdQuick    bool                // opened as quick open (ctrl+p): enter takes the top match
	cmdComp     commands.Completion // completion for the current input
	cmdHist     []string            // command history, newest first
	cmdHistIdx  int                 // position while browsing history with ↑, or -1
	cmdHistPath string              // shared history file ("" disables saving)
	fmDrafts    map[string]string   // note key -> ":frontmatter" text that failed to apply

	tagFilter   string
	query       string
	allTasks    bool                   // Tasks tab: list every task instead of notes with tasks
	taskShow    string                 // Tasks tab, all-tasks view: "open", "done" or "all"
	taskWhen    string                 // all-tasks view: notes.WhenAny, WhenOverdue or WhenWeek
	pick        *pickView              // a list of notes from a command (:recent, :git status)
	fmOpen      bool                   // preview shows the full frontmatter (F toggles)
	recentPath  string                 // shared recent-notes file ("" disables)
	dueTask     *taskItem              // task whose due date the input prompt is editing
	links       *linkView              // b / f: show a note's backlinks or outgoing links
	results     map[string]notes.Match // kind/id -> match while searching
	searchMode  notes.SearchMode       // name, tags or text ("/" bar; tab cycles)
	searchInput textinput.Model
	total       int                         // notes on the tab before searching (for "12 of 340")
	sorts       map[notes.Kind]notes.SortBy // per-tab order ("s"), shared via view.json
	viewPath    string                      // view.json ("" disables saving)

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

	m := model{app: app, style: style, list: l, view: viewport.New(0, 0), help: viewport.New(0, 0), input: ti, cmdInput: newCmdInput(), cache: map[string]string{}, cmdHistIdx: -1}
	if p, err := commands.HistoryPath(); err == nil {
		m.cmdHistPath = p
		m.cmdHist = commands.LoadHistory(p)
	}
	if p, err := recent.Path(); err == nil {
		m.recentPath = p
	}
	m.taskShow = "open"
	m.searchMode = notes.SearchName
	m.searchInput = newSearchInput()
	m.sorts = map[notes.Kind]notes.SortBy{}
	if p, err := viewstate.Path(); err == nil {
		m.viewPath = p
		for k, v := range viewstate.Load(p).Sort {
			if sb, err := notes.ParseSort(v); err == nil && sb != "" {
				m.sorts[notes.Kind(k)] = sb
			}
		}
	}
	m.fmOpen = !app.Cfg.FrontmatterCollapsed
	m.reload()
	m.applyStartPage()
	return m
}

// setItemHeight makes list rows tall enough for the tallest note item
// (title plus its Description lines), and keeps two-line rows otherwise.
func (m *model) setItemHeight(items []list.Item) {
	h := 2
	for _, it := range items {
		if ni, ok := it.(noteItem); ok {
			h = max(h, 1+strings.Count(ni.Description(), "\n")+1)
		}
	}
	d := list.NewDefaultDelegate()
	d.SetHeight(h)
	m.list.SetDelegate(d)
}

func (m model) Init() tea.Cmd { return nil }

func key(n *notes.Note) string { return string(n.Kind) + "/" + n.ID }

// reload rebuilds the list for the current tab, filters and search.
func (m *model) reload() {
	sel := itemKey(m.list.SelectedItem())
	var items []list.Item
	var err error
	switch {
	case m.pick != nil:
		for _, c := range m.pick.items {
			if n, gerr := m.app.Store.Get(c.Kind, c.ID); gerr == nil {
				items = append(items, noteItem{n: *n, match: c.Detail})
			}
		}
	case m.links != nil:
		items, err = m.linkItems()
	case m.tab == tabTasks:
		if m.allTasks {
			// Every task, sorted by due date, then priority.
			var all []notes.TaskRef
			all, err = m.app.Store.AllTasks(m.taskFilter())
			for _, r := range all {
				if m.taskMatches(notes.NoteTasks{Note: r.Note}, &r.Task) {
					items = append(items, taskItem{n: r.Note, t: r.Task})
				}
			}
			break
		}
		var all []notes.NoteTasks
		all, err = m.app.Store.Tasks("")
		for _, nt := range all {
			if m.taskMatches(nt, nil) {
				items = append(items, taskNoteItem{nt})
			}
		}
	case m.tab == tabTags:
		if m.viewPath != "" {
			for _, x := range viewstate.Load(m.viewPath).Searches {
				items = append(items, savedItem{x})
			}
		}
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
		var res []notes.Match
		res, err = m.app.Store.Find(notes.FindQuery{Mode: m.searchMode, Text: m.query, Kind: kind, Tag: m.tagFilter, Sort: m.sortFor(kind)})
		m.results = map[string]notes.Match{}
		for _, r := range res {
			m.results[key(&r.Note)] = r
			it := noteItem{n: r.Note}
			if len(r.Lines) > 0 {
				it.match = strings.TrimSpace(r.Lines[0].Text)
			}
			items = append(items, it)
		}
		m.total = len(items)
		if m.query != "" {
			if all, lerr := m.app.Store.List(kind, notes.ListOpts{Tag: m.tagFilter}); lerr == nil {
				m.total = len(all)
			}
		} else {
			items = withMonthHeaders(items, m.sortFor(kind))
		}
	}
	if err != nil {
		m.setErr(err)
	}
	m.setItemHeight(items)
	m.list.SetItems(items)
	if sel != "" {
		for i, it := range items {
			if itemKey(it) == sel {
				m.list.Select(i)
				break
			}
		}
	}
	if m.list.Index() >= len(items) && len(items) > 0 {
		m.list.Select(len(items) - 1)
	}
	m.skipHeaders("down")
	m.preview = ""
	m.updatePreview()
}

// linkItems lists the notes for the b / f links view.
func (m *model) linkItems() ([]list.Item, error) {
	var items []list.Item
	of := m.links.of
	if m.links.back {
		bl, err := m.app.Store.Backlinks(of.Kind, of.ID)
		for _, b := range bl {
			if n, err := m.app.Store.Get(b.Note.Kind, b.Note.ID); err == nil {
				items = append(items, noteItem{n: *n})
			}
		}
		return items, err
	}
	ls, err := m.app.Store.Links(of.Kind, of.ID)
	seen := map[string]bool{}
	for _, l := range ls {
		k := string(l.Kind) + "/" + l.ID
		if seen[k] {
			continue
		}
		seen[k] = true
		if !l.Exists {
			items = append(items, missingItem{l})
			continue
		}
		if n, err := m.app.Store.Get(l.Kind, l.ID); err == nil {
			items = append(items, noteItem{n: *n})
		}
	}
	return items, err
}

// linkedFrom is the preview footer listing the notes that link to n.
func (m *model) linkedFrom(n *notes.Note) string {
	bl, err := m.app.Store.Backlinks(n.Kind, n.ID)
	if err != nil || len(bl) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n" + filterStyle.Render(fmt.Sprintf("  Linked from (%d)", len(bl))) + statusStyle.Render("  · b lists them") + "\n")
	for _, x := range bl {
		line := x.Lines[0].Text
		if r := []rune(line); len(r) > 60 {
			line = string(r[:59]) + "…"
		}
		fmt.Fprintf(&b, "  • %s %s\n", x.Note.Title, statusStyle.Render(line))
	}
	return b.String()
}

// selectKey moves the cursor to the note with key k, if listed.
func (m *model) selectKey(k string) {
	for i, it := range m.list.Items() {
		if n := noteOf(it); n != nil && key(n) == k {
			m.list.Select(i)
			break
		}
	}
	m.updatePreview()
}

// openLinks switches the list to n's backlinks (back) or outgoing links.
func (m *model) openLinks(n *notes.Note, back bool) {
	m.links = &linkView{back: back, of: *n}
	m.list.Select(0)
	m.reload()
	if back {
		m.setStatus(fmt.Sprintf("%d note(s) link to %s (esc closes)", len(m.list.Items()), n.Title))
	} else {
		m.setStatus(fmt.Sprintf("%s links to %d note(s) (esc closes)", n.Title, len(m.list.Items())))
	}
}

// showNote selects n in the list, switching to its tab (and dropping the
// tag filter and search if they hide it).
func (m *model) showNote(n *notes.Note) {
	m.clearViews()
	want := tabPages
	if n.Kind == notes.KindJournal {
		want = tabJournal
	}
	if m.tab != want {
		m.tab = want
		m.list.Select(0)
	}
	m.reload()
	k := key(n)
	if itemKey(m.list.SelectedItem()) != k && !m.hasItem(k) {
		m.tagFilter, m.query = "", ""
		m.reload()
	}
	m.selectKey(k)
}

func (m *model) hasItem(k string) bool {
	for _, it := range m.list.Items() {
		if itemKey(it) == k {
			return true
		}
	}
	return false
}

func (m *model) setErr(err error)   { m.status, m.isErr = err.Error(), true }
func (m *model) setStatus(s string) { m.status, m.isErr = s, false }

func (m *model) selected() *notes.Note { return noteOf(m.list.SelectedItem()) }

// noteOf returns a copy of the note behind a list item, if any.
func noteOf(it list.Item) *notes.Note {
	var n notes.Note
	switch v := it.(type) {
	case noteItem:
		n = v.n
	case taskNoteItem:
		n = v.nt.Note
	case taskItem:
		n = v.n
	default:
		return nil
	}
	return &n
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
	m.sizeHelp()
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
				out = m.frontmatterView(full) + out
			}
			m.cache[k] = out
		}
		content = out + m.linkedFrom(n)
	} else if it, ok := m.list.SelectedItem().(missingItem); ok {
		k = "missing:" + string(it.l.Kind) + "/" + it.l.ID
		content = statusStyle.Render(fmt.Sprintf("\n  %s/%s doesn't exist yet.\n\n  enter: create it and open it in your editor", it.l.Kind, it.l.ID))
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
		msg := "\n  Nothing here yet. Press n to create a note, T for today's journal."
		if m.tab == tabTasks {
			msg = "\n  No tasks found. Add \"- [ ] something\" to a note."
			if m.allTasks && (m.taskShow != "all" || m.taskWhen != notes.WhenAny) {
				msg += "\n  (filtered: " + m.taskFilterText() + ". a / w change it)"
			}
		}
		content = statusStyle.Render(msg)
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

func (m *model) editCmd(n *notes.Note) tea.Cmd { return m.editCmdAt(n, 0) }

// previewCmd opens n in the configured Markdown previewer. Terminal viewers
// (glow, mdcat…) take over the screen until they exit; windowed ones
// (katdown…) start alongside the TUI.
func (m *model) previewCmd(n *notes.Note) tea.Cmd {
	m.touchRecent(n)
	c, inTerminal, err := editor.PreviewCommand(m.app.Cfg.Previewer, m.app.Cfg.PreviewerMode, n.Path)
	if err != nil {
		m.setErr(err)
		return nil
	}
	if inTerminal {
		return tea.ExecProcess(c, func(err error) tea.Msg { return previewedMsg{err} })
	}
	if err := editor.StartDetached(c); err != nil {
		m.setErr(fmt.Errorf("previewer: %w", err))
		return nil
	}
	m.setStatus("opened " + key(n) + " in " + filepath.Base(c.Args[0]))
	return nil
}

// editCmdAt opens n in the editor at 1-based file line `line` (0 = top).
func (m *model) editCmdAt(n *notes.Note, line int) tea.Cmd {
	m.touchRecent(n)
	before := m.app.Store.ModTime(n.Kind, n.ID)
	c, err := editor.CommandAt(m.app.Cfg.Editor, n.Path, line)
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

	case cmdResultMsg:
		return m, m.applyResult(msg.line, msg.r, msg.err)
	case fmEditedMsg:
		m.afterFrontmatterEdit(msg)
		return m, nil
	case shellDoneMsg:
		// A non-zero exit just means the last command in the shell failed.
		var exit *exec.ExitError
		if msg.err != nil && !errors.As(msg.err, &exit) {
			m.setErr(fmt.Errorf("terminal: %w", msg.err))
		}
		m.cache = map[string]string{}
		m.reload()
		return m, nil
	case previewedMsg:
		if msg.err != nil {
			m.setErr(fmt.Errorf("previewer: %w", msg.err))
		}
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
		case modeHelp:
			return m.updateHelp(msg)
		case modeCommand:
			return m.updateCommand(msg)
		case modeSearch:
			return m.updateSearch(msg)
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
	case ":":
		return m, m.openCommand()
	case "?", "f1":
		m.mode = modeHelp
		m.sizeHelp()
		m.help.SetContent(helpBody())
		m.help.GotoTop()
		return m, nil
	case "tab", "right", "l":
		m.tab = (m.tab + 1) % tab(len(tabNames))
		m.clearViews()
		m.list.Select(0)
		m.reload()
		return m, nil
	case "shift+tab", "left", "h":
		m.tab = (m.tab + tab(len(tabNames)) - 1) % tab(len(tabNames))
		m.clearViews()
		m.list.Select(0)
		m.reload()
		return m, nil
	case "1", "2", "3", "4":
		m.tab = tab(msg.String()[0] - '1')
		m.clearViews()
		m.list.Select(0)
		m.reload()
		return m, nil
	case "esc":
		if m.pick != nil {
			m.pick = nil
			m.list.Select(0)
			m.setStatus(m.pickClosedStatus())
			m.reload()
			return m, nil
		}
		if m.links != nil {
			m.clearViews()
			m.list.Select(0)
			m.setStatus("links view closed")
			m.reload()
			return m, nil
		}
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
		return m, m.openSearch()
	case "ctrl+p":
		return m, m.openQuickOpen()
	case "s":
		switch m.tab {
		case tabPages:
			m.cycleSort(notes.KindPage)
		case tabJournal:
			m.cycleSort(notes.KindJournal)
		}
		return m, nil
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
		m.clearViews()
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
	case "v":
		if m.tab == tabTasks {
			m.allTasks = !m.allTasks
			m.list.Select(0)
			if m.allTasks {
				m.setStatus("all tasks (v: notes with tasks, a: show done)")
			} else {
				m.setStatus("notes with tasks (v: all tasks)")
			}
			m.reload()
		}
		return m, nil
	case "a":
		if m.tab == tabTasks && m.allTasks {
			m.taskShow = map[string]string{"open": "done", "done": "all", "all": "open"}[m.taskShow]
			m.reload()
			m.setStatus("tasks: " + m.taskFilterText())
		}
		return m, nil
	case "w":
		if m.tab == tabTasks && m.allTasks {
			m.taskWhen = map[string]string{notes.WhenAny: notes.WhenOverdue, notes.WhenOverdue: notes.WhenWeek, notes.WhenWeek: notes.WhenAny}[m.taskWhen]
			m.reload()
			m.setStatus("tasks: " + m.taskFilterText())
		}
		return m, nil
	case "F":
		m.fmOpen = !m.fmOpen
		m.cache = map[string]string{}
		m.preview = ""
		m.updatePreview()
		if m.fmOpen {
			m.setStatus("frontmatter shown")
		} else {
			m.setStatus("frontmatter collapsed")
		}
		return m, nil
	case "x", " ":
		if it, ok := m.list.SelectedItem().(taskItem); ok {
			n, err := m.app.Store.SetTask(it.n.Kind, it.n.ID, it.t.Line, !it.t.Done)
			if err != nil {
				m.setErr(err)
			} else if it.t.Done {
				m.setStatus("unchecked in " + key(n))
			} else {
				m.setStatus("checked in " + key(n))
			}
			m.reload()
		}
		return m, nil
	case "o":
		if n := m.selected(); n != nil {
			return m, m.previewCmd(n)
		}
		return m, nil
	case "b", "f":
		if n := m.selected(); n != nil {
			m.openLinks(n, msg.String() == "b")
		}
		return m, nil
	case "@":
		if it, ok := m.list.SelectedItem().(taskItem); ok {
			m.dueTask = &it
			return m, m.startInput(inDue, "Due (YYYY-MM-DD, today, tom, fri, +3d; empty clears): ", it.t.Meta[notes.MetaDue])
		}
		return m, nil
	case "p":
		if it, ok := m.list.SelectedItem().(taskItem); ok {
			next := nextPriority(it.t.Meta[notes.MetaPri])
			if _, err := m.app.Store.SetTaskMeta(it.n.Kind, it.n.ID, it.t.Line, notes.MetaPri, next); err != nil {
				m.setErr(err)
			} else if next == "" {
				m.setStatus("priority cleared")
			} else {
				m.setStatus("priority " + next)
			}
			m.reload()
		}
		return m, nil
	case "e", "enter":
		if m.pick != nil && msg.String() == "enter" {
			if n := m.selected(); n != nil {
				m.pick = nil
				m.touchRecent(n)
				m.showNote(n)
				m.setStatus("opened " + key(n) + " (enter edits)")
			}
			return m, nil
		}
		switch it := m.list.SelectedItem().(type) {
		case missingItem:
			n, _, err := m.app.Store.Ensure(it.l.Kind, it.l.ID, it.l.Text)
			if err != nil {
				m.setErr(err)
				return m, nil
			}
			m.setStatus("created " + key(n))
			m.reload()
			m.selectKey(key(n))
			return m, m.editCmd(n)
		case taskItem:
			n := it.n
			return m, m.editCmdAt(&n, m.app.Store.FileLine(n.Kind, n.ID, it.t.Line))
		case taskNoteItem:
			n := it.nt.Note
			line := 0
			if t, ok := firstOpen(it.nt.Tasks); ok {
				line = m.app.Store.FileLine(n.Kind, n.ID, t.Line)
			}
			return m, m.editCmdAt(&n, line)
		}
		if it, ok := m.list.SelectedItem().(savedItem); ok {
			m.applySearch(it.s.Mode, it.s.Text)
			return m, nil
		}
		if it, ok := m.list.SelectedItem().(tagItem); ok {
			m.tagFilter = it.t.Tag
			m.tab = tabPages
			m.clearViews()
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
	m.skipHeaders(msg.String())
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
	case inTag:
		m.tagFilter = strings.ToLower(strings.TrimPrefix(val, "#"))
		m.list.Select(0)
		m.reload()
	case inDue:
		it := m.dueTask
		m.dueTask = nil
		if it == nil {
			return m, nil
		}
		if _, err := m.app.Store.SetTaskMeta(it.n.Kind, it.n.ID, it.t.Line, notes.MetaDue, val); err != nil {
			m.setErr(err)
			return m, nil
		}
		if val == "" {
			m.setStatus("due date cleared")
		} else {
			m.setStatus("due date set")
		}
		m.reload()
	case inRename:
		if n := m.selected(); n != nil && val != "" {
			r, relinked, err := m.app.Store.Rename(n.Kind, n.ID, val)
			if err != nil {
				m.setErr(err)
				return m, nil
			}
			recent.Rename(m.recentPath, key(n), key(r)) //nolint:errcheck // a convenience
			msg := "renamed to " + key(r)
			if len(relinked) > 0 {
				msg += fmt.Sprintf(" · updated links in %d note(s)", len(relinked))
			}
			m.cache = map[string]string{}
			m.setStatus(msg)
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
			recent.Forget(m.recentPath, key(n)) //nolint:errcheck // a convenience
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
		filters = append(filters, fmt.Sprintf("%s %q · %d of %d", m.searchMode, m.query, len(m.noteItems()), m.total))
	}
	if m.pick != nil {
		filters = append(filters, m.pick.title)
	}
	if m.tab == tabTasks && m.allTasks {
		filters = append(filters, m.taskFilterText())
	}
	if m.links != nil {
		if m.links.back {
			filters = append(filters, "linking to "+m.links.of.Title)
		} else {
			filters = append(filters, "linked from "+m.links.of.Title)
		}
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
	case modeCommand:
		bottom = m.commandInputView()
		body = overlayBottom(body, m.commandDropdown())
	case modeSearch:
		bottom = m.searchBarView()
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
	if m.mode == modeHelp {
		return m.helpView()
	}
	help := statusStyle.Render(": commands · ? help · n new · enter/e edit · o preview · r rename · d delete · T today · / search · t tag · tab switch · J/K scroll · q quit")
	if m.tab == tabTasks {
		view := "v all tasks"
		if m.allTasks {
			view = "v notes · a done · x check · @ due · p priority"
		}
		help = statusStyle.Render(": commands · ? help · enter edit at task · " + view + " · / search · t tag · tab switch · q quit")
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, filterLine, body, bottom, help)
}

// help popup

var (
	helpBox     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#7D56F4")).Padding(0, 1)
	helpBanner  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#7D56F4")).Padding(0, 1)
	helpVersion = lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("#E0AF68"))
	helpSection = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#9D7DF8"))
	helpKey     = lipgloss.NewStyle().Bold(true).Width(16)
	helpCmd     = lipgloss.NewStyle().Bold(true)
)

type helpEntry struct{ key, desc string }

var helpSections = []struct {
	title   string
	entries []helpEntry
}{
	{"Navigation", []helpEntry{
		{"↑/↓  j/k", "move selection"},
		{"tab  →  l", "next tab"},
		{"shift+tab ← h", "previous tab"},
		{"1 2 3 4", "Pages / Journal / Tags / Tasks"},
		{"J / K", "scroll preview (also pgdn/pgup, ctrl+d/u)"},
	}},
	{"Notes", []helpEntry{
		{"n", "new page (or journal entry on the Journal tab)"},
		{"enter / e", "edit in $EDITOR"},
		{"r", "rename"},
		{"d / delete", "delete (asks to confirm)"},
		{"T", "open today's journal"},
		{"R", "reload from disk"},
		{"F", "show or collapse the frontmatter above the preview"},
	}},
	{"Tasks tab", []helpEntry{
		{"v", "switch: notes with tasks / all tasks"},
		{"enter / e", "edit the note at the (first open) task line"},
		{"x / space", "check or uncheck the selected task"},
		{"a", "all-tasks view: cycle open → done → all"},
		{"w", "all-tasks view: cycle any date → overdue → due this week"},
		{"@", "all-tasks view: set the due date (today, tom, fri, +3d, …)"},
		{"p", "all-tasks view: cycle priority (high → med → low → none)"},
	}},
	{"Finding", []helpEntry{
		{"/", "live search; tab switches Name (fuzzy title/filename) · Tags · Text"},
		{"ctrl+p", "quick open: jump to any note by name (recent first)"},
		{"s", "sort Pages/Journal: modified · created · title (remembered)"},
		{"t", "filter by tag"},
		{"enter", "on the Tags tab: filter by that tag, or run a 🔎 saved search"},
		{"esc", "clear search and tag filter (or close the links / :recent list)"},
		{"o", "open the selected note in the Markdown previewer (config: previewer)"},
		{"b", "notes that link to the selected note (backlinks)"},
		{"f", "notes the selected note links to; enter on (missing) creates it"},
	}},
	{"General", []helpEntry{
		{":", "command window: :open, :tag add, :set, :frontmatter… (tab completes, ↑ history)"},
		{"? / F1", "toggle this help"},
		{"q / ctrl+c", "quit"},
	}},
}

func helpBody() string {
	var b strings.Builder
	for i, sec := range helpSections {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(helpSection.Render(sec.title) + "\n")
		for _, e := range sec.entries {
			b.WriteString("  " + helpKey.Render(e.key) + e.desc + "\n")
		}
	}
	// The same command list as the GUI's help (from the shared catalog).
	b.WriteString("\n" + helpSection.Render("Commands (:)") + "\n")
	for _, c := range commands.Catalog(commands.AppTUI) {
		b.WriteString("  " + helpCmd.Render(":"+c.Usage()) + "\n      " + c.Desc + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func helpLines() int { return strings.Count(helpBody(), "\n") + 1 }

// sizeHelp fits the help viewport to its content, capped by the window.
func (m *model) sizeHelp() {
	w := 64
	if m.width-6 < w {
		w = m.width - 6
	}
	// box border (2) + banner (1) + blank (1) + footer (1) + margin (2)
	h := m.height - 7
	if n := helpLines(); n < h {
		h = n
	}
	if w < 10 {
		w = 10
	}
	if h < 1 {
		h = 1
	}
	m.help.Width, m.help.Height = w, h
}

func (m model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "?", "f1", "esc", "q", "enter":
		m.mode = modeNormal
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "g", "home":
		m.help.GotoTop()
		return m, nil
	case "G", "end":
		m.help.GotoBottom()
		return m, nil
	}
	var cmd tea.Cmd
	m.help, cmd = m.help.Update(msg) // j/k, arrows, pgup/pgdn, ctrl+u/d
	return m, cmd
}

func (m model) helpView() string {
	w := m.help.Width
	banner := helpBanner.Width(w).Render("KatNotes — keyboard help")
	ver := helpVersion.Width(w).Align(lipgloss.Right).Render("katnotes " + m.app.Version)
	foot := "esc/? close"
	if !(m.help.AtTop() && m.help.AtBottom()) {
		foot = fmt.Sprintf("↑/↓ scroll · %3.0f%% · ", m.help.ScrollPercent()*100) + foot
	}
	box := helpBox.Render(lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinVertical(lipgloss.Left, banner, ver),
		m.help.View(),
		statusStyle.Width(w).Align(lipgloss.Right).Render(foot),
	))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}
