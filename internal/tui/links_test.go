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
}
