package notes

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseMeta(t *testing.T) {
	cases := []struct {
		in, text string
		meta     map[string]string
	}{
		{"plain task", "plain task", nil},
		{"call {due:2026-11-20 pri:high}", "call", map[string]string{"due": "2026-11-20", "pri": "high"}},
		{"call {due:2026-11-20}  ", "call", map[string]string{"due": "2026-11-20"}},
		{`venue {note:"ask about parking" due:2026-12-01}`, "venue", map[string]string{"note": "ask about parking", "due": "2026-12-01"}},
		{"{due:2026-01-01}", "", map[string]string{"due": "2026-01-01"}},
		{"prose {x} here", "prose {x} here", nil},
		{"ends with {x}", "ends with {x}", nil},
		{"mixed {due:2026-01-01 nope}", "mixed {due:2026-01-01 nope}", nil},
		{"empty {}", "empty {}", nil},
		{"mid {due:2026-01-01} line", "mid {due:2026-01-01} line", nil},
		{"code `x {due:2026-01-01}`", "code `x {due:2026-01-01}`", nil},
		{"code `x` {due:2026-01-01}", "code `x`", map[string]string{"due": "2026-01-01"}},
		{"upper {Due:x}", "upper {Due:x}", nil},
		{"glued {a:b:c}", "glued", map[string]string{"a": "b:c"}},
		{`bad quote {a:"x"y}`, `bad quote {a:"x"y}`, nil},
		{"text {x} {due:2026-01-01}", "text {x}", map[string]string{"due": "2026-01-01"}},
		{"dup {a:1 a:2}", "dup", map[string]string{"a": "2"}},
	}
	for _, c := range cases {
		text, meta := ParseMeta(c.in)
		if text != c.text || !reflect.DeepEqual(meta, c.meta) {
			t.Errorf("ParseMeta(%q) = %q %v, want %q %v", c.in, text, meta, c.text, c.meta)
		}
	}
}

func TestSetLineMeta(t *testing.T) {
	cases := []struct{ line, key, val, want string }{
		{"- [ ] call", "due", "2026-11-20", "- [ ] call {due:2026-11-20}"},
		{"- [ ] call   ", "due", "2026-11-20", "- [ ] call {due:2026-11-20}"},
		{"- [ ] call {pri:high}", "due", "2026-11-20", "- [ ] call {pri:high due:2026-11-20}"},
		{"- [ ] call {due:2026-01-01 pri:high}", "due", "2026-11-20", "- [ ] call {due:2026-11-20 pri:high}"},
		{"- [ ] call {due:2026-01-01 pri:high}", "due", "", "- [ ] call {pri:high}"},
		{"- [ ] call {due:2026-01-01}", "due", "", "- [ ] call"},
		{"- [ ] call", "due", "", "- [ ] call"},
		{"  > 1. [x] q {x}", "note", "two words", `  > 1. [x] q {x} {note:"two words"}`},
		{"- [ ] ", "pri", "low", "- [ ] {pri:low}"},
	}
	for _, c := range cases {
		got, ok := SetLineMeta(c.line, c.key, c.val)
		if !ok || got != c.want {
			t.Errorf("SetLineMeta(%q, %s, %q) = %q, want %q", c.line, c.key, c.val, got, c.want)
		}
		// Whatever was written must parse back to the same value.
		if c.val != "" {
			if _, m := ParseMeta(got[strings.Index(got, "]")+2:]); m[c.key] != c.val {
				t.Errorf("round trip %q: %v", got, m)
			}
		}
	}
	if _, ok := SetLineMeta("not a task", "due", "x"); ok {
		t.Error("non-task line accepted")
	}
}

