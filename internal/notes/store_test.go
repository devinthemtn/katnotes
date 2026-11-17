package notes

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var fixed = time.Date(2026, 11, 16, 9, 30, 0, 0, time.Local)

func newStore(t *testing.T) *Store {
	t.Helper()
	s := New(t.TempDir())
	s.Now = func() time.Time { return fixed }
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Hello World":          "hello-world",
		"  Go: TUI & Wails!! ": "go-tui-wails",
		"Café notes":           "café-notes",
		"***":                  "untitled",
		"a/b\\c..d":            "a-b-c-d",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFrontmatterRoundTrip(t *testing.T) {
	src := "---\ntitle: My Note\ntags: [Go, '#ideas', go]\ncreated: 2026-01-02T03:04:05Z\nupdated: 2026-01-03\nmood: happy\nrefs:\n  - a\n  - b\n---\n# My Note\n\nbody text\n"
	var n Note
	if err := parseNote([]byte(src), &n); err != nil {
		t.Fatal(err)
	}
	if n.Title != "My Note" {
		t.Errorf("title %q", n.Title)
	}
	if strings.Join(n.Tags, ",") != "go,ideas" {
		t.Errorf("tags %v", n.Tags)
	}
	if n.Created.IsZero() || n.Updated.IsZero() {
		t.Errorf("times not parsed: %v %v", n.Created, n.Updated)
	}
	if n.Extra["mood"] != "happy" {
		t.Errorf("extra %v", n.Extra)
	}
	if n.Body != "# My Note\n\nbody text\n" {
		t.Errorf("body %q", n.Body)
	}

	out, err := encodeNote(&n)
	if err != nil {
		t.Fatal(err)
	}
	var back Note
	if err := parseNote(out, &back); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if back.Title != n.Title || back.Body != n.Body || back.Extra["mood"] != "happy" || len(back.Extra["refs"].([]any)) != 2 {
		t.Errorf("round trip mismatch:\n%s", out)
	}
	if !strings.HasPrefix(string(out), "---\ntitle: My Note\ntags: [go, ideas]\n") {
		t.Errorf("unexpected header order:\n%s", out)
	}
}

func TestNoFrontmatter(t *testing.T) {
	s := newStore(t)
	p := filepath.Join(s.Root, "pages", "plain.md")
	os.WriteFile(p, []byte("# Plain Title\n\nsome text\n"), 0o644)
	n, err := s.Get(KindPage, "plain")
	if err != nil {
		t.Fatal(err)
	}
	if n.Title != "Plain Title" || n.Snippet != "some text" {
		t.Errorf("got %+v", n)
	}
}

func TestCRUD(t *testing.T) {
	s := newStore(t)
	n, err := s.Create(KindPage, "Hello World", []string{"Idea"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if n.ID != "hello-world" || n.Path != filepath.Join(s.Root, "pages", "hello-world.md") {
		t.Errorf("got %+v", n)
	}
	if _, err := s.Create(KindPage, "hello world", nil, ""); !errors.Is(err, ErrExists) {
		t.Errorf("want ErrExists, got %v", err)
	}

	got, err := s.Get(KindPage, "hello-world")
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasTag("idea") || got.Body != "# Hello World\n\n" {
		t.Errorf("got %+v", got)
	}

	got.Body += "more\n"
	s.Now = func() time.Time { return fixed.Add(time.Hour) }
	if err := s.Save(got); err != nil {
		t.Fatal(err)
	}
	again, _ := s.Get(KindPage, "hello-world")
	if !again.Updated.Equal(fixed.Add(time.Hour)) || !again.Created.Equal(fixed) {
		t.Errorf("times: created %v updated %v", again.Created, again.Updated)
	}

	r, err := s.Rename(KindPage, "hello-world", "Goodbye World")
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "goodbye-world" || !strings.HasPrefix(r.Body, "# Goodbye World\n") {
		t.Errorf("rename: %q %q", r.ID, r.Body)
	}
	if _, err := s.Get(KindPage, "hello-world"); !errors.Is(err, ErrNotFound) {
		t.Errorf("old file still there: %v", err)
	}

	if err := s.Delete(KindPage, "goodbye-world"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(KindPage, "goodbye-world"); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestInvalidIDs(t *testing.T) {
	s := newStore(t)
	for _, id := range []string{"", "..", "../x", "a/b", ".hidden"} {
		if _, err := s.Get(KindPage, id); !errors.Is(err, ErrInvalidID) {
			t.Errorf("Get(%q): want ErrInvalidID, got %v", id, err)
		}
	}
	if _, err := s.Get(KindJournal, "not-a-date"); !errors.Is(err, ErrInvalidID) {
		t.Errorf("journal id: %v", err)
	}
}

func TestToday(t *testing.T) {
	s := newStore(t)
	s.JournalTemplate = "# {{.Weekday}} {{.Date}}\n\n"
	n, created, err := s.Today()
	if err != nil || !created {
		t.Fatal(err, created)
	}
	if n.ID != "2026-11-16" || n.Body != "# Monday 2026-11-16\n\n" {
		t.Errorf("got %+v", n)
	}
	n2, created, err := s.Today()
	if err != nil || created || n2.ID != n.ID {
		t.Errorf("second Today: %v %v %+v", err, created, n2)
	}
}

func TestListSearchTags(t *testing.T) {
	s := newStore(t)
	s.Create(KindPage, "Alpha", []string{"go", "tui"}, "# Alpha\n\nBubbletea is great\n")
	s.Now = func() time.Time { return fixed.Add(time.Minute) }
	s.Create(KindPage, "Beta", []string{"go"}, "# Beta\n\nwails stuff\n")
	s.Create(KindJournal, "2026-11-01", []string{"daily"}, "met about bubbletea\n")
	s.Create(KindJournal, "2026-11-10", nil, "")

	pages, _ := s.List(KindPage, ListOpts{})
	if len(pages) != 2 || pages[0].ID != "beta" {
		t.Errorf("pages order: %v", ids(pages))
	}
	j, _ := s.List(KindJournal, ListOpts{})
	if len(j) != 2 || j[0].ID != "2026-11-10" {
		t.Errorf("journal order: %v", ids(j))
	}
	tui, _ := s.List("", ListOpts{Tag: "TUI"})
	if len(tui) != 1 || tui[0].ID != "alpha" {
		t.Errorf("tag filter: %v", ids(tui))
	}

	res, err := s.Search("BUBBLETEA", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || len(res[0].Lines) == 0 {
		t.Errorf("search: %+v", res)
	}
	res, _ = s.Search("tui", KindPage)
	if len(res) != 1 || res[0].Note.ID != "alpha" {
		t.Errorf("tag search: %+v", res)
	}

	tags, _ := s.Tags()
	if tags["go"] != 2 || tags["daily"] != 1 || tags["tui"] != 1 {
		t.Errorf("tags %v", tags)
	}
}

func ids(ns []Note) []string {
	var out []string
	for _, n := range ns {
		out = append(out, n.ID)
	}
	return out
}

func TestHasTask(t *testing.T) {
	for body, want := range map[string]bool{
		"- [ ] a\n":                  true,
		"* [x] done\n":               true,
		"1. [X] n\n":                 true,
		"[ ] bare\n":                 true,
		"> - [ ] quoted\n":           true,
		"plain [ ] text\n":           false,
		"- [link](x)\n":              false,
		"```\n- [ ] in code\n```\n":  false,
		"~~~\n[ ] x\n~~~\n- [ ] y\n": true,
		"":                           false,
	} {
		if got := HasTask(body); got != want {
			t.Errorf("HasTask(%q) = %v, want %v", body, got, want)
		}
	}
}

func TestContainsTaskFrontmatter(t *testing.T) {
	s := newStore(t)
	n, err := s.Create(KindPage, "Shopping", nil, "- [ ] milk\n")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(n.Path)
	if !strings.Contains(string(raw), "contains_task: true\n") {
		t.Fatalf("created file missing flag:\n%s", raw)
	}

	n.Extra["mood"] = "ok" // unrelated keys must survive
	n.Body = "no tasks now\n"
	if err := s.Save(n); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(n.Path)
	if strings.Contains(string(raw), TaskKey) || !strings.Contains(string(raw), "mood: ok") {
		t.Fatalf("after removing tasks:\n%s", raw)
	}

	got, _ := s.Get(KindPage, n.ID)
	got.Body = "- [x] milk\n"
	if err := s.Save(got); err != nil {
		t.Fatal(err)
	}
	if again, _ := s.Get(KindPage, n.ID); again.Extra[TaskKey] != true {
		t.Errorf("Extra = %v", again.Extra)
	}
}
