package commands

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/devinthemtn/katnotes/internal/config"
	"github.com/devinthemtn/katnotes/internal/notes"
	"github.com/devinthemtn/katnotes/internal/recent"
)

var fixed = time.Date(2026, 11, 17, 9, 0, 0, 0, time.Local)

func newEnv(t *testing.T) *Env {
	t.Helper()
	t.Setenv(config.EnvConfigDir, t.TempDir()) // history, recent, templates
	st := notes.New(t.TempDir())
	st.Now = func() time.Time { return fixed }
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	return &Env{Store: st, App: AppTUI, Now: func() time.Time { return fixed }}
}

func mustCreate(t *testing.T, env *Env, title string, tags ...string) *notes.Note {
	t.Helper()
	n, err := env.Store.Create(notes.KindPage, title, tags, "# "+title+"\n")
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func values(c Completion) []string {
	out := []string{}
	for _, s := range c.Items {
		out = append(out, s.Value)
	}
	return out
}

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		name string
		args []string
	}{
		{"tag add work ideas", "tag add", []string{"work", "ideas"}},
		{"TAG RM  x", "tag rm", []string{"x"}},
		{"open My Big Page", "open", []string{"My Big Page"}},
		{`open "Quoted Page"`, "open", []string{"Quoted Page"}},
		{`set status "in progress"`, "set", []string{"status", "in progress"}},
		{"set status draft", "set", []string{"status", "draft"}},
		{"journal", "journal", nil},
		{"journal yesterday", "journal", []string{"yesterday"}},
	}
	for _, c := range cases {
		p, err := Parse(AppTUI, c.in)
		if err != nil {
			t.Errorf("Parse(%q): %v", c.in, err)
			continue
		}
		if p.Spec.Name != c.name || !reflect.DeepEqual(p.Args, c.args) {
			t.Errorf("Parse(%q) = %s %q, want %s %q", c.in, p.Spec.Name, p.Args, c.name, c.args)
		}
	}
	for _, bad := range []string{"", "nope", "tag", "open", "copy-link extra", "zoom"} {
		if _, err := Parse(AppTUI, bad); err == nil {
			t.Errorf("Parse(%q) succeeded", bad)
		}
	}
	if _, err := Parse(AppTUI, "external"); err != nil {
		t.Errorf("external in the TUI: %v", err)
	}
	if _, err := Parse(AppTUI, "tags a b"); err == nil || err.Error() != "tags is now tag set" {
		t.Errorf("renamed command: %v", err)
	}
	for _, app := range []string{AppTUI, AppGUI} {
		for _, name := range []string{"config", "config dir"} {
			if r, err := Run(&Env{App: app}, name); err != nil || r.UI != name {
				t.Errorf("%s in %s: %+v, %v", name, app, r, err)
			}
		}
	}
	if _, err := Parse(AppGUI, "zoom"); err != nil {
		t.Errorf("zoom in the GUI: %v", err)
	}
}

func TestCompleteCommands(t *testing.T) {
	env := newEnv(t)
	c := Complete(env, "ta")
	if c.Phase != "command" || len(c.Items) < 3 || c.Items[0].Value != "tag add " {
		t.Fatalf("ta -> %+v", values(c))
	}
	if c := Complete(env, "tag "); c.Phase != "command" || !reflect.DeepEqual(values(c)[:2], []string{"tag add ", "tag rm "}) {
		t.Fatalf("'tag ' -> %v", values(c))
	}
	if c := Complete(env, "contains"); len(c.Items) != 1 || c.Items[0].Value != "sync-tasks" {
		t.Fatalf("description match -> %v", values(c))
	}
	if c := Complete(env, "open"); !c.Exact || c.Usage != "open <note…>" {
		t.Fatalf("exact -> %+v", c)
	}
	env.History = []string{"tag add work", "open Groceries"}
	if c := Complete(env, ""); !c.Items[0].Recent || c.Items[0].Value != "tag add work" {
		t.Fatalf("empty prompt should lead with history: %v", values(c))
	}
}

