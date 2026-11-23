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

func TestSetOpenWith(t *testing.T) {
	t.Setenv(EnvConfigDir, t.TempDir())
	t.Setenv(EnvNotesDir, "")

	if err := Set("open_with", "gedit --new-window"); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.OpenWith != "gedit --new-window" {
		t.Errorf("OpenWith = %q", got.OpenWith)
	}
}

func TestSetStartPage(t *testing.T) {
	t.Setenv(EnvConfigDir, t.TempDir())
	t.Setenv(EnvNotesDir, "")

	if cfg, _ := Load(); cfg.StartPage != "none" {
		t.Errorf("default StartPage = %q", cfg.StartPage)
	}
	for in, want := range map[string]string{"today": "today", "current": "last", "": "none", "page/ideas": "page/ideas"} {
		if err := Set("start_page", in); err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if got, _ := Load(); got.StartPage != want {
			t.Errorf("%q: StartPage = %q, want %q", in, got.StartPage, want)
		}
	}
	for _, bad := range []string{"tomorrow", "page/", "notes/x"} {
		if err := Set("start_page", bad); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
}

func TestFontOptions(t *testing.T) {
	t.Setenv(EnvConfigDir, t.TempDir())
	t.Setenv(EnvNotesDir, "")

	if cfg, _ := Load(); cfg.Font.Size != DefaultFontSize || cfg.Font.Body != "" {
		t.Errorf("default font = %+v", cfg.Font)
	}
	for k, v := range map[string]string{"font.body": "Inter Variable", "font.mono": "Lilex Nerd Font Mono", "font.size": "16px"} {
		if err := Set(k, v); err != nil {
			t.Fatalf("%s: %v", k, err)
		}
	}
	got, _ := Load()
	if got.Font != (Font{Body: "Inter Variable", Mono: "Lilex Nerd Font Mono", Size: 16}) {
		t.Errorf("font = %+v", got.Font)
	}
	for _, bad := range []string{"big", "2", "99"} {
		if err := Set("font.size", bad); err == nil {
			t.Errorf("font.size %q: want error", bad)
		}
	}

	// The file uses a [font] table like other apps' configs.
	p, _ := Path()
	if err := os.WriteFile(p, []byte("[font]\nbody = \"Inter\"\nsize = 200\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ = Load()
	if got.Font.Body != "Inter" || got.Font.Size != DefaultFontSize {
		t.Errorf("hand-written font = %+v", got.Font)
	}
}

func TestFrontmatterCollapsed(t *testing.T) {
	t.Setenv(EnvConfigDir, t.TempDir())
	t.Setenv(EnvNotesDir, "")

	if cfg, _ := Load(); cfg.FrontmatterCollapsed {
		t.Error("default should be expanded")
	}
	if err := Set("frontmatter_collapsed", "true"); err != nil {
		t.Fatal(err)
	}
	if cfg, _ := Load(); !cfg.FrontmatterCollapsed {
		t.Error("not collapsed after Set")
	}
	if err := Set("frontmatter_collapsed", "maybe"); err == nil {
		t.Error("want error for non-bool")
	}
}

func TestTUIMouse(t *testing.T) {
	t.Setenv(EnvConfigDir, t.TempDir())
	t.Setenv(EnvNotesDir, "")

	if cfg, _ := Load(); !cfg.TUIMouse {
		t.Error("the mouse should be on by default")
	}
	if err := Set("tui_mouse", "false"); err != nil {
		t.Fatal(err)
	}
	if cfg, _ := Load(); cfg.TUIMouse {
		t.Error("still on after Set false")
	}
	if err := Set("tui_mouse", "sometimes"); err == nil {
		t.Error("want error for non-bool")
	}
}

func TestOpenWithFor(t *testing.T) {
	t.Setenv(EnvConfigDir, t.TempDir())
	if err := Set("open_with", "emacsclient -c -a ''"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := Load()
	if cfg.OpenWithFor("tui") != "emacsclient -c -a ''" || cfg.OpenWithFor("gui") != "emacsclient -c -a ''" {
		t.Errorf("no tui_open_with: %+v", cfg)
	}
	if err := Set("tui_open_with", "emacsclient -nw -a ''"); err != nil {
		t.Fatal(err)
	}
	cfg, _ = Load()
	if cfg.OpenWithFor("tui") != "emacsclient -nw -a ''" || cfg.OpenWithFor("gui") != "emacsclient -c -a ''" {
		t.Errorf("with tui_open_with: %+v", cfg)
	}
}

func TestEditTarget(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new")
	t.Setenv(EnvConfigDir, dir)
	p, err := EditTarget(false)
	if err != nil || p != filepath.Join(dir, "config.toml") {
		t.Fatalf("file: %q, %v", p, err)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("config dir not created: %v", err)
	}
	if p, err := EditTarget(true); err != nil || p != dir {
		t.Fatalf("dir: %q, %v", p, err)
	}
}

// Every key in Keys can be set, and bad values are refused without
// touching the file.
func TestSetEveryKey(t *testing.T) {
	t.Setenv(EnvConfigDir, t.TempDir())
	t.Setenv(EnvNotesDir, "")
	good := map[string]struct {
		value string
		check func(c Config) bool
	}{
		"notes_dir":             {"/tmp/n", func(c Config) bool { return c.NotesDir == "/tmp/n" }},
		"editor":                {"nvim -p", func(c Config) bool { return c.Editor == "nvim -p" }},
		"previewer":             {" glow ", func(c Config) bool { return c.Previewer == "glow" }},
		"previewer_mode":        {"Window", func(c Config) bool { return c.PreviewerMode == "window" }},
		"default_kind":          {"j", func(c Config) bool { return c.DefaultKind == "journal" }},
		"journal_template":      {`# {{.Title}}\n`, func(c Config) bool { return c.JournalTemplate == "# {{.Title}}\n" }},
		"date_format":           {"dd/mm/yyyy", func(c Config) bool { return c.DateFormat == "dd/mm/yyyy" }},
		"open_with":             {"code -n", func(c Config) bool { return c.OpenWith == "code -n" }},
		"tui_open_with":         {"hx", func(c Config) bool { return c.TUIOpenWith == "hx" }},
		"start_page":            {"page/home", func(c Config) bool { return c.StartPage == "page/home" }},
		"frontmatter_collapsed": {"true", func(c Config) bool { return c.FrontmatterCollapsed }},
		"task_done_date":        {"false", func(c Config) bool { return !c.TaskDoneDate }},
		"tui_mouse":             {"false", func(c Config) bool { return !c.TUIMouse }},
		"theme":                 {"ocean.toml", func(c Config) bool { return c.Theme == "ocean" }},
		"tui_theme":             {"dark", func(c Config) bool { return c.ThemeFor("tui") == "dark" }},
		"gui_theme":             {"light", func(c Config) bool { return c.ThemeFor("gui") == "light" }},
		"font.body":             {"Inter", func(c Config) bool { return c.Font.Body == "Inter" }},
		"font.mono":             {"Iosevka", func(c Config) bool { return c.Font.Mono == "Iosevka" }},
		"font.size":             {"18px", func(c Config) bool { return c.Font.Size == 18 }},
		"todo_txt.enabled":      {"true", func(c Config) bool { return c.TodoTxt.Enabled }},
		"todo_txt.file":         {"~/t/todo.txt", func(c Config) bool { return c.TodoTxt.File == "~/t/todo.txt" }},
		"todo_txt.done_file":    {"/x/done.txt", func(c Config) bool { return c.TodoTxt.DoneFile == "/x/done.txt" }},
	}
	bad := map[string]string{
		"previewer_mode": "sideways", "default_kind": "note", "date_format": "21/11/2026",
		"start_page": "elsewhere", "frontmatter_collapsed": "yes please",
		"task_done_date": "2", "tui_mouse": "maybe", "theme": "../x",
		"font.size": "1", "todo_txt.enabled": "on-ish",
	}
	for _, k := range Keys {
		g, ok := good[k]
		if !ok {
			t.Errorf("no test value for %s", k)
			continue
		}
		if v, ok := bad[k]; ok {
			if err := Set(k, v); err == nil {
				t.Errorf("Set(%s, %q) accepted", k, v)
			}
		}
		if err := Set(k, g.value); err != nil {
			t.Errorf("Set(%s, %q): %v", k, g.value, err)
			continue
		}
		c, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if !g.check(c) {
			t.Errorf("after Set(%s, %q): %+v", k, g.value, c)
		}
	}
	if err := Set("colour", "red"); err == nil {
		t.Error("unknown key accepted")
	}
	if ThemeKey("tui") != "tui_theme" || ThemeKey("cli") != "theme" {
		t.Error("ThemeKey")
	}
}

func TestWriteFileReplaces(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "state")
	for _, s := range []string{"one", "two"} {
		if err := WriteFile(p, []byte(s)); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := os.ReadFile(p)
	entries, _ := os.ReadDir(filepath.Dir(p))
	if string(got) != "two" || len(entries) != 1 {
		t.Fatalf("got %q, %d file(s)", got, len(entries))
	}
}
