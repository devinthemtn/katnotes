package notes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddTask(t *testing.T) {
	s := newStore(t) // Now = 2026-11-16 (a Monday)
	n, _ := s.Create(KindPage, "List", nil, "# List\n\n- [ ] one\n  - [ ] child\n\nafter\n")
	got, line, err := s.AddTask(n.Kind, n.ID, "two {due:fri pri:h}")
	if err != nil {
		t.Fatal(err)
	}
	want := "# List\n\n- [ ] one\n  - [ ] child\n- [ ] two {due:2026-11-20 pri:high}\n\nafter\n"
	if got.Body != want || line != 4 {
		t.Fatalf("line %d body:\n%s", line, got.Body)
	}
	if got.Extra[TaskKey] != true {
		t.Error("contains_task not set")
	}

	plain, _ := s.Create(KindPage, "Plain", nil, "# Plain\n\ntext")
	got, _, _ = s.AddTask(plain.Kind, plain.ID, "first")
	if got.Body != "# Plain\n\ntext\n\n- [ ] first\n" {
		t.Fatalf("no tasks yet:\n%q", got.Body)
	}
	if _, _, err := s.AddTask(plain.Kind, plain.ID, "  "); err == nil {
		t.Error("empty task accepted")
	}
	if _, _, err := s.AddTask(plain.Kind, plain.ID, "x {due:someday}"); err == nil {
		t.Error("bad due accepted")
	}
}

func TestPinnedSortsFirst(t *testing.T) {
	s := newStore(t)
	s.Create(KindPage, "Bravo", nil, "")
	a, _ := s.Create(KindPage, "Alpha", nil, "")
	s.Create(KindPage, "Charlie", nil, "")
	s.SetField(a, "pinned", "true")
	ns, _ := s.List(KindPage, ListOpts{Sort: SortTitle})
	if ns[0].ID != "alpha" || !ns[0].Pinned() || ns[1].ID != "bravo" {
		t.Fatalf("order: %s %s %s", ns[0].ID, ns[1].ID, ns[2].ID)
	}
	c, _ := s.Get(KindPage, "charlie")
	s.SetField(c, "pinned", "true")
	ns, _ = s.List(KindPage, ListOpts{Sort: SortTitle})
	if ns[0].ID != "alpha" || ns[1].ID != "charlie" || ns[2].ID != "bravo" {
		t.Fatalf("two pinned: %s %s %s", ns[0].ID, ns[1].ID, ns[2].ID)
	}
}

func TestTaskFilterWhen(t *testing.T) {
	task := func(due string, done bool) Task {
		t := Task{Done: done}
		if due != "" {
			t.Meta = map[string]string{MetaDue: due}
		}
		return t
	}
	today := "2026-11-16"
	cases := []struct {
		f    TaskFilter
		t    Task
		want bool
	}{
		{TaskFilter{When: WhenOverdue, Today: today}, task("2026-11-15", false), true},
		{TaskFilter{When: WhenOverdue, Today: today}, task("2026-11-16", false), false},
		{TaskFilter{When: WhenOverdue, Today: today}, task("", false), false},
		{TaskFilter{When: WhenWeek, Today: today}, task("2026-11-23", false), true},
		{TaskFilter{When: WhenWeek, Today: today}, task("2026-11-24", false), false},
		{TaskFilter{When: WhenWeek, Today: today}, task("2026-01-01", false), true},
		{TaskFilter{OnlyDone: true}, task("", true), true},
		{TaskFilter{OnlyDone: true}, task("", false), false},
		{TaskFilter{}, task("", true), false},
	}
	for i, c := range cases {
		if got := c.f.Match(c.t); got != c.want {
			t.Errorf("case %d: %+v on %+v = %v", i, c.f, c.t, got)
		}
	}
}

func TestTemplates(t *testing.T) {
	s := newStore(t)
	dir := filepath.Join(t.TempDir(), "templates")
	if err := WriteExampleTemplate(dir); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "bad name.md"), []byte("x"), 0o644)
	if got := ListTemplates(dir); strings.Join(got, ",") != "meeting" {
		t.Fatalf("ListTemplates = %v", got)
	}
	src, err := ReadTemplate(dir, "meeting")
	if err != nil {
		t.Fatal(err)
	}
	n, err := s.CreateFromTemplate("Standup", src, []string{"team"})
	if err != nil {
		t.Fatal(err)
	}
	if n.ID != "standup" || !strings.HasPrefix(n.Body, "# Standup\n\nMonday 2026-11-16 · 09:30") {
		t.Fatalf("body:\n%s", n.Body)
	}
	if strings.Join(n.Tags, ",") != "meeting,team" {
		t.Fatalf("tags = %v", n.Tags)
	}

	n, err = s.CreateFromTemplate("Review", "---\nstatus: draft\ntitle: ignored\n---\n{{.Title}} body\n", nil)
	if err != nil || n.Extra["status"] != "draft" || n.Title != "Review" || n.Body != "Review body\n" {
		t.Fatalf("extra/title: %+v %v", n, err)
	}
	if _, err := ReadTemplate(dir, "nope"); err == nil || !strings.Contains(err.Error(), "have: meeting") {
		t.Errorf("missing template: %v", err)
	}
	if _, err := s.CreateFromTemplate("Broken", "{{.Title", nil); err == nil {
		t.Error("bad template accepted")
	}
}
