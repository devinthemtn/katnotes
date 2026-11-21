// Package theme loads colour themes shared by the TUI and the GUI. A theme
// is a TOML file, <config dir>/themes/<name>.toml:
//
//	dark = true            # a dark theme (also picks the base for missing colours)
//	[colors]
//	bg = "#18181f"
//	accent = "#9d7df8"
//	…
//
// Colours left out come from the built-in light or dark theme, so a theme
// can change just a few. The option `theme` in config.toml picks one by
// name; "auto" (the default) uses the themes named light and dark to follow
// the system (GUI) or the terminal background (TUI). light.toml and
// dark.toml are written to a new themes folder as starting points.
package theme

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/devinthemtn/katnotes/internal/config"
)

// DirName is the themes folder inside the config directory.
const DirName = "themes"

// Dir returns the themes folder in the config directory ("" if unknown).
func Dir() string {
	p, _ := config.File(DirName) // "" when there is no config directory
	return p
}

// Auto follows the system / terminal: the light or dark theme.
const Auto = "auto"

// Names of the built-in themes.
const (
	Light = "light"
	Dark  = "dark"
)

// Key is one colour of a theme.
type Key struct {
	Name string
	Desc string
}

// Keys lists the colours a theme sets, in file order.
var Keys = []Key{
	{"bg", "GUI window background (the TUI keeps the terminal's)"},
	{"panel", "GUI editor, dialogs and buttons"},
	{"sidebar", "GUI note list sidebar"},
	{"text", "GUI main text (the TUI keeps the terminal's)"},
	{"muted", "secondary text: dates, hints, status"},
	{"border", "borders and dividers"},
	{"accent", "highlights: selection, active tab, links"},
	{"accent_text", "text on an accent background"},
	{"accent_soft", "GUI faint accent background (hover, tags)"},
	{"heading", "TUI section titles (help, list groups)"},
	{"danger", "errors, delete, high priority"},
	{"warning", "unsaved, filters, medium priority"},
	{"info", "TUI low-priority tasks"},
	{"code_bg", "GUI code blocks and inline code"},
}

var builtin = map[string]map[string]string{
	Light: {
		"bg": "#f7f6f3", "panel": "#ffffff", "sidebar": "#efede8", "text": "#24232a",
		"muted": "#76737e", "border": "#dedad2", "accent": "#7d56f4", "accent_text": "#ffffff",
		"accent_soft": "#ece6fe", "heading": "#7d56f4", "danger": "#c43c55", "warning": "#c08a1e",
		"info": "#3d6fd8", "code_bg": "#f0eee9",
	},
	Dark: {
		"bg": "#18181f", "panel": "#1e1e27", "sidebar": "#15151b", "text": "#e3e1ea",
		"muted": "#8d8a99", "border": "#2e2d39", "accent": "#9d7df8", "accent_text": "#ffffff",
		"accent_soft": "#2c2547", "heading": "#9d7df8", "danger": "#f7768e", "warning": "#e0af68",
		"info": "#7aa2f7", "code_bg": "#262531",
	},
}

// Theme is a resolved theme: every key in Keys has a colour.
type Theme struct {
	Name   string            `json:"name"`
	Dark   bool              `json:"dark"`
	Colors map[string]string `json:"colors"`
	Path   string            `json:"path,omitempty"` // the file, or "" for a built-in
}

// Color returns the colour for key ("" if unknown).
func (t Theme) Color(key string) string { return t.Colors[key] }

// file is a theme file as written on disk.
type file struct {
	Dark   *bool             `toml:"dark"`
	Colors map[string]string `toml:"colors"`
}

var (
	validName = regexp.MustCompile(`^[\pL\pN_.-]+$`)
	hexColor  = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
)

// Builtin returns the built-in light or dark theme.
func Builtin(dark bool) Theme {
	name := Light
	if dark {
		name = Dark
	}
	c := make(map[string]string, len(Keys))
	for k, v := range builtin[name] {
		c[k] = v
	}
	return Theme{Name: name, Dark: dark, Colors: c}
}

