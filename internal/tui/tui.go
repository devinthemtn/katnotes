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
	"github.com/charmbracelet/x/ansi"

	"github.com/devinthemtn/katnotes/internal/cli"
	"github.com/devinthemtn/katnotes/internal/commands"
	"github.com/devinthemtn/katnotes/internal/config"
	"github.com/devinthemtn/katnotes/internal/editor"
	"github.com/devinthemtn/katnotes/internal/luacmd"
	"github.com/devinthemtn/katnotes/internal/notes"
	"github.com/devinthemtn/katnotes/internal/recent"
	"github.com/devinthemtn/katnotes/internal/theme"
	"github.com/devinthemtn/katnotes/internal/viewstate"
)

// Run starts the TUI.
func Run(app *cli.App) error {
	termDark = lipgloss.HasDarkBackground()
	t, themeErr := loadTheme(app.Cfg.ThemeFor(commands.AppTUI))
	applyTheme(t)
	style := glamourStyle()
	if dir := theme.Dir(); dir != "" {
		theme.WriteDefaults(dir) //nolint:errcheck // the files are starting points
	}
	if dir := commands.TemplatesDir(); dir != "" {
		notes.WriteExampleTemplate(dir) //nolint:errcheck // the example is a convenience
	}
	if d, err := config.Dir(); err == nil {
		luacmd.WriteExamples(d) //nolint:errcheck // the examples are a convenience
	}
	// Sync failures go to the status line, not over the screen.
	app.Store.OnTodoError = nil
	m := newModel(app, style)
	if themeErr != nil {
		m.setErr(themeErr)
	}
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if app.Cfg.TUIMouse {
		opts = append(opts, tea.WithMouseCellMotion())
	}
	_, err := tea.NewProgram(m, opts...).Run()
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
	modeImage  // :image file picker
)

type inputPurpose int

const (
	inNew inputPurpose = iota
	inTag
	inRename
	inDue
	inTags
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
	return t + todoMark(&i.n)
}

// todoMark goes after the title of a note with tasks linked to todo.txt
// (katTasks): " ⇄ ✓" when all its open or linked tasks are, else " ⇄ n/m";
// "" when none are.
func todoMark(n *notes.Note) string {
	if n.TodoLinks == 0 {
		return ""
	}
	if n.TodoLinks >= n.TodoTasks {
		return " ⇄ ✓"
	}
	return fmt.Sprintf(" ⇄ %d/%d", n.TodoLinks, n.TodoTasks)
}

// Description is the lines under the title (the GUI sidebar shows the same):
// pages get "created │ m: modified" then tags; journal entries their date and
// tags on one line. A search hit adds its matching line.
func (i noteItem) Description() string {
	var lines []string
	if i.n.Kind == notes.KindPage {
		const day = "Jan 02 2006"
		lines = append(lines, i.n.Created.Format(day)+"  │  m: "+i.n.Updated.Format(day))
		lines = append(lines, tagsLine(i.n.Tags))
	} else {
		line := i.n.ID
		if len(i.n.Tags) > 0 {
			// The tags come last so their colour doesn't end the row's own.
			line += " · " + tagsLine(i.n.Tags)
		}
		lines = append(lines, line)
	}
	if i.match != "" {
		lines = append(lines, "…"+i.match)
	}
	return strings.Join(lines, "\n")
}

