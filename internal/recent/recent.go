// Package recent keeps the notes opened most recently in any KatNotes app,
// as "kind/id" lines (newest first) in the config directory. It backs the
// ":recent" command and start_page = "last" in both the TUI and the GUI.
package recent

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/devinthemtn/katnotes/internal/config"
)

// FileName is the recent-notes file inside the config directory.
const FileName = "recent"

// Limit is how many notes are remembered.
const Limit = 50

// Path returns the recent-notes file's location.
func Path() (string, error) {
	d, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, FileName), nil
}

// Load returns the remembered note keys, newest first. A missing file is
// empty.
func Load(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{}
	}
	out := []string{}
	for _, l := range strings.Split(string(data), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// Touch records key ("kind/id") as the most recently opened note.
func Touch(path, key string) error {
	return update(path, func(keys []string) []string {
		out := []string{key}
		for _, k := range keys {
			if k != key {
				out = append(out, k)
			}
		}
		return out
	})
}

// Forget drops key, e.g. after the note was deleted.
func Forget(path, key string) error {
	return Rename(path, key, "")
}

// Rename replaces from with to (or drops from when to is ""), keeping its
// place in the list.
func Rename(path, from, to string) error {
	return update(path, func(keys []string) []string {
		out := []string{}
		for _, k := range keys {
			switch {
			case k == to:
			case k != from:
				out = append(out, k)
			case to != "":
				out = append(out, to)
			}
		}
		return out
	})
}

func update(path string, f func([]string) []string) error {
	if path == "" {
		return nil
	}
	keys := f(Load(path))
	if len(keys) > Limit {
		keys = keys[:Limit]
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(keys, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
