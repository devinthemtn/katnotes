package tui

import (
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/devinthemtn/katnotes/internal/cli"
	"github.com/devinthemtn/katnotes/internal/notes"
)

func TestLinksView(t *testing.T) {
	st := notes.New(t.TempDir())
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	st.Create(notes.KindPage, "Groceries", nil, "milk\n")
	st.Create(notes.KindPage, "Plan", nil, "buy [Groceries](../pages/groceries.md) and [Ideas](../pages/ideas.md)\n")
	m := newModel(&cli.App{Store: st, Out: io.Discard}, "dark")
	m.width, m.height = 100, 30
	m.layout()
	press := func(k string) {
		var msg tea.KeyMsg
		switch k {
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		next, _ := m.Update(msg)
		m = next.(model)
	}
	m.reload()
	m.selectKey("page/groceries")
	if !strings.Contains(m.view.View(), "Linked from (1)") {
		t.Errorf("preview lacks backlinks footer:\n%s", m.view.View())
	}
	press("g")
	press("b")
	if items := m.list.Items(); len(items) != 1 || noteOf(items[0]).ID != "plan" {
		t.Fatalf("backlinks items = %+v", items)
	}
	press("g")
	press("f") // from Plan: Groceries and the missing Ideas
	items := m.list.Items()
	if len(items) != 2 || noteOf(items[0]).ID != "groceries" {
		t.Fatalf("links items = %+v", items)
	}
	if mi, ok := items[1].(missingItem); !ok || mi.l.ID != "ideas" {
		t.Fatalf("missing item = %+v", items[1])
	}
	press("esc")
	if m.links != nil || len(m.noteItems()) != 2 {
		t.Errorf("esc did not close links view: %d notes", len(m.noteItems()))
	}
}

func TestPreviewKey(t *testing.T) {
	st := notes.New(t.TempDir())
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	st.Create(notes.KindPage, "P", nil, "x\n")
	app := &cli.App{Store: st, Out: io.Discard}
	app.Cfg.Previewer = "true" // a windowed "viewer" that exits at once
	app.Cfg.PreviewerMode = "window"
	m := newModel(app, "dark")
	m.width, m.height = 100, 30
	m.layout()
	m.reload()
	gx := func() tea.Cmd {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
		next, cmd := next.(model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
		m = next.(model)
		return cmd
	}
	cmd := gx()
	if cmd != nil || m.isErr || !strings.Contains(m.status, "opened page/p in true") {
		t.Errorf("status = %q err=%v cmd=%v", m.status, m.isErr, cmd != nil)
	}
	m.app.Cfg.PreviewerMode = "terminal"
	if cmd := gx(); cmd == nil {
		t.Error("terminal previewer should return an exec command")
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = next.(model)
	if cmd == nil || m.isErr {
		t.Errorf("p should preview like gx: err=%v status=%q", m.isErr, m.status)
	}
}

// T edits the selected note's tags; L links it to another note.
func TestTagAndLinkKeys(t *testing.T) {
	st := notes.New(t.TempDir())
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	st.Create(notes.KindPage, "Groceries", nil, "milk\n")
	st.Create(notes.KindPage, "Plan", []string{"old"}, "# Plan\n")
	m := newModel(&cli.App{Store: st, Out: io.Discard}, "dark")
	m.width, m.height = 100, 30
	m.layout()
	m.reload()
	send := func(msg tea.KeyMsg) {
		next, _ := m.Update(msg)
		m = next.(model)
	}
	keys := func(s string) { send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}) }
	enter := func() { send(tea.KeyMsg{Type: tea.KeyEnter}) }

	m.selectKey("page/plan")
	keys("T")
	if m.mode != modeInput || m.input.Value() != "old" {
		t.Fatalf("T: mode %v, input %q", m.mode, m.input.Value())
	}
	m.input.SetValue("work, ideas")
	enter()
	if n, _ := st.Get(notes.KindPage, "plan"); m.isErr || strings.Join(n.Tags, " ") != "work ideas" {
		t.Fatalf("tags = %v (status %q)", n.Tags, m.status)
	}
	if m.selected() == nil || m.selected().ID != "plan" {
		t.Fatalf("selection moved to %+v", m.selected())
	}

	keys("L")
	if m.mode != modeCommand || m.cmdInput.Value() != "link " {
		t.Fatalf("L: mode %v, input %q", m.mode, m.cmdInput.Value())
	}
	keys("groc")
	enter()
	if n, _ := st.Get(notes.KindPage, "plan"); m.isErr || !strings.Contains(n.Body, "## Links\n\n- [Groceries](../pages/groceries.md)") {
		t.Fatalf("body = %q (status %q)", n.Body, m.status)
	}
	m.selectKey("page/groceries")
	if !strings.Contains(m.view.View(), "Linked from (1)") {
		t.Errorf("groceries preview lacks the backlink:\n%s", m.view.View())
	}
}

// keyMsg sends a special key (tab, enter, backspace) to m.
func keyMsg(m *model, t tea.KeyType) tea.Cmd {
	next, cmd := m.Update(tea.KeyMsg{Type: t})
	*m = next.(model)
	return cmd
}

func TestFollowLinksFromPreview(t *testing.T) {
	m := chromeModel(t, 120, 30) // has page/groceries
	filler := strings.Repeat("filler line\n\n", 40)
	body := "Go to [Groceries](../pages/groceries.md) or https://go.dev.\n\n" + filler + "Later: [New idea](../pages/new-idea.md)\n"
	if _, err := m.app.Store.Create(notes.KindPage, "Plan", nil, body); err != nil {
		t.Fatal(err)
	}
	var opened []string
	defer func(f func(string) error) { openExternal = f }(openExternal)
	openExternal = func(s string) error { opened = append(opened, s); return nil }

	m.reload()
	m.selectKey("page/plan")
	keyMsg(m, tea.KeyTab)
	if !m.focusPreview {
		t.Fatal("tab should focus the preview")
	}
	if len(m.bodyLinks) != 3 {
		t.Fatalf("links = %+v", m.bodyLinks)
	}

	press(m, "n")
	if !strings.Contains(m.status, "link 1/3: Groceries → page/groceries") || !strings.Contains(m.view.View(), "▶") {
		t.Fatalf("n should highlight the first link: status %q\n%s", m.status, m.view.View())
	}
	if strings.Contains(m.view.View(), linkOpen) {
		t.Error("sentinels left in the preview")
	}

	// N wraps to the last link, far down the note: the preview scrolls to it.
	press(m, "N")
	press(m, "N")
	if m.linkIdx != 1 {
		t.Fatalf("N N from 1 = link %d, want 2 (index 1)", m.linkIdx+1)
	}
	press(m, "N")
	press(m, "N")
	if m.linkIdx != 2 || m.view.YOffset == 0 || !strings.Contains(m.view.View(), "▶") {
		t.Fatalf("last link should be in view: idx %d offset %d", m.linkIdx, m.view.YOffset)
	}

	// The URL opens in the system's app.
	press(m, "n") // wraps to 1
	press(m, "n") // the URL
	keyMsg(m, tea.KeyEnter)
	if len(opened) != 1 || opened[0] != "https://go.dev" {
		t.Fatalf("opened %v", opened)
	}

	// A note link shows the note, keeping the preview focused; ⌫ comes back.
	press(m, "N") // Groceries
	if cmd := keyMsg(m, tea.KeyEnter); cmd != nil {
		t.Error("an existing note shouldn't open the editor")
	}
	if n := m.selected(); n == nil || n.ID != "groceries" || !m.focusPreview || m.linkIdx != -1 {
		t.Fatalf("after enter: selected %v, focus %v, link %d", m.selected(), m.focusPreview, m.linkIdx)
	}
	press(m, "b")
	if n := m.selected(); n == nil || n.ID != "plan" || !m.focusPreview {
		t.Fatalf("b should return to plan with the preview focused, at %v", m.selected())
	}
	press(m, "n") // Groceries again; backspace works too
	keyMsg(m, tea.KeyEnter)
	keyMsg(m, tea.KeyBackspace)
	if n := m.selected(); n == nil || n.ID != "plan" {
		t.Fatalf("backspace should return to plan, at %v", m.selected())
	}
	keyMsg(m, tea.KeyBackspace)
	if m.status != "no previous note" {
		t.Errorf("status %q", m.status)
	}

	// A missing note is created and opened in the editor.
	press(m, "N")
	if cmd := keyMsg(m, tea.KeyEnter); cmd == nil {
		t.Errorf("a missing note should open the editor (status %q)", m.status)
	}
	if _, err := m.app.Store.Get(notes.KindPage, "new-idea"); err != nil {
		t.Errorf("new-idea not created: %v", err)
	}
}

func TestLinkKeysNeedFocusAndLinks(t *testing.T) {
	m := chromeModel(t, 120, 30)
	m.selectKey("page/groceries")
	keyMsg(m, tea.KeyTab)
	press(m, "n")
	if m.status != "no links in this note" || m.mode != modeNormal {
		t.Errorf("n without links: status %q, mode %d", m.status, m.mode)
	}
	keyMsg(m, tea.KeyTab)
	press(m, "n") // the list has focus: n is "new note" again
	if m.mode != modeInput {
		t.Error("n in the list should still start a new note")
	}
}
