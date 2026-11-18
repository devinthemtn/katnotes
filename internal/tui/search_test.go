package tui

import (
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/devinthemtn/katnotes/internal/cli"
	"github.com/devinthemtn/katnotes/internal/config"
	"github.com/devinthemtn/katnotes/internal/notes"
	"github.com/devinthemtn/katnotes/internal/viewstate"
)

func searchModel(t *testing.T) (*model, *notes.Store) {
	t.Helper()
	t.Setenv(config.EnvConfigDir, t.TempDir())
	st := notes.New(t.TempDir())
	st.Init()
	st.Create(notes.KindPage, "Weekly Standup", []string{"work"}, "# Weekly Standup\n\nbudget talk\n")
	st.Create(notes.KindPage, "Groceries", []string{"home"}, "# Groceries\n\nmilk and eggs\n")
	st.Create(notes.KindPage, "Budget", []string{"work", "money"}, "# Budget\n\nnumbers\n")
	m := newModel(&cli.App{Store: st, Out: io.Discard}, "dark")
	m.width, m.height = 120, 30
	m.layout()
	return &m, st
}

func keys(m *model, ks ...string) {
	for _, k := range ks {
		var msg tea.KeyMsg
		switch k {
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "ctrl+p":
			msg = tea.KeyMsg{Type: tea.KeyCtrlP}
		default:
			for _, r := range k {
				send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			}
			continue
		}
		send(m, msg)
	}
}

func noteIDs(m *model) string {
	var out []string
	for _, it := range m.noteItems() {
		out = append(out, it.(noteItem).n.ID)
	}
	return strings.Join(out, ",")
}

func TestLiveSearchModes(t *testing.T) {
	m, _ := searchModel(t)
	keys(m, "/", "stdup")
	if m.mode != modeSearch || noteIDs(m) != "weekly-standup" {
		t.Fatalf("name search: mode %v, %s", m.mode, noteIDs(m))
	}
	if v := m.View(); !strings.Contains(v, "fuzzy title or filename") || !strings.Contains(v, "1 of 3") {
		t.Fatalf("search bar / header missing:\n%s", v)
	}
	// Same text, tags mode: nothing tagged "stdup".
	keys(m, "tab")
	if m.searchMode != notes.SearchTags || noteIDs(m) != "" {
		t.Fatalf("tags mode: %s %s", m.searchMode, noteIDs(m))
	}
	m.searchInput.SetValue("")
	m.query = ""
	keys(m, "wo")
	if ids := noteIDs(m); ids != "budget,weekly-standup" && ids != "weekly-standup,budget" {
		t.Fatalf("tag prefix: %s", ids)
	}
	keys(m, "tab") // text
	m.searchInput.SetValue("")
	m.query = ""
	keys(m, "budget talk")
	if noteIDs(m) != "weekly-standup" {
		t.Fatalf("text AND: %s", noteIDs(m))
	}
	keys(m, "enter")
	if m.mode != modeNormal || m.query != "budget talk" {
		t.Fatal("enter should keep the search")
	}
	keys(m, "/", "esc")
	if m.query != "" || len(m.noteItems()) != 3 {
		t.Fatalf("esc should clear: %q %d", m.query, len(m.noteItems()))
	}
}

func TestQuickOpen(t *testing.T) {
	m, _ := searchModel(t)
	keys(m, "ctrl+p")
	if m.mode != modeCommand || m.cmdInput.Value() != "open " {
		t.Fatalf("quick open: %v %q", m.mode, m.cmdInput.Value())
	}
	keys(m, "groc", "enter")
	if itemKey(m.list.SelectedItem()) != "page/groceries" {
		t.Fatalf("selected %q, status %q", itemKey(m.list.SelectedItem()), m.status)
	}
}

func TestSortCycleAndHeaders(t *testing.T) {
	m, st := searchModel(t)
	old := time.Date(2025, 3, 4, 10, 0, 0, 0, time.Local)
	n, _ := st.Get(notes.KindPage, "groceries")
	n.Created = old
	st.Save(n) // updated = now, created in March 2025
	m.reload()

	if _, ok := m.list.SelectedItem().(headerItem); ok {
		t.Fatal("cursor sits on a header")
	}
	keys(m, "s") // modified -> created
	if m.sortFor(notes.KindPage) != notes.SortCreated {
		t.Fatalf("sort = %s", m.sortFor(notes.KindPage))
	}
	if v := viewstate.Load(m.viewPath); v.Sort["page"] != "created" {
		t.Fatalf("sort not saved: %+v", v.Sort)
	}
	var headers []string
	for _, it := range m.list.Items() {
		if h, ok := it.(headerItem); ok {
			headers = append(headers, h.label)
		}
	}
	if len(headers) != 2 || headers[1] != "March 2025" {
		t.Fatalf("headers = %v", headers)
	}
	// The cursor walks past headers.
	keys(m, "down", "down", "down")
	if _, ok := m.list.SelectedItem().(headerItem); ok || noteOf(m.list.SelectedItem()).ID != "groceries" {
		t.Fatalf("at %q", itemKey(m.list.SelectedItem()))
	}
	keys(m, "s") // created -> title: no headers
	for _, it := range m.list.Items() {
		if _, ok := it.(headerItem); ok {
			t.Fatal("title order shouldn't have headers")
		}
	}
	// A new model reads the saved order.
	m2 := newModel(&cli.App{Store: st, Out: io.Discard}, "dark")
	if m2.sortFor(notes.KindPage) != notes.SortTitle {
		t.Fatalf("restored sort = %s", m2.sortFor(notes.KindPage))
	}
}

func TestSavedSearchOnTagsTab(t *testing.T) {
	m, _ := searchModel(t)
	keys(m, "/", "tab", "work", "enter")
	keys(m, ":search save Work notes", "enter")
	if m.isErr {
		t.Fatalf("save: %s", m.status)
	}
	keys(m, "/", "esc") // clear
	keys(m, "3")
	if it, ok := m.list.Items()[0].(savedItem); !ok || it.s.Name != "Work notes" || it.s.Mode != "tags" {
		t.Fatalf("first Tags row: %#v", m.list.Items()[0])
	}
	m.list.Select(0)
	keys(m, "enter")
	if m.tab != tabPages || m.searchMode != notes.SearchTags || m.query != "work" || len(m.noteItems()) != 2 {
		t.Fatalf("applied: tab %v mode %s q %q n %d", m.tab, m.searchMode, m.query, len(m.noteItems()))
	}
}
