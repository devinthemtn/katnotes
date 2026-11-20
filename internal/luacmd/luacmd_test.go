package luacmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, dir, name, src string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListAndRun(t *testing.T) {
	cfg := t.TempDir()
	if err := WriteExamples(cfg); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cfg, SlashDirName) // _example.lua there must not be listed
	write(t, dir, "sig.lua", `return { description = "Signature", run = function(ctx) return "-- " .. ctx.note.title .. " #" .. ctx.note.tags[1] end }`)
	write(t, dir, "upper.lua", `return { run = function(ctx) return { line = string.upper(ctx.before .. ctx.after), cursor = 2 } end }`)
	write(t, dir, "nothing.lua", `return { run = function(ctx) return nil end }`)
	write(t, dir, "norun.lua", `return { description = "x" }`)
	write(t, dir, "Bad Name.lua", `return { run = function() end }`)
	write(t, dir, "boom.lua", "return {\n run = function(ctx)\n  error('kaput')\n end }")
	write(t, dir, "loop.lua", `return { run = function(ctx) while true do end end }`)

	cmds, errs := List(dir)
	var names []string
	for _, c := range cmds {
		names = append(names, c.Name)
	}
	if got := strings.Join(names, ","); got != "boom,loop,nothing,sig,upper" {
		t.Errorf("names = %s", got)
	}
	if len(errs) != 2 {
		t.Errorf("errs = %v", errs)
	}
	for _, c := range cmds {
		if c.Name == "sig" && c.Description != "Signature" {
			t.Errorf("sig description = %q", c.Description)
		}
	}

	ctx := Context{Before: "ab ", After: " cd", Note: NoteInfo{Title: "Shop", Tags: []string{"home"}}}
	if r, err := Run(dir, "sig", ctx); err != nil || r.Kind != "text" || r.Text != "-- Shop #home" {
		t.Errorf("sig = %+v, %v", r, err)
	}
	if r, err := Run(dir, "upper", ctx); err != nil || r.Kind != "line" || r.Line != "AB  CD" || r.Cursor != 2 {
		t.Errorf("upper = %+v, %v", r, err)
	}
	if r, err := Run(dir, "nothing", ctx); err != nil || r.Kind != "text" || r.Text != "" {
		t.Errorf("nothing = %+v, %v", r, err)
	}
	if _, err := Run(dir, "boom", ctx); err == nil || !strings.Contains(err.Error(), "boom.lua:3") || !strings.Contains(err.Error(), "kaput") {
		t.Errorf("boom err = %v", err)
	}
	old := Timeout
	Timeout = 200 * time.Millisecond
	defer func() { Timeout = old }()
	if _, err := Run(dir, "loop", ctx); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("loop err = %v", err)
	}
	// The message names the limit the script actually had.
	write(t, dir, "own.lua", `return { timeout = 0.3, run = function(ctx) while true do end end }`)
	if _, err := Run(dir, "own", Context{Source: SourceCommand}); err == nil || !strings.Contains(err.Error(), "own.lua: timed out after 300ms") {
		t.Errorf("own timeout from command window: err = %v", err)
	}
	if _, err := Run(dir, "own", Context{Source: SourceSlash}); err == nil || !strings.Contains(err.Error(), "own.lua: timed out after 200ms") {
		t.Errorf("own timeout from / menu: err = %v", err)
	}
	if _, err := Run(dir, "../x", ctx); err == nil {
		t.Error("path traversal accepted")
	}
}

func TestExampleRuns(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "ex.lua", SlashExample)
	r, err := Run(dir, "ex", Context{})
	if err != nil || !strings.Contains(r.Text, time.Now().Format("2006-01-02")) {
		t.Errorf("example = %+v, %v", r, err)
	}
}
