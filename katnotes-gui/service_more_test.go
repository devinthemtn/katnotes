package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/devinthemtn/katnotes/internal/config"
	"github.com/devinthemtn/katnotes/internal/notes"
)

// must returns v, or panics on err (the test then fails with a stack trace).
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestServiceLinksRenameAndRecent(t *testing.T) {
	s := newService(t)
	must(s.Create("page", "Beta", nil))
	a := must(s.Create("page", "Alpha", nil))
	must(s.Save("page", a.ID, nil, "see "+notes.LinkMarkdown(notes.KindPage, "beta", "Beta")+"\n", a.Stamp, false))

	if bl := must(s.Backlinks("page", "beta")); len(bl) != 1 || bl[0].Note.ID != "alpha" {
		t.Fatalf("backlinks: %+v", bl)
	}
	s.TouchRecent("page", "beta")
	r := must(s.Rename("page", "beta", "Gamma"))
	if r.Note.ID != "gamma" || !reflect.DeepEqual(r.Updated, []string{"page/alpha"}) {
		t.Fatalf("rename: %+v", r)
	}
	if got := s.LastNote(); got != "page/gamma" {
		t.Errorf("LastNote after rename = %q", got)
	}
	if s.FileStamp("page", "gamma") == "" || s.FileStamp("page", "beta") != "" {
		t.Error("FileStamp should follow the file")
	}
	if tags := must(s.Tags()); len(tags) != 0 {
		t.Errorf("tags: %+v", tags)
	}
}

func TestServiceTasks(t *testing.T) {
	s := newService(t)
	n := must(s.Create("page", "Jobs", nil))
	must(s.Save("page", n.ID, nil, "- [ ] mow\n- [ ] dig {pri:high}\n", n.Stamp, false))

	must(s.SetTask("page", "jobs", 0, true))
	must(s.SetTaskMeta("page", "jobs", 1, "due", "2026-12-01"))
	all := must(s.Tasks())
	if len(all) != 1 || all[0].Done != 1 || all[0].Open != 1 || all[0].Tasks[1].Meta["due"] != "2026-12-01" {
		t.Fatalf("tasks: %+v", all)
	}
	stats := must(s.TaskMetaStats())
	keys := []string{}
	for _, st := range stats {
		keys = append(keys, st.Key)
	}
	if !strings.Contains(strings.Join(keys, " "), "pri") {
		t.Errorf("meta stats: %v", keys)
	}
}

func TestServiceCommands(t *testing.T) {
	s := newService(t)
	must(s.Create("page", "Notes", nil))
	if len(s.Commands()) == 0 {
		t.Fatal("empty catalog")
	}
	if c := s.CompleteCommand("tag s", "page", "notes"); len(c.Items) == 0 || c.Items[0].Value != "tag set " {
		t.Errorf("complete: %+v", c)
	}
	r := must(s.RunCommand("tag set a, b", "page", "notes", "", ""))
	if !r.Changed || r.Note == nil || !reflect.DeepEqual(r.Note.Tags, []string{"a", "b"}) {
		t.Fatalf("run: %+v", r)
	}
	if r := must(s.RunCommand("config dir", "", "", "", "")); r.UI != "config dir" {
		t.Errorf("config dir should go to the frontend: %+v", r)
	}
	if h := s.CommandHistory(); len(h) == 0 || h[0] != "config dir" {
		t.Errorf("history: %v", h)
	}
	if _, err := s.RunCommand("tags x", "page", "notes", "", ""); err == nil {
		t.Error("the old :tags should fail")
	}
}

func TestServiceFrontmatterAndEnsure(t *testing.T) {
	s := newService(t)
	must(s.Create("page", "Plain", nil))
	doc := must(s.FrontmatterText("page", "plain"))
	if !strings.Contains(doc.YAML, "title: Plain") || doc.Inferred {
		t.Fatalf("frontmatter: %+v", doc)
	}
	r := must(s.ApplyFrontmatter("page", "plain", "title: Fancy\ntags: [x]\n"))
	if r.Note.ID != "fancy" || !reflect.DeepEqual(r.Note.Tags, []string{"x"}) {
		t.Fatalf("apply: %+v", r.Note)
	}

	n := must(s.EnsureNote("page", "new-one", "New One"))
	if n.ID != "new-one" || n.Title != "New One" {
		t.Fatalf("ensure: %+v", n)
	}
	if again := must(s.EnsureNote("page", "new-one", "Other")); again.Title != "New One" {
		t.Errorf("ensure should not recreate: %+v", again)
	}
}

func TestServiceTemplatesAndViewState(t *testing.T) {
	s := newService(t)
	tpls := s.Templates()
	if len(tpls) == 0 {
		t.Fatal("NewNotesService should write an example template")
	}
	if n := must(s.CreateFromTemplate(tpls[0], "From Template", []string{"t"})); n.ID != "from-template" {
		t.Errorf("template note: %+v", n)
	}
	if err := s.SetSort("page", "title"); err != nil {
		t.Fatal(err)
	}
	if got := s.ViewState().Sort["page"]; got != "title" {
		t.Errorf("sort = %q", got)
	}
}

func TestServiceConfig(t *testing.T) {
	s := newService(t)
	if err := os.WriteFile(must(config.Path()), []byte("editor = \"ed\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if s.GetConfig().Editor == "ed" {
		t.Fatal("config should not change before ReloadConfig")
	}
	if c := must(s.ReloadConfig()); c.Editor != "ed" || s.GetConfig().Editor != "ed" {
		t.Errorf("reload: %+v", c)
	}

	// "true" is not a terminal editor, so it is started detached and exits.
	if err := s.SetOption("editor", "true"); err != nil {
		t.Fatal(err)
	}
	p := must(s.EditConfig(false))
	if p != must(config.Path()) {
		t.Errorf("EditConfig opened %q", p)
	}
	if d := must(s.EditConfig(true)); d != filepath.Dir(p) {
		t.Errorf("EditConfig(dir) opened %q", d)
	}
	if err := s.SetOption("editor", "no-such-editor-katnotes"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EditConfig(false); err == nil {
		t.Error("a missing editor should be an error")
	}
	if err := s.OpenWith("page", "missing", "true"); err == nil {
		t.Error("OpenWith on a missing note should fail")
	}
}

func TestLaunchHelpers(t *testing.T) {
	if got := terminalExecArgs("gnome-terminal", []string{"vim", "a"}); !reflect.DeepEqual(got, []string{"--", "vim", "a"}) {
		t.Errorf("gnome-terminal: %v", got)
	}
	if got := terminalExecArgs("xterm", []string{"vim"}); !reflect.DeepEqual(got, []string{"-e", "vim"}) {
		t.Errorf("xterm: %v", got)
	}
	if terminalExecArgs("kitty", nil) != nil {
		t.Error("no argv: no args")
	}
	if got := shellQuote("it's"); got != `'it'\''s'` {
		t.Errorf("shellQuote: %s", got)
	}
	if got := appleQuote(`a "b" \c`); got != `a \"b\" \\c` {
		t.Errorf("appleQuote: %s", got)
	}
	if got := terminalDirArgs("konsole", "/x"); !reflect.DeepEqual(got, []string{"--workdir", "/x"}) {
		t.Errorf("konsole: %v", got)
	}
	dir := t.TempDir()
	if noteDir("", "/root") != "/root" || noteDir(filepath.Join(dir, "n.md"), "/root") != dir ||
		noteDir("/no/such/dir/n.md", "/root") != "/root" {
		t.Error("noteDir")
	}
}
