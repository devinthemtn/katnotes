// Package config loads and saves KatNotes options from
// os.UserConfigDir()/katnotes/config.toml.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	// OpenWith is the GUI's "open in external app" command; empty means the
	// system default (xdg-open / open / start).
	OpenWith string `toml:"open_with" json:"open_with"`
	// StartPage is what the GUI opens on launch: "none" (welcome screen),
	// "last" (last opened note), "today" (today's journal entry), or a note
	// as "page/<id>" or "journal/<id>".
	StartPage string `toml:"start_page" json:"start_page"`
	// FrontmatterCollapsed starts the GUI's frontmatter table folded.
	FrontmatterCollapsed bool `toml:"frontmatter_collapsed" json:"frontmatter_collapsed"`
	// Font sets the GUI's fonts and base text size.
	Font Font `toml:"font" json:"font"`
}

// Font holds the GUI font options ([font] in config.toml). Empty families use
// the built-in system stacks.
type Font struct {
	// Body is the interface and preview font family, e.g. "Inter Variable".
	Body string `toml:"body" json:"body"`
	// Mono is the editor and code font family, e.g. "JetBrains Mono".
	Mono string `toml:"mono" json:"mono"`
	// Size is the base text size in pixels.
	Size int `toml:"size" json:"size"`
}

// Font size limits accepted by Set and Load.
const (
	MinFontSize     = 8
	MaxFontSize     = 40
	DefaultFontSize = 14
)

// Default returns the built-in defaults.
func Default() Config {
	return Config{
		NotesDir:        "~/KatNotes",
		Editor:          "",
		DefaultKind:     "page",
		JournalTemplate: "# {{.Date}}\n\n",
		DateFormat:      "2006-01-02",
		StartPage:       "none",
		Font:            Font{Size: DefaultFontSize},
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
	if cfg.Font.Size < MinFontSize || cfg.Font.Size > MaxFontSize {
		cfg.Font.Size = def.Font.Size
	}
	if cfg.StartPage == "" {
		cfg.StartPage = def.StartPage
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
var Keys = []string{"notes_dir", "editor", "default_kind", "journal_template", "date_format", "open_with", "start_page", "frontmatter_collapsed", "font.body", "font.mono", "font.size"}

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
	case "open_with":
		cfg.OpenWith = value
	case "frontmatter_collapsed":
		b, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("frontmatter_collapsed must be true or false, got %q", value)
		}
		cfg.FrontmatterCollapsed = b
	case "font.body":
		cfg.Font.Body = strings.TrimSpace(value)
	case "font.mono":
		cfg.Font.Mono = strings.TrimSpace(value)
	case "font.size":
		n, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "px")))
		if err != nil || n < MinFontSize || n > MaxFontSize {
			return fmt.Errorf("font.size must be a number of pixels from %d to %d, got %q", MinFontSize, MaxFontSize, value)
		}
		cfg.Font.Size = n
	case "start_page":
		v, err := normStartPage(value)
		if err != nil {
			return err
		}
		cfg.StartPage = v
	default:
		return fmt.Errorf("unknown option %q (want one of %s)", key, strings.Join(Keys, ", "))
	}
	return Save(cfg)
}

// normStartPage validates a start_page value.
func normStartPage(v string) (string, error) {
	v = strings.TrimSpace(v)
	switch strings.ToLower(v) {
	case "", "none", "welcome":
		return "none", nil
	case "last", "current":
		return "last", nil
	case "today", "journal":
		return "today", nil
	}
	kind, id, ok := strings.Cut(v, "/")
	if ok && id != "" && (kind == "page" || kind == "journal") {
		return v, nil
	}
	return "", fmt.Errorf("start_page must be none, last, today, page/<id> or journal/<id>, got %q", v)
}
