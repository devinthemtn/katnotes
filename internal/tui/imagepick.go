package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/devinthemtn/katnotes/internal/commands"
	"github.com/devinthemtn/katnotes/internal/notes"
)

// :image — a small file picker for attaching an image to the selected note.
// It lists the folders and image files in one folder; typing filters them,
// or takes a path (anything with a "/" or starting with "~"). The image is
// copied into assets/ and linked at the end of the note.

type pickEntry struct {
	name string
	dir  bool
	size int64
}

type imagePicker struct {
	note    string // key of the note the image goes into
	dir     string
	entries []pickEntry
	input   textinput.Model
	sel     int
	top     int
	err     string
}

// pickerRows is how many entries the picker shows at once.
func (m *model) pickerRows() int { return max(min(12, m.height-14), 3) }

// openImagePicker starts the picker in the last folder used, else
// ~/Pictures, else the home folder.
func (m *model) openImagePicker(n *notes.Note) {
	dir := m.imgDir
	if dir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			dir = home
			if fi, err := os.Stat(filepath.Join(home, "Pictures")); err == nil && fi.IsDir() {
				dir = filepath.Join(home, "Pictures")
			}
		} else if wd, err := os.Getwd(); err == nil {
			dir = wd
		}
	}
	in := textinput.New()
	in.Prompt = "› "
	in.Placeholder = "filter, or type a path"
	in.Focus()
	m.img = &imagePicker{note: n.Key(), input: in}
	m.img.chdir(dir)
	m.mode = modeImage
}

// chdir lists dir. On failure the picker stays where it was.
func (p *imagePicker) chdir(dir string) {
	dir = filepath.Clean(dir)
	des, err := os.ReadDir(dir)
	if err != nil {
		p.err = err.Error()
		return
	}
	var out []pickEntry
	for _, de := range des {
		e := pickEntry{name: de.Name(), dir: de.IsDir()}
		fi, err := os.Stat(filepath.Join(dir, de.Name())) // follows symlinks
		if err != nil {
			continue
		}
		e.dir, e.size = fi.IsDir(), fi.Size()
		if e.dir || notes.IsImageFile(e.name) {
			out = append(out, e)
		}
	}
	slices.SortFunc(out, func(a, b pickEntry) int {
		if a.dir != b.dir {
			if a.dir {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name))
	})
	p.dir, p.entries, p.err = dir, out, ""
	p.input.SetValue("")
	p.sel, p.top = 0, 0
}

// typedPath reports whether the input is a path rather than a filter.
func (p *imagePicker) typedPath() bool {
	v := p.input.Value()
	return strings.ContainsRune(v, '/') || strings.ContainsRune(v, filepath.Separator) || strings.HasPrefix(v, "~")
}

// visible is what the list shows: "..", then the entries matching the
// filter. Hidden files show only while the filter starts with ".".
func (p *imagePicker) visible() []pickEntry {
	if p.typedPath() {
		return nil
	}
	q := strings.ToLower(p.input.Value())
	var out []pickEntry
	if q == "" && filepath.Dir(p.dir) != p.dir {
		out = append(out, pickEntry{name: "..", dir: true})
	}
	for _, e := range p.entries {
		if strings.HasPrefix(e.name, ".") && !strings.HasPrefix(q, ".") {
			continue
		}
		if q == "" || strings.Contains(strings.ToLower(e.name), q) {
			out = append(out, e)
		}
	}
	return out
}

// expand turns a typed path into an absolute one: ~ is the home folder and
// a relative path starts in the picker's folder.
func (p *imagePicker) expand(v string) string {
	if v == "~" || strings.HasPrefix(v, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			v = filepath.Join(home, v[1:])
		}
	}
	if !filepath.IsAbs(v) {
		v = filepath.Join(p.dir, v)
	}
	return filepath.Clean(v)
}

func (m model) updateImage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.img
	vis := p.visible()
	move := func(d int) {
		if len(vis) == 0 {
			return
		}
		p.sel = min(max(p.sel+d, 0), len(vis)-1)
	}
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode, m.img = modeNormal, nil
		return m, nil
	case "down", "ctrl+n":
		move(1)
	case "up", "ctrl+p":
		move(-1)
	case "pgdown":
		move(m.pickerRows())
	case "pgup":
		move(-m.pickerRows())
	case "backspace":
		if p.input.Value() == "" {
			p.chdir(filepath.Dir(p.dir))
			return m, nil
		}
		return m.imageInput(msg)
	case "tab":
		// Into the selected folder, or complete the file's name.
		if p.sel < len(vis) {
			e := vis[p.sel]
			if e.dir {
				p.chdir(filepath.Join(p.dir, e.name))
			} else {
				p.input.SetValue(e.name)
				p.input.CursorEnd()
			}
		}
		return m, nil
	case "enter":
		var path string
		switch {
		case p.typedPath():
			path = p.expand(p.input.Value())
		case p.sel < len(vis):
			path = filepath.Join(p.dir, vis[p.sel].name)
		default:
			return m, nil
		}
		fi, err := os.Stat(path)
		if err != nil {
			p.err = err.Error()
			return m, nil
		}
		if fi.IsDir() {
			p.chdir(path)
			return m, nil
		}
		m.attachImage(path)
		return m, nil
	default:
		return m.imageInput(msg)
	}
	m.scrollPicker()
	return m, nil
}

