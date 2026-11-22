package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestMarkHighlights(t *testing.T) {
	o, c := hlOpen, hlClose
	cases := []struct{ in, want string }{
		{"a {{b}} c {{ d e }}", "a " + o + "{{b}}" + c + " c " + o + "{{ d e }}" + c},
		{"`{{x}}` {{y}}", "`{{x}}` " + o + "{{y}}" + c},
		{"``a ` {{x}}`` {{y}}", "``a ` {{x}}`` " + o + "{{y}}" + c},
		{"{{}} {{ }} {{open", "{{}} {{ }} {{open"},
		{"```\n{{x}}\n```\n{{y}}", "```\n{{x}}\n```\n" + o + "{{y}}" + c},
		{"~~~md\n```\n{{x}}\n~~~\n{{y}}", "~~~md\n```\n{{x}}\n~~~\n" + o + "{{y}}" + c},
	}
	for _, tc := range cases {
		if got := markHighlights(tc.in); got != tc.want {
			t.Errorf("markHighlights(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestApplyHighlights(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)
	on, _, _ := strings.Cut(lipgloss.NewStyle().Foreground(highlightColor).Background(highlightBg).Render("x"), "x")
	const fg, reset = "\x1b[38;5;252m", "\x1b[0m"

	// glamour's own codes inside the span are followed by the highlight again,
	// and its style comes back after the span.
	got := applyHighlights(fg + "a " + hlOpen + "{{x" + reset + fg + "y}}" + hlClose + " b")
	want := fg + "a " + on + "{{x" + reset + on + fg + on + "y}}" + reset + fg + " b"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}

	// Wrapped onto two lines: padding and indent stay unhighlighted.
	got = applyHighlights(hlOpen + "{{a b" + fg + "  \n  c}}" + hlClose)
	want = on + "{{a b" + fg + on + reset + fg + "  \n  " + on + "c}}" + reset + fg
	if got != want {
		t.Errorf("wrapped: got  %q\nwant %q", got, want)
	}

	// No colour profile: the runes just go.
	lipgloss.SetColorProfile(termenv.Ascii)
	if got := applyHighlights("a " + hlOpen + "{{x}}" + hlClose); got != "a {{x}}" {
		t.Errorf("ascii: %q", got)
	}
}
