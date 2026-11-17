package tui

import (
	"io"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/devinthemtn/katnotes/internal/cli"
	"github.com/devinthemtn/katnotes/internal/notes"
)

func TestTasksTab(t *testing.T) {
	st := notes.New(t.TempDir())
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	st.Create(notes.KindPage, "Shop", nil, "- [ ] milk\n- [x] eggs\n")
	st.Create(notes.KindPage, "Plain", nil, "nothing\n")
	m := newModel(&cli.App{Store: st, Out: io.Discard}, "dark")
	m.width, m.height = 100, 30
	m.layout()

	press := func(k string) {
		var msg tea.KeyMsg
		if k == " " {
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
		} else {
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		next, _ := m.Update(msg)
		m = next.(model)
	}

	press("4")
	if m.tab != tabTasks || len(m.list.Items()) != 1 {
		t.Fatalf("notes-with-tasks items = %d", len(m.list.Items()))
	}
	if _, ok := m.list.Items()[0].(taskNoteItem); !ok {
		t.Fatalf("item type %T", m.list.Items()[0])
	}

	press("v")
	if len(m.list.Items()) != 1 { // eggs is done and hidden
		t.Fatalf("open tasks = %d", len(m.list.Items()))
	}
	press("a")
	if len(m.list.Items()) != 2 {
		t.Fatalf("all tasks = %d", len(m.list.Items()))
	}
	if v := m.View(); v == "" {
		t.Error("empty view")
	}
	press(" ") // check "milk"
	n, _ := st.Get(notes.KindPage, "shop")
	if n.Body != "- [x] milk\n- [x] eggs\n" {
		t.Errorf("body after check = %q", n.Body)
	}
	if it, ok := m.list.SelectedItem().(taskItem); !ok || it.t.Text != "milk" || !it.t.Done {
		t.Errorf("selection after toggle = %+v", m.list.SelectedItem())
	}
}
