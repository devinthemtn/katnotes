package commands

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/devinthemtn/katnotes/internal/notes"
)

func TestImportTask(t *testing.T) {
	env := newEnv(t)
	dir := t.TempDir()
	todo := filepath.Join(dir, "todo.txt")
	env.Store.Todo = &notes.TodoSync{File: todo, StatePath: filepath.Join(dir, "state.json")}
	os.WriteFile(todo, []byte(""+
		"(A) 2026-11-01 call the plumber +house due:2026-11-20 rec:1w\n"+
		"x 2026-11-02 already done\n"+
		"buy milk @store kn:abcdef note:page/x\n"+
		"(D) water plants @home\n"), 0o644)
	env.Note = mustCreate(t, env, "House")

	c := Complete(env, "import-task plum")
	if len(c.Items) != 1 || c.Items[0].Label != "call the plumber +house" || !strings.Contains(c.Items[0].Detail, "due 2026-11-20") {
		t.Fatalf("completion = %+v", c.Items)
	}
	if all := Complete(env, "import-task "); len(all.Items) != 2 {
		t.Fatalf("all = %+v (done and linked lines must be left out)", all.Items)
	}

	r, err := Run(env, "import-task "+c.Items[0].Value)
	if err != nil || r.Note == nil {
		t.Fatalf("import: %+v %v", r, err)
	}
	m := regexp.MustCompile(`- \[ \] call the plumber \+house \{due:2026-11-20 pri:high kt:([0-9a-f]{6})\}`).FindStringSubmatch(r.Note.Body)
	if m == nil {
		t.Fatalf("body = %q", r.Note.Body)
	}
	got, _ := os.ReadFile(todo)
	want := "(A) 2026-11-01 call the plumber +house due:2026-11-20 rec:1w kn:" + m[1] + " note:page/house\n"
	if !strings.HasPrefix(string(got), want) {
		t.Errorf("todo.txt =\n%s\nwant first line\n%s", got, want)
	}
	if again := Complete(env, "import-task plum"); len(again.Items) != 0 {
		t.Errorf("still offered after import: %+v", again.Items)
	}

	// A fragment works when it matches one task; with no note, today's journal.
	env.Note = nil
	if _, err := Run(env, "import-task water"); err != nil {
		t.Fatal(err)
	}
	j, err := env.Store.Get(notes.KindJournal, "2026-11-17")
	if err != nil || !strings.Contains(j.Body, "- [ ] water plants @home {pri:low kt:") {
		t.Fatalf("journal: %v %q", err, j.Body)
	}
	if got, _ := os.ReadFile(todo); !strings.Contains(string(got), "(D) water plants @home kn:") {
		t.Errorf("priority D not kept:\n%s", got)
	}
	if _, err := Run(env, "import-task nothing like it"); err == nil {
		t.Error("no match accepted")
	}
}