// tagsLine is tagsText in the tag colour.
func tagsLine(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	return tagStyle.Render(tagsText(tags))
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

func (i taskNoteItem) Title() string { return i.nt.Note.Title + todoMark(&i.nt.Note) }
func (i taskNoteItem) Description() string {
	parts := []string{fmt.Sprintf("%d/%d done", i.nt.Done, i.nt.Open+i.nt.Done)}
	if t, ok := firstOpen(i.nt.Tasks); ok {
		parts = append(parts, "☐ "+t.Text)
	} else {
		parts = append(parts, notes.Key(i.nt.Note.Kind, i.nt.Note.ID))
	}
	// The bar sets its own colours, so the text after it is styled too.
	return progressBar(i.nt.Done, i.nt.Open+i.nt.Done, 6) + " " + statusStyle.Render(strings.Join(parts, " · "))
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
	indent := strings.Repeat(" ", min(i.t.Indent, 8))
	if i.t.Done {
		return indent + doneStyle.Render(box+text)
	}
	return indent + box + priMark(i.t) + text
}
func (i taskItem) Description() string {
	var parts []string
	if i.t.Meta[notes.MetaLink] != "" {
		parts = append(parts, "⇄ katTasks")
	}
	if s, ok := i.t.Meta[notes.MetaStart]; ok && !i.t.Done && s > time.Now().Format(notes.DateLayout) {
		parts = append(parts, "starts "+s)
	}
	parts = append(parts, fmt.Sprintf("%s · %s/%s:%d", i.n.Title, i.n.Kind, i.n.ID, i.t.Line+1))
	rest := strings.Join(parts, " · ")
	if d := dueLabel(i.t, time.Now()); d != "" {
		// A badge sets its own colours, so the text after it is styled too.
		return d + statusStyle.Render(" · "+rest)
	}
	return rest
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
		return badgeOverdue.Render("overdue " + d)
	case d == today:
		return badgeToday.Render("due today")
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
	if p == "" {
		return notes.Priorities[0]
	}
	for i, x := range notes.Priorities {
		if x == p && i+1 < len(notes.Priorities) {
			return notes.Priorities[i+1]
		}
	}
	return ""
}

// itemKey identifies a list item across reloads.
func itemKey(it list.Item) string {
	switch v := it.(type) {
	case noteItem:
		return v.n.Key()
	case taskNoteItem:
		return v.nt.Note.Key()
	case taskItem:
		return fmt.Sprintf("%s#%d", v.n.Key(), v.t.Line)
	case missingItem:
		return "missing:" + notes.Key(v.l.Kind, v.l.ID)
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

	todoStamp string // todo.txt/done.txt stamp at the last sync
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
	img         *imagePicker        // :image file picker, while open
	imgDir      string              // folder the last image was picked from

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
	graph       *graphView             // gn / :graph: the notes around a note, or every note by links
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
	pending string // first key of a two-key sequence (g, d, Z)

	inputTitle  string      // the input prompt's question, shown as its dialog title
	previewPref previewPref // P: show or hide the preview pane (auto: by width)

	focusPreview bool // tab: keys scroll the preview instead of moving in the list

	bodyLinks []notes.BodyLink // the previewed note's links (n / N cycle them)
	linkedBy  []notes.Backlink // notes linking to it, cycled after bodyLinks
	linkIdx   int              // highlighted link (bodyLinks then linkedBy), or -1
	linksKey  string           // note (and version) bodyLinks was read from
	back      []string         // notes left by following links (backspace)

	mouse        bool      // tui_mouse: the mouse is on (for the hints)
	rowH         int       // lines per list row (for mouse clicks)
	lastClickRow int       // row (or suggestion) last clicked, for double clicks
	lastClick    time.Time // when
}

// previewPref is whether the preview pane shows: by window width, or as P
// last set it.
type previewPref int

const (
	previewAuto previewPref = iota
	previewOn
	previewOff
)

// narrowWidth is the window width below which the preview hides (auto).
const narrowWidth = 80

// showPreview reports whether the preview pane is drawn.
func (m *model) showPreview() bool {
	switch m.previewPref {
	case previewOn:
		return true
	case previewOff:
		return false
	}
	return m.width >= narrowWidth
}

func newModel(app *cli.App, style string) model {
	l := list.New(nil, newDelegate(), 0, 0)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()
	// updateNormal handles paging and gg itself; drop the list's own
	// b/f/d/u/g bindings so those letters don't page the list.
	l.KeyMap.PrevPage.SetEnabled(false)
	l.KeyMap.NextPage.SetEnabled(false)
	l.KeyMap.GoToStart.SetKeys("home")

	ti := textinput.New()
	ti.CharLimit = 200

	m := model{app: app, style: style, list: l, view: viewport.New(0, 0), help: viewport.New(0, 0), input: ti, cmdInput: newCmdInput(), cache: map[string]string{}, cmdHistIdx: -1, linkIdx: -1}
	if p, err := commands.HistoryPath(); err == nil {
		m.cmdHistPath = p
		m.cmdHist = commands.LoadHistory(p)
	}
	if p, err := recent.Path(); err == nil {
		m.recentPath = p
	}
	m.taskShow = showOpen
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
	m.mouse = app.Cfg.TUIMouse
	todoStatus := m.syncTodo()
	m.reload()
	if app.OpenNote != "" {
		if n := m.noteByKey(app.OpenNote); n != nil {
			m.showNote(n)
		} else {
			m.setErr(fmt.Errorf("--open: no note %s", app.OpenNote))
		}
	} else {
		m.applyStartPage()
	}
	if todoStatus != "" && m.status == "" {
		m.setStatus(todoStatus)
	}
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
	d := newDelegate()
	d.SetHeight(h)
	m.list.SetDelegate(d)
	m.rowH = h
}

func (m model) Init() tea.Cmd { return m.todoTick() }

// todoTickMsg polls todo.txt so tasks checked in katTasks show up while the
// TUI is open.
type todoTickMsg struct{}

const todoPoll = 2 * time.Second

func (m *model) todoTick() tea.Cmd {
	if m.app.Store.Todo == nil {
		return nil
	}
	return tea.Tick(todoPoll, func(time.Time) tea.Msg { return todoTickMsg{} })
}

// checkTodo syncs when todo.txt or done.txt changed since the last sync.
// It waits while a prompt is open, so the list doesn't move under it.
func (m *model) checkTodo() {
	if m.mode != modeNormal || m.app.Store.Todo == nil || m.app.Store.Todo.Stamp() == m.todoStamp {
		return
	}
	if s := m.syncTodo(); s != "" {
		m.resetCache()
		m.reload()
		m.setStatus(s)
	}
}

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
	case m.graph != nil:
		items, err = m.graphItems()
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
			m.results[r.Note.Key()] = r
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
		k := notes.Key(l.Kind, l.ID)
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

// linkedFrom is the preview footer listing the notes that link to the
// previewed note (m.linkedBy). It also returns the footer line of the one
// n / N highlighted, or -1.
func (m *model) linkedFrom() (string, int) {
	bl := m.linkedBy
	if len(bl) == 0 {
		return "", -1
	}
	sel, line := m.linkIdx-len(m.bodyLinks), -1
	var b strings.Builder
	b.WriteString("\n" + filterStyle.Render(fmt.Sprintf("  Linked from (%d)", len(bl))) + statusStyle.Render("  · gb lists them") + "\n")
	for i, x := range bl {
		text := ""
		if len(x.Lines) > 0 {
			text = notes.PlainLinks(x.Lines[0].Text)
		}
		if r := []rune(text); len(r) > 60 {
			text = string(r[:59]) + "…"
		}
		title := x.Note.Title
		if i == sel {
			line = i + 2
			mark := lipgloss.NewStyle().Bold(true).Foreground(accentColor)
			title = mark.Render("▶") + title + mark.Render("◀")
		}
		fmt.Fprintf(&b, "  • %s %s\n", title, statusStyle.Render(text))
	}
	return b.String(), line
}

// selectKey moves the cursor to the note with key k, if listed.
func (m *model) selectKey(k string) {
	for i, it := range m.list.Items() {
		if n := noteOf(it); n != nil && n.Key() == k {
			m.list.Select(i)
			break
		}
	}
	m.updatePreview()
}

// openLinks switches the list to n's backlinks (back) or outgoing links.
func (m *model) openLinks(n *notes.Note, back bool) {
	m.graph = nil
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
	k := n.Key()
	if !m.hasItem(k) {
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

func (m *model) setErr(err error) { m.status, m.isErr = err.Error(), true }

// setStatus shows s, or a todo.txt sync failure from the last save.
func (m *model) setStatus(s string) {
	if err := m.app.Store.TakeTodoError(); err != nil {
		m.setErr(err)
		return
	}
	m.status, m.isErr = s, false
}

// syncTodo pulls tasks checked in todo.txt (katTasks) into the notes when
// [todo_txt] is on. It returns a status line, empty when nothing changed.
func (m *model) syncTodo() string {
	if m.app.Store.Todo == nil {
		return ""
	}
	changes, err := m.app.Store.SyncTodo(false)
	m.todoStamp = m.app.Store.Todo.Stamp()
	if err != nil {
		return "todo.txt: " + err.Error()
	}
	if len(changes) == 0 {
		return ""
	}
	return "todo.txt: " + commands.SummarizeTodo(changes)
}

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
	bodyH := m.height - 4 // tabs + filter line + status + help
	if bodyH < 3 {
		bodyH = 3
	}
	// Both panes are boxed: the list (width listW) and, with a space
	// between, the preview (width pw), together filling the window.
	listW, pw := m.width, 0
	if m.showPreview() {
		listW = max(m.width*2/5, 28)
		pw = max(m.width-listW-1, 12)
	}
	m.listW = listW
	if !m.showPreview() {
		m.focusPreview = false
	}
	m.list.SetSize(max(listW-3, 1), bodyH-2) // box border + left margin
	headerWidth = m.list.Width()
	if pw == 0 {
		pw = m.width // hidden: the renderer still needs a width
	}
	m.view.Width, m.view.Height = pw-2, bodyH-2
	m.sizeHelp()
	r, err := glamour.NewTermRenderer(glamour.WithStyles(glamourConfig()), glamour.WithWordWrap(m.view.Width-2))
	if err == nil {
		m.render = r
		m.resetCache()
	}
	m.preview = ""
	m.updatePreview()
}

func (m *model) updatePreview() {
	var content, k string
	if n := m.selected(); n != nil && m.graph != nil {
		k = fmt.Sprintf("graph:%s:%d:%s", n.Key(), m.graph.depth, n.Updated)
		if k == m.preview {
			return
		}
		out, err := graphTree(m.app.Store, n.Key(), m.graph.depth)
		if err != nil {
			out = errStyle.Render(err.Error())
		}
		content = out
	} else if n := m.selected(); n != nil {
		m.syncLinks(n)
		m.syncLinkedBy(n)
		k = n.Path + n.Updated.String()
		if m.linkIdx >= 0 {
			k += fmt.Sprintf("#link%d", m.linkIdx)
		}
		if k == m.preview {
			return
		}
		if m.linkIdx >= 0 {
			// A highlighted link: rendered afresh (not cached) and scrolled to.
			out, line := m.renderWithLink(n)
			foot, fline := m.linkedFrom()
			if fline >= 0 {
				line = strings.Count(out, "\n") + fline
			}
			m.preview = k
			m.view.SetContent(out + foot)
			m.scrollToLine(line)
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
					if r, err := m.render.Render(previewBody(full.Body, notes.BodyLinks(full), -1)); err == nil {
						out = applyHighlights(r)
					}
				}
				out = m.frontmatterView(full) + out
			}
			m.cache[k] = out
		}
		foot, _ := m.linkedFrom()
		content = out + foot
	} else if it, ok := m.list.SelectedItem().(missingItem); ok {
		k = "missing:" + notes.Key(it.l.Kind, it.l.ID)
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
		content = m.emptyView()
	}
	m.preview = k
	m.view.SetContent(content)
	m.view.GotoTop()
}

// emptyView fills the preview when nothing is selected: a short note on
// why, centred, with the keys that help.
func (m *model) emptyView() string {
	lines := []string{catStyle.Render("=^.^="), ""}
	switch {
	case m.tab == tabTasks:
		lines = append(lines, "No tasks found.", statusStyle.Render(`Add "- [ ] something" to a note.`))
		if m.allTasks && (m.taskShow != showAll || m.taskWhen != notes.WhenAny) {
			lines = append(lines, "", statusStyle.Render("filtered: "+m.taskFilterText()), keyHints("a", "open/done", "w", "dates"))
		}
	case m.query != "" || m.tagFilter != "":
		lines = append(lines, "No notes match.", "", keyHints("esc", "clear the search and tag filter"))
	default:
		lines = append(lines, "Nothing here yet.", "", keyHints("n", "new note", "J", "today's journal"))
	}
	block := lipgloss.JoinVertical(lipgloss.Center, lines...)
	return lipgloss.Place(m.view.Width, m.view.Height, lipgloss.Center, lipgloss.Center, block)
}

// startInput asks for a value in a dialog; prompt is its question.
func (m *model) startInput(p inputPurpose, prompt, value string) tea.Cmd {
	m.mode, m.purpose = modeInput, p
	m.inputTitle = strings.TrimSuffix(strings.TrimSpace(prompt), ":")
	m.input.Prompt = "› "
	m.input.Width = max(m.dialogWidth()-4, 10)
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
	m.setStatus("opened " + n.Key() + " in " + filepath.Base(c.Args[0]))
	return nil
}

// externalCmd opens n with tui_open_with (else open_with), or the system
// default app when neither is set.
func (m *model) externalCmd(n *notes.Note) tea.Cmd {
	if cmdline := m.app.Cfg.OpenWithFor(commands.AppTUI); strings.TrimSpace(cmdline) != "" {
		return m.openWithCmd(n, cmdline)
	}
	m.touchRecent(n)
	if err := editor.StartDetached(editor.SystemOpenCommand(n.Path)); err != nil {
		m.setErr(fmt.Errorf("external: %w (set tui_open_with or open_with)", err))
		return nil
	}
	m.setStatus("opened " + n.Key() + " in the default app")
	return nil
}

// openWithCmd opens n once with cmdline instead of the configured editor.
// Terminal programs take over the screen, then the note reloads as after
// an edit; window programs start alongside the TUI.
func (m *model) openWithCmd(n *notes.Note, cmdline string) tea.Cmd {
	m.touchRecent(n)
	before := m.app.Store.ModTime(n.Kind, n.ID)
	c, inTerminal, err := editor.OpenWithCommand(cmdline, n.Path)
	if err != nil {
		m.setErr(fmt.Errorf("open-with: %w", err))
		return nil
	}
	if inTerminal {
		return tea.ExecProcess(c, func(err error) tea.Msg { return editedMsg{n: n, before: before, err: err} })
	}
	if err := editor.StartDetached(c); err != nil {
		m.setErr(fmt.Errorf("open-with: %w", err))
		return nil
	}
	m.setStatus("opened " + n.Key() + " in " + editor.ProgramName(c.Args[0]))
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

	case todoTickMsg:
		m.checkTodo()
		return m, m.todoTick()

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
		m.resetCache()
		m.reload()
		return m, nil
	case configEditedMsg:
		if msg.err != nil {
			m.setErr(fmt.Errorf("editor: %w", msg.err))
		} else if msg.dir {
			m.setStatus("closed the config folder · config.toml changes apply when KatNotes restarts")
		} else {
			m.setStatus("closed config.toml · changes apply when KatNotes restarts")
		}
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
			m.setStatus("saved " + n.Key())
		}
		m.reload()
		return m, nil

	case tea.MouseMsg:
		return m.updateMouse(msg)

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
		case modeImage:
			return m.updateImage(msg)
		}
		return m.updateNormal(msg)
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// Keys that start a two-key sequence, vim style.
var seqPrefixes = map[string]bool{"g": true, "d": true, "Z": true}

