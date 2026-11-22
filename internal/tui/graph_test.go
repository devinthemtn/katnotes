package tui

import (
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/devinthemtn/katnotes/internal/cli"
	"github.com/devinthemtn/katnotes/internal/commands"
	"github.com/devinthemtn/katnotes/internal/notes"
)

func TestGraphView(t *testing.T) {
	st := notes.New(t.TempDir())
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	// Plan → Groceries → Recipes, Plan → Ideas (missing), Lonely alone.
	st.Create(notes.KindPage, "Recipes", nil, "pasta\n")
	st.Create(notes.KindPage, "Groceries", nil, "for [Recipes](../pages/recipes.md)\n")
	st.Create(notes.KindPage, "Plan", nil, "buy [Groceries](../pages/groceries.md) and [Ideas](../pages/ideas.md)\n")
	st.Create(notes.KindPage, "Lonely", nil, "x\n")
	m := newModel(&cli.App{Store: st, Out: io.Discard}, "dark")
	m.width, m.height = 100, 30
	m.layout()
	press := func(k string) {
		var msg tea.KeyMsg
		if k == "esc" {
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		} else {
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		next, _ := m.Update(msg)
		m = next.(model)
	}
	ids := func() string {
		var out []string
		for _, it := range m.list.Items() {
			if n := noteOf(it); n != nil {
				out = append(out, n.ID)
			} else if mi, ok := it.(missingItem); ok {
				out = append(out, "?"+mi.l.ID)
			}
		}
		return strings.Join(out, ",")
	}

	m.reload()
	m.selectKey("page/groceries")
	press("g")
	press("n") // depth 2 around Groceries
	if got := ids(); got != "groceries,plan,recipes,?ideas" {
		t.Fatalf("graph items = %s", got)
	}
	tree := ansi.Strip(m.view.View())
	for _, want := range []string{"Groceries", "├─→ Recipes", "└─← Plan", "    └─→ Ideas (missing)"} {
		if !strings.Contains(tree, want) {
			t.Errorf("tree lacks %q:\n%s", want, tree)
		}
	}
	press("-")
	if m.graph.depth != 1 || ids() != "groceries,plan,recipes" {
		t.Errorf("depth 1: %d %s", m.graph.depth, ids())
	}
	press("esc")
	if m.graph != nil || len(m.noteItems()) != 4 {
		t.Errorf("esc did not close the graph view")
	}

	// :graph all: every note, most linked first.
	tuiActions["graph"](&m, commands.Result{Args: []string{"all"}})
	if got := ids(); !strings.HasPrefix(got, "groceries,plan,") || !strings.HasSuffix(got, "lonely") {
		t.Errorf("graph all = %s", got)
	}
	if !strings.Contains(m.listTitle(), "every note") {
		t.Errorf("title = %s", m.listTitle())
	}
}
