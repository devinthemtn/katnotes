package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/devinthemtn/katnotes/internal/editor"
	"github.com/devinthemtn/katnotes/internal/notes"
)

// ":frontmatter" opens the selected note's frontmatter, alone, as a YAML file
// in the editor. Saving and quitting applies it; quitting without saving
// cancels. Text that fails to apply is kept and offered again next time.

// fmEditedMsg reports that the editor on a frontmatter file exited.
type fmEditedMsg struct {
	n      *notes.Note
	path   string    // temporary YAML file
	before time.Time // its mtime before editing
	err    error
}

func (m *model) frontmatterCmd() tea.Cmd {
	sel := m.selected()
	if sel == nil {
		m.setErr(fmt.Errorf("frontmatter: select a note first"))
		return nil
	}
	n, err := m.app.Store.Get(sel.Kind, sel.ID)
	if err != nil {
		m.setErr(err)
		return nil
	}
	text, ok := m.fmDrafts[n.Key()]
	if !ok {
		if text, err = frontmatterTemplate(m.app.Store, n); err != nil {
			m.setErr(fmt.Errorf("frontmatter: %w", err))
			return nil
		}
	}

	f, err := os.CreateTemp("", "katnotes-frontmatter-*.yaml")
	if err != nil {
		m.setErr(err)
		return nil
	}
	_, err = f.WriteString(text)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		m.setErr(err)
		return nil
	}
	fi, err := os.Stat(f.Name())
	if err != nil {
		os.Remove(f.Name())
		m.setErr(err)
		return nil
	}
	c, err := editor.Command(m.app.Cfg.Editor, f.Name())
	if err != nil {
		os.Remove(f.Name())
		m.setErr(err)
		return nil
	}
	msg := fmEditedMsg{n: n, path: f.Name(), before: fi.ModTime()}
	return tea.ExecProcess(c, func(err error) tea.Msg { msg.err = err; return msg })
}

// frontmatterTemplate is the text the editor opens: a comment header and the
// note's frontmatter as YAML (inferred values when the file has none).
func frontmatterTemplate(s *notes.Store, n *notes.Note) (string, error) {
	has, err := s.HasFrontmatter(n.Kind, n.ID)
	if err != nil {
		return "", err
	}
	y, err := notes.FrontmatterYAML(n)
	if err != nil {
		return "", err
	}
	head := "# Frontmatter for " + n.Key() + ". Save and quit to apply, quit without saving to cancel.\n"
	if !has {
		head += "# This note has no frontmatter yet. These values were inferred from the\n" +
			"# file (first heading, file dates); saving adds them to it.\n"
	}
	head += "# tags: [a, b] · created: 2006-01-02 · \"updated\" is set when you save.\n" +
		"# Changing the title renames the note. Other keys are kept as they are.\n\n"
	return head + string(y), nil
}

func (m *model) afterFrontmatterEdit(msg fmEditedMsg) {
	defer os.Remove(msg.path)
	k := msg.n.Key()
	if msg.err != nil {
		m.setErr(fmt.Errorf("editor: %w", msg.err))
		return
	}
	fi, err := os.Stat(msg.path)
	if err != nil {
		m.setErr(err)
		return
	}
	if fi.ModTime().Equal(msg.before) {
		if _, retry := m.fmDrafts[k]; retry {
			delete(m.fmDrafts, k)
			m.setStatus("frontmatter: discarded the unsaved edits")
		} else {
			m.setStatus("frontmatter: unchanged")
		}
		return
	}
	text, err := os.ReadFile(msg.path)
	if err != nil {
		m.setErr(err)
		return
	}
	if onlyComments(string(text)) {
		delete(m.fmDrafts, k)
		m.setStatus("frontmatter: file was emptied, nothing saved")
		return
	}
	r, relinked, err := m.app.Store.ApplyFrontmatter(msg.n, text)
	if err != nil {
		if m.fmDrafts == nil {
			m.fmDrafts = map[string]string{}
		}
		m.fmDrafts[k] = string(text)
		m.setErr(fmt.Errorf("%w (not saved; :frontmatter reopens your text)", err))
		return
	}
	delete(m.fmDrafts, k)
	status := "frontmatter saved for " + r.Key()
	if r.Key() != k {
		status = "frontmatter saved; renamed to " + r.Key()
	}
	if len(relinked) > 0 {
		status += fmt.Sprintf(" · updated links in %d note(s)", len(relinked))
	}
	m.cache = map[string]string{}
	m.setStatus(status)
	m.reload()
	m.selectKey(r.Key())
}

// onlyComments reports whether text has no YAML besides # comments.
func onlyComments(text string) bool {
	for _, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); l != "" && l != "---" && !strings.HasPrefix(l, "#") {
			return false
		}
	}
	return true
}
