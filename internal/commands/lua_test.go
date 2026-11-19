package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devinthemtn/katnotes/internal/config"
	"github.com/devinthemtn/katnotes/internal/luacmd"
	"github.com/devinthemtn/katnotes/internal/notes"
)

func writeLua(t *testing.T, name, src string) { writeLuaIn(t, luacmd.CommandDirName, name, src) }

func writeLuaIn(t *testing.T, sub, name, src string) {
	t.Helper()
	d, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(d, sub)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".lua"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLuaCommandsInCatalog(t *testing.T) {
	env := newEnv(t)
	writeLua(t, "hello", `return { description = "Say hi", long = true, run = function(ctx) return "hi " .. ctx.args end }`)
	writeLua(t, "sync", `return { run = function(ctx) return "shadowed" end }`) // a built-in's name
	var got *Spec
	for _, s := range Catalog(AppTUI) {
		if s.Name == "hello" {
			got = s
		}
		if s.Lua && s.Name == "sync" {
			t.Error("a Lua file must not shadow the built-in sync")
		}
	}
	if got == nil || !got.Lua || !got.Long || got.Desc != "Say hi" || got.Usage() != "hello [args…]" {
		t.Fatalf("hello spec = %+v", got)
	}
	r, err := Run(env, "hello there world")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "hi there world" || !r.Changed || r.Command != "hello" {
		t.Errorf("result = %+v", r)
	}
	if r, err := Run(env, "sync"); err == nil && r.Status == "shadowed" {
		t.Error("sync ran the Lua file")
	}
	c := Complete(env, "hel")
	if !strings.Contains(strings.Join(values(c), " "), "hello") {
		t.Errorf("completion %v lacks hello", values(c))
	}
}

func TestLuaCommandContextAndOpen(t *testing.T) {
	env := newEnv(t)
	n := mustCreate(t, env, "Groceries", "food")
	env.Note = n
	writeLua(t, "mk", `return { run = function(ctx)
		assert(ctx.source == "command", ctx.source)
		assert(ctx.app == "tui", ctx.app)
		assert(ctx.note.id == "groceries" and ctx.note.tags[1] == "food")
		assert(ctx.body:find("# Groceries"), ctx.body)
		local f = assert(io.open(ctx.notes_dir .. "/pages/made-by-lua.md", "w"))
		f:write("# Made by Lua\n"); f:close()
		return { status = "made it", open = "page/made-by-lua" }
	end }`)
	r, err := Run(env, "mk")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "made it" || r.Note == nil || r.Note.ID != "made-by-lua" || r.Note.Kind != notes.KindPage {
		t.Errorf("result = %+v", r)
	}
}

func TestLuaCommandTimeout(t *testing.T) {
	env := newEnv(t)
	old := luacmd.Timeout
	luacmd.Timeout = 100e6 // 100ms
	t.Cleanup(func() { luacmd.Timeout = old })
	// Runs ~300ms: too long for the default, fine with its own timeout.
	busy := `local t = os.clock() while os.clock() - t < 0.3 do end return "ok"`
	writeLua(t, "slow", `return { run = function(ctx) `+busy+` end }`)
	writeLua(t, "patient", `return { timeout = 5, run = function(ctx) `+busy+` end }`)
	if _, err := Run(env, "slow"); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("slow: err = %v, want a timeout", err)
	}
	if r, err := Run(env, "patient"); err != nil || r.Status != "ok" {
		t.Errorf("patient: %+v, %v", r, err)
	}
	// From the editor's / menu the script's timeout does not apply.
	d, _ := config.Dir()
	if _, err := luacmd.Run(filepath.Join(d, luacmd.CommandDirName), "patient", luacmd.Context{}); err == nil {
		t.Error("/patient should time out")
	}
}

func TestSlashCommandsStayOutOfCatalog(t *testing.T) {
	env := newEnv(t)
	writeLuaIn(t, luacmd.SlashDirName, "sig", `return { run = function(ctx) return "me" end }`)
	for _, s := range Catalog(AppTUI) {
		if s.Name == "sig" {
			t.Fatal("a slash/ script showed up in the : catalog")
		}
	}
	if _, err := Run(env, "sig"); err == nil {
		t.Error(":sig ran a slash/ script")
	}
}

func TestWriteExamples(t *testing.T) {
	env := newEnv(t)
	d, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := luacmd.WriteExamples(d); err != nil {
		t.Fatal(err)
	}
	for sub, want := range map[string]string{luacmd.SlashDirName: `"/" command`, luacmd.CommandDirName: `":" command`} {
		b, err := os.ReadFile(filepath.Join(d, sub, "_example.lua"))
		if err != nil || !strings.Contains(string(b), want) {
			t.Errorf("%s/_example.lua: %v %q", sub, err, b)
		}
	}
	// The ":" example works as written once copied to a real name.
	src, _ := os.ReadFile(filepath.Join(d, luacmd.CommandDirName, "_example.lua"))
	writeLua(t, "wordcount", string(src))
	env.Note = mustCreate(t, env, "Three Words Here")
	if r, err := Run(env, "wordcount"); err != nil || !strings.HasSuffix(r.Status, "4 words") {
		t.Errorf("wordcount: %+v %v", r, err)
	}
}