func TestCompleteArgs(t *testing.T) {
	env := newEnv(t)
	mustCreate(t, env, "Groceries", "home", "errands")
	mustCreate(t, env, "Work Log", "work")
	env.Note = mustCreate(t, env, "Current", "home")
	env.Store.SetField(env.Note, "status", "draft")
	env.Note, _ = env.Store.Get(notes.KindPage, "current")

	c := Complete(env, "tag add ")
	if got := values(c); !reflect.DeepEqual(got, []string{"errands ", "work "}) {
		t.Errorf("tag add: %v (home is already on the note)", got)
	}
	if got := values(Complete(env, "tag add errands w")); !reflect.DeepEqual(got, []string{"work "}) {
		t.Errorf("second tag: %v", got)
	}
	if c := Complete(env, "tag add w"); c.Keep != "tag add " || c.Fragment != "w" {
		t.Errorf("keep/fragment = %q/%q", c.Keep, c.Fragment)
	}
	if got := values(Complete(env, "tag rm ")); !reflect.DeepEqual(got, []string{"home "}) {
		t.Errorf("tag rm: %v", got)
	}
	c = Complete(env, "open work l")
	if got := values(c); !reflect.DeepEqual(got, []string{"Work Log"}) || c.Keep != "open " {
		t.Errorf("open: %v keep=%q", got, c.Keep)
	}
	if got := values(Complete(env, "set st")); !reflect.DeepEqual(got, []string{"status "}) {
		t.Errorf("set field: %v", got)
	}
	if got := values(Complete(env, "set status ")); !reflect.DeepEqual(got, []string{"draft"}) {
		t.Errorf("set value: %v", got)
	}
	if got := values(Complete(env, "unset ")); !reflect.DeepEqual(got, []string{"tags ", "status "}) {
		t.Errorf("unset: %v", got)
	}
	if got := values(Complete(env, "journal y")); len(got) == 0 || got[0] != "yesterday " {
		t.Errorf("journal: %v", got)
	}
	if c := Complete(env, "copy-link "); c.Phase != "none" || len(c.Items) != 0 {
		t.Errorf("no-arg command: %+v", c)
	}
}

