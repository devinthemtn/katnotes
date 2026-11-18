package tui

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devinthemtn/katnotes/internal/cli"
	"github.com/devinthemtn/katnotes/internal/notes"
)

// editFM simulates the editor: writes text to a temp file that looks modified
// (unless text is "") and hands it to afterFrontmatterEdit.
func editFM(t *testing.T, m *model, n *notes.Note, text string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fm.yaml")
	if err := os.WriteFile(path, []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := time.Now().Add(-time.Hour)
	os.Chtimes(path, before, before)
	if text != "" {
		os.WriteFile(path, []byte(text), 0o644)
	}
	m.afterFrontmatterEdit(fmEditedMsg{n: n, path: path, before: before})
}

func TestFrontmatterCommandOnImportedNote(t *testing.T) {
	st := notes.New(t.TempDir())
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(st.Path(notes.KindPage, "imported"), []byte("# Imported\n\ntext\n"), 0o644)
	m := newModel(&cli.App{Store: st, Out: io.Discard}, "dark")
	n, err := st.Get(notes.KindPage, "imported")
	if err != nil {
		t.Fatal(err)
	}

	tmpl, err := frontmatterTemplate(st, n)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tmpl, "no frontmatter yet") || !strings.Contains(tmpl, "title: Imported") {
		t.Fatalf("template:\n%s", tmpl)
	}

	// Broken YAML: nothing saved, the text is kept for the next try.
	editFM(t, &m, n, "title: [oops\n")
	if !m.isErr || m.fmDrafts["page/imported"] != "title: [oops\n" {
		t.Fatalf("status=%q drafts=%v", m.status, m.fmDrafts)
	}
	if has, _ := st.HasFrontmatter(notes.KindPage, "imported"); has {
		t.Fatal("broken YAML was written")
	}

	// Quitting the retried draft without saving discards it.
	editFM(t, &m, n, "")
	if _, ok := m.fmDrafts["page/imported"]; ok {
		t.Fatal("draft not discarded")
	}

	editFM(t, &m, n, strings.Replace(tmpl, "tags: []", "tags: [imported]", 1))
	if m.isErr {
		t.Fatalf("apply failed: %s", m.status)
	}
	got, _ := st.Get(notes.KindPage, "imported")
	if has, _ := st.HasFrontmatter(notes.KindPage, "imported"); !has || !got.HasTag("imported") {
		t.Fatalf("has=%v tags=%v", has, got.Tags)
	}

	// Emptying the file is a cancel, not "clear everything".
	editFM(t, &m, n, "# just comments\n")
	if got2, _ := st.Get(notes.KindPage, "imported"); !got2.HasTag("imported") {
		t.Fatal("emptied file cleared the frontmatter")
	}
}
