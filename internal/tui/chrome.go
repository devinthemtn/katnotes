package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Drawing helpers for the TUI's frame: boxes with titles in their borders,
// centred modal dialogs and small inline widgets.

// Theme colours and styles used by these helpers; applyTheme sets them.
var (
	accentColor = lipgloss.Color("#7D56F4")
	borderColor = lipgloss.Color("238")
	dangerColor = lipgloss.Color("#F7768E")
	// {{ … }} in the preview (highlight.go).
	highlightColor = lipgloss.Color("#18181F")
	highlightBg    = lipgloss.Color("#E0AF68")

	tagStyle     = lipgloss.NewStyle().Foreground(accentColor)
	doneStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Strikethrough(true)
	badgeOverdue = lipgloss.NewStyle().Foreground(dangerColor).Reverse(true).Padding(0, 1)
	badgeToday   = lipgloss.NewStyle().Foreground(lipgloss.Color("#E0AF68")).Reverse(true).Padding(0, 1)
	keyStyle     = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	barOn        = lipgloss.NewStyle().Foreground(accentColor)
	barOff       = lipgloss.NewStyle().Foreground(borderColor)
	catStyle     = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
)

// titledBox draws content in a rounded box whose inner size is w×h, with
// title set into the top border and footer into the bottom one (either may
// be ""). Content lines are cut or padded to fit.
func titledBox(content string, w, h int, color lipgloss.Color, title, footer string) string {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	bc := lipgloss.NewStyle().Foreground(color)
	lines := strings.Split(content, "\n")
	var b strings.Builder
	b.WriteString(bc.Render("╭") + borderLabel(bc, title, w, true) + bc.Render("╮") + "\n")
	for i := 0; i < h; i++ {
		line := ""
		if i < len(lines) {
			line = ansi.Truncate(lines[i], w, "")
		}
		if pad := w - ansi.StringWidth(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		b.WriteString(bc.Render("│") + line + bc.Render("│") + "\n")
	}
	b.WriteString(bc.Render("╰") + borderLabel(bc, footer, w, false) + bc.Render("╯"))
	return b.String()
}

// borderLabel is a horizontal border w wide with label set into it: on the
// left ("─ Title ───") for a title, on the right ("─── 42% ─") for a footer.
// The lines are drawn in bc; the label keeps the terminal's text colour
// (a title in bold).
func borderLabel(bc lipgloss.Style, label string, w int, left bool) string {
	label = strings.TrimSpace(label)
	if label == "" || w < 6 {
		return bc.Render(strings.Repeat("─", w))
	}
	label = ansi.Truncate(label, w-4, "…")
	// "─ " + label + " " + fill (title) or fill + " " + label + " ─" (footer)
	fill := w - ansi.StringWidth(label) - 3
	text := label
	if left {
		text = lipgloss.NewStyle().Bold(true).Render(label)
		return bc.Render("─ ") + text + bc.Render(" "+strings.Repeat("─", fill))
	}
	return bc.Render(strings.Repeat("─", fill)+" ") + text + bc.Render(" ─")
}

// modal is a dialog box w wide (inner): a bold title, the body and a muted
// hint line, framed in color.
func modal(title, body, hint string, w int, color lipgloss.Color) string {
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(color).Padding(0, 1).Width(w)
	parts := []string{lipgloss.NewStyle().Bold(true).Foreground(color).Render(title)}
	if body != "" {
		parts = append(parts, "", body)
	}
	if hint != "" {
		parts = append(parts, "", statusStyle.Render(hint))
	}
	return box.Render(strings.Join(parts, "\n"))
}

// overlayCenter draws box over the middle of base, which is width columns
// wide; the base shows on either side of the box.
func overlayCenter(base, box string, width int) string {
	bl := strings.Split(base, "\n")
	xl := strings.Split(box, "\n")
	if len(xl) >= len(bl) {
		return box
	}
	bw := lipgloss.Width(box)
	x := max((width-bw)/2, 0)
	y := (len(bl) - len(xl)) / 2
	for i, line := range xl {
		under := bl[y+i]
		left := ansi.Truncate(under, x, "")
		if pad := x - ansi.StringWidth(left); pad > 0 {
			left += strings.Repeat(" ", pad)
		}
		bl[y+i] = left + line + ansi.TruncateLeft(under, x+bw, "")
	}
	return strings.Join(bl, "\n")
}

// progressBar is a cells-wide bar showing done of total, like "▰▰▰▱▱".
func progressBar(done, total, cells int) string {
	if total <= 0 || cells <= 0 {
		return ""
	}
	on := done * cells / total
	if done > 0 && on == 0 {
		on = 1
	}
	return barOn.Render(strings.Repeat("▰", on)) + barOff.Render(strings.Repeat("▱", cells-on))
}

// keyHints renders "key desc" pairs with the keys picked out, e.g.
// keyHints("n", "new note", "J", "today").
func keyHints(pairs ...string) string {
	var out []string
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, keyStyle.Render(pairs[i])+" "+statusStyle.Render(pairs[i+1]))
	}
	return strings.Join(out, statusStyle.Render("  ·  "))
}
