package tui

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
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

	accentColor, borderColor, dangerColor = c("accent"), c("border"), c("danger")
	tagStyle = lipgloss.NewStyle().Foreground(c("accent"))
	doneStyle = lipgloss.NewStyle().Foreground(c("muted")).Strikethrough(true)
	badgeOverdue = lipgloss.NewStyle().Foreground(c("danger")).Reverse(true).Padding(0, 1)
	badgeToday = lipgloss.NewStyle().Foreground(c("warning")).Reverse(true).Padding(0, 1)
	keyStyle = lipgloss.NewStyle().Bold(true).Foreground(c("accent"))
	barOn = lipgloss.NewStyle().Foreground(c("accent"))
	barOff = lipgloss.NewStyle().Foreground(c("border"))
	catStyle = lipgloss.NewStyle().Bold(true).Foreground(c("accent"))
}

// glamourConfig is the Markdown preview style: glamour's light or dark
// style with headings, links, code and rules in the theme's colours.
func glamourConfig() ansi.StyleConfig {
	cfg := styles.DarkStyleConfig
	if !curTheme.Dark {
		cfg = styles.LightStyleConfig
	}
	col := func(k string) *string { v := curTheme.Color(k); return &v }
	yes := true

	cfg.Heading.Color = col("heading")
	cfg.H1.Color = col("accent_text")
	cfg.H1.BackgroundColor = col("accent")
	cfg.H6.Color = col("muted")
	cfg.Link.Color = col("accent")
	cfg.LinkText.Color = col("accent")
	cfg.LinkText.Bold = &yes
	cfg.Code.BackgroundColor = col("code_bg")
	cfg.BlockQuote.Color = col("muted")
	bar := "│ "
	cfg.BlockQuote.IndentToken = &bar
	cfg.HorizontalRule.Color = col("border")
	return cfg
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
