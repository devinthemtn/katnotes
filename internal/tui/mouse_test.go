package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/devinthemtn/katnotes/internal/notes"
)

func mouse(m *model, b tea.MouseButton, x, y int) tea.Cmd {
	next, cmd := m.Update(tea.MouseMsg{X: x, Y: y, Button: b, Action: tea.MouseActionPress})
	*m = next.(model)
	return cmd
}

// screenRow finds the first screen row whose plain text contains s.
func screenRow(t *testing.T, m *model, s string) int {
	t.Helper()
	for i, l := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(l, s) {
			return i
		}
	}
	t.Fatalf("%q not on screen:\n%s", s, ansi.Strip(m.View()))
	return -1
}

func longModel(t *testing.T) *model {
	t.Helper()
	m := chromeModel(t, 120, 30)
	body := "# Long\n\n" + strings.Repeat("a line of the long note\n\n", 60)
	if _, err := m.app.Store.Create(notes.KindPage, "Long", nil, body); err != nil {
		t.Fatal(err)
	}
	m.reload()
	m.selectKey("page/long")
	return m
}

func TestWheelScrollsPreviewAndList(t *testing.T) {
	m := longModel(t)
	px := m.listW + 5
	mouse(m, tea.MouseButtonWheelDown, px, 10)
	if m.view.YOffset != wheelStep {
		t.Fatalf("preview offset after wheel = %d", m.view.YOffset)
	}
	mouse(m, tea.MouseButtonWheelUp, px, 10)
	if m.view.YOffset != 0 {
		t.Fatalf("preview offset after wheel up = %d", m.view.YOffset)
	}
	if !strings.Contains(ansi.Strip(m.View()), "[ ] scroll") {
		t.Error("an overflowing preview should hint at its scroll keys")
	}

	before := m.list.Index() // "Long", the last row
	mouse(m, tea.MouseButtonWheelUp, 3, 10)
	if m.list.Index() == before {
		t.Error("wheel over the list should move the selection")
	}
}

func TestScrollKeys(t *testing.T) {
	m := longModel(t)
	press(m, "]")
	press(m, "]")
	if m.view.YOffset != 2*wheelStep {
		t.Fatalf("] ] offset = %d", m.view.YOffset)
	}
	press(m, "[")
	if m.view.YOffset != wheelStep {
		t.Fatalf("[ offset = %d", m.view.YOffset)
	}
}

func TestClickTabsAndRows(t *testing.T) {
	m := longModel(t)
	x := strings.Index(ansi.Strip(m.View()), "3 Tags")
	mouse(m, tea.MouseButtonLeft, x+2, 0)
	if m.tab != tabTags {
		t.Fatalf("clicked Tags, on tab %d", m.tab)
	}
	mouse(m, tea.MouseButtonLeft, 6, 0) // "1 Pages"
	if m.tab != tabPages {
		t.Fatalf("clicked Pages, on tab %d", m.tab)
	}

	y := screenRow(t, m, "│ Groceries") // the title line of its row
	if cmd := mouse(m, tea.MouseButtonLeft, 5, y); cmd != nil {
		t.Error("a single click should only select")
	}
	if n := m.selected(); n == nil || n.ID != "groceries" {
		t.Fatalf("click selected %v", m.selected())
	}
	// The first click renders the preview, which can be slow on a cold run;
	// time the double click from here.
	m.lastClick = time.Now()
	if cmd := mouse(m, tea.MouseButtonLeft, 5, y+1); cmd == nil {
		t.Errorf("a double click should open the editor (status %q, row %d)", m.status, m.lastClickRow)
	}

	hy := screenRow(t, m, "── November")
	sel := m.list.Index()
	mouse(m, tea.MouseButtonLeft, 5, hy)
	if m.list.Index() != sel {
		t.Error("clicking a month header should do nothing")
	}
}

