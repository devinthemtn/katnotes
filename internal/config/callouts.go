package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Callout styles one "> [!type]" block type ([callouts.<type>] in
// config.toml). Built-in types (note, tip, important, warning, caution) can
// be restyled; any other name adds a new type, which needs a Color.
type Callout struct {
	// Color is a CSS color, e.g. "#e0af68" or "orange". Empty keeps a
	// built-in type's theme colors.
	Color string `toml:"color,omitempty" json:"color"`
	// Icon is shown before the title, e.g. "💡". Empty uses the default.
	Icon string `toml:"icon,omitempty" json:"icon"`
	// Title is the heading shown when the marker line has none. Empty uses
	// the type name, capitalized.
	Title string `toml:"title,omitempty" json:"title"`
}

// BuiltinCallouts are the GitHub alert types, which work without config.
var BuiltinCallouts = []string{"note", "tip", "important", "warning", "caution"}

var (
	calloutName = regexp.MustCompile(`^[a-z0-9-]+$`)
	cssColor    = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|[a-zA-Z]+|(rgb|rgba|hsl|hsla)\([0-9.,%/ ]+\))$`)
)

// normalizeCallouts lowercases names and drops entries with a bad name, a
// bad color, or (for a new type) no color. It returns what it dropped.
func normalizeCallouts(in map[string]Callout) (map[string]Callout, []string) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make(map[string]Callout, len(in))
	var bad []string
	names := make([]string, 0, len(in))
	for name := range in {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, raw := range names {
		c := in[raw]
		name := strings.ToLower(strings.TrimSpace(raw))
		c.Color = strings.TrimSpace(c.Color)
		c.Icon = strings.TrimSpace(c.Icon)
		c.Title = strings.TrimSpace(c.Title)
		switch {
		case !calloutName.MatchString(name):
			bad = append(bad, fmt.Sprintf("callout %q: name may only use a-z, 0-9 and -", raw))
		case c.Color != "" && !cssColor.MatchString(c.Color):
			bad = append(bad, fmt.Sprintf("callout %q: %q is not a CSS color", raw, c.Color))
		case c.Color == "" && !isBuiltinCallout(name):
			bad = append(bad, fmt.Sprintf("callout %q: a new type needs a color", raw))
		default:
			out[name] = c
		}
	}
	if len(out) == 0 {
		out = nil
	}
	return out, bad
}

func isBuiltinCallout(name string) bool {
	for _, b := range BuiltinCallouts {
		if b == name {
			return true
		}
	}
	return false
}