func TestRunNoteActions(t *testing.T) {
	env := newEnv(t)
	env.Note = mustCreate(t, env, "Target", "a")
	linker, _ := env.Store.Create(notes.KindPage, "Linker", nil, notes.LinkMarkdown(notes.KindPage, "target", "Target")+"\n")

	run := func(in string) Result {
		t.Helper()
		r, err := Run(env, in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if r.Note != nil {
			env.Note = r.Note
		}
		return r
	}
	run("tag add B c")
	run("tag rm a")
	if got := env.Note.Tags; !reflect.DeepEqual(got, []string{"b", "c"}) {
		t.Fatalf("tags = %v", got)
	}
	if _, err := Run(env, "tag rm zzz"); err == nil {
		t.Error("removing a missing tag succeeded")
	}
	run("tag set work, #Ideas  b")
	if got := env.Note.Tags; !reflect.DeepEqual(got, []string{"work", "ideas", "b"}) {
		t.Fatalf("tags = %v", got)
	}
	run("tag set")
	if got := env.Note.Tags; len(got) != 0 {
		t.Fatalf("tags after clearing = %v", got)
	}
	run("tag set b c")
	run("link Linker")
	if bl, _ := env.Store.Backlinks(notes.KindPage, "linker"); len(bl) != 1 || bl[0].Note.ID != "target" {
		t.Fatalf("backlinks of linker = %+v", bl)
	}
	if r := run("link Linker"); r.Changed {
		t.Error("linking twice changed the note")
	}
	if got := values(Complete(env, "link ")); slices.Contains(got, "Target") {
		t.Errorf("link completion offers the note itself: %v", got)
	}
	run("set status in progress")
	run("set rating 3")
	if env.Note.Extra["status"] != "in progress" || env.Note.Extra["rating"] != 3 {
		t.Fatalf("extra = %v", env.Note.Extra)
	}
	run("unset rating")
	if _, ok := env.Note.Extra["rating"]; ok {
		t.Fatal("rating still set")
	}
	if _, err := Run(env, "set updated 2020-01-01"); err == nil {
		t.Error("setting updated succeeded")
	}

	r := run("rename Renamed Target")
	if env.Note.ID != "renamed-target" || len(r.Relinked) != 1 {
		t.Fatalf("rename: id=%s relinked=%v", env.Note.ID, r.Relinked)
	}
	body, _ := os.ReadFile(linker.Path)
	if !strings.Contains(string(body), "renamed-target.md") {
		t.Fatalf("link not rewritten: %s", body)
	}
	if r := run("backlinks"); len(r.Pick) != 1 || r.Pick[0].ID != "linker" || r.UI != "backlinks" {
		t.Fatalf("backlinks: %+v", r)
	}
	if r := run("copy-link"); r.Clipboard != "[Renamed Target](../pages/renamed-target.md)" {
		t.Fatalf("copy-link: %q", r.Clipboard)
	}
	if r := run("open-with code --wait &"); r.UI != "open-with" || !reflect.DeepEqual(r.Args, []string{"code --wait &"}) {
		t.Fatalf("open-with: %+v", r)
	}
	if r := run("graph local"); r.UI != "graph" || !reflect.DeepEqual(r.Args, []string{"local"}) {
		t.Fatalf("graph: %+v", r)
	}
	if r := run("delete"); r.UI != "delete" || r.Changed {
		t.Fatalf("delete should be handed to the UI: %+v", r)
	}
	env.Note = nil
	if _, err := Run(env, "tag add x"); err == nil || !strings.Contains(err.Error(), "needs a note") {
		t.Errorf("no note: %v", err)
	}
}

func TestRunNavigation(t *testing.T) {
	env := newEnv(t)
	mustCreate(t, env, "Groceries")
	mustCreate(t, env, "Grocery Budget")

	r, err := Run(env, "open groceries")
	if err != nil || r.Note.ID != "groceries" {
		t.Fatalf("exact title: %v %v", r.Note, err)
	}
	if _, err := Run(env, "open groc"); err == nil || !strings.Contains(err.Error(), "matches 2") {
		t.Errorf("ambiguous prefix: %v", err)
	}
	if r, err := Run(env, "open budget"); err != nil || r.Note.ID != "grocery-budget" {
		t.Errorf("unique substring: %v %v", r.Note, err)
	}
	r, err = Run(env, "new Fresh Idea")
	if err != nil || r.Note.ID != "fresh-idea" || !r.Edit || !r.Changed {
		t.Fatalf("new: %+v %v", r, err)
	}
	r, err = Run(env, "journal yesterday")
	if err != nil || r.Note.ID != "2026-11-16" || !r.Edit {
		t.Fatalf("journal yesterday: %+v %v", r, err)
	}
	if r, err = Run(env, "journal -1"); err != nil || r.Note.ID != "2026-11-16" || r.Edit {
		t.Fatalf("existing journal: %+v %v", r, err)
	}
	if _, err := Run(env, "journal someday"); err == nil {
		t.Error("bad day accepted")
	}
}

func TestHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h")
	if h := LoadHistory(path); len(h) != 0 {
		t.Fatalf("missing file: %v", h)
	}
	AddHistory(path, "open a")
	AddHistory(path, "tag  add   x")
	h, err := AddHistory(path, "open a")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(h, []string{"open a", "tag add x"}) || !reflect.DeepEqual(LoadHistory(path), h) {
		t.Fatalf("history = %v / %v", h, LoadHistory(path))
	}
	for i := 0; i < HistoryLimit+10; i++ {
		h, _ = AddHistory(path, "open "+strings.Repeat("x", i+1))
	}
	if len(h) != HistoryLimit || len(LoadHistory(path)) != HistoryLimit {
		t.Fatalf("not capped: %d", len(h))
	}
}

