package notes

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// TaskKey is the frontmatter key set to true while a note's body contains a
// Markdown task ("- [ ] …" or "- [x] …"). It is removed when the last task is.
const TaskKey = "contains_task"

// TodoLinkKey is the frontmatter key set to true while a note has a task
// linked to todo.txt for katTasks ({kt:<id>}; not {kt:new}, which has no
// line yet). It is removed when the last link is.
const TodoLinkKey = "kattasks"

// IsAutoField reports whether a frontmatter key is kept up to date from the
// body (contains_task, kattasks) rather than set by hand.
func IsAutoField(key string) bool {
	return key == TaskKey || key == TodoLinkKey
}

// taskLine matches a task marker at the start of a line, optionally inside a
// blockquote and/or list item. It mirrors the GUI's frontend/src/tasks.ts.
// Groups: 1 = everything before the box, 2 = the mark, 3 = the task text.
// Like GFM, the box must be followed by whitespace: a bare "- [ ]" is not a task.
var taskLine = regexp.MustCompile(`^(\s*(?:>\s*)*(?:(?:[-*+]|\d+[.)])\s+)?)\[( |x|X)\]\s+(.*)$`)

// Task is one Markdown task line in a note body.
type Task struct {
	Line   int    `json:"line"`   // 0-based line in the body
	Done   bool   `json:"done"`   // [x] / [X]
	Text   string `json:"text"`   // text after the box, without the {…} metadata block
	Indent int    `json:"indent"` // leading whitespace width (tabs count as 4)
	// Meta holds the {key:value …} block at the end of the line, if any.
	Meta map[string]string `json:"meta,omitempty"`
}

// ParseTasks returns the tasks in body, skipping fenced code blocks.
func ParseTasks(body string) []Task {
	var out []Task
	fence := ""
	for i, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, "\r")
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			f := t[:3]
			if fence == "" {
				fence = f
			} else if f == fence {
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		m := taskLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		text, meta := ParseMeta(strings.TrimSpace(m[3]))
		out = append(out, Task{Line: i, Done: m[2] != " ", Text: text, Indent: indentWidth(line), Meta: meta})
	}
	return out
}

func indentWidth(line string) int {
	w := 0
	for _, r := range line {
		switch r {
		case ' ':
			w++
		case '\t':
			w += 4
		default:
			return w
		}
	}
	return w
}

// HasTask reports whether body contains a task outside fenced code blocks.
func HasTask(body string) bool { return len(ParseTasks(body)) > 0 }

// NoteTasks is a note together with its tasks.
type NoteTasks struct {
	Note  Note   `json:"note"`
	Tasks []Task `json:"tasks"`
	Open  int    `json:"open"`
	Done  int    `json:"done"`
}

// Tasks lists every note of kind (all kinds when empty) that has tasks, with
// the most open tasks first, then the most recently updated.
func (s *Store) Tasks(kind Kind) ([]NoteTasks, error) {
	ns, err := s.List(kind, ListOpts{})
	if err != nil {
		return nil, err
	}
	var out []NoteTasks
	for _, n := range ns {
		ts := n.tasks()
		if len(ts) == 0 {
			continue
		}
		nt := NoteTasks{Note: n, Tasks: ts}
		for _, t := range ts {
			if t.Done {
				nt.Done++
			} else {
				nt.Open++
			}
		}
		out = append(out, nt)
	}
	sortByIndex(out, func(i, j int) int {
		if out[i].Open != out[j].Open {
			return out[j].Open - out[i].Open
		}
		return out[j].Note.Updated.Compare(out[i].Note.Updated)
	})
	return out, nil
}

// ErrTaskMoved is returned by SetTask when the line is no longer a task.
var ErrTaskMoved = errors.New("note changed: that line is no longer a task")

// SetTask checks (done) or unchecks the task on body line `line` and saves.
// Unless NoDoneDate is set, checking stamps done:<today> into the task's
// metadata and unchecking removes it.
func (s *Store) SetTask(kind Kind, id string, line int, done bool) (*Note, error) {
	return s.editTaskLine(kind, id, line, func(l string) string {
		return markTaskLine(l, done, "", s.now(), s.NoDoneDate)
	})
}

