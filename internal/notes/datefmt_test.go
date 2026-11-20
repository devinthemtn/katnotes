package notes

import (
	"testing"
	"time"
)

func TestJournalLayout(t *testing.T) {
	d := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct{ format, want string }{
		{"", "2026-11-20"},
		{"2006-01-02", "2026-11-20"},
		{"Monday, January 2", "Friday, November 20"},
		{"dd/mm/yyyy", "20/11/2026"},
		{"MM/DD/YYYY", "11/20/2026"},
		{"d.m.yy", "20.11.26"},
		{"dddd d mmmm yyyy", "Friday 20 November 2026"},
		{"ddd, mmm d", "Fri, Nov 20"},
	} {
		l, err := JournalLayout(c.format)
		if err != nil {
			t.Errorf("%q: %v", c.format, err)
			continue
		}
		if got := d.Format(l); got != c.want {
			t.Errorf("%q formats as %q, want %q", c.format, got, c.want)
		}
	}
	for _, bad := range []string{"01/01/2020", "20/11/2026", "---"} {
		if _, err := JournalLayout(bad); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
}

func TestDatePattern(t *testing.T) {
	for format, want := range map[string]string{
		"":                  "YYYY-MM-DD",
		"dd/mm/yyyy":        "DD/MM/YYYY",
		"2006-01-02":        "YYYY-MM-DD",
		"Monday, January 2": "DDDD, MMMM D",
		"01/01/2020":        "YYYY-MM-DD",
	} {
		if got := DatePattern(format); got != want {
			t.Errorf("DatePattern(%q) = %q, want %q", format, got, want)
		}
	}
}

func TestParseDate(t *testing.T) {
	want := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct{ format, in string }{
		{"dd/mm/yyyy", "05/11/2026"},
		{"dd/mm/yyyy", "5/11/2026"},
		{"dd/mm/yyyy", "2026-11-05"},
		{"mm/dd/yyyy", "11/05/2026"},
		{"d mmm yyyy", "5 Nov 2026"},
		{"", "2026-11-05"},
	} {
		got, err := ParseDate(c.format, c.in, time.UTC)
		if err != nil || !got.Equal(want) {
			t.Errorf("ParseDate(%q, %q) = %v, %v", c.format, c.in, got, err)
		}
	}
	if _, err := ParseDate("dd/mm/yyyy", "31/02/2026", time.UTC); err == nil {
		t.Error("31/02 should not parse")
	}
}

func TestCreateJournalInDateFormat(t *testing.T) {
	s := New(t.TempDir())
	s.DateFormat = "dd/mm/yyyy"
	n, err := s.Create(KindJournal, "05/11/2026", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if n.ID != "2026-11-05" || n.Title != "05/11/2026" {
		t.Errorf("id %q title %q", n.ID, n.Title)
	}
}