func TestRunFeatures(t *testing.T) {
	env := newEnv(t)
	env.Note = mustCreate(t, env, "Here")

	r, err := Run(env, "todo call Sam {due:tomorrow}")
	if err != nil || r.Note != nil || !r.Changed {
		t.Fatalf("todo: %+v %v", r, err)
	}
	j, err := env.Store.Get(notes.KindJournal, "2026-11-17")
	if err != nil || !strings.Contains(j.Body, "- [ ] call Sam {due:2026-11-18}") {
		t.Fatalf("journal body: %v %q", err, j.Body)
	}
	if r, err = Run(env, "todo here write it"); err != nil || !strings.Contains(r.Note.Body, "- [ ] write it") {
		t.Fatalf("todo here: %+v %v", r, err)
	}

	r, _ = Run(env, "pin")
	env.Note = r.Note
	if !env.Note.Pinned() {
		t.Fatal("not pinned")
	}
	if r, _ = Run(env, "pin"); !strings.Contains(r.Status, "already") {
		t.Errorf("second pin: %q", r.Status)
	}
	if r, _ = Run(env, "unpin"); r.Note.Pinned() {
		t.Error("still pinned")
	}

	tdir := env.TemplatesPath()
	notes.WriteExampleTemplate(tdir)
	if got := values(Complete(env, "new from ")); !reflect.DeepEqual(got, []string{"meeting "}) {
		t.Fatalf("template completion: %v", got)
	}
	r, err = Run(env, "new from meeting Weekly Sync")
	if err != nil || r.Note.ID != "weekly-sync" || !r.Note.HasTag("meeting") || !r.Edit {
		t.Fatalf("new from: %+v %v", r, err)
	}

	rp, _ := recent.Path()
	recent.Touch(rp, "page/gone")
	recent.Touch(rp, "page/here")
	if r, _ = Run(env, "recent"); len(r.Pick) != 1 || r.Pick[0].ID != "here" || r.PickTitle == "" {
		t.Fatalf("recent: %+v", r)
	}
	if _, err := Run(env, "rename Over Here"); err != nil {
		t.Fatal(err)
	}
	if got := recent.Load(rp); got[0] != "page/over-here" {
		t.Fatalf("recent after rename: %v", got)
	}

	if _, err := Run(env, "tasks overdue done"); err != nil {
		t.Errorf("tasks filters: %v", err)
	}
	if _, err := Run(env, "tasks later"); err == nil {
		t.Error("bad tasks filter accepted")
	}
	if _, err := Run(env, "sync"); err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("sync outside a repo: %v", err)
	}
}

