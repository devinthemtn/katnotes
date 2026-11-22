package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAttachAndOrphans(t *testing.T) {
	dir, _ := setup(t)
	img := filepath.Join(t.TempDir(), "Cat Photo.png")
	os.WriteFile(img, []byte("\x89PNG\r\n\x1a\nfake"), 0o644)
	other := filepath.Join(t.TempDir(), "spare.gif")
	os.WriteFile(other, []byte("GIF89a"), 0o644)

	if out := mustRun(t, "attach", img); strings.TrimSpace(out) != "![](../assets/cat-photo.png)" {
		t.Errorf("attach = %q", out)
	}
	mustRun(t, "new", "Pets")
	if out := mustRun(t, "attach", "--alt", "cat", img, "pets"); !strings.Contains(out, "assets/cat-photo.png") {
		t.Errorf("attach to note = %q", out)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "pages", "pets.md"))
	if !strings.HasSuffix(string(b), "\n\n![cat](../assets/cat-photo.png)\n") {
		t.Errorf("note body:\n%s", b)
	}
	mustRun(t, "attach", other)
	if out := mustRun(t, "assets", "ls"); out != "cat-photo.png\nspare.gif\n" {
		t.Errorf("assets ls = %q", out)
	}
	if out := mustRun(t, "assets", "orphans"); out != "spare.gif\n" {
		t.Errorf("orphans = %q", out)
	}
	mustRun(t, "assets", "orphans", "--delete")
	if _, err := os.Stat(filepath.Join(dir, "assets", "spare.gif")); !os.IsNotExist(err) {
		t.Errorf("orphan not deleted: %v", err)
	}
	if _, err := run(t, "attach", filepath.Join(t.TempDir(), "x.txt")); err == nil {
		t.Error("attach of a missing file succeeded")
	}
}
