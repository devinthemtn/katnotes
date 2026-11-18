package tui

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"

	"github.com/devinthemtn/katnotes/internal/config"
	"github.com/devinthemtn/katnotes/internal/theme"
)

// Colour themes (internal/theme), shared with the GUI. The TUI draws on the
// terminal's own background and text colour and takes the rest from the
// theme: accent, muted, border, heading, danger, warning, info.

var (
	// curTheme is the theme the styles were last set from.
	curTheme = theme.Builtin(true)
	// termDark is whether the terminal background is dark, checked once at
	// start: asking again while Bubble Tea reads the input would race it.
	termDark = true
)

// loadTheme resolves the `theme` option against the terminal background.
func loadTheme(option string) (theme.Theme, error) {
	return theme.Resolve(theme.Dir(), option, termDark)
}

// applyTheme sets every TUI style from t.
func applyTheme(t theme.Theme) {
	curTheme = t
	c := func(k string) lipgloss.Color { return lipgloss.Color(t.Color(k)) }
	onAccent := lipgloss.NewStyle().Foreground(c("accent_text")).Background(c("accent"))

	activeTab = onAccent.Bold(true).Padding(0, 1)
	inactiveTab = lipgloss.NewStyle().Padding(0, 1).Foreground(c("muted"))
	filterStyle = lipgloss.NewStyle().Foreground(c("warning"))
	statusStyle = lipgloss.NewStyle().Foreground(c("muted"))
	errStyle = lipgloss.NewStyle().Foreground(c("danger"))
	priHigh = lipgloss.NewStyle().Bold(true).Foreground(c("danger"))
	priMed = lipgloss.NewStyle().Foreground(c("warning"))
	priLow = lipgloss.NewStyle().Foreground(c("info"))
	paneBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(c("border"))

	helpBox = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(c("accent")).Padding(0, 1)
	helpBanner = onAccent.Bold(true).Padding(0, 1)
	helpVersion = lipgloss.NewStyle().Italic(true).Foreground(c("warning"))
	helpSection = lipgloss.NewStyle().Bold(true).Foreground(c("heading"))

	cmdBox = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(c("accent"))
	cmdSelected = onAccent
	cmdGhost = lipgloss.NewStyle().Foreground(c("muted"))

	chipOn = onAccent.Bold(true).Padding(0, 1)
	chipOff = lipgloss.NewStyle().Foreground(c("muted")).Padding(0, 1)
	groupHd = lipgloss.NewStyle().Bold(true).Foreground(c("heading"))
	fmKey = lipgloss.NewStyle().Foreground(c("muted")).Width(10)
}

// newDelegate is the note list's row renderer in the theme's colours.
func newDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	accent := lipgloss.Color(curTheme.Color("accent"))
	muted := lipgloss.Color(curTheme.Color("muted"))
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.Foreground(accent).BorderLeftForeground(accent)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.Foreground(accent).BorderLeftForeground(accent)
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(muted)
	d.Styles.DimmedDesc = d.Styles.DimmedDesc.Foreground(muted)
	return d
}

// glamourStyle is the Markdown preview style for the current theme.
func glamourStyle() string {
	if curTheme.Dark {
		return "dark"
	}
	return "light"
}

// switchTheme applies the theme option now in effect for the TUI while the TUI runs (":theme").
func (m *model) switchTheme(option string) {
	t, err := loadTheme(option)
	applyTheme(t)
	if cfg, err := config.Load(); err == nil {
		m.app.Cfg.Theme, m.app.Cfg.TUITheme = cfg.Theme, cfg.TUITheme
	}
	m.style = glamourStyle()
	m.setItemHeight(m.list.Items())
	m.layout() // rebuilds the preview renderer in the new style
	if err != nil {
		m.setErr(err)
		return
	}
	m.setStatus("theme: " + t.Name)
}
