package notes

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func newTodoStore(t *testing.T) (*Store, string) {
	t.Helper()
	s := newStore(t)
	dir := t.TempDir()
	s.Todo = &TodoSync{
		File:      filepath.Join(dir, "todo.txt"),
		DoneFile:  filepath.Join(dir, "done.txt"),
		StatePath: filepath.Join(dir, "state.json"),
	}
	s.OnTodoError = func(err error) { t.Errorf("todo sync: %v", err) }
	return s, dir
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(b)
}

var ktID = regexp.MustCompile(`kt:([0-9a-f]{6})`)

func linkedID(t *testing.T, body string) string {
	t.Helper()
	m := ktID.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no kt: id in %q", body)
	}
	return m[1]
}

func TestTodoLinkAndPush(t *testing.T) {
	s, dir := newTodoStore(t)
	todo := filepath.Join(dir, "todo.txt")
	os.WriteFile(todo, []byte("(B) keep me +home\n\nx 2026-01-01 old one\n"), 0o644)

	n, err := s.Create(KindPage, "House", nil, "# House\n\n- [ ] Call the plumber +fix {due:2026-11-20 pri:high kt:new}\n- [ ] not linked\n")
	if err != nil {
		t.Fatal(err)
	}
	id := linkedID(t, n.Body)
	if strings.Contains(n.Body, "kt:new") {
		t.Fatalf("kt:new not replaced: %q", n.Body)
	}
	got := readFile(t, todo)
	want := "(B) keep me +home\n\nx 2026-01-01 old one\n(A) 2026-11-16 Call the plumber +fix due:2026-11-20 kn:" + id + " note:page/house\n"
	if got != want {
		t.Fatalf("todo.txt =\n%s\nwant\n%s", got, want)
	}
	disk, _ := s.Get(KindPage, n.ID)
	if n.TodoLinks != 1 || disk.TodoLinks != 1 {
		t.Errorf("TodoLinks = %d after save, %d after load", n.TodoLinks, disk.TodoLinks)
	}
	if linkedID(t, disk.Body) != id {
		t.Errorf("ID not saved in the note: %q", disk.Body)
	}

	// katTasks adds a context; the note's text changes. The context stays.
	os.WriteFile(todo, []byte(strings.Replace(got, "note:page/house", "note:page/house @phone", 1)), 0o644)
	n.Body = strings.Replace(n.Body, "Call the plumber", "Call Bob the plumber", 1)
	if err := s.Save(n); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, todo); !strings.Contains(got, "(A) 2026-11-16 Call Bob the plumber +fix @phone due:2026-11-20 kn:"+id+" note:page/house\n") {
		t.Errorf("after edit todo.txt =\n%s", got)
	}

	// Checking pushes x <date>, with the priority kept as pri:A.
	if _, err := s.SetTask(KindPage, n.ID, 2, true); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, todo); !strings.Contains(got, "x 2026-11-16 2026-11-16 Call Bob the plumber +fix @phone due:2026-11-20 pri:A kn:"+id) {
		t.Errorf("after check todo.txt =\n%s", got)
	}
	if !strings.HasPrefix(readFile(t, todo), "(B) keep me +home\n\nx 2026-01-01 old one\n") {
		t.Errorf("other lines changed:\n%s", readFile(t, todo))
	}
}

func TestTodoPullDoneFromKatTasks(t *testing.T) {
	s, dir := newTodoStore(t)
	todo := filepath.Join(dir, "todo.txt")
	n, _ := s.Create(KindPage, "P", nil, "- [ ] write it {kt:new}\n")
	id := linkedID(t, n.Body)

	// Completed in katTasks.
	line := readFile(t, todo)
	os.WriteFile(todo, []byte("x 2026-11-15 "+line), 0o644)

	changes, err := s.SyncTodo(true)
	if err != nil || len(changes) != 1 || changes[0].Action != TodoChecked {
		t.Fatalf("dry run = %+v %v", changes, err)
	}
	if disk, _ := s.Get(KindPage, n.ID); strings.Contains(disk.Body, "[x]") {
		t.Fatal("dry run wrote the note")
	}
	if _, err := s.SyncTodo(false); err != nil {
		t.Fatal(err)
	}
	disk, _ := s.Get(KindPage, n.ID)
	if disk.Body != "- [x] write it {kt:"+id+" done:2026-11-15}\n" {
		t.Errorf("note = %q", disk.Body)
	}
	if changes, _ := s.SyncTodo(false); len(changes) != 0 {
		t.Errorf("second sync = %+v", changes)
	}

	// Saving the note before a sync also takes the change in.
	os.WriteFile(todo, []byte(strings.Replace(readFile(t, todo), "x 2026-11-15 ", "", 1)), 0o644)
	disk.Body += "more\n"
	if err := s.Save(disk); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(disk.Body, "- [ ] write it {kt:"+id+"}\n") {
		t.Errorf("after save = %q", disk.Body)
	}
}

