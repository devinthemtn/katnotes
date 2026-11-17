package main

import (
	"errors"
	"os"
	"testing"
	"time"

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

	base := n.Updated.Format(time.RFC3339Nano)
	saved, err := s.Save("page", n.ID, []string{"gui", "x"}, "# GUI Note\n\nhello\n", base, false)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate the CLI/TUI editing the file in between: stale base -> conflict.
	time.Sleep(1100 * time.Millisecond) // updated has 1s resolution
	if _, err := s.Save("page", n.ID, nil, "from cli\n", saved.Updated.Format(time.RFC3339Nano), false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save("page", n.ID, nil, "stale gui\n", saved.Updated.Format(time.RFC3339Nano), false); !errors.Is(err, ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
	if _, err := s.Save("page", n.ID, nil, "forced\n", saved.Updated.Format(time.RFC3339Nano), true); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get("page", n.ID)
	if got.Body != "forced\n" {
		t.Errorf("body %q", got.Body)
	}

	res, _ := s.Search("forced", "")
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
