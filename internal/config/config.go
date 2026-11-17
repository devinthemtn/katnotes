// Package config loads and saves KatNotes options from
// os.UserConfigDir()/katnotes/config.toml.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// AppName is the directory name used under the user's config dir.
const AppName = "katnotes"

// EnvNotesDir overrides NotesDir when set.
const EnvNotesDir = "KATNOTES_DIR"

// EnvConfigDir overrides the config directory when set (mainly for tests).
const EnvConfigDir = "KATNOTES_CONFIG_DIR"

// Config holds user options.
type Config struct {
	NotesDir        string `toml:"notes_dir" json:"notes_dir"`
	Editor          string `toml:"editor" json:"editor"`
	DefaultKind     string `toml:"default_kind" json:"default_kind"`
	JournalTemplate string `toml:"journal_template" json:"journal_template"`
	DateFormat      string `toml:"date_format" json:"date_format"`
}

// Default returns the built-in defaults.
func Default() Config {
	return Config{
		NotesDir:        "~/KatNotes",
		Editor:          "",
		DefaultKind:     "page",
		JournalTemplate: "# {{.Date}}\n\n",
		DateFormat:      "2006-01-02",
	}
}

// Dir returns the katnotes config directory.
func Dir() (string, error) {
	if d := os.Getenv(EnvConfigDir); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, AppName), nil
}

// Path returns the full path of config.toml.
func Path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.toml"), nil
}

// Load reads the config file, filling missing values with defaults. A missing
// file is not an error. NotesDir is returned with ~ expanded and the
// KATNOTES_DIR override applied.
func Load() (Config, error) {
	cfg := Default()
	p, err := Path()
	if err != nil {
		return cfg, err
	}
	if _, err := toml.DecodeFile(p, &cfg); err != nil && !errors.Is(err, os.ErrNotExist) {
		return cfg, fmt.Errorf("reading %s: %w", p, err)
	}
	def := Default()
	if cfg.NotesDir == "" {
		cfg.NotesDir = def.NotesDir
	}
	if cfg.DefaultKind == "" {
		cfg.DefaultKind = def.DefaultKind
	}
	if cfg.DateFormat == "" {
		cfg.DateFormat = def.DateFormat
	}
	if cfg.JournalTemplate == "" {
		cfg.JournalTemplate = def.JournalTemplate
	}
	if env := os.Getenv(EnvNotesDir); env != "" {
		cfg.NotesDir = env
	}
	cfg.NotesDir = ExpandHome(cfg.NotesDir)
	return cfg, nil
}

// Save writes cfg to the config file, creating the directory if needed.
func Save(cfg Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return err
	}
	return os.WriteFile(p, buf.Bytes(), 0o644)
}

// Install writes the default config if no config file exists yet.
func Install() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); err == nil {
		return nil
	}
	return Save(Default())
}

// ExpandHome replaces a leading ~ with the user's home directory.
func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// Keys lists the option names accepted by Set.
var Keys = []string{"notes_dir", "editor", "default_kind", "journal_template", "date_format"}

// Set updates one option in the config file. It edits the file's own values,
// not the env-overridden view returned by Load.
func Set(key, value string) error {
	p, err := Path()
	if err != nil {
		return err
	}
	cfg := Default()
	if _, err := toml.DecodeFile(p, &cfg); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	switch key {
	case "notes_dir":
		cfg.NotesDir = value
	case "editor":
		cfg.Editor = value
	case "default_kind":
		switch strings.ToLower(value) {
		case "page", "pages", "p":
			cfg.DefaultKind = "page"
		case "journal", "j":
			cfg.DefaultKind = "journal"
		default:
			return fmt.Errorf("default_kind must be page or journal, got %q", value)
		}
	case "journal_template":
		cfg.JournalTemplate = strings.ReplaceAll(value, `\n`, "\n")
	case "date_format":
		cfg.DateFormat = value
	default:
		return fmt.Errorf("unknown option %q (want one of %s)", key, strings.Join(Keys, ", "))
	}
	return Save(cfg)
}
