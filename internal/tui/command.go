package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/devinthemtn/katnotes/internal/commands"
)

// The ":" command window. Commands, their arguments and completion come from
// internal/commands, shared with the GUI's palette; this file draws the
// window and carries out the results, including the UI-only commands.

// tuiActions runs the catalog's UI commands (Result.UI) in the TUI.
var tuiActions = map[string]func(m *model, r commands.Result) tea.Cmd{
	"frontmatter": func(m *model, _ commands.Result) tea.Cmd { return m.frontmatterCmd() },
	"delete": func(m *model, _ commands.Result) tea.Cmd {
		m.confirmDelete()
		return nil
	},
	"tasks": func(m *model, r commands.Result) tea.Cmd {
		m.setTaskFilters(r.Args)
		m.switchTab(tabTasks)
		return nil
	},
	"terminal": func(m *model, _ commands.Result) tea.Cmd { return m.shellCmd() },
	"theme": func(m *model, r commands.Result) tea.Cmd {
		if len(r.Args) == 1 {
			m.switchTheme(r.Args[0])
		}
		return nil
	},
	"external": func(m *model, _ commands.Result) tea.Cmd {
		if n := m.selected(); n != nil {
			return m.externalCmd(n)
		}
		return nil
	},
	"open-with": func(m *model, r commands.Result) tea.Cmd {
		if n := m.selected(); n != nil && len(r.Args) == 1 {
			return m.openWithCmd(n, r.Args[0])
		}
		return nil
	},
	"reload": func(m *model, _ commands.Result) tea.Cmd {
		m.reloadAll()
		return nil
	},
	"help": func(m *model, _ commands.Result) tea.Cmd {
		m.openHelp()
		return nil
	},
	"quit": func(m *model, _ commands.Result) tea.Cmd { return tea.Quit },
	"search": func(m *model, r commands.Result) tea.Cmd {
		if len(r.Args) == 2 {
			m.applySearch(r.Args[0], r.Args[1])
		}
		return nil
	},
	"links": func(m *model, _ commands.Result) tea.Cmd {
		if n := m.selected(); n != nil {
			m.openLinks(n, false)
		}
		return nil
	},
	"backlinks": func(m *model, _ commands.Result) tea.Cmd {
		if n := m.selected(); n != nil {
			m.openLinks(n, true)
		}
		return nil
	},
}

// maxCmdRows caps the dropdown height.
const maxCmdRows = 6

