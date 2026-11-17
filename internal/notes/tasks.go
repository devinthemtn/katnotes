package notes

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// TaskKey is the frontmatter key set to true while a note's body contains a
// Markdown task ("- [ ] …" or "- [x] …"). It is removed when the last task is.
const TaskKey = "contains_task"

// taskLine matches a task marker at the start of a line, optionally inside a
// blockquote and/or list item. It mirrors the GUI's frontend/src/tasks.ts.
// Groups: 1 = everything before the box, 2 = the mark, 3 = the task text.
var taskLine = regexp.MustCompile(`^(\s*(?:>\s*)*(?:(?:[-*+]|\d+[.)])\s+)?)\[( |x|X)\](?:\s+(.*))?$`)

// Task is one Markdown task line in a note body.
type Task struct {
	Line   int    `json:"line"`   // 0-based line in the body
	Done   bool   `json:"done"`   // [x] / [X]
	Text   string `json:"text"`   // text after the box
	Indent int    `json:"indent"` // leading whitespace width (tabs count as 4)
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
		out = append(out, Task{Line: i, Done: m[2] != " ", Text: strings.TrimSpace(m[3]), Indent: indentWidth(line)})
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
		ts := ParseTasks(n.Body)
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
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Open != out[j].Open {
			return out[i].Open > out[j].Open
		}
		return out[i].Note.Updated.After(out[j].Note.Updated)
	})
	return out, nil
}

// ErrTaskMoved is returned by SetTask when the line is no longer a task.
var ErrTaskMoved = errors.New("note changed: that line is no longer a task")

// SetTask checks (done) or unchecks the task on body line `line` and saves.
func (s *Store) SetTask(kind Kind, id string, line int, done bool) (*Note, error) {
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
	mark := " "
	if done {
		mark = "x"
	}
	lines[line] = taskLine.ReplaceAllString(lines[line], "${1}["+mark+"]"+taskTail(lines[line]))
	n.Body = strings.Join(lines, "\n")
	if err := s.Save(n); err != nil {
		return nil, err
	}
	return n, nil
}

// taskTail returns what follows the box on a task line, unchanged.
func taskTail(line string) string {
	loc := taskLine.FindStringSubmatchIndex(line)
	// loc[4]/loc[5] bound the mark; the box's "]" is right after it.
	return strings.ReplaceAll(line[loc[5]+1:], "$", "$$")
}

// syncTaskFlag sets or clears the contains_task frontmatter key from the body.
func syncTaskFlag(n *Note) {
	if HasTask(n.Body) {
		n.extra()[TaskKey] = true
		return
	}
	if n.Extra != nil {
		delete(n.Extra, TaskKey)
		if len(n.Extra) == 0 {
			n.Extra = nil
		}
	}
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

// TaskFlagChange is a note whose contains_task flag SyncTaskFlags changed.
type TaskFlagChange struct {
	Kind         Kind   `json:"kind"`
	ID           string `json:"id"`
	Path         string `json:"path"`
	ContainsTask bool   `json:"contains_task"`
}

// SyncTaskFlags rescans every note and sets or removes the contains_task
// frontmatter key to match its body. Only notes whose flag is wrong are
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
		want := HasTask(n.Body)
		v, present := n.Extra[TaskKey]
		if (want && v == true) || (!want && !present) {
			continue
		}
		out = append(out, TaskFlagChange{Kind: n.Kind, ID: n.ID, Path: n.Path, ContainsTask: want})
		if dryRun {
			continue
		}
		if err := s.write(n, false); err != nil { // write runs syncTaskFlag
			return out, fmt.Errorf("%s/%s: %w", n.Kind, n.ID, err)
		}
	}
	return out, nil
}
