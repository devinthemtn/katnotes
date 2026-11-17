package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The ":" command window: type a command name, pick from the dropdown, enter
// runs it. Mirrors the GUI's Ctrl+Shift+P palette.

type tuiCommand struct {
	name string
	desc string
	run  func(m *model) tea.Cmd
}

var commands = []tuiCommand{
	{"sync-tasks", "Update contains_task in every note's frontmatter", func(m *model) tea.Cmd {
		changes, err := m.app.Store.SyncTaskFlags(false)
		switch {
		case err != nil:
			m.setErr(err)
		case len(changes) == 0:
			m.setStatus("sync-tasks: all notes are up to date")
		default:
			ids := make([]string, 0, len(changes))
			for _, c := range changes {
				ids = append(ids, string(c.Kind)+"/"+c.ID)
			}
			m.setStatus(fmt.Sprintf("sync-tasks: updated %d note(s): %s", len(changes), strings.Join(ids, ", ")))
		}
		m.cache = map[string]string{}
		m.reload()
		return nil
	}},
	{"tasks", "Go to the Tasks tab", func(m *model) tea.Cmd {
		m.tab = tabTasks
		m.list.Select(0)
		m.reload()
		return nil
	}},
	{"reload", "Reload notes from disk", func(m *model) tea.Cmd {
		m.cache = map[string]string{}
		m.reload()
		m.setStatus("reloaded")
		return nil
	}},
	{"help", "Show keyboard help", func(m *model) tea.Cmd {
		m.mode = modeHelp
		m.sizeHelp()
		m.help.SetContent(helpBody())
		m.help.GotoTop()
		return nil
	}},
	{"quit", "Quit KatNotes", func(m *model) tea.Cmd { return tea.Quit }},
}

// maxCmdRows caps the dropdown height.
const maxCmdRows = 6

var (
	cmdBox      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#7D56F4"))
	cmdName     = lipgloss.NewStyle().Bold(true).Width(14)
	cmdSelected = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#7D56F4"))
	cmdGhost    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

func isSubseq(needle, hay string) bool {
	i := 0
	for _, r := range hay {
		if i < len(needle) && byte(r) == needle[i] {
			i++
		}
	}
	return i == len(needle)
}

// matchCommands ranks prefix matches first, then subsequence and description matches.
func matchCommands(q string) []tuiCommand {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return commands
	}
	var prefix, rest []tuiCommand
	for _, c := range commands {
		switch {
		case strings.HasPrefix(c.name, q):
			prefix = append(prefix, c)
		case isSubseq(q, c.name) || strings.Contains(strings.ToLower(c.desc), q):
			rest = append(rest, c)
		}
	}
	return append(prefix, rest...)
}

func newCmdInput() textinput.Model {
	ti := textinput.New()
	ti.Prompt = ":"
	ti.CharLimit = 80
	return ti
}

func (m *model) openCommand() tea.Cmd {
	m.mode = modeCommand
	m.cmdSel = 0
	m.cmdInput.SetValue("")
	return m.cmdInput.Focus()
}

func (m model) updateCommand(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	matches := matchCommands(m.cmdInput.Value())
	move := func(d int) {
		if n := len(matches); n > 0 {
			m.cmdSel = (m.cmdSel + d + n) % n
		}
	}
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = modeNormal
		m.cmdInput.Blur()
		return m, nil
	case "down", "ctrl+n":
		move(1)
		return m, nil
	case "up", "ctrl+p":
		move(-1)
		return m, nil
	case "tab", "shift+tab":
		if len(matches) == 0 {
			return m, nil
		}
		if strings.TrimSpace(m.cmdInput.Value()) == matches[m.cmdSel].name {
			// Already complete: cycle through the matches.
			if msg.String() == "tab" {
				move(1)
			} else {
				move(-1)
			}
		}
		m.cmdInput.SetValue(matches[m.cmdSel].name)
		m.cmdInput.CursorEnd()
		return m, nil
	case "enter":
		m.mode = modeNormal
		m.cmdInput.Blur()
		q := strings.ToLower(strings.TrimSpace(m.cmdInput.Value()))
		var c *tuiCommand
		for i := range commands {
			if commands[i].name == q {
				c = &commands[i]
			}
		}
		if c == nil && len(matches) > 0 {
			c = &matches[m.cmdSel]
		}
		if c == nil {
			if q != "" {
				m.setErr(fmt.Errorf("unknown command: %s", q))
			}
			return m, nil
		}
		cmd := c.run(&m)
		return m, cmd
	}
	before := m.cmdInput.Value()
	var cmd tea.Cmd
	m.cmdInput, cmd = m.cmdInput.Update(msg)
	if m.cmdInput.Value() != before {
		m.cmdSel = 0
	}
	return m, cmd
}

// commandInputView is the ":" prompt with a ghost completion of the selection.
func (m model) commandInputView() string {
	v := m.cmdInput.View()
	matches := matchCommands(m.cmdInput.Value())
	q := strings.ToLower(m.cmdInput.Value())
	if len(matches) > 0 && m.cmdSel < len(matches) && q != "" && strings.HasPrefix(matches[m.cmdSel].name, q) {
		v += cmdGhost.Render(strings.TrimPrefix(matches[m.cmdSel].name, q))
	}
	return v
}

// commandDropdown renders the suggestion box, or "" when nothing matches.
func (m model) commandDropdown() string {
	matches := matchCommands(m.cmdInput.Value())
	if len(matches) == 0 {
		return cmdBox.Render(errStyle.Render(" no matching command "))
	}
	start := 0
	if m.cmdSel >= maxCmdRows {
		start = m.cmdSel - maxCmdRows + 1
	}
	end := min(start+maxCmdRows, len(matches))
	w := min(max(m.width-4, 20), 72)
	var rows []string
	for i := start; i < end; i++ {
		c := matches[i]
		row := " " + cmdName.Render(c.name) + c.desc
		if r := []rune(row); len(r) > w {
			row = string(r[:w-1]) + "…"
		}
		row = lipgloss.NewStyle().Width(w).Render(row)
		if i == m.cmdSel {
			row = cmdSelected.Render(row)
		}
		rows = append(rows, row)
	}
	hint := cmdGhost.Render(" tab complete · ↑↓ select · enter run · esc close")
	return cmdBox.Render(strings.Join(append(rows, hint), "\n"))
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