var (
	cmdBox      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#7D56F4"))
	cmdName     = lipgloss.NewStyle().Bold(true)
	cmdSelected = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#7D56F4"))
	cmdGhost    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

func newCmdInput() textinput.Model {
	ti := textinput.New()
	ti.Prompt = ":"
	ti.CharLimit = 200
	return ti
}

func (m *model) cmdEnv() *commands.Env {
	return &commands.Env{Store: m.app.Store, Note: m.selected(), App: commands.AppTUI, History: m.cmdHist,
		Search: commands.CurrentSearch{Mode: string(m.searchMode), Text: m.query}}
}

// refreshCompletion recomputes the suggestions after the input changed.
func (m *model) refreshCompletion() {
	m.cmdComp = commands.Complete(m.cmdEnv(), m.cmdInput.Value())
	m.cmdSel, m.cmdMoved = 0, false
}

func (m *model) setCmdInput(v string) {
	m.cmdInput.SetValue(v)
	m.cmdInput.CursorEnd()
	m.refreshCompletion()
}

func (m *model) openCommand() tea.Cmd {
	m.mode = modeCommand
	m.cmdHistIdx = -1
	m.cmdQuick = false
	m.setCmdInput("")
	return m.cmdInput.Focus()
}

// openQuickOpen is the command window as a jump-to-note box: ":open " with
// fuzzy title / filename matching, recent notes first. Enter opens the
// highlighted note.
func (m *model) openQuickOpen() tea.Cmd { return m.openNotePicker("open ") }

// openLinkPicker is the same box for ":link ": enter links the selected note
// to the highlighted one.
func (m *model) openLinkPicker() tea.Cmd {
	if m.selected() == nil {
		m.setErr(fmt.Errorf("link: select a note first"))
		return nil
	}
	return m.openNotePicker("link ")
}

// openNotePicker opens the command window on cmd (a command taking a note),
// where enter takes the highlighted note even if it wasn't picked with ↑↓.
func (m *model) openNotePicker(cmd string) tea.Cmd {
	c := m.openCommand()
	m.cmdQuick = true
	m.setCmdInput(cmd)
	return c
}

func (m *model) closeCommand() {
	m.mode = modeNormal
	m.cmdInput.Blur()
}

// accept puts suggestion i into the input.
func (m *model) accept(i int) {
	if i < len(m.cmdComp.Items) {
		m.setCmdInput(m.cmdComp.Keep + m.cmdComp.Items[i].Value)
	}
}

func (m model) updateCommand(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.cmdComp.Items
	move := func(d int) {
		if n := len(items); n > 0 {
			m.cmdSel = (m.cmdSel + d + n) % n
			m.cmdMoved = true
		}
	}
	switch msg.String() {
	case "esc", "ctrl+c":
		m.closeCommand()
		return m, nil
	case "down", "ctrl+n":
		if m.cmdHistIdx >= 0 {
			// Walking back towards the newest history entry.
			if m.cmdHistIdx--; m.cmdHistIdx < 0 {
				m.setCmdInput("")
			} else {
				m.setCmdInput(m.cmdHist[m.cmdHistIdx])
			}
			return m, nil
		}
		move(1)
		return m, nil
	case "up", "ctrl+p":
		// On an empty prompt (or while browsing), ↑ walks the history.
		if (m.cmdInput.Value() == "" || m.cmdHistIdx >= 0) && m.cmdHistIdx+1 < len(m.cmdHist) {
			m.cmdHistIdx++
			m.setCmdInput(m.cmdHist[m.cmdHistIdx])
			return m, nil
		}
		move(-1)
		return m, nil
	case "tab", "shift+tab":
		if len(items) == 0 {
			return m, nil
		}
		if m.cmdComp.Fragment == items[m.cmdSel].Value {
			// Already complete: cycle through the matches.
			if msg.String() == "tab" {
				move(1)
			} else {
				move(-1)
			}
			m.cmdInput.SetValue(m.cmdComp.Keep + items[m.cmdSel].Value)
			m.cmdInput.CursorEnd()
			return m, nil
		}
		m.cmdHistIdx = -1
		m.accept(m.cmdSel)
		return m, nil
	case "enter":
		// Use the highlighted suggestion when the user picked one with ↑↓,
		// or when what was typed isn't a command name yet.
		if len(items) > 0 && (m.cmdMoved || (m.cmdComp.Phase == "command" && !m.cmdComp.Exact) ||
			(m.cmdQuick && m.cmdComp.Phase == "arg")) {
			m.accept(m.cmdSel)
		}
		line := strings.TrimSpace(m.cmdInput.Value())
		m.closeCommand()
		if line == "" {
			return m, nil
		}
		return m, m.runCommandLine(line)
	}
	before := m.cmdInput.Value()
	var cmd tea.Cmd
	m.cmdInput, cmd = m.cmdInput.Update(msg)
	if m.cmdInput.Value() != before {
		m.cmdHistIdx = -1
		m.refreshCompletion()
	}
	return m, cmd
}

// cmdResultMsg carries the result of a command run in the background
// (Spec.Long, e.g. :sync).
type cmdResultMsg struct {
	line string
	r    commands.Result
	err  error
}

// runCommandLine runs a command and applies what it returned. Slow commands
// run in the background and report back with cmdResultMsg.
func (m *model) runCommandLine(line string) tea.Cmd {
	env := m.cmdEnv()
	if p, err := commands.Parse(commands.AppTUI, line); err == nil && p.Spec.Long {
		m.setStatus(p.Spec.Name + "…")
		return func() tea.Msg {
			r, err := commands.Run(env, line)
			return cmdResultMsg{line: line, r: r, err: err}
		}
	}
	r, err := commands.Run(env, line)
	return m.applyResult(line, r, err)
}

func (m *model) applyResult(line string, r commands.Result, err error) tea.Cmd {
	if r.Changed {
		m.resetCache()
	}
	if err != nil {
		m.setErr(err)
		if r.Changed {
			m.reload()
		}
		return nil
	}
	if m.cmdHistPath != "" && line != "" { // "" when run by a key, not typed
		if h, err := commands.AddHistory(m.cmdHistPath, line); err == nil {
			m.cmdHist = h
		}
	}
	if r.Status != "" {
		m.setStatus(r.Status)
	}
	if r.Clipboard != "" {
		copyToClipboard(r.Clipboard)
	}
	if act, ok := tuiActions[r.UI]; ok {
		return act(m, r)
	}
	if r.PickTitle != "" {
		if len(r.Pick) == 0 {
			if r.Changed {
				m.reload()
			}
			return nil // the status says there's nothing to list
		}
		m.showPick(r.PickTitle, r.Pick)
		return nil
	}
	switch {
	case r.Note != nil:
		m.touchRecent(r.Note)
		m.showNote(r.Note)
	case r.Changed:
		m.reload()
	}
	if r.Edit && r.Note != nil {
		return m.editCmd(r.Note)
	}
	return nil
}

// copyToClipboard uses the system clipboard when there is one, and the
// terminal's OSC 52 escape otherwise (works over SSH and in most modern
// terminals, including Windows Terminal under WSL).
func copyToClipboard(s string) {
	if err := clipboard.WriteAll(s); err != nil {
		termenv.Copy(s)
	}
}

// commandInputView is the ":" prompt with a ghost completion of the selection.
func (m model) commandInputView() string {
	v := m.cmdInput.View()
	c := m.cmdComp
	if m.cmdSel < len(c.Items) && c.Fragment != "" {
		val := strings.TrimRight(c.Items[m.cmdSel].Value, " ")
		if len(val) > len(c.Fragment) && strings.EqualFold(val[:len(c.Fragment)], c.Fragment) {
			v += cmdGhost.Render(val[len(c.Fragment):])
		}
	}
	return v
}

// commandDropdown renders the suggestion box.
func (m model) commandDropdown() string {
	c := m.cmdComp
	w := min(max(m.width-4, 20), 72)
	var rows []string
	if c.Phase == "arg" || c.Phase == "none" || c.Exact {
		head := " " + c.Usage
		if c.Desc != "" {
			head += " — " + c.Desc
		}
		rows = append(rows, cmdGhost.Render(truncate(head, w)))
	}
	if len(c.Items) == 0 {
		switch c.Phase {
		case "command":
			rows = append(rows, errStyle.Render(" no matching command "))
		case "arg":
			rows = append(rows, cmdGhost.Render(" <"+c.Arg+">: type a value"))
		default:
			rows = append(rows, cmdGhost.Render(" enter runs it"))
		}
		return cmdBox.Render(strings.Join(rows, "\n"))
	}
	start := 0
	if m.cmdSel >= maxCmdRows {
		start = m.cmdSel - maxCmdRows + 1
	}
	end := min(start+maxCmdRows, len(c.Items))
	nameW := 0
	for _, s := range c.Items[start:end] {
		nameW = max(nameW, len([]rune(s.Label)))
	}
	nameW = min(nameW+2, w/2)
	for i := start; i < end; i++ {
		s := c.Items[i]
		label := s.Label
		if s.Recent {
			label = "↺ " + label
		}
		row := " " + cmdName.Width(nameW).Render(truncate(label, nameW-1)) + s.Detail
		row = lipgloss.NewStyle().Width(w).Render(truncate(row, w))
		if i == m.cmdSel {
			row = cmdSelected.Render(row)
		}
		rows = append(rows, row)
	}
	hint := " tab complete · ↑↓ select · enter run · esc close"
	if len(m.cmdHist) > 0 && m.cmdInput.Value() == "" {
		hint = " ↑ history · tab complete · enter run · esc close"
	}
	rows = append(rows, cmdGhost.Render(hint))
	return cmdBox.Render(strings.Join(rows, "\n"))
}

// truncate shortens s to at most w cells, ending in "…" when cut.
func truncate(s string, w int) string {
	if w <= 1 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r)) > w-1 {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// overlayBottom replaces the last lines of base with box (left aligned).
func overlayBottom(base, box string) string {
	bl := strings.Split(base, "\n")
	xl := strings.Split(box, "\n")
	if len(xl) >= len(bl) {
		return box
	}
	copy(bl[len(bl)-len(xl):], xl)
	return strings.Join(bl, "\n")
}

// shellCmd suspends the TUI and runs the user's shell in the selected note's
// folder (the notes root when nothing is selected). Exiting the shell
// returns to KatNotes and reloads, in case files changed meanwhile.
func (m *model) shellCmd() tea.Cmd {
	dir := m.app.Store.Root
	if n := m.selected(); n != nil && n.Path != "" {
		dir = filepath.Dir(n.Path)
	}
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	c := exec.Command(sh)
	c.Dir = dir
	return tea.ExecProcess(c, func(err error) tea.Msg { return shellDoneMsg{err} })
}

// shellDoneMsg reports that the ":terminal" shell exited.
type shellDoneMsg struct{ err error }