// Parse reads a theme file's contents. Missing colours come from the
// built-in light or dark theme; `dark` defaults to whether name is "dark".
func Parse(name string, data []byte) (Theme, error) {
	var f file
	md, err := toml.Decode(string(data), &f)
	if err != nil {
		return Theme{}, fmt.Errorf("theme %s: %w", name, err)
	}
	dark := name == Dark
	if f.Dark != nil {
		dark = *f.Dark
	}
	t := Builtin(dark)
	t.Name = name
	known := map[string]bool{}
	for _, k := range Keys {
		known[k.Name] = true
	}
	var bad []string
	for k, v := range f.Colors {
		k = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(k), "-", "_"))
		v = strings.TrimSpace(v)
		switch {
		case !known[k]:
			bad = append(bad, fmt.Sprintf("unknown colour %q", k))
		case !hexColor.MatchString(v):
			bad = append(bad, fmt.Sprintf("%s = %q isn't a #rrggbb colour", k, v))
		default:
			t.Colors[k] = expand(strings.ToLower(v))
		}
	}
	for _, k := range md.Undecoded() {
		if s := k.String(); s != "colors" && !strings.HasPrefix(s, "colors.") {
			bad = append(bad, fmt.Sprintf("unknown setting %q", s))
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return t, fmt.Errorf("theme %s: %s", name, strings.Join(bad, "; "))
	}
	return t, nil
}

// expand turns #abc into #aabbcc.
func expand(c string) string {
	if len(c) != 4 {
		return c
	}
	return "#" + strings.Repeat(c[1:2], 2) + strings.Repeat(c[2:3], 2) + strings.Repeat(c[3:4], 2)
}

// Load returns the theme called name from dir: <dir>/<name>.toml, or the
// built-in light / dark theme when there is no such file. A file with
// mistakes still returns a usable theme (bad colours keep the built-in
// value) together with the error.
func Load(dir, name string) (Theme, error) {
	name = strings.TrimSpace(name)
	if !validName.MatchString(name) {
		return Builtin(false), fmt.Errorf("invalid theme name %q", name)
	}
	path := filepath.Join(dir, name+".toml")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || dir == "" {
		if _, ok := builtin[name]; ok {
			return Builtin(name == Dark), nil
		}
		return Builtin(false), fmt.Errorf("no theme %q (looked for %s)", name, path)
	}
	if err != nil {
		return Builtin(false), err
	}
	t, err := Parse(name, data)
	t.Path = path
	return t, err
}

// Resolve picks the theme for the `theme` option: a name, or "auto" / ""
// for light or dark depending on systemDark.
func Resolve(dir, option string, systemDark bool) (Theme, error) {
	name := strings.TrimSpace(option)
	if name == "" || strings.EqualFold(name, Auto) {
		name = Light
		if systemDark {
			name = Dark
		}
	}
	return Load(dir, name)
}

// List returns the names of the available themes: light, dark and every
// <name>.toml in dir, sorted.
func List(dir string) []string {
	seen := map[string]bool{Light: true, Dark: true}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			name, ok := strings.CutSuffix(e.Name(), ".toml")
			if ok && !e.IsDir() && validName.MatchString(name) {
				seen[name] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Exists reports whether name can be loaded from dir.
func Exists(dir, name string) bool {
	for _, n := range List(dir) {
		if n == name {
			return true
		}
	}
	return false
}

// Encode writes t as a theme file, with every colour and a comment for each.
func Encode(t Theme) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# KatNotes theme %q. Copy this file to make your own, then set\n", t.Name)
	fmt.Fprintf(&b, "# theme = \"<file name without .toml>\" in config.toml (or run :theme).\n")
	fmt.Fprintf(&b, "# Colours are #rrggbb; any you leave out come from the built-in\n")
	fmt.Fprintf(&b, "# light or dark theme, picked by `dark`.\n\n")
	fmt.Fprintf(&b, "dark = %v\n\n[colors]\n", t.Dark)
	for _, k := range Keys {
		fmt.Fprintf(&b, "%-12s = %q  # %s\n", k.Name, t.Colors[k.Name], k.Desc)
	}
	return b.Bytes()
}

// WriteDefaults creates dir with light.toml and dark.toml if dir doesn't
// exist yet, so deleting a theme file later doesn't bring it back.
func WriteDefaults(dir string) error {
	if dir == "" {
		return errors.New("no themes folder")
	}
	if _, err := os.Stat(dir); err == nil {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, dark := range []bool{false, true} {
		t := Builtin(dark)
		if err := os.WriteFile(filepath.Join(dir, t.Name+".toml"), Encode(t), 0o644); err != nil {
			return err
		}
	}
	return nil
}
