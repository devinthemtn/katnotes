package main

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/devinthemtn/katnotes/internal/config"
)

func newService(t *testing.T) *NotesService {
	t.Helper()
	t.Setenv(config.EnvConfigDir, t.TempDir())
	t.Setenv(config.EnvNotesDir, t.TempDir())
	s, err := NewNotesService()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestServiceFlow(t *testing.T) {
	s := newService(t)
	n, err := s.Create("page", "GUI Note", []string{"gui"})
	if err != nil {
		t.Fatal(err)
	}
	list, _ := s.List("page", "gui")
	if len(list) != 1 || list[0].Body != "" {
		t.Fatalf("list: %+v", list)
	}

	saved, err := s.Save("page", n.ID, []string{"gui", "x"}, "# GUI Note\n\nhello\n", n.Stamp, false)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Stamp == "" || saved.Stamp == n.Stamp {
		t.Fatalf("save didn't refresh the stamp: %q -> %q", n.Stamp, saved.Stamp)
	}

	// Another program edits the file in between, keeping its updated:
	// field as it was: the stale stamp must still conflict.
	before, _ := os.ReadFile(saved.Path)
	edited := strings.Replace(string(before), "hello", "edited elsewhere", 1)
	if edited == string(before) {
		t.Fatalf("unexpected file: %s", before)
	}
	if err := os.WriteFile(saved.Path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save("page", n.ID, nil, "stale gui\n", saved.Stamp, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
	if _, err := s.Save("page", n.ID, nil, "forced\n", saved.Stamp, true); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get("page", n.ID)
	if got.Body != "forced\n" {
		t.Errorf("body %q", got.Body)
	}

	res, _ := s.Find("text", "forced", "", "", "")
	if len(res) != 1 {
		t.Errorf("search: %+v", res)
	}
	if _, err := s.Today(); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("page", n.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSetOptionSwitchesDir(t *testing.T) {
	s := newService(t)
	os.Unsetenv(config.EnvNotesDir)
	dir := t.TempDir()
	if err := s.SetOption("notes_dir", dir); err != nil {
		t.Fatal(err)
	}
	if s.Info().NotesDir != dir {
		t.Errorf("notes dir = %q", s.Info().NotesDir)
	}
	if _, err := os.Stat(dir + "/journal"); err != nil {
		t.Errorf("journal dir not created: %v", err)
	}
}
