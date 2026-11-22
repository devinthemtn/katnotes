package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// {{ … }} in a note is highlighted in the preview, in the theme's highlight
// / highlight_bg colours (the GUI does the same). Like the selected link,
// each span is wrapped in private-use runes before glamour renders the
// Markdown, and the runes are swapped for colour codes afterwards. Code
// spans and fenced code blocks are left alone.

const (
	hlOpen  = ""
	hlClose = ""
)

var hlSpan = regexp.MustCompile(`\{\{[^\n{}]*[^\s{}][^\n{}]*\}\}`)

// markHighlights wraps each {{ … }} outside code in the sentinel runes.
func markHighlights(body string) string {
	if !strings.Contains(body, "{{") {
		return body
	}
	lines := strings.Split(body, "\n")
	fence := ""
	for i, l := range lines {
		t := strings.TrimLeft(l, " ")
		if len(l)-len(t) <= 3 && (strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")) {
			switch {
			case fence == "":
				fence = t[:3]
			case strings.HasPrefix(t, fence):
				fence = ""
			}
			continue
		}
		if fence != "" || !strings.Contains(l, "{{") {
			continue
		}
		lines[i] = markOutsideCode(l)
	}
	return strings.Join(lines, "\n")
}

// markOutsideCode marks the spans in one line, skipping `code` spans.
func markOutsideCode(l string) string {
	var b strings.Builder
	for l != "" {
		i := strings.IndexByte(l, '`')
		if i < 0 {
			b.WriteString(hlSpan.ReplaceAllString(l, hlOpen+"$0"+hlClose))
			break
		}
		b.WriteString(hlSpan.ReplaceAllString(l[:i], hlOpen+"$0"+hlClose))
		l = l[i:]
		n := len(l) - len(strings.TrimLeft(l, "`")) // the run of backticks
		end := strings.Index(l[n:], l[:n])
		if end < 0 { // unclosed: the backticks are plain text
			b.WriteString(l[:n])
			l = l[n:]
			continue
		}
		b.WriteString(l[:n+end+n])
		l = l[n+end+n:]
	}
	return b.String()
}

// applyHighlights swaps the sentinel runes in rendered text for the
// highlight colours. glamour styles every piece of text with its own codes
// (and resets after each), so the highlight is switched back on after each
// of them inside a span. A span glamour wrapped onto several lines is
// coloured line by line, leaving out the indent and the line's padding.
func applyHighlights(out string) string {
	if !strings.Contains(out, hlOpen) {
		return out
	}
	on := ""
	st := lipgloss.NewStyle().Foreground(highlightColor).Background(highlightBg)
	if s := st.Render("x"); strings.Count(s, "x") == 1 {
		on, _, _ = strings.Cut(s, "x")
	}
	if on == "" { // no colours (e.g. not a terminal): just drop the runes
		return strings.NewReplacer(hlOpen, "", hlClose, "").Replace(out)
	}
	const reset = "\x1b[0m"
	var b strings.Builder
	active := ""            // glamour's codes since its last reset
	in, lit := false, false // inside a span; highlight colours on
	off := func() {
		if lit {
			b.WriteString(reset + active)
			lit = false
		}
	}
	for i := 0; i < len(out); {
		switch {
		case strings.HasPrefix(out[i:], hlOpen):
			in = true
			i += len(hlOpen)
		case strings.HasPrefix(out[i:], hlClose):
			off()
			in = false
			i += len(hlClose)
		case out[i] == 0x1b:
			j := escEnd(out, i)
			seq := out[i:j]
			b.WriteString(seq)
			if strings.HasPrefix(seq, "\x1b[") && strings.HasSuffix(seq, "m") {
				if seq == reset || seq == "\x1b[m" {
					active = ""
				} else {
					active += seq
				}
				if lit {
					b.WriteString(on)
				}
			}
			i = j
		case out[i] == '\n':
			off()
			b.WriteByte('\n')
			i++
		case out[i] == ' ':
			if lit && blankToEOL(out[i:]) {
				off()
			}
			b.WriteByte(' ')
			i++
		default:
			if in && !lit {
				b.WriteString(on)
				lit = true
			}
			b.WriteByte(out[i])
			i++
		}
	}
	off()
	return b.String()
}

// escEnd returns the index just past the escape sequence starting at s[i].
func escEnd(s string, i int) int {
	if i+1 >= len(s) {
		return len(s)
	}
	switch s[i+1] {
	case '[': // CSI: parameters, then a final byte @ to ~
		for j := i + 2; j < len(s); j++ {
			if s[j] >= 0x40 && s[j] <= 0x7e {
				return j + 1
			}
		}
	case ']': // OSC (e.g. hyperlinks): up to BEL or ESC \
		for j := i + 2; j < len(s); j++ {
			if s[j] == 0x07 {
				return j + 1
			}
			if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
				return j + 2
			}
		}
	default:
		return i + 2
	}
	return len(s)
}

// blankToEOL reports whether s holds only spaces and escape codes up to the
// end of its line.
func blankToEOL(s string) bool {
	for i := 0; i < len(s); {
		switch {
		case s[i] == '\n':
			return true
		case s[i] == ' ':
			i++
		case s[i] == 0x1b:
			i = escEnd(s, i)
		default:
			return false
		}
	}
	return true
}
