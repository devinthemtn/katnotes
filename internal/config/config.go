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

	"github.com/devinthemtn/katnotes/internal/notes"
)

// AppName is the directory name used under the user's config dir.
const AppName = "katnotes"

// EnvNotesDir overrides NotesDir when set.
const EnvNotesDir = "KATNOTES_DIR"

// EnvConfigDir overrides the config directory when set (mainly for tests).
const EnvConfigDir = "KATNOTES_CONFIG_DIR"

// Config holds user options.
type Config struct {
	NotesDir string `toml:"notes_dir" json:"notes_dir"`
	Editor   string `toml:"editor" json:"editor"`
	// Previewer is the Markdown viewer the TUI's "p" (or "gx") key opens the selected
	// note in (e.g. "katdown", "glow -p"). Empty tries katdown, glow,
	// frogmouth, then mdcat.
	Previewer string `toml:"previewer" json:"previewer"`
	// PreviewerMode is "auto" (terminal viewers like glow take over the
	// terminal; anything else opens in its own window), "terminal" or "window".
	PreviewerMode   string `toml:"previewer_mode" json:"previewer_mode"`
	DefaultKind     string `toml:"default_kind" json:"default_kind"`
	JournalTemplate string `toml:"journal_template" json:"journal_template"`
	DateFormat      string `toml:"date_format" json:"date_format"`
	// OpenWith is the program ":external" (GUI: Ctrl+Shift+O) opens a note
	// in, e.g. "emacsclient -c -a ''"; empty means the system default app
	// (xdg-open / open / start).
	OpenWith string `toml:"open_with" json:"open_with"`
	// TUIOpenWith overrides OpenWith in the TUI, e.g. "emacsclient -nw";
	// empty uses OpenWith.
	TUIOpenWith string `toml:"tui_open_with" json:"tui_open_with"`
	// StartPage is what the GUI opens on launch: "none" (welcome screen),
	// "last" (last opened note), "today" (today's journal entry), or a note
	// as "page/<id>" or "journal/<id>".
	StartPage string `toml:"start_page" json:"start_page"`
	// FrontmatterCollapsed starts the GUI's frontmatter table folded.
	FrontmatterCollapsed bool `toml:"frontmatter_collapsed" json:"frontmatter_collapsed"`
	// TaskDoneDate stamps done:<date> into a task's {…} metadata when it is
	// checked from KatNotes, and removes it when it is unchecked.
	TaskDoneDate bool `toml:"task_done_date" json:"task_done_date"`
	// TUIMouse lets the TUI take the mouse: click tabs and rows, scroll with
	// the wheel. Off leaves the terminal's own text selection alone. TUI
	// only: the GUI always has the mouse.
	TUIMouse bool `toml:"tui_mouse" json:"tui_mouse"`
	// Theme is the colour theme for both apps: a file name (without .toml)
	// in the themes folder, or "auto" for the light / dark theme following
	// the system (GUI) or the terminal background (TUI).
	Theme string `toml:"theme" json:"theme"`
	// TUITheme and GUITheme override Theme for one app; empty uses Theme.
	TUITheme string `toml:"tui_theme" json:"tui_theme"`
	GUITheme string `toml:"gui_theme" json:"gui_theme"`
	// Font sets the GUI's fonts and base text size.
	Font Font `toml:"font" json:"font"`
	// TodoTxt links tasks marked {kt:new} to a todo.txt file, e.g. the one
	// katTasks uses ([todo_txt] in config.toml).
	TodoTxt TodoTxt `toml:"todo_txt" json:"todo_txt"`
}

// TodoTxt holds the todo.txt link options.
type TodoTxt struct {
	Enabled bool `toml:"enabled" json:"enabled"`
	// File is the todo.txt file. Empty uses files.todo from katTasks'
	// config.toml, else ~/todo.txt.
	File string `toml:"file" json:"file"`
	// DoneFile is where completed tasks are archived. Empty uses files.done
	// from katTasks' config, else done.txt next to File.
	DoneFile string `toml:"done_file" json:"done_file"`
}

// TodoFiles returns the todo.txt and done.txt paths, with ~ and $VARS
// expanded, filling empty ones from katTasks' own settings.
func (t TodoTxt) TodoFiles() (todo, done string) {
	todo, done = expandPath(t.File), expandPath(t.DoneFile)
	if todo == "" {
		kt := katTasksFiles()
		todo = kt.Todo
		if done == "" {
			done = kt.Done
		}
	}
	if todo == "" {
		todo = ExpandHome("~/todo.txt")
	}
	if done == "" {
		done = filepath.Join(filepath.Dir(todo), "done.txt")
	}
	return todo, done
}

// katTasksFiles reads [files] from katTasks' config.toml
// (os.UserConfigDir()/katTasks/config.toml). Missing values are empty.
func katTasksFiles() (f struct{ Todo, Done string }) {
	base, err := os.UserConfigDir()
	if err != nil {
		return f
	}
	var kt struct {
		Files struct {
			Todo string `toml:"todo"`
			Done string `toml:"done"`
		} `toml:"files"`
	}
	if _, err := toml.DecodeFile(filepath.Join(base, "katTasks", "config.toml"), &kt); err != nil {
		return f
	}
	f.Todo, f.Done = expandPath(kt.Files.Todo), expandPath(kt.Files.Done)
	return f
}

func expandPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	return ExpandHome(os.ExpandEnv(p))
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
		PreviewerMode:   "auto",
		DefaultKind:     "page",
		JournalTemplate: "# {{.Date}}\n\n",
		DateFormat:      "2006-01-02",
		StartPage:       "none",
		TaskDoneDate:    true,
		TUIMouse:        true,
		Theme:           "auto",
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

// File returns the path of name (a file or folder) in the config directory.
func File(name string) (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, name), nil
}

// Path returns the full path of config.toml.
func Path() (string, error) { return File("config.toml") }

// EditTarget returns what ":config" opens in an editor: config.toml, or the
// config directory when dir is set. The directory is created if missing so
// the editor can save a new config.toml.
func EditTarget(dir bool) (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	if dir {
		return d, nil
	}
	return Path()
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
	if cfg.PreviewerMode == "" {
		cfg.PreviewerMode = def.PreviewerMode
	}
	if cfg.StartPage == "" {
		cfg.StartPage = def.StartPage
	}
	if cfg.JournalTemplate == "" {
		cfg.JournalTemplate = def.JournalTemplate
	}
	if cfg.Theme = strings.TrimSpace(cfg.Theme); cfg.Theme == "" {
		cfg.Theme = def.Theme
	}
	cfg.TUITheme = strings.TrimSpace(cfg.TUITheme)
	cfg.GUITheme = strings.TrimSpace(cfg.GUITheme)
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

// OpenWithFor is the ":external" program for app ("tui" or "gui"): the
// TUI's own tui_open_with, else open_with. "" means the system default.
func (c Config) OpenWithFor(app string) string {
	if app == "tui" && strings.TrimSpace(c.TUIOpenWith) != "" {
		return c.TUIOpenWith
	}
	return c.OpenWith
}

// ThemeKey is the option that holds app's theme: "tui_theme", "gui_theme",
// or "theme" for any other app.
func ThemeKey(app string) string {
	switch app {
	case "tui", "gui":
		return app + "_theme"
	}
	return "theme"
}

// ThemeFor is the theme option in effect for app ("tui" or "gui"): its own
// override, else Theme.
func (c Config) ThemeFor(app string) string {
	switch {
	case app == "tui" && c.TUITheme != "":
		return c.TUITheme
	case app == "gui" && c.GUITheme != "":
		return c.GUITheme
	}
	return c.Theme
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
var Keys = []string{"notes_dir", "editor", "previewer", "previewer_mode", "default_kind", "journal_template", "date_format", "open_with", "tui_open_with", "start_page", "frontmatter_collapsed", "task_done_date", "tui_mouse", "theme", "tui_theme", "gui_theme", "font.body", "font.mono", "font.size", "todo_txt.enabled", "todo_txt.file", "todo_txt.done_file"}

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
	case "previewer":
		cfg.Previewer = strings.TrimSpace(value)
	case "previewer_mode":
		switch m := strings.ToLower(strings.TrimSpace(value)); m {
		case "auto", "terminal", "window":
			cfg.PreviewerMode = m
		default:
			return fmt.Errorf("previewer_mode must be auto, terminal or window, got %q", value)
		}
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
		value = strings.TrimSpace(value)
		if _, err := notes.JournalLayout(value); err != nil {
			return err
		}
		cfg.DateFormat = value
	case "open_with":
		cfg.OpenWith = strings.TrimSpace(value)
	case "tui_open_with":
		cfg.TUIOpenWith = strings.TrimSpace(value)
	case "frontmatter_collapsed":
		b, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("frontmatter_collapsed must be true or false, got %q", value)
		}
		cfg.FrontmatterCollapsed = b
	case "tui_mouse":
		b, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("tui_mouse must be true or false, got %q", value)
		}
		cfg.TUIMouse = b
	case "task_done_date":
		b, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("task_done_date must be true or false, got %q", value)
		}
		cfg.TaskDoneDate = b
	case "theme", "tui_theme", "gui_theme":
		v := strings.TrimSuffix(strings.TrimSpace(value), ".toml")
		if strings.ContainsAny(v, `/\ `) || strings.HasPrefix(v, ".") {
			return fmt.Errorf("%s must be auto or a theme file name (without .toml), got %q", key, value)
		}
		switch key {
		case "theme":
			if v == "" {
				v = "auto"
			}
			cfg.Theme = v
		case "tui_theme":
			cfg.TUITheme = v // "" follows theme
		case "gui_theme":
			cfg.GUITheme = v
		}
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
	case "todo_txt.enabled":
		b, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("todo_txt.enabled must be true or false, got %q", value)
		}
		cfg.TodoTxt.Enabled = b
	case "todo_txt.file":
		cfg.TodoTxt.File = strings.TrimSpace(value)
	case "todo_txt.done_file":
		cfg.TodoTxt.DoneFile = strings.TrimSpace(value)
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

// WriteFile replaces the file at path with data in one step (a temporary
// file in the same folder, then a rename), creating the folder if needed, so
// the TUI and GUI never read a half-written state file.
func WriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