// imageInput passes a key to the filter / path field.
func (m model) imageInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.img
	before := p.input.Value()
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	if p.input.Value() != before {
		p.sel, p.top, p.err = 0, 0, ""
	}
	return m, cmd
}

// scrollPicker keeps the selection in view.
func (m *model) scrollPicker() {
	p, rows := m.img, m.pickerRows()
	if p.sel < p.top {
		p.top = p.sel
	}
	if p.sel >= p.top+rows {
		p.top = p.sel - rows + 1
	}
}

// attachImage copies path into assets/ and links it from the picker's note.
// On failure the picker stays open with the error.
func (m *model) attachImage(path string) {
	key := ""
	if m.img != nil {
		key = m.img.note
	} else if n := m.selected(); n != nil {
		key = n.Key()
	}
	n, err := m.app.Store.GetKey(key)
	if err == nil {
		var rel string
		if rel, err = m.app.Store.AttachImage(n, path, ""); err == nil {
			if m.img != nil {
				m.imgDir = m.img.dir
			}
			m.mode, m.img = modeNormal, nil
			m.setStatus(fmt.Sprintf("added %s to %s", rel, n.Key()))
			m.reload()
			return
		}
	}
	if m.img != nil {
		m.img.err = err.Error()
		return
	}
	m.setErr(fmt.Errorf("image: %w", err))
}

// imageAction runs ":image [path]": with a path it attaches straight away.
func imageAction(m *model, r commands.Result) tea.Cmd {
	n := m.selected()
	if n == nil {
		m.setErr(fmt.Errorf("image: select a note first"))
		return nil
	}
	if len(r.Args) == 1 && strings.TrimSpace(r.Args[0]) != "" {
		p := &imagePicker{dir: "."}
		if wd, err := os.Getwd(); err == nil {
			p.dir = wd
		}
		m.attachImage(p.expand(strings.TrimSpace(r.Args[0])))
		return nil
	}
	m.openImagePicker(n)
	return nil
}

// imagePickerView is the picker dialog.
func (m *model) imagePickerView() string {
	p := m.img
	w := m.dialogWidth()
	inner := w - 2 // the modal's padding
	dir := p.dir
	if home, err := os.UserHomeDir(); err == nil && (dir == home || strings.HasPrefix(dir, home+string(filepath.Separator))) {
		dir = "~" + dir[len(home):]
	}
	p.input.Width = inner - 4
	lines := []string{statusStyle.Render(ansi.TruncateLeft(dir, max(ansi.StringWidth(dir)-inner, 0), "…")), p.input.View(), ""}
	vis := p.visible()
	rows := m.pickerRows()
	switch {
	case p.typedPath():
		lines = append(lines, statusStyle.Render(ansi.Truncate("enter opens "+p.expand(p.input.Value()), inner, "…")))
	case len(vis) == 0:
		lines = append(lines, statusStyle.Render("no folders or images here"))
	}
	for i := p.top; i < len(vis) && i < p.top+rows; i++ {
		e := vis[i]
		name, size := e.name, ""
		if e.dir {
			name += "/"
		} else {
			size = humanSize(e.size)
		}
		pad := max(inner-2-ansi.StringWidth(size), 1)
		row := ansi.Truncate(name, pad-1, "…")
		row += strings.Repeat(" ", max(pad-ansi.StringWidth(row), 1))
		if i == p.sel {
			lines = append(lines, cmdSelected.Render(" "+row+size+" "))
		} else if e.dir {
			lines = append(lines, " "+tagStyle.Render(row)+statusStyle.Render(size))
		} else {
			lines = append(lines, " "+row+statusStyle.Render(size))
		}
	}
	if len(vis) > rows {
		lines = append(lines, statusStyle.Render(fmt.Sprintf("%d–%d of %d", p.top+1, min(p.top+rows, len(vis)), len(vis))))
	}
	if p.err != "" {
		lines = append(lines, "", errStyle.Render(ansi.Truncate(p.err, inner, "…")))
	}
	return modal("Attach an image to "+p.note, strings.Join(lines, "\n"),
		"enter attach/open · tab folder · ⌫ up · .x hidden · esc", w, accentColor)
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d B", n)
}
