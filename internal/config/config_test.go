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
