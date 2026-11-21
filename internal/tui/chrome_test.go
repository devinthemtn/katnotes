package tui

import (
	"io"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/devinthemtn/katnotes/internal/cli"
	"github.com/devinthemtn/katnotes/internal/commands"
	"github.com/devinthemtn/katnotes/internal/config"
	"github.com/devinthemtn/katnotes/internal/notes"
)

func TestTitledBox(t *testing.T) {
	box := titledBox("one\na much longer line than fits", 12, 3, borderColor, "A very long title", "3/9")
	lines := strings.Split(box, "\n")
	if len(lines) != 5 {
		t.Fatalf("box has %d lines, want 5:\n%s", len(lines), box)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 14 {
			t.Errorf("line %d is %d wide, want 14: %q", i, w, ansi.Strip(l))
		}
	}
	top, bottom := ansi.Strip(lines[0]), ansi.Strip(lines[4])
	if !strings.HasPrefix(top, "╭─ A very") || !strings.Contains(top, "…") || !strings.HasSuffix(top, "╮") {
		t.Errorf("top border %q", top)
	}
	if !strings.HasSuffix(bottom, " 3/9 ─╯") {
		t.Errorf("bottom border %q", bottom)
	}
	if got := ansi.Strip(lines[3]); got != "│            │" {
		t.Errorf("padding line %q", got)
	}
}

func TestOverlayCenter(t *testing.T) {
	base := strings.Repeat(strings.Repeat(".", 20)+"\n", 6) + strings.Repeat(".", 20)
	out := strings.Split(overlayCenter(base, "XXXX\nXXXX", 20), "\n")
	if len(out) != 7 {
		t.Fatalf("%d lines", len(out))
	}
	if out[2] != "........XXXX........" || out[3] != "........XXXX........" || out[0] != strings.Repeat(".", 20) {
		t.Errorf("overlay:\n%s", strings.Join(out, "\n"))
	}
}

func TestProgressBar(t *testing.T) {
	cases := []struct{ done, total int }{{0, 4}, {1, 10}, {4, 4}}
	want := []string{"▱▱▱▱", "▰▱▱▱", "▰▰▰▰"}
	for i, c := range cases {
		if got := ansi.Strip(progressBar(c.done, c.total, 4)); got != want[i] {
			t.Errorf("progressBar(%d,%d) = %q, want %q", c.done, c.total, got, want[i])
		}
	}
	if progressBar(0, 0, 4) != "" {
		t.Error("no tasks should draw no bar")
	}
}

func chromeModel(t *testing.T, w, h int) *model {
	t.Helper()
	st := notes.New(t.TempDir())
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	st.Create(notes.KindPage, "Groceries", []string{"home"}, "# Groceries\n\n- [ ] milk\n- [x] eggs\n")
	m := newModel(&cli.App{Store: st, Out: io.Discard}, "dark")
	m.width, m.height = w, h
	m.layout()
	m.reload()
	return &m
}

func press(m *model, k string) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	if k == "delete" {
		msg = tea.KeyMsg{Type: tea.KeyDelete}
	}
	next, _ := m.Update(msg)
	*m = next.(model)
}

// The frame fills the window exactly: both panes boxed, titled, and the
// preview hidden on narrow windows unless P shows it.
func TestPanesFillWindow(t *testing.T) {
	for _, w := range []int{120, 79} {
		m := chromeModel(t, w, 30)
		v := m.View()
		lines := strings.Split(v, "\n")
		if len(lines) != 30 {
			t.Errorf("width %d: %d lines, want 30", w, len(lines))
		}
		for i, l := range lines {
			if lw := ansi.StringWidth(l); lw > w {
				t.Errorf("width %d: line %d is %d wide: %q", w, i, lw, ansi.Strip(l))
			}
		}
		plain := ansi.Strip(v)
		if !strings.Contains(plain, "╭─ Pages · modified") || !strings.Contains(plain, "1/1 ─╯") {
			t.Errorf("width %d: list title/position missing:\n%s", w, plain)
		}
		hasPreview := strings.Contains(plain, "╭─ Groceries")
		if hasPreview != (w >= narrowWidth) {
			t.Errorf("width %d: preview shown = %v", w, hasPreview)
		}
		press(m, "P")
		if got := strings.Contains(ansi.Strip(m.View()), "╭─ Groceries"); got == hasPreview {
			t.Errorf("width %d: P didn't toggle the preview", w)
		}
	}
}

func TestDialogs(t *testing.T) {
	m := chromeModel(t, 100, 30)
	press(m, "r")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Rename to") || !strings.Contains(v, "› Groceries") {
		t.Errorf("rename dialog missing:\n%s", v)
	}
	press(m, "esc")
	m.mode = modeNormal
	press(m, "delete")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Delete this note?") || !strings.Contains(v, "page/groceries") {
		t.Errorf("delete dialog missing:\n%s", v)
	}
	press(m, "n")
	if m.mode != modeNormal || len(m.list.Items()) == 0 {
		t.Error("any key but y should cancel the delete")
	}
}

func TestHelpTwoColumns(t *testing.T) {
	narrow, wide := helpBody(64), helpBody(helpTwoColumns+8)
	if lipgloss.Width(narrow) > 64 {
		t.Errorf("narrow help is %d wide", lipgloss.Width(narrow))
	}
	if helpLines(helpTwoColumns+8) >= helpLines(64) {
		t.Error("two columns should take fewer lines")
	}
	// Navigation and the Tasks tab keys sit side by side when wide.
	for _, l := range strings.Split(ansi.Strip(wide), "\n") {
		if strings.Contains(l, "Navigation") && strings.Contains(l, "Tasks tab") {
			return
		}
	}
	t.Error("wide help: Navigation and Tasks tab not on one line")
}

// Writes the screens to $KATNOTES_TUI_DUMP for a look by eye.
func TestDumpScreens(t *testing.T) {
	dir := os.Getenv("KATNOTES_TUI_DUMP")
	if dir == "" {
		t.Skip("set KATNOTES_TUI_DUMP to write screens")
	}
	m := chromeModel(t, 120, 30)
	write := func(name string) {
		os.WriteFile(dir+"/"+name+".txt", []byte(m.View()), 0o644) //nolint:errcheck
	}
	write("pages")
	press(m, "4")
	write("tasks")
	press(m, "v")
	write("alltasks")
	press(m, "1")
	press(m, "r")
	write("rename")
	m.mode = modeNormal
	press(m, "?")
	write("help")
	press(m, "?")
	m.fmOpen = false
	m.resetCache()
	m.preview = ""
	m.updatePreview()
	write("collapsed")
	press(m, "2")
	write("empty")
	m.width = 160
	m.layout()
	press(m, "?")
	write("help-wide")
}

func TestConfigEditedStatus(t *testing.T) {
	t.Setenv(config.EnvConfigDir, t.TempDir())
	t.Setenv("EDITOR", "true")
	m := chromeModel(t, 100, 30)
	for _, c := range []struct {
		dir  bool
		want string
	}{{false, "closed config.toml"}, {true, "closed the config folder"}} {
		next, _ := m.Update(configEditedMsg{dir: c.dir})
		*m = next.(model)
		if !strings.HasPrefix(m.status, c.want) {
			t.Errorf("dir=%v: status %q", c.dir, m.status)
		}
	}
	if cmd := tuiActions["config dir"](m, commands.Result{}); cmd == nil {
		t.Error(":config dir should hand the terminal to the editor")
	}
}