// SetTaskMeta sets one metadata key on the task at body line `line` and
// saves. Well-known keys are normalized (dates accept "tomorrow", "fri",
// "+3d"…; pri accepts h/m/l); an empty value removes the key.
func (s *Store) SetTaskMeta(kind Kind, id string, line int, key, value string) (*Note, error) {
	key = strings.ToLower(strings.TrimSpace(key))
	v, err := NormalizeMeta(key, value, s.now())
	if err != nil {
		return nil, err
	}
	return s.editTaskLine(kind, id, line, func(l string) string {
		l, _ = SetLineMeta(l, key, v)
		return l
	})
}

// editTaskLine rewrites the task on body line `line` with edit and saves.
func (s *Store) editTaskLine(kind Kind, id string, line int, edit func(string) string) (*Note, error) {
	n, err := s.Get(kind, id)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(n.Body, "\n")
	if line < 0 || line >= len(lines) {
		return nil, ErrTaskMoved
	}
	found := false
	for _, t := range ParseTasks(n.Body) {
		if t.Line == line {
			found = true
			break
		}
	}
	if !found {
		return nil, ErrTaskMoved
	}
	cr := strings.HasSuffix(lines[line], "\r")
	l := edit(strings.TrimSuffix(lines[line], "\r"))
	if cr {
		l += "\r"
	}
	if l == lines[line] {
		return n, nil
	}
	lines[line] = l
	n.Body = strings.Join(lines, "\n")
	if err := s.Save(n); err != nil {
		return nil, err
	}
	return n, nil
}

// syncTaskFlag sets or clears the contains_task and kattasks frontmatter
// keys from the body.
func syncTaskFlag(n *Note) {
	tasks := ParseTasks(n.Body)
	setFlag(n, TaskKey, len(tasks) > 0)
	setFlag(n, TodoLinkKey, hasLinkedTodo(tasks))
}

func setFlag(n *Note, key string, on bool) {
	if on {
		n.extra()[key] = true
		return
	}
	if n.Extra != nil {
		delete(n.Extra, key)
		if len(n.Extra) == 0 {
			n.Extra = nil
		}
	}
}

// flagOK reports whether frontmatter key matches want (true, or absent).
func flagOK(n *Note, key string, want bool) bool {
	v, present := n.Extra[key]
	return (want && v == true) || (!want && !present)
}

// FileLine converts a 0-based body line of a note into a 1-based line number
// in its file, accounting for the frontmatter block.
func (s *Store) FileLine(kind Kind, id string, bodyLine int) int {
	data, err := os.ReadFile(s.Path(kind, id))
	if err != nil {
		return bodyLine + 1
	}
	header, _, ok := splitFrontmatter(data)
	if !ok {
		return bodyLine + 1
	}
	// Opening ---, the header lines, closing ---.
	return 2 + strings.Count(string(header), "\n") + bodyLine + 1
}

// TaskFlagChange is a note whose contains_task or kattasks flag
// SyncTaskFlags changed; the fields are the new values.
type TaskFlagChange struct {
	Kind         Kind   `json:"kind"`
	ID           string `json:"id"`
	Path         string `json:"path"`
	ContainsTask bool   `json:"contains_task"`
	Kattasks     bool   `json:"kattasks"`
}

// SyncTaskFlags rescans every note and sets or removes the contains_task
// and kattasks frontmatter keys to match its body. Only notes whose flag is wrong are
// rewritten, and their updated time is left alone. With dryRun nothing is
// written; the changes that would be made are still returned.
func (s *Store) SyncTaskFlags(dryRun bool) ([]TaskFlagChange, error) {
	ns, err := s.List("", ListOpts{})
	if err != nil {
		return nil, err
	}
	var out []TaskFlagChange
	for i := range ns {
		n := &ns[i]
		tasks := n.tasks()
		want, linked := len(tasks) > 0, hasLinkedTodo(tasks)
		if flagOK(n, TaskKey, want) && flagOK(n, TodoLinkKey, linked) {
			continue
		}
		out = append(out, TaskFlagChange{Kind: n.Kind, ID: n.ID, Path: n.Path, ContainsTask: want, Kattasks: linked})
		if dryRun {
			continue
		}
		if err := s.write(n, false); err != nil { // write runs syncTaskFlag
			return out, fmt.Errorf("%s/%s: %w", n.Kind, n.ID, err)
		}
	}
	return out, nil
}