func TestMouseInDialogsAndHelp(t *testing.T) {
	m := longModel(t)
	press(m, "delete")
	y := screenRow(t, m, "Groceries")
	mouse(m, tea.MouseButtonLeft, 5, y)
	mouse(m, tea.MouseButtonWheelDown, m.listW+5, 10)
	if m.mode != modeConfirm || m.view.YOffset != 0 || m.selected().ID != "long" {
		t.Fatal("the mouse should be ignored while confirming")
	}
	press(m, "n")

	press(m, "?")
	mouse(m, tea.MouseButtonWheelDown, m.width/2, m.height/2)
	if m.help.YOffset != wheelStep {
		t.Fatalf("help offset = %d", m.help.YOffset)
	}
	mouse(m, tea.MouseButtonLeft, m.width/2, m.height/2)
	if m.mode != modeHelp {
		t.Fatal("a click inside help should keep it open")
	}
	mouse(m, tea.MouseButtonLeft, 0, 0)
	if m.mode != modeNormal {
		t.Fatal("a click outside help should close it")
	}
}

func TestClickSearchChipsAndCommands(t *testing.T) {
	m := longModel(t)
	press(m, "/")
	line := ansi.Strip(strings.Split(m.View(), "\n")[m.height-2])
	x := strings.Index(line, "Text")
	if x < 0 {
		t.Fatalf("search bar: %q", line)
	}
	mouse(m, tea.MouseButtonLeft, x+1, m.height-2)
	if m.searchMode != notes.SearchText {
		t.Fatalf("search mode = %s", m.searchMode)
	}
	m.mode = modeNormal

	press(m, ":")
	if len(m.cmdComp.Items) < 2 {
		t.Fatalf("want suggestions, got %d", len(m.cmdComp.Items))
	}
	second := m.cmdComp.Items[1].Label
	y := screenRow(t, m, " "+second+" ")
	mouse(m, tea.MouseButtonLeft, 3, y)
	if m.cmdSel != 1 {
		t.Fatalf("clicked suggestion 1, selected %d", m.cmdSel)
	}
	mouse(m, tea.MouseButtonWheelDown, 3, y)
	if m.cmdSel != 2 {
		t.Fatalf("wheel should move the selection, at %d", m.cmdSel)
	}
}

func TestTabFocusesPreview(t *testing.T) {
	m := longModel(t)
	sel := m.list.Index()
	tab := func() {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		*m = next.(model)
	}
	tab()
	if !m.focusPreview {
		t.Fatal("tab should focus the preview")
	}
	press(m, "j")
	press(m, "j")
	if m.view.YOffset != 2 || m.list.Index() != sel {
		t.Fatalf("j should scroll the preview: offset %d, index %d→%d", m.view.YOffset, sel, m.list.Index())
	}
	press(m, "G")
	if !m.view.AtBottom() {
		t.Error("G should go to the end of the note")
	}
	press(m, "g")
	press(m, "g")
	if m.view.YOffset != 0 || m.list.Index() != sel {
		t.Errorf("gg should go to the top of the note: offset %d", m.view.YOffset)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "tab/esc list") {
		t.Error("focused hints missing")
	}
	tab()
	if m.focusPreview {
		t.Fatal("tab again should give the list the focus back")
	}
	press(m, "k")
	if m.list.Index() == sel {
		t.Error("k should move in the list again")
	}

	tab()
	press(m, "esc")
	if m.focusPreview {
		t.Error("esc should give the list the focus back")
	}

	// Clicking a pane focuses it; hiding the preview drops its focus.
	mouse(m, tea.MouseButtonLeft, m.listW+5, 10)
	if !m.focusPreview {
		t.Error("a click on the preview should focus it")
	}
	mouse(m, tea.MouseButtonLeft, 3, 10)
	if m.focusPreview {
		t.Error("a click on the list should focus it")
	}
	tab()
	press(m, "P")
	if m.focusPreview {
		t.Error("hiding the preview should drop its focus")
	}
	tab()
	if m.focusPreview {
		t.Error("a hidden preview can't take the focus")
	}
}
