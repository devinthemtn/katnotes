package notes

import (
	"fmt"
	"strings"
	"time"
)

// dateTokens maps the human date pattern tokens accepted in date_format to
// Go time layout pieces. Longest tokens come first so "yyyy" wins over "yy".
var dateTokens = []struct{ tok, layout string }{
	{"yyyy", "2006"},
	{"yy", "06"},
	{"mmmm", "January"},
	{"mmm", "Jan"},
	{"mm", "01"},
	{"m", "1"},
	{"dddd", "Monday"},
	{"ddd", "Mon"},
	{"dd", "02"},
	{"d", "2"},
}

// JournalLayout turns a date_format setting into a Go time layout. It accepts
// either a Go layout (anything mentioning 2006, Jan or Mon, e.g. "2006-01-02"
// or "Monday, January 2") or a human pattern such as "dd/mm/yyyy",
// "mm-dd-yy" or "dddd d mmmm yyyy" (tokens are case-insensitive). An empty
// format gives the default YYYY-MM-DD.
//
// An example date such as "01/01/2020" is rejected: it can't say which part
// is the day and which the month, and as a Go layout it formats as garbage.
func JournalLayout(format string) (string, error) {
	f := strings.TrimSpace(format)
	if f == "" {
		return JournalIDLayout, nil
	}
	if strings.Contains(f, "2006") || strings.Contains(f, "Jan") || strings.Contains(f, "Mon") {
		return f, nil
	}
	if strings.ContainsAny(f, "0123456789") {
		return "", fmt.Errorf("date_format %q looks like a date, not a pattern: use something like dd/mm/yyyy, mm/dd/yyyy or yyyy-mm-dd", format)
	}
	var b strings.Builder
	found := false
	lower := strings.ToLower(f)
	for i := 0; i < len(f); {
		matched := false
		for _, t := range dateTokens {
			if strings.HasPrefix(lower[i:], t.tok) {
				b.WriteString(t.layout)
				i += len(t.tok)
				matched, found = true, true
				break
			}
		}
		if !matched {
			b.WriteByte(f[i])
			i++
		}
	}
	if !found {
		return "", fmt.Errorf("date_format %q has no date in it: use something like dd/mm/yyyy, mm/dd/yyyy or yyyy-mm-dd", format)
	}
	return b.String(), nil
}

// dateLayout is the Go layout for the store's DateFormat, falling back to
// YYYY-MM-DD when the setting can't be understood.
func (s *Store) dateLayout() string {
	l, err := JournalLayout(s.DateFormat)
	if err != nil {
		return JournalIDLayout
	}
	return l
}

// datePatternNames maps Go layout pieces back to the human tokens shown in
// prompts. Longest pieces come first.
var datePatternNames = []struct{ layout, name string }{
	{"January", "MMMM"}, {"Monday", "DDDD"}, {"2006", "YYYY"},
	{"Jan", "MMM"}, {"Mon", "DDD"}, {"01", "MM"}, {"02", "DD"},
	{"06", "YY"}, {"_2", "D"}, {"1", "M"}, {"2", "D"},
}

// DatePattern is the date_format setting written for people, e.g.
// "DD/MM/YYYY", for prompts that ask for a date. An unusable setting
// gives "YYYY-MM-DD".
func DatePattern(format string) string {
	l, err := JournalLayout(format)
	if err != nil {
		l = JournalIDLayout
	}
	var b strings.Builder
	for i := 0; i < len(l); {
		matched := false
		for _, p := range datePatternNames {
			if strings.HasPrefix(l[i:], p.layout) {
				b.WriteString(p.name)
				i += len(p.layout)
				matched = true
				break
			}
		}
		if !matched {
			b.WriteByte(l[i])
			i++
		}
	}
	return b.String()
}

// ParseDate reads a date typed by a person: YYYY-MM-DD always works, and so
// does the date_format setting (e.g. 20/11/2026 with dd/mm/yyyy).
func ParseDate(format, s string, loc *time.Location) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.ParseInLocation(JournalIDLayout, s, loc); err == nil {
		return t, nil
	}
	if l, err := JournalLayout(format); err == nil {
		if t, err := time.ParseInLocation(l, s, loc); err == nil {
			return t, nil
		}
		// Let 1/2/2026 match dd/mm/yyyy and 01/02/2026 match d/m/yyyy.
		loose := strings.NewReplacer("01", "1", "02", "2").Replace(l)
		if t, err := time.ParseInLocation(loose, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("can't read %q as a date: use %s or YYYY-MM-DD", s, DatePattern(format))
}
