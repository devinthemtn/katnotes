package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/devinthemtn/katnotes/internal/notes"
)

// Mouse support (config: tui_mouse). Run turns mouse reporting on; this
// maps clicks and the wheel onto the same actions as the keys. The screen
// is laid out by View():
//
//	row 0            tabs
//	row 1            filter line
//	rows 2…          the panes: list box at x [0, listW), preview after it
//	row height-2     status / input line (the search bar while searching)
//	row height-1     key hints

// doubleClick is how close two clicks on one row must be to count as a
// double click (bubbletea reports single presses only).
const doubleClick = 400 * time.Millisecond

// wheelStep is how many lines one wheel notch scrolls.
const wheelStep = 3

// bodyTop is the screen row of the panes' top border.
const bodyTop = 2

func (m model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	wheel := msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown
	click := msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress
	if !wheel && !click {
		return m, nil
	}
	up := msg.Button == tea.MouseButtonWheelUp

	switch m.mode {
	case modeHelp:
		if wheel {
			if up {
				m.help.ScrollUp(wheelStep)
			} else {
				m.help.ScrollDown(wheelStep)
			}
		} else if !m.inHelpBox(msg.X, msg.Y) {
			m.mode = modeNormal
		}
		return m, nil
	case modeCommand:
		return m.mouseCommand(msg, wheel, up)
	case modeInput, modeConfirm:
		// A stray click mustn't cancel typing or answer the delete question.
		return m, nil
	}

	// Normal and search modes.
	if m.mode == modeSearch && click && msg.Y == m.height-2 {
		if md, ok := m.searchChipAt(msg.X); ok && md != m.searchMode {
			m.searchMode = md
			m.list.Select(0)
			m.reload()
		}
		return m, nil
	}
	if m.mode == modeNormal && click && msg.Y == 0 {
		if t, ok := tabAt(msg.X); ok {
			m.switchTab(t)
		}
		return m, nil
	}
	if !m.inBody(msg.Y) {
		return m, nil
	}
	if m.showPreview() && msg.X > m.listW {
		if click && m.mode == modeNormal {
			m.setFocusPreview(true)
		}
		if wheel {
			if up {
				m.scrollPreview(-wheelStep)
			} else {
				m.scrollPreview(wheelStep)
			}
		}
		return m, nil
	}
	if msg.X >= m.listW {
		return m, nil
	}
	if click && m.mode == modeNormal {
		m.setFocusPreview(false)
	}
	if wheel {
		if up {
			m.list.CursorUp()
			m.skipHeaders("up")
		} else {
			m.list.CursorDown()
			m.skipHeaders("down")
		}
		m.updatePreview()
		return m, nil
	}
	i, ok := m.listRowAt(msg.Y)
	if !ok {
		return m, nil
	}
	now := time.Now()
	double := i == m.lastClickRow && now.Sub(m.lastClick) < doubleClick
	m.lastClickRow, m.lastClick = i, now
	m.list.Select(i)
	m.updatePreview()
	if double && m.mode == modeNormal {
		m.lastClick = time.Time{} // a third click starts over
		return m, m.enter(true)
	}
	return m, nil
}

// mouseCommand handles the mouse while the ":" window is open: the wheel
// moves through the suggestions, a click picks one, a double click runs it.
func (m model) mouseCommand(msg tea.MouseMsg, wheel, up bool) (tea.Model, tea.Cmd) {
	n := len(m.cmdComp.Items)
	if n == 0 {
		return m, nil
	}
	if wheel {
		d := 1
		if up {
			d = -1
		}
		m.cmdSel = (m.cmdSel + d + n) % n
		m.cmdMoved = true
		return m, nil
	}
	i, ok := m.cmdItemAt(msg.X, msg.Y)
	if !ok {
		return m, nil
	}
	now := time.Now()
	double := i == m.lastClickRow && now.Sub(m.lastClick) < doubleClick
	m.lastClickRow, m.lastClick = i, now
	m.cmdSel, m.cmdMoved = i, true
	if double {
		m.lastClick = time.Time{}
		return m.updateCommand(tea.KeyMsg{Type: tea.KeyEnter})
	}
	return m, nil
}

// bodyLines is how many screen rows the panes take, borders included.
func (m *model) bodyLines() int { return m.list.Height() + 2 }

// inBody reports whether screen row y is inside the panes.
func (m *model) inBody(y int) bool { return y >= bodyTop && y < bodyTop+m.bodyLines() }

// listRowAt is the list item drawn at screen row y, if any. The list
// draws each row as its title and description lines (up to rowH lines in
// all, not padded) with one blank line between rows. Month headers and the
// blank lines aren't clickable.
func (m *model) listRowAt(y int) (int, bool) {
	off := y - bodyTop - 1 // below the box's top border
	if off < 0 {
		return 0, false
	}
	items := m.list.Items()
	start, end := m.list.Paginator.GetSliceBounds(len(items))
	top := 0
	for i := start; i < end; i++ {
		h := 1
		if d, ok := items[i].(list.DefaultItem); ok {
			h = min(1+strings.Count(d.Description(), "\n")+1, max(m.rowH, 1))
		}
		if off < top+h {
			if _, header := items[i].(headerItem); header {
				return 0, false
			}
			return i, true
		}
		top += h + 1
		if off < top {
			return 0, false // the blank line under a row
		}
	}
	return 0, false
}

// tabAt is the tab whose label covers screen column x of the tab bar.
func tabAt(x int) (tab, bool) {
	left := 0
	for i, name := range tabNames {
		// Active and inactive tabs are padded alike, so either measures.
		w := lipgloss.Width(inactiveTab.Render(tabLabel(i, name)))
		if x >= left && x < left+w {
			return tab(i), true
		}
		left += w
	}
	return 0, false
}

// tabLabel is the text of tab i in the tab bar.
func tabLabel(i int, name string) string { return string(rune('1'+i)) + " " + name }

// searchChipAt is the search mode whose chip covers column x of the
// search bar.
func (m *model) searchChipAt(x int) (notes.SearchMode, bool) {
	left := 0
	for _, md := range notes.SearchModes {
		w := lipgloss.Width(m.searchChip(md))
		if x >= left && x < left+w {
			return md, true
		}
		left += w
	}
	return "", false
}

// cmdItemAt is the suggestion drawn at (x, y) in the ":" dropdown, which
// sits over the bottom of the panes.
func (m *model) cmdItemAt(x, y int) (int, bool) {
	box := m.commandDropdown()
	if x < 0 || x >= lipgloss.Width(box) {
		return 0, false
	}
	top := bodyTop + m.bodyLines() - lipgloss.Height(box)
	head, start, end := m.cmdDropdownRows()
	i := start + y - top - 1 - head // below the border and the usage row
	if i < start || i >= end {
		return 0, false
	}
	return i, true
}

// inHelpBox reports whether (x, y) falls on the help popup, which View
// centres in the window.
func (m *model) inHelpBox(x, y int) bool {
	box := m.helpBox()
	w, h := lipgloss.Width(box), lipgloss.Height(box)
	x0, y0 := (m.width-w)/2, (m.height-h)/2
	// Place may round the other way; a column or row of slack is fine.
	return x >= x0-1 && x <= x0+w && y >= y0-1 && y <= y0+h
}
