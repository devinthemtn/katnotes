package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devinthemtn/katnotes/internal/notes"
)

// Each subcommand with the output it should print. They run in order
// against one notes folder: Alpha (tag work) links to Beta.
func TestSubcommands(t *testing.T) {
	notesDir, cfgDir := setup(t)
	mustRun(t, "new", "--no-edit", "Alpha", "-t", "work")
	mustRun(t, "new", "--no-edit", "Beta")
	alpha := filepath.Join(notesDir, "pages", "alpha.md")
	data, err := os.ReadFile(alpha)
	if err != nil {
		t.Fatal(err)
	}
	link := notes.LinkMarkdown(notes.KindPage, "beta", "Beta")
	if err := os.WriteFile(alpha, append(data, []byte("\nsee "+link+"\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		args []string
		want []string // substrings of the output
	}{
		{[]string{"init"}, []string{notesDir, filepath.Join(cfgDir, "config.toml")}},
		{[]string{"show", "alpha"}, []string{"title: Alpha", "tags: [work]", link}},
		{[]string{"link", "beta"}, []string{link}},
		{[]string{"links", "alpha"}, []string{"page/beta", "Beta"}},
		{[]string{"backlinks", "beta"}, []string{"page/alpha"}},
		{[]string{"graph"}, []string{"page/alpha -> page/beta"}},
		{[]string{"graph", "beta", "--tags", "--format", "dot"}, []string{`"page/alpha" -> "page/beta"`, `"page/beta" [label="Beta", penwidth=2]`}},
		{[]string{"graph", "alpha", "--tags"}, []string{"page/alpha -- tag:work"}},
		{[]string{"tags"}, []string{"work"}},
		{[]string{"today", "--no-edit"}, []string{filepath.Join(notesDir, "journal")}},
		{[]string{"edit", "beta"}, nil},                  // $EDITOR is true(1)
		{[]string{"recent"}, []string{"recent note(s)"}}, // only the TUI and GUI add to it
		{[]string{"config", "path"}, []string{filepath.Join(cfgDir, "config.toml")}},
		{[]string{"config", "set", "tui_mouse", "false"}, nil},
		{[]string{"config", "show"}, []string{"tui_mouse = false"}},
		{[]string{"config", "edit"}, nil},
		{[]string{"theme"}, []string{"shared theme: auto"}},
		{[]string{"theme", "--app", "tui", "light"}, []string{"TUI theme: light"}},
	} {
		out := mustRun(t, c.args...)
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Errorf("katnotes %s: no %q in\n%s", strings.Join(c.args, " "), w, out)
			}
		}
	}

	var g notes.Graph
	if err := json.Unmarshal([]byte(mustRun(t, "graph", "--json")), &g); err != nil || len(g.Edges) != 1 {
		t.Errorf("graph --json: %+v, %v", g, err)
	}

	var n notes.Note
	if err := json.Unmarshal([]byte(mustRun(t, "show", "--json", "alpha")), &n); err != nil || n.Title != "Alpha" {
		t.Errorf("show --json: %+v, %v", n, err)
	}
}

func TestSubcommandErrors(t *testing.T) {
	setup(t)
	for _, args := range [][]string{
		{"show", "nope"},
		{"edit", "nope"},
		{"links", "nope"},
		{"graph", "nope"},
		{"graph", "--format", "png"},
		{"config", "set", "tui_mouse", "maybe"},
		{"config", "set", "no_such_key", "x"},
		{"theme", "--app", "phone", "dark"},
		{"git", "status"}, // not a git repository
		{"sync"},
	} {
		if out, err := run(t, args...); err == nil {
			t.Errorf("katnotes %s should fail:\n%s", strings.Join(args, " "), out)
		}
	}
}

func TestGitStatus(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	notesDir, _ := setup(t)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "T"}} {
		if out, err := exec.Command("git", append([]string{"-C", notesDir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	mustRun(t, "new", "--no-edit", "Gamma")
	if out := mustRun(t, "git", "status"); !strings.Contains(out, "page/gamma") {
		t.Errorf("git status should list the new note:\n%s", out)
	}
}

func TestRunLua(t *testing.T) {
	_, cfgDir := setup(t)
	mustRun(t, "new", "--no-edit", "Alpha")
	if out := mustRun(t, "run"); !strings.Contains(out, "no Lua commands") {
		t.Errorf("run with none: %s", out)
	}
	dir := filepath.Join(cfgDir, "commands")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "hi.lua"), []byte(`return {
  description = "Say hi",
  run = function(ctx) return { status = ctx.app .. " " .. ctx.args .. " " .. ctx.note.id, open = ctx.note.id ~= "" and "page/alpha" or nil } end,
}`), 0o644)
	os.WriteFile(filepath.Join(dir, "sync.lua"), []byte(`return { run = function() return "shadowed" end }`), 0o644)

	if out := mustRun(t, "run"); !strings.Contains(out, "hi") || !strings.Contains(out, "Say hi") || strings.Contains(out, "sync") {
		t.Errorf("run list: %s", out)
	}
	if out := mustRun(t, "run", "hi", "a", "--b"); strings.TrimSpace(out) != "cli a --b" {
		t.Errorf("run hi: %q", out)
	}
	if out := mustRun(t, "run", "--note", "alpha", "hi", "x"); !strings.Contains(out, "cli x alpha") || !strings.Contains(out, "open: page/alpha") {
		t.Errorf("run --note: %s", out)
	}
	if _, err := run(t, "run", "sync"); err == nil {
		t.Error("a built-in's name must not run the Lua file")
	}
	if _, err := run(t, "run", "nope"); err == nil {
		t.Error("unknown command should fail")
	}
}
