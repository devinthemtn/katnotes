package gitsync

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func setupEnv(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	for k, v := range map[string]string{
		"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com",
		"GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com",
		"GIT_CONFIG_GLOBAL": os.DevNull, "GIT_CONFIG_NOSYSTEM": "1",
	} {
		t.Setenv(k, v)
	}
}

func TestNotRepo(t *testing.T) {
	setupEnv(t)
	if _, err := Open(t.TempDir()); !errors.Is(err, ErrNotRepo) {
		t.Fatalf("Open = %v", err)
	}
}

func TestSyncLocalAndRemote(t *testing.T) {
	setupEnv(t)
	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	run(t, base, "init", "-q", "--bare", "-b", "main", remote)

	// The notes folder is a subfolder of the repo; files outside it are left alone.
	a := filepath.Join(base, "a")
	run(t, base, "clone", "-q", remote, a)
	run(t, a, "checkout", "-q", "-b", "main")
	notes := filepath.Join(a, "notes")
	os.MkdirAll(filepath.Join(notes, "pages"), 0o755)
	os.WriteFile(filepath.Join(notes, "pages", "x.md"), []byte("x\n"), 0o644)
	os.WriteFile(filepath.Join(a, "outside.txt"), []byte("o\n"), 0o644)

	r, err := Open(notes)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := r.Status()
	if err != nil || len(cs) != 1 || cs[0].Path != "pages/x.md" || cs[0].Code != "??" {
		t.Fatalf("status = %+v, %v", cs, err)
	}
	now := time.Date(2026, 11, 17, 14, 5, 0, 0, time.Local)

	// No upstream yet: commit only.
	s, err := r.Sync(now)
	if err != nil || s.Committed != 1 || s.Upstream != "" {
		t.Fatalf("local sync = %+v, %v", s, err)
	}
	if log := run(t, a, "log", "--format=%s"); !strings.Contains(log, "katnotes sync 2026-11-17 14:05") {
		t.Fatalf("log: %s", log)
	}
	if st := run(t, a, "status", "--porcelain"); !strings.Contains(st, "outside.txt") {
		t.Fatalf("outside file was committed: %q", st)
	}
	run(t, a, "push", "-q", "-u", "origin", "main")

	// A second clone makes a change; syncing a pulls it and pushes its own.
	b := filepath.Join(base, "b")
	run(t, base, "clone", "-q", remote, b)
	os.WriteFile(filepath.Join(b, "notes", "pages", "y.md"), []byte("y\n"), 0o644)
	run(t, b, "add", ".")
	run(t, b, "commit", "-q", "-m", "from b")
	run(t, b, "push", "-q")

	os.WriteFile(filepath.Join(notes, "pages", "x.md"), []byte("x2\n"), 0o644)
	s, err = r.Sync(now)
	if err != nil || s.Committed != 1 || !s.Pulled || !s.Pushed || s.Upstream != "origin/main" {
		t.Fatalf("remote sync = %+v, %v", s, err)
	}
	if _, err := os.Stat(filepath.Join(notes, "pages", "y.md")); err != nil {
		t.Fatal("pulled file missing")
	}
	if !strings.Contains(s.String(), "pulled changes from origin/main") {
		t.Errorf("summary: %s", s)
	}

	// Nothing new: no commit, nothing pushed.
	s, err = r.Sync(now)
	if err != nil || s.Committed != 0 || s.Pushed || s.Pulled {
		t.Fatalf("idle sync = %+v, %v", s, err)
	}
}

func TestStatusOddNamesAndRenames(t *testing.T) {
	setupEnv(t)
	dir := t.TempDir()
	run(t, dir, "init", "-q", "-b", "main", dir)
	os.MkdirAll(filepath.Join(dir, "pages"), 0o755)
	os.WriteFile(filepath.Join(dir, "pages", "old.md"), []byte("o\n"), 0o644)
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "first")
	run(t, dir, "mv", "pages/old.md", "pages/new.md")
	os.WriteFile(filepath.Join(dir, "pages", `a "b" c.md`), []byte("x\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "pages", "café.md"), []byte("x\n"), 0o644)

	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range cs {
		got[c.Path] = c.Code
	}
	want := map[string]string{"pages/new.md": "R ", `pages/a "b" c.md`: "??", "pages/café.md": "??"}
	if len(got) != len(want) {
		t.Fatalf("status = %+v", cs)
	}
	for p, code := range want {
		if got[p] != code {
			t.Errorf("%s: code %q, want %q (all: %+v)", p, got[p], code, cs)
		}
	}
	// No upstream: Sync commits and stops without an error.
	if s, err := r.Sync(time.Now()); err != nil || s.Upstream != "" || s.Committed != 3 {
		t.Errorf("sync = %+v, %v", s, err)
	}
}
