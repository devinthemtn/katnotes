package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsAndOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvConfigDir, dir)
	t.Setenv(EnvNotesDir, "")
	t.Setenv("HOME", "/home/tester")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.NotesDir != "/home/tester/KatNotes" {
		t.Errorf("NotesDir = %q", cfg.NotesDir)
	}
	if cfg.DateFormat != "2006-01-02" {
		t.Errorf("DateFormat = %q", cfg.DateFormat)
	}

	t.Setenv(EnvNotesDir, "/tmp/other")
	cfg, _ = Load()
	if cfg.NotesDir != "/tmp/other" {
		t.Errorf("env override: NotesDir = %q", cfg.NotesDir)
	}
}

func TestInstallAndSave(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvConfigDir, dir)
	t.Setenv(EnvNotesDir, "")

	if err := Install(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.toml")); err != nil {
		t.Fatal(err)
	}
	c := Default()
	c.NotesDir = "/x/notes"
	c.Editor = "hx"
	if err := Save(c); err != nil {
		t.Fatal(err)
	}
	// Install must not clobber an existing file.
	if err := Install(); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.NotesDir != "/x/notes" || got.Editor != "hx" {
		t.Errorf("got %+v", got)
	}
}
