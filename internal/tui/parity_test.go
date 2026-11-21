package tui

import (
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/devinthemtn/katnotes/internal/cli"
	"github.com/devinthemtn/katnotes/internal/config"
	"github.com/devinthemtn/katnotes/internal/notes"
	"github.com/devinthemtn/katnotes/internal/recent"
)

func parityModel(t *testing.T, cfg config.Config) (*model, *notes.Store, func(string)) {
	t.Helper()
	t.Setenv(config.EnvConfigDir, t.TempDir())
	st := notes.New(t.TempDir())
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	st.Create(notes.KindPage, "Alpha", []string{"x"}, "# Alpha\n\nthe quick fox\n")
	st.Create(notes.KindPage, "Beta", nil, "# Beta\n")
	m := newModel(&cli.App{Store: st, Cfg: cfg, Out: io.Discard}, "dark")
	m.width, m.height = 110, 30
	m.layout()
	mp := &m
	typ := func(s string) {
		for _, r := range s {
			send(mp, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
	}
	return mp, st, typ
}

func send(m *model, msg tea.Msg) tea.Cmd {
	next, cmd := m.Update(msg)
	*m = next.(model)
	return cmd
}

func TestStartPage(t *testing.T) {
	m, _, _ := parityModel(t, config.Config{StartPage: "page/beta"})
	if itemKey(m.list.SelectedItem()) != "page/beta" {
		t.Fatalf("start page: %q", itemKey(m.list.SelectedItem()))
	}
	m, _, _ = parityModel(t, config.Config{StartPage: "today"})
	if m.tab != tabJournal || !strings.HasPrefix(itemKey(m.list.SelectedItem()), "journal/") {
		t.Fatalf("today: tab %v sel %q", m.tab, itemKey(m.list.SelectedItem()))
	}
}

func TestStartPageLastAndRecent(t *testing.T) {
	t.Setenv(config.EnvConfigDir, t.TempDir())
	p, _ := recent.Path()
	st := notes.New(t.TempDir())
	st.Init()
	st.Create(notes.KindPage, "Alpha", nil, "")
	st.Create(notes.KindPage, "Beta", nil, "")
	recent.Touch(p, "page/alpha")
	recent.Touch(p, "page/beta")
	m := newModel(&cli.App{Store: st, Cfg: config.Config{StartPage: "last"}, Out: io.Discard}, "dark")
	m.width, m.height = 110, 30
	m.layout()
	if itemKey(m.list.SelectedItem()) != "page/beta" {
		t.Fatalf("last: %q", itemKey(m.list.SelectedItem()))
	}

	for _, r := range ":recent" {
		send(&m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	send(&m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.pick == nil || len(m.list.Items()) != 2 || !strings.Contains(m.View(), "Recent notes") {
		t.Fatalf("pick view: %v items=%d status=%q", m.pick, len(m.list.Items()), m.status)
	}
	send(&m, tea.KeyMsg{Type: tea.KeyDown})
	send(&m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.pick != nil || itemKey(m.list.SelectedItem()) != "page/alpha" {
		t.Fatalf("enter in pick: pick=%v sel=%q", m.pick, itemKey(m.list.SelectedItem()))
	}
	if got := recent.Load(p); got[0] != "page/alpha" {
		t.Fatalf("recent not touched: %v", got)
	}
}

func TestListRowsPinSearchAndFrontmatter(t *testing.T) {
	m, st, typ := parityModel(t, config.Config{FrontmatterCollapsed: true})
	a, _ := st.Get(notes.KindPage, "alpha")
	st.SetField(a, "pinned", "true")
	m.reload()
	if h, ok := m.list.Items()[0].(headerItem); !ok || h.label != "Pinned" {
		t.Fatalf("first row should be the Pinned header: %#v", m.list.Items()[0])
	}
	if it := m.list.Items()[1].(noteItem); it.n.ID != "alpha" || !strings.HasPrefix(it.Title(), "📌 ") {
		t.Fatalf("pinned first: %q", it.Title())
	}

	m.searchMode, m.query = notes.SearchText, "quick"
	m.reload()
	it := m.list.Items()[0].(noteItem)
	if it.match != "the quick fox" || !strings.HasSuffix(it.Description(), "…the quick fox") {
		t.Fatalf("match row: %q", it.Description())
	}
	m.query = ""
	m.reload()

	m.selectKey("page/alpha")
	// Collapsed, the preview has the short header (key and dates), not
	// the field table ("updated", "pinned"…).
	if v := m.view.View(); strings.Contains(v, "updated") || !strings.Contains(v, "page/alpha · created") {
		t.Fatalf("frontmatter should start collapsed:\n%s", v)
	}
	typ("F")
	if v := m.view.View(); !strings.Contains(v, "updated") || !strings.Contains(v, "pinned") {
		t.Fatalf("frontmatter not shown:\n%s", v)
	}
}

func TestLongCommandRunsInBackground(t *testing.T) {
	m, _, typ := parityModel(t, config.Config{})
	typ(":sync")
	cmd := send(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || m.status != "sync…" {
		t.Fatalf("sync should be async: cmd=%v status=%q", cmd != nil, m.status)
	}
	send(m, cmd()) // the notes folder isn't a git repo
	if !m.isErr || !strings.Contains(m.status, "not a git repository") {
		t.Fatalf("status %q", m.status)
	}
}
