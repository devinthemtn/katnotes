package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCallouts(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvConfigDir, dir)
	t.Setenv(EnvNotesDir, "")
	toml := `
[callouts.Idea]
color = "#e0af68"
icon = "💡"
title = "Idea"

[callouts.warning]
color = "orange"

[callouts.nocolor]
icon = "x"

[callouts.badcolor]
color = "red; background: url(x)"

[callouts."bad name"]
color = "red"
`
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	errs := strings.Join(cfg.CalloutErrors, "\n")
	for _, want := range []string{"nocolor", "badcolor", "bad name"} {
		if !strings.Contains(errs, want) {
			t.Errorf("errors %q should mention %q", errs, want)
		}
	}
	if got := cfg.Callouts["idea"]; got != (Callout{Color: "#e0af68", Icon: "💡", Title: "Idea"}) {
		t.Errorf("idea = %+v", got)
	}
	if got := cfg.Callouts["warning"].Color; got != "orange" {
		t.Errorf("warning color = %q", got)
	}
	if len(cfg.Callouts) != 2 {
		t.Errorf("callouts = %+v, want only idea and warning", cfg.Callouts)
	}

	// Save round-trips them.
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	again, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if again.Callouts["idea"] != cfg.Callouts["idea"] {
		t.Errorf("after save: %+v", again.Callouts)
	}
}

func TestNoCalloutsSavesNoTable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvConfigDir, dir)
	if err := Save(Default()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
	if strings.Contains(string(b), "callouts") {
		t.Errorf("empty callouts written:\n%s", b)
	}
}
