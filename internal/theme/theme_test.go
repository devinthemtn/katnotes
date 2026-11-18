package theme

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBuiltinsAreComplete(t *testing.T) {
	for _, dark := range []bool{false, true} {
		th := Builtin(dark)
		for _, k := range Keys {
			if !hexColor.MatchString(th.Color(k.Name)) {
				t.Errorf("%s: %s = %q", th.Name, k.Name, th.Color(k.Name))
			}
		}
		if len(th.Colors) != len(Keys) {
			t.Errorf("%s has %d colours, want %d", th.Name, len(th.Colors), len(Keys))
		}
	}
}

func TestWriteDefaultsRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), DirName)
	if err := WriteDefaults(dir); err != nil {
		t.Fatal(err)
	}
	for _, dark := range []bool{false, true} {
		want := Builtin(dark)
		got, err := Load(dir, want.Name)
		if err != nil {
			t.Fatal(err)
		}
		if got.Dark != dark || !reflect.DeepEqual(got.Colors, want.Colors) || got.Path == "" {
			t.Errorf("%s: %+v", want.Name, got)
		}
	}
	// A deleted default stays deleted (the folder exists now)...
	os.Remove(filepath.Join(dir, "dark.toml"))
	if err := WriteDefaults(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dark.toml")); err == nil {
		t.Error("dark.toml came back")
	}
	// ...and the built-in still works.
	if th, err := Load(dir, Dark); err != nil || !th.Dark || th.Path != "" {
		t.Errorf("built-in dark: %+v %v", th, err)
	}
}

func TestParsePartialAndErrors(t *testing.T) {
	th, err := Parse("ocean", []byte("dark = true\n[colors]\naccent = \"#0AF\"\naccent-soft = \"#102030\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if th.Color("accent") != "#00aaff" || th.Color("accent_soft") != "#102030" || th.Color("bg") != Builtin(true).Color("bg") {
		t.Errorf("partial theme: %+v", th.Colors)
	}

	th, err = Parse("oops", []byte("[colors]\naccent = \"purple\"\nbackground = \"#000000\"\nmuted = \"#111111\"\n"))
	if err == nil || !strings.Contains(err.Error(), `accent = "purple"`) || !strings.Contains(err.Error(), `unknown colour "background"`) {
		t.Fatalf("errors: %v", err)
	}
	if th.Color("accent") != Builtin(false).Color("accent") || th.Color("muted") != "#111111" {
		t.Errorf("bad colours should keep the built-in, good ones apply: %+v", th.Colors)
	}
}

func TestResolveAndList(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "ocean.toml"), []byte("dark = true\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), nil, 0o644)
	if got := List(dir); !reflect.DeepEqual(got, []string{"dark", "light", "ocean"}) {
		t.Errorf("List = %v", got)
	}
	for option, want := range map[string]string{"auto": "dark", "": "dark", "ocean": "ocean", "light": "light"} {
		if th, err := Resolve(dir, option, true); err != nil || th.Name != want {
			t.Errorf("Resolve(%q) = %s, %v", option, th.Name, err)
		}
	}
	if th, _ := Resolve(dir, "auto", false); th.Name != "light" {
		t.Errorf("auto on a light system = %s", th.Name)
	}
	if _, err := Resolve(dir, "missing", false); err == nil {
		t.Error("missing theme accepted")
	}
	if _, err := Load(dir, "../etc/passwd"); err == nil {
		t.Error("path in a theme name accepted")
	}
}
