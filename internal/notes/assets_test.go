package notes

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// pngBytes is a minimal PNG header, enough for content sniffing.
var pngBytes = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")

func TestSaveAssetNaming(t *testing.T) {
	s := newStore(t)
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"My Screenshot.PNG", pngBytes, "assets/my-screenshot.png"},
		{"My Screenshot.png", append([]byte{}, append(pngBytes, 1)...), "assets/my-screenshot-2.png"},
		{"photo.jpeg", []byte("jpegdata"), "assets/photo.jpg"},
		{"", append([]byte{}, append(pngBytes, 2)...), "assets/pasted-20261116-093000.png"},
		{"../../etc/evil.gif", []byte("GIF89a"), "assets/evil.gif"},
		{`C:\Users\me\diagram.svg`, []byte("<svg/>"), "assets/diagram.svg"},
	}
	for _, c := range cases {
		got, err := s.SaveAsset(c.name, c.data)
		if err != nil {
			t.Fatalf("SaveAsset(%q): %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("SaveAsset(%q) = %q, want %q", c.name, got, c.want)
		}
		b, err := os.ReadFile(filepath.Join(s.Root, got))
		if err != nil || string(b) != string(c.data) {
			t.Errorf("%s: content %q, %v", got, b, err)
		}
	}
	if _, err := os.Stat(filepath.Join(s.Root, "..", "etc", "evil.gif")); err == nil {
		t.Error("asset written outside the notes folder")
	}
	left, _ := filepath.Glob(filepath.Join(s.AssetsPath(), ".katnotes-*"))
	if len(left) != 0 {
		t.Errorf("temp files left behind: %v", left)
	}
}

func TestSaveAssetReusesSameBytes(t *testing.T) {
	s := newStore(t)
	a, err := s.SaveAsset("a.png", pngBytes)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.SaveAsset("b.png", pngBytes)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("same bytes saved twice: %q and %q", a, b)
	}
	files, _ := s.Assets()
	if len(files) != 1 {
		t.Errorf("assets = %v, want one file", files)
	}
}

func TestSaveAssetRejects(t *testing.T) {
	s := newStore(t)
	for name, data := range map[string][]byte{
		"script.sh": []byte("#!/bin/sh"),
		"empty.png": nil,
		"noext":     []byte("plain text"),
		"huge.png":  make([]byte, MaxAssetSize+1),
		"page.html": []byte("<html>"),
	} {
		if _, err := s.SaveAsset(name, data); !errors.Is(err, ErrAsset) {
			t.Errorf("SaveAsset(%q) err = %v, want ErrAsset", name, err)
		}
	}
}

func TestAssetPath(t *testing.T) {
	s := newStore(t)
	if p, ok := s.AssetPath("x.png"); !ok || p != filepath.Join(s.Root, "assets", "x.png") {
		t.Errorf("AssetPath(x.png) = %q, %v", p, ok)
	}
	for _, bad := range []string{"", "..", "../x.png", "a/b.png", `a\b.png`, ".hidden"} {
		if _, ok := s.AssetPath(bad); ok {
			t.Errorf("AssetPath(%q) accepted", bad)
		}
	}
}

func TestImageMarkdown(t *testing.T) {
	if got := ImageMarkdown("a [b]", "assets/x.png"); got != `![a \[b\]](../assets/x.png)` {
		t.Errorf("got %q", got)
	}
}

func TestOrphanAssets(t *testing.T) {
	s := newStore(t)
	if o, err := s.OrphanAssets(); err != nil || o != nil {
		t.Fatalf("no assets folder: %v, %v", o, err)
	}
	used, _ := s.SaveAsset("used.png", pngBytes)
	if _, err := s.SaveAsset("unused.gif", []byte("GIF89a")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(KindPage, "Pics", nil, "# Pics\n\n"+ImageMarkdown("", used)+"\n"); err != nil {
		t.Fatal(err)
	}
	got, err := s.OrphanAssets()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"unused.gif"}) {
		t.Errorf("orphans = %v", got)
	}
	// Notes never list asset files.
	ns, _ := s.List("", ListOpts{})
	for _, n := range ns {
		if strings.Contains(n.Path, AssetsDir) {
			t.Errorf("asset listed as a note: %s", n.Path)
		}
	}
}
