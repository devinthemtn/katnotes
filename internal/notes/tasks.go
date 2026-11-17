package notes

import (
	"regexp"
	"strings"
)

// TaskKey is the frontmatter key set to true while a note's body contains a
// Markdown task ("- [ ] …" or "- [x] …"). It is removed when the last task is.
const TaskKey = "contains_task"

// taskLine matches a task marker at the start of a line, optionally inside a
// blockquote and/or list item. It mirrors the GUI's frontend/src/tasks.ts.
var taskLine = regexp.MustCompile(`^\s*(?:>\s*)*(?:(?:[-*+]|\d+[.)])\s+)?\[( |x|X)\](?:\s|$)`)

// HasTask reports whether body contains a task outside fenced code blocks.
func HasTask(body string) bool {
	fence := ""
	for _, line := range strings.Split(body, "\n") {
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
		if fence == "" && taskLine.MatchString(line) {
			return true
		}
	}
	return false
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
