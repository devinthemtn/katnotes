package tui

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/devinthemtn/katnotes/internal/cli"
	"github.com/devinthemtn/katnotes/internal/config"
	"github.com/devinthemtn/katnotes/internal/notes"
)

// TestMain keeps the tests away from the real config dir (the command
// history is written there).
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "katnotes-tui-test")
	if err != nil {
		panic(err)
	}
	os.Setenv(config.EnvConfigDir, dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func TestCommandWindowSyncTasks(t *testing.T) {
	st := notes.New(t.TempDir())
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	legacy := st.Path(notes.KindPage, "legacy")
	os.WriteFile(legacy, []byte("- [ ] old task\n"), 0o644)

	m := newModel(&cli.App{Store: st, Out: io.Discard}, "dark")
	m.width, m.height = 100, 30
	m.layout()
	send := func(msg tea.KeyMsg) {
		next, _ := m.Update(msg)
		m = next.(model)
	}
	runes := func(s string) { send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}) }

	runes(":")
	if m.mode != modeCommand {
		t.Fatal("':' did not open the command window")
	}
	for _, r := range "sy" {
		runes(string(r))
	}
	if v := m.View(); !strings.Contains(v, "sync-tasks") || !strings.Contains(v, "contains_task") {
		t.Errorf("dropdown not shown:\n%s", v)
	}
	send(tea.KeyMsg{Type: tea.KeyTab})
	if m.cmdInput.Value() != "sync-tasks" {
		t.Errorf("tab completed to %q", m.cmdInput.Value())
	}
	send(tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeNormal || !strings.Contains(m.status, "updated 1 note") {
		t.Errorf("mode %v, status %q", m.mode, m.status)
	}
	if n, _ := st.Get(notes.KindPage, "legacy"); n.Extra[notes.TaskKey] != true {
		t.Errorf("flag not set: %v", n.Extra)
	}

	runes(":")
	send(tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeNormal {
		t.Error("esc did not close the command window")
	}
	runes(":")
	for _, r := range "nope" {
		runes(string(r))
	}
	send(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.isErr || !strings.Contains(m.status, "unknown command") {
		t.Errorf("unknown command status %q", m.status)
	}
}

func TestCommandWindowArgsAndHistory(t *testing.T) {
	st := notes.New(t.TempDir())
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	st.Create(notes.KindPage, "Other", []string{"work"}, "")
	if _, err := st.Create(notes.KindPage, "Alpha", nil, "# Alpha\n"); err != nil {
		t.Fatal(err)
	}
	m := newModel(&cli.App{Store: st, Out: io.Discard}, "dark")
	m.width, m.height = 100, 30
	m.layout()
	m.cmdHist, m.cmdHistPath = nil, filepath.Join(t.TempDir(), "history")
	m.selectKey("page/alpha")
	send := func(msg tea.KeyMsg) {
		next, _ := m.Update(msg)
		m = next.(model)
	}
	typ := func(s string) {
		for _, r := range s {
			send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
	}

	typ(":tag add w")
	if v := m.View(); !strings.Contains(v, "#work") || !strings.Contains(v, "tag add <tag…>") {
		t.Errorf("tag suggestions not shown:\n%s", v)
	}
	send(tea.KeyMsg{Type: tea.KeyTab})
	if m.cmdInput.Value() != "tag add work " {
		t.Fatalf("tab completed to %q", m.cmdInput.Value())
	}
	send(tea.KeyMsg{Type: tea.KeyEnter})
	if n, _ := st.Get(notes.KindPage, "alpha"); !n.HasTag("work") {
		t.Fatalf("tag not added (status %q)", m.status)
	}

	typ(":set status draft")
	send(tea.KeyMsg{Type: tea.KeyEnter})
	if n, _ := st.Get(notes.KindPage, "alpha"); n.Extra["status"] != "draft" {
		t.Fatalf("field not set (status %q)", m.status)
	}

	// ↑ on an empty prompt recalls the newest command, then the one before.
	typ(":")
	send(tea.KeyMsg{Type: tea.KeyUp})
	if m.cmdInput.Value() != "set status draft" {
		t.Fatalf("history[0] = %q", m.cmdInput.Value())
	}
	send(tea.KeyMsg{Type: tea.KeyUp})
	if m.cmdInput.Value() != "tag add work" {
		t.Fatalf("history[1] = %q", m.cmdInput.Value())
	}
	send(tea.KeyMsg{Type: tea.KeyDown})
	send(tea.KeyMsg{Type: tea.KeyDown})
	if m.cmdInput.Value() != "" {
		t.Fatalf("back past the newest = %q", m.cmdInput.Value())
	}
	send(tea.KeyMsg{Type: tea.KeyEsc})

	// open switches tabs and selects the note.
	st.Journal(time.Date(2026, 1, 2, 0, 0, 0, 0, time.Local))
	typ(":open 2026-01-02")
	send(tea.KeyMsg{Type: tea.KeyEnter})
	if m.tab != tabJournal || itemKey(m.list.SelectedItem()) != "journal/2026-01-02" {
		t.Fatalf("open: tab %v, selected %q, status %q", m.tab, itemKey(m.list.SelectedItem()), m.status)
	}
}