func TestNormalizeDate(t *testing.T) {
	now := time.Date(2026, 11, 17, 15, 0, 0, 0, time.Local) // a Tuesday
	cases := map[string]string{
		"2026-12-01": "2026-12-01", "today": "2026-11-17", "tomorrow": "2026-11-18",
		"tom": "2026-11-18", "+3d": "2026-11-20", "+2w": "2026-12-01", "fri": "2026-11-20",
		"Friday": "2026-11-20", "tue": "2026-11-24", "": "", "none": "",
	}
	for in, want := range cases {
		if got, err := NormalizeDate(in, now); err != nil || got != want {
			t.Errorf("NormalizeDate(%q) = %q %v, want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"2026-13-01", "soon", "11/20"} {
		if _, err := NormalizeDate(bad, now); err == nil {
			t.Errorf("NormalizeDate(%q) accepted", bad)
		}
	}
	if p, err := NormalizePriority("H"); err != nil || p != "high" {
		t.Errorf("NormalizePriority(H) = %q %v", p, err)
	}
	if _, err := NormalizeMeta("Bad Key", "x", now); err == nil {
		t.Error("bad key accepted")
	}
}

func TestSetTaskDoneDateAndMeta(t *testing.T) {
	s := newStore(t)
	n, _ := s.Create(KindPage, "M", nil, "- [ ] call {pri:high}\n- [ ] other\n")
	today := fixed.Format(DateLayout)

	n, err := s.SetTask(KindPage, n.ID, 0, true)
	if err != nil || n.Body != "- [x] call {pri:high done:"+today+"}\n- [ ] other\n" {
		t.Fatalf("check: %q %v", n.Body, err)
	}
	n, _ = s.SetTask(KindPage, n.ID, 0, false)
	if n.Body != "- [ ] call {pri:high}\n- [ ] other\n" {
		t.Errorf("uncheck: %q", n.Body)
	}

	n, err = s.SetTaskMeta(KindPage, n.ID, 1, "due", "tomorrow")
	want := "- [ ] other {due:" + fixed.AddDate(0, 0, 1).Format(DateLayout) + "}"
	if err != nil || strings.Split(n.Body, "\n")[1] != want {
		t.Errorf("SetTaskMeta: %q %v", n.Body, err)
	}
	if _, err := s.SetTaskMeta(KindPage, n.ID, 1, "pri", "urgent"); err == nil {
		t.Error("bad pri accepted")
	}
	if _, err := s.SetTaskMeta(KindPage, n.ID, 5, "pri", "low"); !errors.Is(err, ErrTaskMoved) {
		t.Errorf("missing line err = %v", err)
	}
	ts := ParseTasks(n.Body)
	if ts[1].Text != "other" || ts[1].Meta["due"] == "" {
		t.Errorf("parsed = %+v", ts[1])
	}
}

func TestAllTasks(t *testing.T) {
	s := newStore(t)
	s.Create(KindPage, "A", nil, "- [ ] late {due:2026-01-05}\n- [ ] nodue {pri:high}\n- [x] gone {due:2026-01-01}\n")
	s.Create(KindPage, "B", nil, "- [ ] early {due:2026-01-02 pri:low}\n- [ ] same {due:2026-01-02 pri:high}\n- [ ] plain\n")
	texts := func(f TaskFilter) string {
		ts, err := s.AllTasks(f)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range ts {
			out = append(out, r.Task.Text)
		}
		return strings.Join(out, ",")
	}
	if got := texts(TaskFilter{}); got != "same,early,late,nodue,plain" {
		t.Errorf("all = %s", got)
	}
	if got := texts(TaskFilter{DueBefore: "2026-01-02", IncludeDone: true}); got != "gone,same,early" {
		t.Errorf("due before = %s", got)
	}
	if got := texts(TaskFilter{Pri: "high"}); got != "same,nodue" {
		t.Errorf("pri = %s", got)
	}
	if got := texts(TaskFilter{Has: map[string]string{"pri": ""}}); got != "same,early,nodue" {
		t.Errorf("has = %s", got)
	}
}

func TestMetaStats(t *testing.T) {
	s := newStore(t)
	s.Create(KindPage, "A", nil, "- [ ] a {project:house due:2026-01-01}\n- [ ] b {project:house}\n- [ ] c {project:garden pri:high}\n")
	s.Create(KindPage, "B", nil, "- [ ] d {due:2026-02-02}\n- [ ] plain\n")
	st, err := s.MetaStats()
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, k := range st {
		keys = append(keys, fmt.Sprintf("%s:%d", k.Key, k.Count))
	}
	if strings.Join(keys, ",") != "project:3,due:2,pri:1" {
		t.Errorf("keys = %v", keys)
	}
	if v := st[0].Values; len(v) != 2 || v[0] != (MetaValueStat{"house", 2}) || v[1] != (MetaValueStat{"garden", 1}) {
		t.Errorf("project values = %v", v)
	}
	if len(st[1].Values) != 0 {
		t.Errorf("date values kept: %v", st[1].Values)
	}
}
