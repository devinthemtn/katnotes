package tui

import (
	"io"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/devinthemtn/katnotes/internal/cli"
	"github.com/devinthemtn/katnotes/internal/notes"
)

func TestMatchCommands(t *testing.T) {
	if got := matchCommands("sy"); len(got) == 0 || got[0].name != "sync-tasks" {
		t.Errorf("sy -> %v", got)
	}
	if got := matchCommands("contains"); len(got) != 1 || got[0].name != "sync-tasks" {
		t.Errorf("description match -> %v", got)
	}
	if got := matchCommands("zzz"); len(got) != 0 {
		t.Errorf("zzz -> %v", got)
	}
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