func TestTodoArchivedAndMissing(t *testing.T) {
	s, dir := newTodoStore(t)
	todo, done := filepath.Join(dir, "todo.txt"), filepath.Join(dir, "done.txt")
	n, _ := s.Create(KindPage, "P", nil, "- [ ] a {kt:new}\n- [ ] b {kt:new}\n")
	ids := ktID.FindAllStringSubmatch(n.Body, -1)
	if len(ids) != 2 || ids[0][1] == ids[1][1] {
		t.Fatalf("ids = %v", ids)
	}
	a, b := ids[0][1], ids[1][1]
	lines := strings.SplitAfter(readFile(t, todo), "\n")

	// katTasks completes and archives a; b is deleted by hand.
	os.WriteFile(done, []byte("x 2026-11-16 "+lines[0]), 0o644)
	os.WriteFile(todo, []byte("other task\n"), 0o644)
	changes, err := s.SyncTodo(false)
	if err != nil {
		t.Fatal(err)
	}
	acts := map[string]string{}
	for _, c := range changes {
		acts[c.ID] = c.Action
	}
	if acts[a] != TodoChecked || acts[b] != TodoMissing {
		t.Errorf("changes = %+v", changes)
	}
	if readFile(t, todo) != "other task\n" {
		t.Errorf("todo.txt = %q", readFile(t, todo))
	}

	// Unchecking an archived task puts a fresh open line in todo.txt; push
	// brings back the deleted one.
	if _, err := s.SetTask(KindPage, n.ID, 0, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PushTask(KindPage, n.ID, 1); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, todo)
	if !strings.Contains(got, "2026-11-16 a kn:"+a) || !strings.Contains(got, "2026-11-16 b kn:"+b) || strings.HasPrefix(got, "x") {
		t.Errorf("todo.txt = %q", got)
	}
}

func TestTodoNoteWinsAndOrphans(t *testing.T) {
	s, dir := newTodoStore(t)
	todo := filepath.Join(dir, "todo.txt")
	n, _ := s.Create(KindPage, "P", nil, "- [ ] a {kt:new}\n")
	id := linkedID(t, n.Body)

	// Checked in an outside editor (no push), still open in todo.txt.
	os.WriteFile(n.Path, []byte(strings.Replace(readFile(t, n.Path), "- [ ]", "- [x]", 1)), 0o644)
	os.WriteFile(todo, []byte(readFile(t, todo)+"stray kn:abcdef note:page/gone\n"), 0o644)
	changes, err := s.SyncTodo(false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(readFile(t, todo), "x 2026-11-16 2026-11-16 a kn:"+id) {
		t.Errorf("todo.txt = %q", readFile(t, todo))
	}
	var orphan bool
	for _, c := range changes {
		orphan = orphan || (c.Action == TodoOrphan && c.ID == "abcdef")
	}
	if !orphan || !strings.Contains(readFile(t, todo), "stray") {
		t.Errorf("changes = %+v", changes)
	}
}

func TestTodoHalfTypedLinkIgnored(t *testing.T) {
	s, dir := newTodoStore(t)
	if _, err := s.Create(KindPage, "P", nil, "- [ ] a {kt:ne}\n"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dir, "todo.txt")); got != "" {
		t.Errorf("todo.txt = %q", got)
	}
}

func TestTodoKeepsKatTasksOrder(t *testing.T) {
	s, dir := newTodoStore(t)
	todo := filepath.Join(dir, "todo.txt")
	n, _ := s.Create(KindPage, "P", nil, "- [x] call the plumber {pri:high kt:new done:2026-11-16}\n")
	id := linkedID(t, n.Body)
	// katTasks's own order: pri:A last.
	mine := "x 2026-11-16 2026-11-16 call the plumber kn:" + id + " note:page/p pri:A\n"
	os.WriteFile(todo, []byte(mine), 0o644)
	if changes, _ := s.SyncTodo(false); len(changes) != 0 || readFile(t, todo) != mine {
		t.Errorf("changes = %+v, todo.txt = %q", changes, readFile(t, todo))
	}
	// Reordering the words of the text itself is a real change.
	n, _ = s.Get(KindPage, n.ID)
	n.Body = strings.Replace(n.Body, "call the plumber", "the plumber call", 1)
	s.Save(n)
	if !strings.Contains(readFile(t, todo), " the plumber call ") {
		t.Errorf("todo.txt = %q", readFile(t, todo))
	}
}

