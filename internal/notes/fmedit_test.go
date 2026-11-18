package notes

import (
	"os"
	"strings"
	"testing"
)

func writeRaw(t *testing.T, s *Store, kind Kind, id, data string) {
	t.Helper()
	if err := os.WriteFile(s.Path(kind, id), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestApplyFrontmatterToNoteWithoutOne(t *testing.T) {
	s := newStore(t)
	body := "# Imported Thing\n\nSome text.\n"
	writeRaw(t, s, KindPage, "imported-thing", body)

	if has, err := s.HasFrontmatter(KindPage, "imported-thing"); err != nil || has {
		t.Fatalf("HasFrontmatter = %v, %v; want false", has, err)
	}
	n, err := s.Get(KindPage, "imported-thing")
	if err != nil {
		t.Fatal(err)
	}
	y, err := FrontmatterYAML(n)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(y), "title: Imported Thing") {
		t.Fatalf("inferred YAML missing title:\n%s", y)
	}

	edited := strings.Replace(string(y), "tags: []", "tags: [Work, \"#ideas\"]", 1) + "source: evernote\n"
	got, changed, err := s.ApplyFrontmatter(n, []byte(edited))
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 || got.ID != "imported-thing" {
		t.Fatalf("unexpected rename: id=%s changed=%v", got.ID, changed)
	}
	if has, _ := s.HasFrontmatter(KindPage, "imported-thing"); !has {
		t.Fatal("file still has no frontmatter")
	}
	back, err := s.Get(KindPage, "imported-thing")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(back.Tags, ",") != "work,ideas" || back.Extra["source"] != "evernote" || back.Body != body {
		t.Fatalf("got tags=%v extra=%v body=%q", back.Tags, back.Extra, back.Body)
	}
	if back.Created.Unix() != n.Created.Unix() { // inferred from the file; RFC 3339 drops sub-seconds
		t.Fatalf("created changed: %v -> %v", n.Created, back.Created)
	}
}

func TestApplyFrontmatterTitleRenames(t *testing.T) {
	s := newStore(t)
	n, err := s.Create(KindPage, "Old Name", nil, "# Old Name\n\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := s.ApplyFrontmatter(n, []byte("---\ntitle: New Name\ntags: []\n---\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "new-name" || !strings.HasPrefix(got.Body, "# New Name\n") {
		t.Fatalf("id=%s body=%q", got.ID, got.Body)
	}
	if _, err := os.Stat(s.Path(KindPage, "old-name")); !os.IsNotExist(err) {
		t.Fatal("old file still exists")
	}
}

func TestApplyFrontmatterRejectsBadInput(t *testing.T) {
	s := newStore(t)
	n, err := s.Create(KindPage, "Keep", []string{"a"}, "body\n")
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(n.Path)
	for _, src := range []string{"title: [unclosed\n", "title: Keep\ncreated: someday\n"} {
		if _, _, err := s.ApplyFrontmatter(n, []byte(src)); err == nil {
			t.Errorf("ApplyFrontmatter(%q) succeeded", src)
		}
	}
	after, _ := os.ReadFile(n.Path)
	if string(before) != string(after) {
		t.Fatal("file changed after rejected input")
	}
}

func TestSetFieldOnNoteWithoutFrontmatter(t *testing.T) {
	s := newStore(t)
	body := "# Loose\n\nimported text\n"
	writeRaw(t, s, KindPage, "loose", body)
	n, _ := s.Get(KindPage, "loose")

	got, _, err := s.SetField(n, "status", "draft")
	if err != nil {
		t.Fatal(err)
	}
	if has, _ := s.HasFrontmatter(KindPage, "loose"); !has || got.Extra["status"] != "draft" || got.Body != body {
		t.Fatalf("has=%v extra=%v body=%q", has, got.Extra, got.Body)
	}
	if got, _, _ = s.SetField(got, "tags", "[a, B]"); strings.Join(got.Tags, ",") != "a,b" {
		t.Fatalf("tags = %v", got.Tags)
	}
	if got, _, _ = s.SetField(got, "done", "true"); got.Extra["done"] != true {
		t.Fatalf("done = %#v", got.Extra["done"])
	}
	if _, _, err := s.SetField(got, "created", "nope"); err == nil {
		t.Error("bad created accepted")
	}
	if _, err := s.UnsetField(got, "title"); err == nil {
		t.Error("unset title accepted")
	}
	if got, err = s.UnsetField(got, "status"); err != nil || got.Extra["status"] != nil {
		t.Fatalf("unset status: %v %v", got.Extra, err)
	}
	if _, err := s.UnsetField(got, "status"); err == nil {
		t.Error("unsetting a missing field succeeded")
	}
	keys, _ := s.FieldKeys()
	if strings.Join(keys[:3], ",") != "title,tags,created" || !strings.Contains(strings.Join(keys, ","), "done") {
		t.Fatalf("FieldKeys = %v", keys)
	}
}
