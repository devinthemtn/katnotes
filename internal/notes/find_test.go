package notes

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func matchIDs(ms []Match) string {
	var out []string
	for _, m := range ms {
		out = append(out, m.Note.ID)
	}
	return strings.Join(out, ",")
}

func TestCacheReparsesOnlyChangedFiles(t *testing.T) {
	s := newStore(t)
	s.Create(KindPage, "One", nil, "")
	s.Create(KindPage, "Two", nil, "")
	s.List("", ListOpts{})
	before := s.Parses()
	s.List("", ListOpts{})
	if got := s.Parses(); got != before {
		t.Fatalf("unchanged files re-parsed: %d -> %d", before, got)
	}
	// An outside edit (new size and time) is picked up.
	p := s.Path(KindPage, "two")
	os.WriteFile(p, []byte("---\ntitle: Two!\n---\nbody\n"), 0o644)
	os.Chtimes(p, time.Now().Add(time.Minute), time.Now().Add(time.Minute))
	ns, _ := s.List(KindPage, ListOpts{Sort: SortTitle})
	if ns[1].Title != "Two!" || s.Parses() != before+1 {
		t.Fatalf("edit missed: %q parses %d", ns[1].Title, s.Parses()-before)
	}
	// Callers can't change the cached copy.
	ns[1].Tags = append(ns[1].Tags, "x")
	again, _ := s.Get(KindPage, "two")
	if len(again.Tags) != 0 {
		t.Fatal("cache was modified through a returned note")
	}
	s.Delete(KindPage, "one")
	if ns, _ := s.List(KindPage, ListOpts{}); len(ns) != 1 {
		t.Fatalf("deleted note still listed: %d", len(ns))
	}
}

func TestBrokenFrontmatterIsListed(t *testing.T) {
	s := newStore(t)
	raw := "---\ntitle: [oops\n---\n# Imported\n\ntext\n"
	os.WriteFile(s.Path(KindPage, "imported"), []byte(raw), 0o644)
	s.Create(KindPage, "Fine", nil, "")
	ns, err := s.List(KindPage, ListOpts{})
	if err != nil || len(ns) != 2 {
		t.Fatalf("list: %d %v", len(ns), err)
	}
	n, _ := s.Get(KindPage, "imported")
	if n.Problem == "" || n.Body != raw || n.Title != "Imported" {
		t.Fatalf("problem=%q title=%q body=%q", n.Problem, n.Title, n.Body)
	}
	if err := s.Save(n); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(n.Path)
	if !strings.Contains(string(data), "title: [oops") || !strings.Contains(string(data), "text") {
		t.Fatalf("content lost on save:\n%s", data)
	}
	if again, _ := s.Get(KindPage, "imported"); again.Problem != "" {
		t.Errorf("still a problem after save: %s", again.Problem)
	}
}

func TestFindModes(t *testing.T) {
	s := newStore(t)
	s.Create(KindPage, "Weekly Standup", []string{"work", "meeting"}, "# Weekly Standup\n\nagenda: budget review\n")
	s.Create(KindPage, "Standing Desk", []string{"home"}, "# Standing Desk\n\nbuy one\n")
	s.Create(KindPage, "Budget 2026", []string{"work", "money"}, "# Budget 2026\n\nthe budget review is friday\n")
	s.Create(KindPage, "Groceries", []string{"home"}, "milk\n")

	find := func(mode SearchMode, text string) []Match {
		t.Helper()
		ms, err := s.Find(FindQuery{Mode: mode, Text: text})
		if err != nil {
			t.Fatal(err)
		}
		return ms
	}
	if got := matchIDs(find(SearchName, "stdup")); got != "weekly-standup" {
		t.Errorf("fuzzy stdup = %s", got)
	}
	ms := find(SearchName, "stand")
	if len(ms) != 2 {
		t.Fatalf("stand = %s", matchIDs(ms))
	}
	for _, m := range ms {
		if m.Note.ID == "standing-desk" && fmt.Sprint(m.NameHits) != "[0 1 2 3 4]" {
			t.Errorf("hits = %v", m.NameHits)
		}
	}
	if got := matchIDs(find(SearchName, "budget-2026")); got != "budget-2026" {
		t.Errorf("filename match = %s", got)
	}

	if got := matchIDs(find(SearchTags, "work")); got != "budget-2026,weekly-standup" && got != "weekly-standup,budget-2026" {
		t.Errorf("tags work = %s", got)
	}
	if got := matchIDs(find(SearchTags, "#work mee")); got != "weekly-standup" {
		t.Errorf("tags prefix = %s", got)
	}
	if got := matchIDs(find(SearchTags, "mee ")); got != "" {
		t.Errorf("finished word should be exact: %s", got)
	}
	if got := matchIDs(find(SearchTags, "work -money")); got != "weekly-standup" {
		t.Errorf("exclude = %s", got)
	}

	if got := matchIDs(find(SearchText, "budget review")); got != "budget-2026,weekly-standup" {
		t.Errorf("words AND = %s", got)
	}
	if got := matchIDs(find(SearchText, `"review is"`)); got != "budget-2026" {
		t.Errorf("phrase = %s", got)
	}
	if got := matchIDs(find(SearchText, "budget milk")); got != "" {
		t.Errorf("AND across notes = %s", got)
	}
	if m := find(SearchText, "friday"); len(m) != 1 || m[0].Lines[0].Text != "the budget review is friday" {
		t.Errorf("lines = %+v", m)
	}
	if got := find(SearchName, ""); len(got) != 4 {
		t.Errorf("empty query lists everything: %d", len(got))
	}
	if m, _ := ParseSearchMode("TEXT"); m != SearchText || NextSearchMode(SearchText, 1) != SearchName {
		t.Error("mode helpers")
	}
}

func BenchmarkListCached(b *testing.B) {
	s := New(b.TempDir())
	s.Init()
	for i := 0; i < 2000; i++ {
		s.Create(KindPage, fmt.Sprintf("Note number %d", i), []string{"t"}, fmt.Sprintf("# Note %d\n\nsome body text %d\n", i, i))
	}
	s.List("", ListOpts{})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Find(FindQuery{Mode: SearchName, Text: "numb 19"})
	}
}