func TestKattasksFrontmatter(t *testing.T) {
	s, _ := newTodoStore(t)
	n, err := s.Create(KindPage, "Flags", nil, "- [ ] plain\n")
	if err != nil {
		t.Fatal(err)
	}
	raw := func() string { return readFile(t, s.Path(KindPage, "flags")) }
	if strings.Contains(raw(), TodoLinkKey+":") {
		t.Fatalf("kattasks set without a link:\n%s", raw())
	}

	// Linked on save: the flag follows.
	n.Body = "- [ ] plain {kt:new}\n"
	if err := s.Save(n); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw(), "kattasks: true\n") {
		t.Fatalf("kattasks not set:\n%s", raw())
	}

	// Link removed: so is the flag; contains_task stays.
	n.Body = "- [ ] plain\n"
	if err := s.Save(n); err != nil {
		t.Fatal(err)
	}
	if got := raw(); strings.Contains(got, TodoLinkKey+":") || !strings.Contains(got, "contains_task: true\n") {
		t.Fatalf("after unlinking:\n%s", got)
	}

	// {kt:new} with sync off has no todo.txt line yet: no flag.
	s.Todo = nil
	n.Body = "- [ ] plain {kt:new}\n"
	if err := s.Save(n); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw(), TodoLinkKey+":") {
		t.Fatalf("kattasks set for kt:new:\n%s", raw())
	}

	// A file written elsewhere with a link but no flag: SyncTaskFlags backfills it.
	os.WriteFile(s.Path(KindPage, "other"), []byte("---\ntitle: Other\ncontains_task: true\n---\n- [ ] x {kt:abc123}\n"), 0o644)
	changes, err := s.SyncTaskFlags(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].ID != "other" || !changes[0].Kattasks {
		t.Fatalf("changes = %+v", changes)
	}
	if !strings.Contains(readFile(t, s.Path(KindPage, "other")), "kattasks: true\n") {
		t.Fatal("backfill didn't set kattasks")
	}
}

// A task saved as {kt:new} while sync was off is linked by push.
func TestPushSavedKtNew(t *testing.T) {
	s, dir := newTodoStore(t)
	todo := s.Todo
	s.Todo = nil
	if _, err := s.Create(KindPage, "Later", nil, "- [ ] do it {kt:new}\n"); err != nil {
		t.Fatal(err)
	}
	s.Todo = todo
	n, err := s.PushTask(KindPage, "later", 0)
	if err != nil {
		t.Fatal(err)
	}
	id := linkedID(t, n.Body)
	if got := readFile(t, filepath.Join(dir, "todo.txt")); !strings.Contains(got, "do it kn:"+id+" note:page/later") {
		t.Fatalf("todo.txt = %q", got)
	}
}

func TestPushAllTasks(t *testing.T) {
	s, dir := newTodoStore(t)
	n, err := s.Create(KindPage, "Chores", nil, "# Chores\n\n- [ ] one\n- [ ] two {due:2026-11-20}\nnot a task\n- [x] three\n")
	if err != nil {
		t.Fatal(err)
	}
	n, count, err := s.PushAllTasks(KindPage, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || n.TodoLinks != 2 {
		t.Fatalf("count = %d, TodoLinks = %d, want 2: %q", count, n.TodoLinks, n.Body)
	}
	if !strings.Contains(n.Body, "- [x] three\n") {
		t.Errorf("done task was linked: %q", n.Body)
	}
	if strings.Contains(n.Body, "kt:new") {
		t.Errorf("kt:new left in body: %q", n.Body)
	}
	if got := strings.Count(readFile(t, filepath.Join(dir, "todo.txt")), "note:page/chores"); got != 2 {
		t.Errorf("todo.txt has %d linked lines, want 2", got)
	}
	if _, count, _ = s.PushAllTasks(KindPage, n.ID); count != 0 {
		t.Errorf("second push linked %d, want 0", count)
	}
}

func TestTodoStampChanges(t *testing.T) {
	dir := t.TempDir()
	ts := &TodoSync{File: filepath.Join(dir, "todo.txt")}
	empty := ts.Stamp()
	if err := os.WriteFile(ts.File, []byte("a task\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	one := ts.Stamp()
	if one == empty {
		t.Fatal("stamp didn't change when todo.txt was written")
	}
	if err := os.WriteFile(filepath.Join(dir, "done.txt"), []byte("x done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ts.Stamp() == one {
		t.Fatal("stamp didn't change when done.txt was written")
	}
}
