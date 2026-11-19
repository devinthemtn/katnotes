package tui

import (
	"io"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/devinthemtn/katnotes/internal/cli"
	"github.com/devinthemtn/katnotes/internal/commands"
	"github.com/devinthemtn/katnotes/internal/notes"
)

// The vim-style keys: two-key sequences, list paging and today's journal.
func TestVimKeys(t *testing.T) {
	st := notes.New(t.TempDir())
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"A", "B", "C", "D", "E"} {
		st.Create(notes.KindPage, title, nil, "x\n")
	}
	m := newModel(&cli.App{Store: st, Out: io.Discard}, "dark")
	m.width, m.height = 100, 30
	m.layout()
	m.reload()
	press := func(ks ...string) tea.Cmd {
		var cmd tea.Cmd
		for _, k := range ks {
			msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
			switch k {
			case "ctrl+d":
				msg = tea.KeyMsg{Type: tea.KeyCtrlD}
			case "ctrl+j":
				msg = tea.KeyMsg{Type: tea.KeyCtrlJ}
			case "esc":
				msg = tea.KeyMsg{Type: tea.KeyEsc}
			}
			var next tea.Model
			next, cmd = m.Update(msg)
			m = next.(model)
		}
		return cmd
	}

	last := len(m.list.Items()) - 1
	m.list.Select(last)
	m.skipHeaders("up")
	last = m.list.Index()
	m.list.Select(0)
	m.skipHeaders("down")
	first := m.list.Index()
	press("G")
	if m.list.Index() != last {
		t.Errorf("G: index = %d", m.list.Index())
	}
	press("g", "g")
	if m.list.Index() != first {
		t.Errorf("gg: index = %d", m.list.Index())
	}
	press("ctrl+d")
	if m.list.Index() == first {
		t.Error("ctrl+d did not move the list")
	}
	press("g", "t")
	if m.tab != tabJournal {
		t.Errorf("gt: tab = %d", m.tab)
	}
	press("g", "T")
	if m.tab != tabPages {
		t.Errorf("gT: tab = %d", m.tab)
	}
	press("d")
	if m.mode == modeConfirm {
		t.Fatal("a single d should wait for a second key")
	}
	press("esc")
	if m.pending != "" || m.mode != modeNormal {
		t.Errorf("esc after d: pending=%q mode=%d", m.pending, m.mode)
	}
	press("d", "d")
	if m.mode != modeConfirm {
		t.Errorf("dd: mode = %d", m.mode)
	}
	m.mode = modeNormal
	if cmd := press("Z", "Z"); cmd == nil {
		t.Error("ZZ should quit")
	}
	for _, k := range []string{"J", "ctrl+j"} {
		m.tab = tabPages
		press(k)
		if m.tab != tabJournal {
			t.Errorf("%s: tab = %d, want journal", k, m.tab)
		}
	}
}

func TestCommandAliases(t *testing.T) {
	for in, want := range map[string]string{"q": "quit", "q!": "quit", "h": "help"} {
		p, err := commands.Parse(commands.AppTUI, in)
		if err != nil || p.Spec.Name != want {
			t.Errorf("Parse(%q) = %v, %v; want %s", in, p, err, want)
		}
	}
}