// TaskRef is one task with the note it belongs to.
type TaskRef struct {
	Note Note `json:"note"`
	Task Task `json:"task"`
}

// TaskFilter narrows AllTasks. The zero value lists every open task.
type TaskFilter struct {
	// DueBefore keeps tasks due on or before this YYYY-MM-DD date.
	DueBefore string `json:"dueBefore,omitempty"`
	// Pri keeps tasks with this priority (high, med or low).
	Pri string `json:"pri,omitempty"`
	// Has keeps tasks whose metadata has every key with that value; an
	// empty value only requires the key to be present.
	Has map[string]string `json:"has,omitempty"`
	// IncludeDone also lists checked tasks.
	IncludeDone bool `json:"includeDone,omitempty"`
	// Kind limits the notes searched; empty means all kinds.
	Kind Kind `json:"kind,omitempty"`
	// When is "" (any), "overdue" (due before Today) or "week" (due within
	// 7 days of Today, overdue included). Tasks without a due date only pass
	// "". The GUI's taskVisible does the same in TypeScript.
	When string `json:"when,omitempty"`
	// Today is the YYYY-MM-DD date When is measured from (default: today).
	Today string `json:"today,omitempty"`
	// OnlyDone keeps checked tasks only (implies IncludeDone).
	OnlyDone bool `json:"onlyDone,omitempty"`
}

// TaskWhen values for TaskFilter.When.
const (
	WhenAny     = ""
	WhenOverdue = "overdue"
	WhenWeek    = "week"
)

// Match reports whether t passes the filter.
func (f TaskFilter) Match(t Task) bool {
	if t.Done && !f.IncludeDone && !f.OnlyDone {
		return false
	}
	if f.OnlyDone && !t.Done {
		return false
	}
	if f.When != WhenAny {
		d, ok := t.Meta[MetaDue]
		if !ok {
			return false
		}
		today := f.Today
		if today == "" {
			today = time.Now().Format(DateLayout)
		}
		switch f.When {
		case WhenOverdue:
			if d >= today {
				return false
			}
		case WhenWeek:
			day, err := time.ParseInLocation(DateLayout, today, time.Local)
			if err != nil || d > day.AddDate(0, 0, 7).Format(DateLayout) {
				return false
			}
		}
	}
	if f.DueBefore != "" {
		d, ok := t.Meta[MetaDue]
		if !ok || d > f.DueBefore { // ISO dates compare as strings
			return false
		}
	}
	if f.Pri != "" && t.Meta[MetaPri] != f.Pri {
		return false
	}
	for k, v := range f.Has {
		got, ok := t.Meta[k]
		if !ok || (v != "" && got != v) {
			return false
		}
	}
	return true
}

// AllTasks lists the tasks across notes that pass f, sorted by due date
// (undated last), then priority, then the note's updated time, then line.
func (s *Store) AllTasks(f TaskFilter) ([]TaskRef, error) {
	ns, err := s.List(f.Kind, ListOpts{})
	if err != nil {
		return nil, err
	}
	var out []TaskRef
	for _, n := range ns {
		for _, t := range n.tasks() {
			if f.Match(t) {
				out = append(out, TaskRef{Note: n, Task: t})
			}
		}
	}
	SortTasks(out)
	return out, nil
}

// SortTasks orders tasks by due date (undated last), priority (highest
// first), the note's updated time (newest first) and then line.
func SortTasks(ts []TaskRef) {
	due := make([]string, len(ts))
	pri := make([]int, len(ts))
	for i := range ts {
		due[i], pri[i] = ts[i].Task.Meta[MetaDue], ts[i].Task.Priority()
	}
	sortByIndex(ts, func(i, j int) int {
		a, b := &ts[i], &ts[j]
		da, db := due[i], due[j]
		if da != db {
			if da == "" {
				return 1
			}
			if db == "" {
				return -1
			}
			return strings.Compare(da, db)
		}
		if pa, pb := pri[i], pri[j]; pa != pb {
			return pb - pa
		}
		if c := b.Note.Updated.Compare(a.Note.Updated); c != 0 {
			return c
		}
		if c := strings.Compare(a.Note.Path, b.Note.Path); c != 0 {
			return c
		}
		return a.Task.Line - b.Task.Line
	})
}
