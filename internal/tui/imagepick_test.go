package tui

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/devinthemtn/katnotes/internal/cli"
	"github.com/devinthemtn/katnotes/internal/commands"
	"github.com/devinthemtn/katnotes/internal/notes"
)

func TestImagePicker(t *testing.T) {
	st := notes.New(t.TempDir())
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	st.Create(notes.KindPage, "Pets", nil, "# Pets\n")
	pics := t.TempDir()
	os.MkdirAll(filepath.Join(pics, "cats"), 0o755)
	os.WriteFile(filepath.Join(pics, "cats", "Tom.png"), []byte("\x89PNG\r\n\x1a\ntom"), 0o644)
	os.WriteFile(filepath.Join(pics, "dog.jpg"), []byte("jpgdata"), 0o644)
	os.WriteFile(filepath.Join(pics, "notes.txt"), []byte("not an image"), 0o644)
	os.WriteFile(filepath.Join(pics, ".hidden.png"), []byte("\x89PNG\r\n\x1a\nhid"), 0o644)

	m := newModel(&cli.App{Store: st, Out: io.Discard}, "dark")
	m.width, m.height = 100, 30
	m.layout()
	m.reload()
	m.imgDir = pics
	key := func(k string) {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "backspace":
			msg = tea.KeyMsg{Type: tea.KeyBackspace}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		}
		next, _ := m.Update(msg)
		m = next.(model)
	}
	typ := func(s string) {
		for _, r := range s {
			key(string(r))
		}
	}
	names := func() string {
		var out []string
		for _, e := range m.img.visible() {
			out = append(out, e.name)
		}
		return strings.Join(out, " ")
	}

	tuiActions["image"](&m, commands.Result{})
	if m.mode != modeImage {
		t.Fatalf("mode = %v", m.mode)
	}
	if got := names(); got != ".. cats dog.jpg" {
		t.Errorf("entries = %q", got)
	}
	if v := m.View(); !strings.Contains(v, "Attach an image to page/pets") || !strings.Contains(v, "dog.jpg") {
		t.Errorf("view:\n%s", v)
	}
	typ(".")
	if got := names(); got != ".hidden.png dog.jpg" {
		t.Errorf("hidden filter = %q", got)
	}
	key("backspace")
	key("down") // cats
	key("enter")
	if m.img.dir != filepath.Join(pics, "cats") || names() != ".. Tom.png" {
		t.Fatalf("in cats: %s %q", m.img.dir, names())
	}
	key("backspace") // up a folder
	if m.img.dir != pics {
		t.Fatalf("backspace: %s", m.img.dir)
	}
	typ("dog")
	key("enter")
	if m.mode != modeNormal || m.img != nil {
		t.Fatalf("picker still open: %v %+v", m.mode, m.img)
	}
	n, _ := st.Get(notes.KindPage, "pets")
	if !strings.HasSuffix(n.Body, "\n\n![](../assets/dog.jpg)\n") {
		t.Errorf("body = %q", n.Body)
	}

	// A typed path, relative to the picker's folder.
	tuiActions["image"](&m, commands.Result{})
	if m.img.dir != pics {
		t.Errorf("didn't reopen in the last folder: %s", m.img.dir)
	}
	typ("cats/tom.png")
	key("enter")
	if m.img == nil || m.img.err == "" {
		t.Fatal("missing file should keep the picker open with an error")
	}
	for range len("tom.png") {
		key("backspace")
	}
	typ("Tom.png")
	key("enter")
	n, _ = st.Get(notes.KindPage, "pets")
	if m.img != nil || !strings.Contains(n.Body, "![](../assets/tom.png)") {
		t.Errorf("typed path: img=%+v body=%q", m.img, n.Body)
	}

	// :image <path> attaches without the picker; esc closes it.
	tuiActions["image"](&m, commands.Result{Args: []string{filepath.Join(pics, "notes.txt")}})
	if !m.isErr {
		t.Errorf("text file attached: %s", m.status)
	}
	tuiActions["image"](&m, commands.Result{})
	key("esc")
	if m.mode != modeNormal {
		t.Errorf("esc: mode = %v", m.mode)
	}
}
