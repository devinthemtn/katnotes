package commands

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/devinthemtn/katnotes/internal/config"
)

// HistoryFile is the shared command history, one line per command, oldest
// first, in the KatNotes config directory. The TUI and GUI both use it.
const HistoryFile = "command_history"

// HistoryLimit is how many distinct commands are kept.
const HistoryLimit = 200

// HistoryPath returns the history file's location.
func HistoryPath() (string, error) {
	d, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, HistoryFile), nil
}

// LoadHistory reads the history, newest first. A missing file is empty.
func LoadHistory(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{}
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	out := make([]string, 0, len(lines))
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// AddHistory records entry as the newest command (moving it up if it was
// already there), trims to HistoryLimit, and returns the history newest
// first.
func AddHistory(path, entry string) ([]string, error) {
	entry = strings.Join(strings.Fields(entry), " ")
	h := LoadHistory(path)
	if entry == "" {
		return h, nil
	}
	out := []string{entry}
	for _, e := range h {
		if e != entry && len(out) < HistoryLimit {
			out = append(out, e)
		}
	}
	var b strings.Builder
	for i := len(out) - 1; i >= 0; i-- {
		b.WriteString(out[i] + "\n")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return out, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return out, err
	}
	return out, os.Rename(tmp, path)
}
