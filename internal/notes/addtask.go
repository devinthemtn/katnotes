package notes

import (
	"errors"
	"strings"
)

// AddTask adds "- [ ] text" to a note and saves it. The task goes right
// after the note's last top-level task (so it joins an existing list) or at
// the end of the body. A trailing {key:value} block in text is normalized
// like SetTaskMeta does: due:fri becomes a date, pri:h becomes high. It
// returns the saved note and the new task's 0-based body line.
func (s *Store) AddTask(kind Kind, id, text string) (*Note, int, error) {
	text = strings.TrimSpace(text)
	text = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(text, "- [ ]"), "[ ]"))
	if text == "" {
		return nil, 0, errors.New("todo: the task is empty")
	}
	if plain, pairs, at := splitMeta(text); at >= 0 {
		for i, p := range pairs {
			v, err := NormalizeMeta(p.Key, p.Value, s.now())
			if err != nil {
				return nil, 0, err
			}
			pairs[i].Value = v
		}
		text = strings.TrimSpace(plain + " " + FormatMeta(pairs))
	}
	n, err := s.Get(kind, id)
	if err != nil {
		return nil, 0, err
	}
	line := "- [ ] " + text
	lines := strings.Split(strings.TrimRight(n.Body, "\n"), "\n")
	if n.Body == "" {
		lines = nil
	}
	at := -1
	for _, t := range ParseTasks(n.Body) {
		if t.Indent == 0 {
			at = t.Line
		}
	}
	if at < 0 {
		// After the body, separated by a blank line unless it ends in one.
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		at = len(lines)
		lines = append(lines, line)
	} else {
		// Skip the last task's indented children.
		for at+1 < len(lines) && indentWidth(lines[at+1]) > 0 && strings.TrimSpace(lines[at+1]) != "" {
			at++
		}
		at++
		lines = append(lines[:at], append([]string{line}, lines[at:]...)...)
	}
	n.Body = strings.Join(lines, "\n") + "\n"
	if err := s.Save(n); err != nil {
		return nil, 0, err
	}
	return n, at, nil
}
