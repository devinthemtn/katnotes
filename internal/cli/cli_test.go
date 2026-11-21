package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devinthemtn/katnotes/internal/config"
	"github.com/devinthemtn/katnotes/internal/recent"
)

// setup points the CLI at empty temp notes and config folders.
func setup(t *testing.T) (notesDir, cfgDir string) {
	t.Helper()
	notesDir, cfgDir = t.TempDir(), t.TempDir()
	t.Setenv(config.EnvConfigDir, cfgDir)
	t.Setenv("KATNOTES_DIR", notesDir)
	t.Setenv("EDITOR", "true")
	t.Setenv("VISUAL", "")
	return notesDir, cfgDir
}

// run runs one katnotes command line and returns its output.
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := NewRoot("test", nil)
	var out bytes.Buffer
	root.SetArgs(args)
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(""))
	err := root.Execute()
	return out.String(), err
}

func mustRun(t *testing.T, args ...string) string {
	t.Helper()
	out, err := run(t, args...)
	if err != nil {
		t.Fatalf("katnotes %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func TestSharedNoteCommands(t *testing.T) {
	notesDir, _ := setup(t)
	mustRun(t, "new", "--no-edit", "Alpha")
	mustRun(t, "tag", "add", "alpha", "work", "home")
	mustRun(t, "set", "alpha", "project", "big", "house")
	mustRun(t, "pin", "alpha")

	data, err := os.ReadFile(filepath.Join(notesDir, "pages", "alpha.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"tags: [work, home]", "project: big house", "pinned: true"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("alpha.md lacks %q:\n%s", want, data)
		}
	}

	mustRun(t, "unset", "alpha", "project")
	mustRun(t, "tag", "rm", "page/alpha", "home") // kind/id, as tasks list prints
	mustRun(t, "unpin", "alpha")
	data, _ = os.ReadFile(filepath.Join(notesDir, "pages", "alpha.md"))
	for _, gone := range []string{"project:", "home", "pinned"} {
		if strings.Contains(string(data), gone) {
			t.Errorf("alpha.md still has %q:\n%s", gone, data)
		}
	}

	if out := mustRun(t, "link", "alpha"); strings.TrimSpace(out) != "[Alpha](../pages/alpha.md)" {
		t.Errorf("link = %q", out)
	}
}

func TestJSONResult(t *testing.T) {
	setup(t)
	mustRun(t, "new", "--no-edit", "-m", "body text", "Alpha")
	out := mustRun(t, "--json", "pin", "alpha")
	var r struct {
		Status string `json:"status"`
		Note   struct {
			ID   string `json:"id"`
			Body string `json:"body"`
		} `json:"note"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if r.Note.ID != "alpha" || r.Note.Body != "" || !strings.Contains(r.Status, "pinned") {
		t.Errorf("got %+v", r)
	}
}

func TestRecentFollowsRenameAndDelete(t *testing.T) {
	_, cfgDir := setup(t)
	mustRun(t, "new", "--no-edit", "Alpha")
	p := filepath.Join(cfgDir, recent.FileName)
	if err := recent.Touch(p, "page/alpha"); err != nil {
		t.Fatal(err)
	}
	mustRun(t, "mv", "alpha", "Beta")
	if got := recent.Load(p); len(got) != 1 || got[0] != "page/beta" {
		t.Fatalf("after mv, recent = %v", got)
	}
	mustRun(t, "rm", "-f", "beta")
	if got := recent.Load(p); len(got) != 0 {
		t.Fatalf("after rm, recent = %v", got)
	}
}

func TestTodoAndJournal(t *testing.T) {
	notesDir, _ := setup(t)
	mustRun(t, "new", "--no-edit", "Alpha")
	if out := mustRun(t, "todo", "--here", "alpha", "buy", "milk"); !strings.Contains(out, "page/alpha") {
		t.Errorf("todo --here: %q", out)
	}
	data, _ := os.ReadFile(filepath.Join(notesDir, "pages", "alpha.md"))
	if !strings.Contains(string(data), "- [ ] buy milk") {
		t.Errorf("no task in alpha.md:\n%s", data)
	}
	if out := mustRun(t, "journal", "--no-edit", "2026-01-02"); !strings.Contains(out, "created journal/2026-01-02") {
		t.Errorf("journal: %q", out)
	}
}

func TestSavedSearches(t *testing.T) {
	setup(t)
	mustRun(t, "new", "--no-edit", "-t", "work", "Alpha")
	mustRun(t, "new", "--no-edit", "Beta")
	mustRun(t, "search", "--tags", "work", "--save", "w")
	if out := mustRun(t, "saved"); !strings.Contains(out, "w") || !strings.Contains(out, "tags") {
		t.Errorf("saved: %q", out)
	}
	out := mustRun(t, "search", "--saved", "w")
	if !strings.Contains(out, "page/alpha") || strings.Contains(out, "page/beta") {
		t.Errorf("search --saved: %q", out)
	}
	mustRun(t, "saved", "rm", "w")
	if _, err := run(t, "search", "--saved", "w"); err == nil {
		t.Error("search --saved after rm: want an error")
	}
}

func TestRmJSONNeedsForce(t *testing.T) {
	setup(t)
	mustRun(t, "new", "--no-edit", "Alpha")
	if _, err := run(t, "--json", "rm", "alpha"); err == nil {
		t.Error("rm --json without --force: want an error")
	}
}

func TestLsSort(t *testing.T) {
	setup(t)
	mustRun(t, "new", "--no-edit", "Beta")
	mustRun(t, "new", "--no-edit", "Alpha")
	out := mustRun(t, "ls", "--sort", "Title")
	if strings.Index(out, "alpha") > strings.Index(out, "beta") {
		t.Errorf("--sort Title not by title:\n%s", out)
	}
	mustRun(t, "ls", "--sort", "modified")
	if _, err := run(t, "ls", "--sort", "bogus"); err == nil {
		t.Error("--sort bogus: no error")
	}
}

func TestWikiLinks(t *testing.T) {
	notesDir, _ := setup(t)
	mustRun(t, "new", "--no-edit", "Groceries")
	mustRun(t, "new", "--no-edit", "Scratch")
	p := filepath.Join(notesDir, "pages", "scratch.md")
	data, _ := os.ReadFile(p)
	os.WriteFile(p, append(data, []byte("\n- [[Groceries]]\n  - milk\n- [[Brand New|new one]]\n")...), 0o644)
	out := mustRun(t, "wikilinks", "--move", "scratch")
	if !strings.Contains(out, "created page/brand-new") {
		t.Errorf("output: %s", out)
	}
	data, _ = os.ReadFile(p)
	if s := string(data); !strings.Contains(s, "- [Groceries](../pages/groceries.md)\n- [new one](../pages/brand-new.md)") || strings.Contains(s, "milk") {
		t.Errorf("scratch.md:\n%s", s)
	}
	g, _ := os.ReadFile(filepath.Join(notesDir, "pages", "groceries.md"))
	if !strings.Contains(string(g), "## From [Scratch](../pages/scratch.md)\n\n- milk") {
		t.Errorf("groceries.md:\n%s", g)
	}
}

// The address todo prints is the one tasks list prints and tasks set takes.
func TestTodoAddressWorksWithTasksSet(t *testing.T) {
	setup(t)
	out := mustRun(t, "todo", "call Sam")
	addr := strings.TrimSpace(out[strings.LastIndex(out, " ")+1:])
	if list := mustRun(t, "tasks", "list"); !strings.Contains(list, addr) {
		t.Fatalf("todo said %q; tasks list:\n%s", addr, list)
	}
	mustRun(t, "tasks", "set", addr, "pri", "high")
	if list := mustRun(t, "tasks", "list"); !strings.Contains(list, "high") {
		t.Fatalf("pri not set:\n%s", list)
	}
}