func TestGitStatusPick(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	env := newEnv(t)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	if out, err := exec.Command("git", "-C", env.Store.Root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	mustCreate(t, env, "Changed")
	os.WriteFile(filepath.Join(env.Store.Root, "loose.txt"), []byte("x"), 0o644)
	r, err := Run(env, "git status")
	if err != nil || len(r.Pick) != 1 || r.Pick[0].ID != "changed" || !strings.Contains(r.Status, "new loose.txt") {
		t.Fatalf("git status: %+v %v", r, err)
	}
}

func TestFuzzyOpenAndSavedSearches(t *testing.T) {
	env := newEnv(t)
	mustCreate(t, env, "Weekly Standup")
	mustCreate(t, env, "Groceries")
	mustCreate(t, env, "Budget")

	if got := values(Complete(env, "open stdup")); !reflect.DeepEqual(got, []string{"Weekly Standup"}) {
		t.Fatalf("fuzzy open: %v", got)
	}
	rp, _ := recent.Path()
	recent.Touch(rp, "page/groceries")
	got := Complete(env, "open ")
	if len(got.Items) != 3 || got.Items[0].Value != "Groceries" || !strings.Contains(got.Items[0].Detail, "recent") {
		t.Fatalf("quick open order: %+v", got.Items)
	}

	if _, err := Run(env, "search save Shopping"); err == nil {
		t.Error("saved an empty search")
	}
	env.Search = CurrentSearch{Mode: "tags", Text: "home"}
	if _, err := Run(env, "search save Shopping list"); err != nil {
		t.Fatal(err)
	}
	if got := values(Complete(env, "search sho")); !reflect.DeepEqual(got, []string{"Shopping list"}) {
		t.Fatalf("saved completion: %v", got)
	}
	r, err := Run(env, "search shopping list")
	if err != nil || r.UI != "search" || !reflect.DeepEqual(r.Args, []string{"tags", "home"}) {
		t.Fatalf("apply: %+v %v", r, err)
	}
	if _, err := Run(env, "search rm Shopping list"); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(env, "search Shopping list"); err == nil {
		t.Error("deleted search still applies")
	}
}

func TestThemeCommand(t *testing.T) {
	env := newEnv(t) // App: tui
	env.ThemesDir = t.TempDir()
	os.WriteFile(filepath.Join(env.ThemesDir, "ocean.toml"), []byte("dark = true\n"), 0o644)
	os.WriteFile(filepath.Join(env.ThemesDir, "broken.toml"), []byte("[colors]\naccent = \"nope\"\n"), 0o644)

	// :theme in the TUI sets tui_theme only.
	r, err := Run(env, "theme ocean")
	if err != nil || r.UI != "theme" || !reflect.DeepEqual(r.Args, []string{"ocean"}) {
		t.Fatalf("theme ocean: %+v %v", r, err)
	}
	cfg, _ := config.Load()
	if cfg.TUITheme != "ocean" || cfg.Theme != "auto" || cfg.ThemeFor(AppGUI) != "auto" {
		t.Errorf("after tui :theme ocean: %+v", cfg)
	}
	if r, err := Run(env, "theme"); err != nil || !strings.Contains(r.Status, "TUI theme: ocean") || r.UI != "" {
		t.Errorf("theme with no name: %+v %v", r, err)
	}

	// The GUI gets its own.
	gui := *env
	gui.App = AppGUI
	if r, err := Run(&gui, "theme light"); err != nil || r.Args[0] != "light" {
		t.Fatalf("gui theme light: %+v %v", r, err)
	}
	cfg, _ = config.Load()
	if cfg.GUITheme != "light" || cfg.ThemeFor(AppTUI) != "ocean" {
		t.Errorf("after gui :theme light: %+v", cfg)
	}

	// shared clears the override: the app follows `theme` again.
	if err := config.Set("theme", "dark"); err != nil {
		t.Fatal(err)
	}
	if r, err := Run(env, "theme shared"); err != nil || r.Args[0] != "dark" || !strings.Contains(r.Status, "(shared)") {
		t.Errorf("theme shared: %+v %v", r, err)
	}
	if cfg, _ := config.Load(); cfg.TUITheme != "" || cfg.ThemeFor(AppTUI) != "dark" || cfg.ThemeFor(AppGUI) != "light" {
		t.Errorf("after shared: %+v", cfg)
	}

	if _, err := Run(env, "theme missing"); err == nil {
		t.Error("missing theme accepted")
	}
	if _, err := Run(env, "theme broken"); err == nil {
		t.Error("broken theme accepted")
	}
	if r, err := Run(env, "theme AUTO"); err != nil || r.Args[0] != "auto" {
		t.Errorf("theme auto: %+v %v", r, err)
	}
	c := Complete(env, "theme oc")
	if len(c.Items) != 1 || strings.TrimSpace(c.Items[0].Value) != "ocean" {
		t.Errorf("completion: %+v", c.Items)
	}
	if c := Complete(env, "theme sh"); len(c.Items) != 1 || strings.TrimSpace(c.Items[0].Value) != "shared" {
		t.Errorf("completion of shared: %+v", c.Items)
	}
}

func TestRunWikiLinks(t *testing.T) {
	env := newEnv(t)
	mustCreate(t, env, "Groceries")
	n, _ := env.Store.Create(notes.KindPage, "Plan", nil, "- [[Groceries]]\n  - milk\n- [[New Thing]]\n")
	env.Note = n
	if _, err := Run(env, "wikilinks bogus"); err == nil {
		t.Fatal("wikilinks bogus: want an error")
	}
	r, err := Run(env, "wikilinks move")
	if err != nil || !r.Changed || !strings.Contains(r.Status, "created page/new-thing") || !strings.Contains(r.Status, "moved 1 block(s) to page/groceries") {
		t.Fatalf("wikilinks move: %+v %v", r, err)
	}
	if r.Note.Body != "- [Groceries](../pages/groceries.md)\n- [New Thing](../pages/new-thing.md)\n" {
		t.Errorf("body = %q", r.Note.Body)
	}
	env.Note = r.Note
	if r, err = Run(env, "wikilinks"); err != nil || r.Changed || !strings.HasPrefix(r.Status, "no [[links]]") {
		t.Errorf("second run: %+v %v", r, err)
	}
}
