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
