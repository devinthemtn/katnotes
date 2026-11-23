package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTasksImport(t *testing.T) {
	_, cfgDir := setup(t)
	if _, err := run(t, "tasks", "import", "x"); err == nil || !strings.Contains(err.Error(), "[todo_txt]") {
		t.Fatalf("sync off: %v", err)
	}
	todo := filepath.Join(t.TempDir(), "todo.txt")
	os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte("[todo_txt]\nenabled = true\nfile = \""+todo+"\"\n"), 0o644)
	os.WriteFile(todo, []byte("(B) call the plumber +house\nother\n"), 0o644)
	mustRun(t, "new", "House")

	out := mustRun(t, "--json", "tasks", "import", "(B) call the plumber +house", "--to", "page/house")
	var r struct {
		Note, ID, Text string
		Line           int
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil || r.Note != "page/house" || len(r.ID) != 6 {
		t.Fatalf("json %q: %+v %v", out, r, err)
	}
	if !strings.Contains(r.Text, "- [ ] call the plumber +house {pri:med kt:"+r.ID+"}") {
		t.Errorf("text = %q", r.Text)
	}
	got, _ := os.ReadFile(todo)
	if !strings.HasPrefix(string(got), "(B) call the plumber +house kn:"+r.ID+" note:page/house\n") {
		t.Errorf("todo.txt = %q", got)
	}

	// --file naming another todo.txt warns and uses it; no --to is today's journal.
	other := filepath.Join(t.TempDir(), "todo.txt")
	os.WriteFile(other, []byte("water plants\n"), 0o644)
	out = mustRun(t, "tasks", "import", "water", "--file", other)
	if !strings.Contains(out, "warning") || !strings.Contains(out, "journal/") {
		t.Errorf("out = %q", out)
	}
	if got, _ := os.ReadFile(other); !strings.Contains(string(got), "kn:") {
		t.Errorf("other = %q", got)
	}
}