func (m model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if m.pending != "" {
		seq := m.pending + k
		m.pending = ""
		m.setStatus("")
		if k == "ctrl+c" {
			return m, tea.Quit
		}
		return m.updateSeq(seq)
	}
	if seqPrefixes[k] {
		m.pending = k
		m.setStatus(k + "…")
		return m, nil
	}
	if m.focusPreview {
		if done, cmd := m.previewKey(k); done {
			return m, cmd
		}
	}
	if m.graph != nil && (k == "+" || k == "=" || k == "-") {
		if k == "-" {
			m.graphDepth(-1)
		} else {
			m.graphDepth(1)
		}
		return m, nil
	}
	switch k {
	case "q", "ctrl+c":
		return m, tea.Quit
	case ":":
		return m, m.openCommand()
	case "?", "f1":
		m.openHelp()
		return m, nil
	case "tab":
		m.setFocusPreview(true)
		return m, nil
	case "right", "l":
		m.switchTab((m.tab + 1) % tab(len(tabNames)))
		return m, nil
	case "shift+tab", "left", "h":
		m.switchTab((m.tab + tab(len(tabNames)) - 1) % tab(len(tabNames)))
		return m, nil
	case "1", "2", "3", "4":
		m.switchTab(tab(msg.String()[0] - '1'))
		return m, nil
	case "esc":
		if m.pick != nil {
			m.pick = nil
			m.list.Select(0)
			m.setStatus("list closed")
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
		if m.graph != nil {
			m.clearViews()
			m.list.Select(0)
			m.setStatus("graph closed")
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
			return m, m.startInput(inNew, "New journal date ("+notes.DatePattern(m.app.Store.DateFormat)+", empty = today): ", "")
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
		if n := m.selected(); n != nil {
			return m, m.startInput(inTags, "Tags for "+n.Title+" (comma separated, empty clears): ", strings.Join(n.Tags, ", "))
		}
		m.setErr(fmt.Errorf("tags: select a note first"))
		return m, nil
	case "L":
		return m, m.openLinkPicker()
	case "J", "ctrl+j":
		n, created, err := m.app.Store.Today()
		if err != nil {
			m.setErr(err)
			return m, nil
		}
		if created {
			m.setStatus("created " + n.Key())
		}
		m.tab = tabJournal
		m.clearViews()
		m.reload()
		m.selectKey(n.Key())
		return m, m.editCmd(n)
	case "r":
		if n := m.selected(); n != nil {
			return m, m.startInput(inRename, "Rename to: ", n.Title)
		}
	case "delete":
		m.confirmDelete()
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
			m.taskShow = map[string]string{showOpen: showDone, showDone: showAll, showAll: showOpen}[m.taskShow]
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
	case "P":
		if m.showPreview() {
			m.previewPref = previewOff
			m.setStatus("preview hidden (P shows it)")
		} else {
			m.previewPref = previewOn
			m.setStatus("preview shown")
		}
		m.layout()
		return m, nil
	case "F":
		m.fmOpen = !m.fmOpen
		m.resetCache()
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
				m.setStatus("unchecked in " + n.Key())
			} else {
				m.setStatus("checked in " + n.Key())
			}
			m.reload()
		}
		return m, nil
	case "K":
		if it, ok := m.list.SelectedItem().(taskItem); ok {
			if _, err := m.app.Store.PushTask(it.n.Kind, it.n.ID, it.t.Line); err != nil {
				m.setErr(err)
			} else {
				m.setStatus("linked to todo.txt (katTasks)")
			}
			m.reload()
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
		} else if n := m.selected(); n != nil {
			return m, m.previewCmd(n)
		}
		return m, nil
	case "e", "enter":
		return m, m.enter(msg.String() == "enter")
	case "ctrl+f":
		m.list.NextPage()
		m.skipHeaders("down")
		m.updatePreview()
		return m, nil
	case "ctrl+b":
		m.list.PrevPage()
		m.skipHeaders("down")
		m.updatePreview()
		return m, nil
	case "R":
		m.reloadAll()
		return m, nil
	case "backspace":
		m.goBack()
		return m, nil
	case "ctrl+d", "ctrl+u":
		m.halfPage(k == "ctrl+d")
		return m, nil
	case "ctrl+e":
		m.scrollPreview(1)
		return m, nil
	case "ctrl+y":
		m.scrollPreview(-1)
		return m, nil
	case "]":
		m.scrollPreview(wheelStep)
		return m, nil
	case "[":
		m.scrollPreview(-wheelStep)
		return m, nil
	case "pgdown":
		m.view.HalfPageDown()
		return m, nil
	case "pgup":
		m.view.HalfPageUp()
		return m, nil
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	m.skipHeaders(msg.String())
	m.updatePreview()
	return m, cmd
}

// updateSeq runs a two-key sequence (gg, gt, dd, ZZ, …). An unknown one,
// or a prefix followed by esc, does nothing.
func (m model) updateSeq(seq string) (tea.Model, tea.Cmd) {
	switch seq {
	case "gg":
		if m.focusPreview {
			m.view.GotoTop()
			break
		}
		m.list.Select(0)
		m.skipHeaders("down")
		m.updatePreview()
	case "gt":
		m.switchTab((m.tab + 1) % tab(len(tabNames)))
	case "gT":
		m.switchTab((m.tab + tab(len(tabNames)) - 1) % tab(len(tabNames)))
	case "gx":
		if n := m.selected(); n != nil {
			return m, m.previewCmd(n)
		}
	case "gf", "gb":
		if n := m.selected(); n != nil {
			m.openLinks(n, seq == "gb")
		}
	case "gn":
		if n := m.selected(); n != nil {
			m.openGraph(n)
		}
	case "dd":
		m.confirmDelete()
	case "ZZ", "ZQ":
		return m, tea.Quit
	}
	return m, nil
}

// setFocusPreview moves the keyboard focus to the preview (on) or back to
// the list. The preview can't take it while hidden.
func (m *model) setFocusPreview(on bool) {
	if on && !m.showPreview() {
		m.setStatus("the preview is hidden (P shows it)")
		return
	}
	if m.focusPreview == on {
		return
	}
	m.focusPreview = on
	if on {
		m.setStatus("preview focused: j/k scroll · tab or esc back to the list")
	} else {
		m.setStatus("")
	}
}

// previewKey handles a key while the preview has the focus: the movement
// keys scroll it, and tab / esc hand the focus back to the list. Other keys
// (e edit, p preview, :, ?…) do what they always do, so it returns false.
func (m *model) previewKey(k string) (bool, tea.Cmd) {
	switch k {
	case "n":
		m.cycleLink(1)
	case "N":
		m.cycleLink(-1)
	case "b":
		m.goBack()
	case "enter":
		if m.linkIdx < 0 {
			return false, nil // no link highlighted: edit the note as usual
		}
		return true, m.openSelectedLink()
	case "tab", "esc":
		m.setFocusPreview(false)
	case "j", "down", "ctrl+e":
		m.scrollPreview(1)
	case "k", "up", "ctrl+y":
		m.scrollPreview(-1)
	case "ctrl+d", "pgdown", " ", "ctrl+f":
		m.view.HalfPageDown()
	case "ctrl+u", "pgup", "ctrl+b":
		m.view.HalfPageUp()
	case "G", "end":
		m.view.GotoBottom()
	case "home":
		m.view.GotoTop()
	default:
		return false, nil
	}
	return true, nil
}

// scrollPreview scrolls the preview n lines (up when n < 0).
func (m *model) scrollPreview(n int) {
	if n < 0 {
		m.view.ScrollUp(-n)
	} else {
		m.view.ScrollDown(n)
	}
}

// halfPage moves the list selection half a screen, like vim's ctrl+d/u.
func (m *model) halfPage(down bool) {
	step := m.list.Paginator.PerPage / 2
	if step < 1 {
		step = 1
	}
	i := m.list.Index()
	dir := "down"
	if down {
		i += step
	} else {
		i -= step
		dir = "up"
	}
	if n := len(m.list.Items()); i >= n {
		i = n - 1
	}
	if i < 0 {
		i = 0
	}
	m.list.Select(i)
	m.skipHeaders(dir)
	m.updatePreview()
}

// enter acts on the selected row: e edits it, and Enter does what the row
// is for (open a picked note, create a missing one, apply a saved search or
// tag filter, edit a note or task).
func (m *model) enter(isEnter bool) tea.Cmd {
	if m.pick != nil && isEnter {
		if n := m.selected(); n != nil {
			m.pick = nil
			m.touchRecent(n)
			m.showNote(n)
			m.setStatus("opened " + n.Key() + " (enter edits)")
		}
		return nil
	}
	switch it := m.list.SelectedItem().(type) {
	case missingItem:
		n, _, err := m.app.Store.Ensure(it.l.Kind, it.l.ID, it.l.Text)
		if err != nil {
			m.setErr(err)
			return nil
		}
		m.setStatus("created " + n.Key())
		m.reload()
		m.selectKey(n.Key())
		return m.editCmd(n)
	case taskItem:
		n := it.n
		return m.editCmdAt(&n, m.app.Store.FileLine(n.Kind, n.ID, it.t.Line))
	case taskNoteItem:
		n := it.nt.Note
		line := 0
		if t, ok := firstOpen(it.nt.Tasks); ok {
			line = m.app.Store.FileLine(n.Kind, n.ID, t.Line)
		}
		return m.editCmdAt(&n, line)
	case savedItem:
		m.applySearch(it.s.Mode, it.s.Text)
		return nil
	case tagItem:
		m.tagFilter = it.t.Tag
		m.switchTab(tabPages)
		m.setStatus("filtering by #" + it.t.Tag + " (esc clears)")
		return nil
	}
	if n := m.selected(); n != nil {
		return m.editCmd(n)
	}
	return nil
}

// switchTab shows tab t from the top, leaving any links or pick view.
func (m *model) switchTab(t tab) {
	m.tab = t
	m.clearViews()
	m.list.Select(0)
	m.reload()
}

// resetCache drops the rendered previews, e.g. after notes changed on disk.
func (m *model) resetCache() { m.cache = map[string]string{} }

// reloadAll rereads every note and redraws the previews (R and :reload).
func (m *model) reloadAll() {
	m.resetCache()
	todo := m.syncTodo()
	m.reload()
	if todo != "" {
		m.setStatus("reloaded · " + todo)
	} else {
		m.setStatus("reloaded")
	}
}

func (m *model) openHelp() {
	m.mode = modeHelp
	m.sizeHelp()
	m.help.SetContent(helpBody(m.help.Width))
	m.help.GotoTop()
}

// confirmDelete asks before deleting the selected note.
func (m *model) confirmDelete() {
	if n := m.selected(); n != nil {
		m.mode, m.confirm = modeConfirm, n
	}
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
		m.setStatus("created " + n.Key())
		m.reload()
		m.selectKey(n.Key())
		return m, m.editCmd(n)
	case inTags:
		if m.selected() == nil {
			return m, nil
		}
		r, err := commands.Run(m.cmdEnv(), "tag set "+val)
		return m, m.applyResult("", r, err)
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
			recent.Rename(m.recentPath, n.Key(), r.Key()) //nolint:errcheck // a convenience
			msg := "renamed to " + r.Key()
			if len(relinked) > 0 {
				msg += fmt.Sprintf(" · updated links in %d note(s)", len(relinked))
			}
			m.resetCache()
			m.setStatus(msg)
			m.reload()
			m.selectKey(r.Key())
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
			m.setStatus("deleted " + n.Key())
			recent.Forget(m.recentPath, n.Key()) //nolint:errcheck // a convenience
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
		label := tabLabel(i, name)
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
	if m.graph != nil {
		filters = append(filters, m.graphTitle())
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

	// A column of space keeps the selection bar off the box's border.
	list := lipgloss.NewStyle().PaddingLeft(1).Render(m.list.View())
	// The focused pane has the accent border.
	listC, prevC := accentColor, borderColor
	if m.focusPreview {
		listC, prevC = borderColor, accentColor
	}
	body := titledBox(list, m.list.Width()+1, m.list.Height(), listC, m.listTitle(), m.listPosition())
	if m.showPreview() {
		footer := ""
		if total := len(m.bodyLinks) + len(m.linkedBy); m.linkIdx >= 0 && total > 0 {
			footer = fmt.Sprintf("link %d/%d", m.linkIdx+1, total)
		}
		if m.view.TotalLineCount() > m.view.Height {
			if footer != "" {
				footer += " · "
			}
			footer += fmt.Sprintf("%3.0f%% · [ ] scroll", m.view.ScrollPercent()*100)
			if m.mouse {
				footer += " · wheel"
			}
		}
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, " ",
			titledBox(m.view.View(), m.view.Width, m.view.Height, prevC, m.previewTitle(), footer))
	}

	var bottom string
	switch m.mode {
	case modeCommand:
		bottom = m.commandInputView()
		body = overlayBottom(body, m.commandDropdown())
	case modeSearch:
		bottom = m.searchBarView()
	case modeInput:
		body = overlayCenter(body, modal(m.inputTitle, m.input.View(), "enter ok · esc cancel", m.dialogWidth(), accentColor), m.width)
	case modeConfirm:
		what := fmt.Sprintf("%s\n%s", m.confirm.Title, statusStyle.Render(m.confirm.Key()))
		body = overlayCenter(body, modal("Delete this note?", what, "y delete · any other key cancels", m.dialogWidth(), dangerColor), m.width)
	case modeImage:
		body = overlayCenter(body, m.imagePickerView(), m.width)
	}
	switch m.mode {
	case modeCommand, modeSearch:
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
	help := statusStyle.Render(": commands · ? help · n new · enter/e edit · p preview · r rename · T tags · L link · dd delete · J today · / search · t tag · tab preview · l/h tabs · [ ] scroll preview · q quit")
	if m.tab == tabTasks {
		view := "v all tasks"
		if m.allTasks {
			view = "v notes · a done · x check · @ due · p priority"
		}
		help = statusStyle.Render(": commands · ? help · enter edit at task · " + view + " · / search · t tag · tab preview · l/h tabs · q quit")
	}
	if m.focusPreview {
		help = statusStyle.Render("preview: n/N links · enter open · b/⌫ back · j/k scroll · space/ctrl+d/u page · gg/G ends · e edit · tab/esc list")
	}
	help = ansi.Truncate(help, m.width, "…")
	return lipgloss.JoinVertical(lipgloss.Left, header, filterLine, body, bottom, help)
}

// dialogWidth is the inner width of the input and confirm dialogs.
func (m *model) dialogWidth() int { return max(min(60, m.width-8), 20) }

// listTitle names what the list shows, for its border: the tab and its
// order or view, or the links / picked list it shows instead.
func (m *model) listTitle() string {
	switch {
	case m.pick != nil:
		return m.pick.title
	case m.links != nil && m.links.back:
		return "linking to " + m.links.of.Title
	case m.links != nil:
		return "linked from " + m.links.of.Title
	case m.graph != nil:
		return m.graphTitle()
	}
	t := tabNames[m.tab]
	switch m.tab {
	case tabPages:
		t += " · " + sortNames[m.sortFor(notes.KindPage)]
	case tabJournal:
		t += " · " + sortNames[m.sortFor(notes.KindJournal)]
	case tabTasks:
		if m.allTasks {
			t += " · all tasks"
		} else {
			t += " · by note"
		}
	}
	return t
}

// listPosition is "3/12": the selected row among the list's rows (month
// headers not counted).
func (m *model) listPosition() string {
	pos, n := 0, 0
	sel := m.list.Index()
	for i, it := range m.list.Items() {
		if _, ok := it.(headerItem); ok {
			continue
		}
		n++
		if i <= sel {
			pos = n
		}
	}
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d", pos, n)
}

// previewTitle names what the preview shows, for its border.
func (m *model) previewTitle() string {
	if n := m.selected(); n != nil {
		return n.Title
	}
	switch it := m.list.SelectedItem().(type) {
	case tagItem:
		return "#" + it.t.Tag
	case missingItem:
		return "(missing) " + it.l.Title
	case savedItem:
		return "🔎 " + it.s.Name
	}
	return "Preview"
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

// helpSec is a titled group of keys in the help.
type helpSec struct {
	title   string
	entries []helpEntry
}

var helpSections = []helpSec{
	{"Navigation", []helpEntry{
		{"↑/↓  j/k", "move selection"},
		{"gg / G", "first / last item"},
		{"ctrl+d / ctrl+u", "half a page down / up the list"},
		{"ctrl+f / ctrl+b", "next / previous page of the list"},
		{"tab", "focus the preview: j/k, ctrl+d/u, space, gg/G scroll it; tab or esc goes back"},
		{"n / N", "focused preview: highlight the next / previous link (then the notes linking here)"},
		{"enter", "focused preview: open the highlighted link (note: show it · missing: create it · URL: browser)"},
		{"b / backspace", "back to the note you followed a link from (b: in the focused preview)"},
		{"l  →  gt", "next tab"},
		{"shift+tab h gT", "previous tab"},
		{"1 2 3 4", "Pages / Journal / Tags / Tasks"},
		{"] / [", "scroll the preview (also ctrl+e/y a line, pgdn/pgup half a page, mouse wheel)"},
		{"P", "show or hide the preview pane (hidden below 80 columns)"},
	}},
	{"Notes", []helpEntry{
		{"n", "new page (or journal entry on the Journal tab)"},
		{"enter / e", "edit in $EDITOR"},
		{"r", "rename"},
		{"dd / delete", "delete (asks to confirm)"},
		{"J / ctrl+j", "open today's journal"},
		{"R", "reload from disk"},
		{"F", "show or collapse the frontmatter above the preview"},
		{"T", "edit the note's tags (comma separated)"},
		{"L", "link the note to another one (added under ## Links)"},
	}},
	{"Tasks tab", []helpEntry{
		{"v", "switch: notes with tasks / all tasks"},
		{"enter / e", "edit the note at the (first open) task line"},
		{"x / space", "check or uncheck the selected task"},
		{"a", "all-tasks view: cycle open → done → all"},
		{"w", "all-tasks view: cycle any date → overdue → due this week"},
		{"@", "all-tasks view: set the due date (today, tom, fri, +3d, …)"},
		{"p", "all-tasks view: cycle priority (high → med → low → none)"},
		{"K", "all-tasks view: link the task to todo.txt for katTasks ([todo_txt] in config)"},
	}},
	{"Finding", []helpEntry{
		{"/", "live search; tab switches Name (fuzzy title/filename) · Tags · Text"},
		{"ctrl+p", "quick open: jump to any note by name (recent first)"},
		{"s", "sort Pages/Journal: modified · created · title (remembered)"},
		{"t", "filter by tag"},
		{"enter", "on the Tags tab: filter by that tag, or run a 🔎 saved search"},
		{"esc", "clear search and tag filter (or close the links / graph / :recent list)"},
		{"p / gx", "open the selected note in the Markdown previewer (config: previewer)"},
		{"gb", "notes that link to the selected note (backlinks)"},
		{"gf", "notes the selected note links to; enter on (missing) creates it"},
		{"gn / :graph", "graph: the notes around the selected note, as a tree (+/- depth; :graph all lists every note)"},
	}},
	{"General", []helpEntry{
		{":", "command window: :open, :tag add, :set, :frontmatter… (tab completes, ↑ history)"},
		{"? / F1", "toggle this help"},
		{"mouse", "click tabs, rows and search modes; double-click opens; the wheel scrolls (tui_mouse = false turns it off; hold shift to select text)"},
		{"q  ZZ  :q", "quit (also ctrl+c)"},
	}},
}

// helpTwoColumns is the help width from which the key sections are laid
// out in two columns.
const helpTwoColumns = 120

// helpBody is the help text for a box w wide: the key sections (in two
// columns when there's room), then the commands.
func helpBody(w int) string {
	var b strings.Builder
	if w >= helpTwoColumns {
		colW := (w - 2) / 2
		split := 2 // Navigation, Notes | Tasks, Finding, General (about even)
		left := helpSectionsText(helpSections[:split], colW)
		right := helpSectionsText(helpSections[split:], colW)
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(colW).Render(left), "  ",
			lipgloss.NewStyle().Width(colW).Render(right)))
		b.WriteString("\n")
	} else {
		b.WriteString(helpSectionsText(helpSections, w))
	}
	// The same command list as the GUI's help (from the shared catalog).
	b.WriteString("\n" + helpSection.Render("Commands (:)") + "\n")
	desc := lipgloss.NewStyle().Width(max(w-6, 10))
	for _, c := range commands.Catalog(commands.AppTUI) {
		b.WriteString("  " + helpCmd.Render(":"+c.Usage()) + "\n")
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, "      ", desc.Render(c.Desc)) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// helpSectionsText renders key sections w wide, wrapping each description
// under itself rather than under the key.
func helpSectionsText(secs []helpSec, w int) string {
	descW := max(w-2-helpKey.GetWidth(), 10)
	desc := lipgloss.NewStyle().Width(descW)
	var b strings.Builder
	for i, sec := range secs {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(helpSection.Render(sec.title) + "\n")
		for _, e := range sec.entries {
			row := lipgloss.JoinHorizontal(lipgloss.Top, "  ", helpKey.Render(e.key), desc.Render(e.desc))
			b.WriteString(row + "\n")
		}
	}
	return b.String()
}

func helpLines(w int) int { return strings.Count(helpBody(w), "\n") + 1 }

// sizeHelp fits the help viewport to its content, capped by the window.
func (m *model) sizeHelp() {
	w := 64
	if m.width >= helpTwoColumns+20 {
		w = helpTwoColumns + 8
	}
	if m.width-6 < w {
		w = m.width - 6
	}
	// box border (2) + banner (1) + blank (1) + footer (1) + margin (2)
	h := m.height - 7
	if n := helpLines(w); n < h {
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

// helpBox is the help popup, which helpView centres in the window.
func (m model) helpBox() string {
	w := m.help.Width
	banner := helpBanner.Width(w).Render("KatNotes — keyboard help")
	ver := helpVersion.Width(w).Align(lipgloss.Right).Render("katnotes " + m.app.Version)
	foot := "esc/? close"
	if !(m.help.AtTop() && m.help.AtBottom()) {
		foot = fmt.Sprintf("↑/↓ scroll · %3.0f%% · ", m.help.ScrollPercent()*100) + foot
	}
	return helpBox.Render(lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinVertical(lipgloss.Left, banner, ver),
		m.help.View(),
		statusStyle.Width(w).Align(lipgloss.Right).Render(foot),
	))
}

func (m model) helpView() string {
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.helpBox())
}
