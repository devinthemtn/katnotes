package notes

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"
)

// Page templates are Markdown files, <dir>/<name>.md, used by "new from
// <name> <title>". They are Go text/templates with .Title, .Date (in the
// store's DateFormat), .ID (today as YYYY-MM-DD), .Weekday and .Time, and may
// start with frontmatter: its tags and other fields are copied to the new
// page (title, created and updated are always the new page's own).

// TemplatesDirName is the templates folder inside the config directory.
const TemplatesDirName = "templates"

var validTemplate = regexp.MustCompile(`^[\pL\pN_-]+$`)

// ExampleTemplate is written to an empty templates folder on first run.
const ExampleTemplate = `---
tags: [meeting]
---
# {{.Title}}

{{.Weekday}} {{.Date}} · {{.Time}}

## Attendees

## Notes

## Actions

`

// WriteExampleTemplate creates dir with meeting.md if dir doesn't exist yet.
func WriteExampleTemplate(dir string) error {
	if _, err := os.Stat(dir); err == nil {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "meeting.md"), []byte(ExampleTemplate), 0o644)
}

// ListTemplates returns the template names in dir, sorted. A missing dir
// has none.
func ListTemplates(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{}
	}
	out := []string{}
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".md")
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") && validTemplate.MatchString(name) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// ReadTemplate returns the source of template name in dir.
func ReadTemplate(dir, name string) (string, error) {
	if !validTemplate.MatchString(name) {
		return "", fmt.Errorf("template %q: invalid name", name)
	}
	data, err := os.ReadFile(filepath.Join(dir, name+".md"))
	if errors.Is(err, os.ErrNotExist) {
		have := ListTemplates(dir)
		if len(have) == 0 {
			return "", fmt.Errorf("no template %q: add Markdown files to %s", name, dir)
		}
		return "", fmt.Errorf("no template %q (have: %s)", name, strings.Join(have, ", "))
	}
	return string(data), err
}

// CreateFromTemplate creates a page titled title from template source src.
func (s *Store) CreateFromTemplate(title, src string, tags []string) (*Note, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("%w: a page needs a title", ErrInvalidID)
	}
	tmpl, err := template.New("page").Option("missingkey=zero").Parse(src)
	if err != nil {
		return nil, fmt.Errorf("template: %w", err)
	}
	now := s.now()
	layout := s.DateFormat
	if layout == "" {
		layout = JournalIDLayout
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]string{
		"Title":   title,
		"Date":    now.Format(layout),
		"ID":      now.Format(JournalIDLayout),
		"Weekday": now.Weekday().String(),
		"Time":    now.Format("15:04"),
	}); err != nil {
		return nil, fmt.Errorf("template: %w", err)
	}
	var t Note
	if err := parseNote(buf.Bytes(), &t); err != nil {
		return nil, fmt.Errorf("template: %w", err)
	}
	body := t.Body
	if strings.TrimSpace(body) == "" {
		body = "# " + title + "\n\n"
	}
	n, err := s.Create(KindPage, title, append(t.Tags, tags...), body)
	if err != nil {
		return nil, err
	}
	delete(t.Extra, TaskKey) // follows the body, set on save
	if len(t.Extra) > 0 {
		for k, v := range t.Extra {
			n.extra()[k] = v
		}
		if err := s.Save(n); err != nil {
			return nil, err
		}
	}
	return n, nil
}
