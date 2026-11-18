package notes

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Editing a note's frontmatter on its own, as a YAML document (the TUI's
// ":frontmatter" command). Notes that were imported or written by hand may
// have no frontmatter at all; for those the YAML is filled in from what the
// store infers (title from the first H1, dates from the file), and applying
// it adds a real frontmatter block.

// HasFrontmatter reports whether the note's file on disk starts with a
// frontmatter block.
func (s *Store) HasFrontmatter(kind Kind, id string) (bool, error) {
	data, err := os.ReadFile(s.Path(kind, id))
	if err != nil {
		return false, err
	}
	_, _, ok := splitFrontmatter(data)
	return ok, nil
}

// FrontmatterYAML returns n's frontmatter as YAML (no --- lines), including
// the inferred values for a note that has none on disk.
func FrontmatterYAML(n *Note) ([]byte, error) {
	c := *n
	if c.Tags == nil {
		c.Tags = []string{}
	}
	syncTaskFlag(&c)
	return encodeFrontmatter(&c)
}

// ApplyFrontmatter replaces n's frontmatter with the YAML in src and saves
// it, leaving the body alone. Lines starting with # are comments. A removed
// title falls back to the first H1 (or the id); a removed or invalid
// created keeps the old one; updated is always set to now by Save. A changed
// title goes through Rename, so a leading "# Old title" heading follows it
// and a page moves to the new slug with links to it rewritten. It returns the saved note and the notes whose links
// were rewritten.
func (s *Store) ApplyFrontmatter(n *Note, src []byte) (*Note, []string, error) {
	src = bytes.TrimPrefix(src, []byte("\uFEFF"))
	// Tolerate the user pasting in the --- delimiters.
	var lines []string
	for _, l := range strings.Split(string(src), "\n") {
		if strings.TrimRight(l, "\r") != fmDelim {
			lines = append(lines, l)
		}
	}
	header := strings.Join(lines, "\n")
	if !strings.HasSuffix(header, "\n") {
		header += "\n"
	}

	fresh, err := s.Get(n.Kind, n.ID)
	if err != nil {
		return nil, nil, err
	}
	edited := &Note{Kind: fresh.Kind, ID: fresh.ID, Path: fresh.Path}
	if err := parseNote([]byte(fmDelim+"\n"+header+fmDelim+"\n"+fresh.Body), edited); err != nil {
		return nil, nil, err
	}
	if v, ok := edited.Extra["created"]; ok {
		return nil, nil, fmt.Errorf("frontmatter: created: can't read %v as a date (use 2006-01-02 or RFC 3339)", v)
	}
	delete(edited.Extra, "updated") // Save sets it; an unreadable value is dropped
	title := strings.TrimSpace(edited.Title)
	if title == "" {
		title = titleFromBody(fresh.Body)
	}
	if title == "" {
		title = fresh.ID
	}
	if edited.Created.IsZero() {
		edited.Created = fresh.Created
	}
	if edited.Tags == nil {
		edited.Tags = []string{}
	}

	rename := title != fresh.Title
	if rename {
		edited.Title = fresh.Title // Rename applies the new one
	} else {
		edited.Title = title
	}
	if err := s.Save(edited); err != nil {
		return nil, nil, err
	}
	if !rename {
		return edited, nil, nil
	}
	return s.Rename(edited.Kind, edited.ID, title)
}

// ErrReservedField is returned when a field can't be set directly.
var ErrReservedField = errors.New("field is managed by katnotes")

// SetField sets one frontmatter field on a note and saves it, adding a
// frontmatter block if the file has none. value is read as a YAML value, so
// "true", "3" and "[a, b]" become a bool, number and list; anything that
// isn't valid YAML is kept as a plain string. title renames the note, tags
// replaces the tags, created must be a date and updated can't be set. It
// returns the saved note and, for a rename, the notes whose links changed.
func (s *Store) SetField(n *Note, key, value string) (*Note, []string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, nil, errors.New("set: missing field name")
	}
	fresh, err := s.Get(n.Kind, n.ID)
	if err != nil {
		return nil, nil, err
	}
	value = strings.TrimSpace(value)
	var v any = value
	if value != "" {
		var parsed any
		if err := yaml.Unmarshal([]byte(value), &parsed); err == nil && parsed != nil {
			v = parsed
		}
	}
	switch key {
	case "title":
		return s.Rename(fresh.Kind, fresh.ID, value)
	case "tags":
		fresh.Tags = toTags(v)
	case "created":
		t, ok := toTime(v)
		if !ok {
			return nil, nil, fmt.Errorf("created: can't read %q as a date (use 2006-01-02 or RFC 3339)", value)
		}
		fresh.Created = t
	case "updated":
		return nil, nil, fmt.Errorf("updated: %w (it is set on every save)", ErrReservedField)
	default:
		fresh.extra()[key] = v
	}
	return fresh, nil, s.Save(fresh)
}

// UnsetField removes a frontmatter field. tags clears the tags; title,
// created and updated can't be removed (every saved note has them).
func (s *Store) UnsetField(n *Note, key string) (*Note, error) {
	key = strings.TrimSpace(key)
	fresh, err := s.Get(n.Kind, n.ID)
	if err != nil {
		return nil, err
	}
	switch key {
	case "tags":
		fresh.Tags = []string{}
	case "title", "created", "updated":
		return nil, fmt.Errorf("%s: %w", key, ErrReservedField)
	case "contains_task":
		return nil, fmt.Errorf("%s: %w (it follows the note's tasks)", key, ErrReservedField)
	default:
		if _, ok := fresh.Extra[key]; !ok {
			return nil, fmt.Errorf("unset: %s/%s has no field %q", fresh.Kind, fresh.ID, key)
		}
		delete(fresh.Extra, key)
	}
	return fresh, s.Save(fresh)
}

// FieldKeys lists the frontmatter keys used across all notes, the built-in
// ones first, then the rest by how many notes use them.
func (s *Store) FieldKeys() ([]string, error) {
	ns, err := s.List("", ListOpts{})
	if err != nil {
		return nil, err
	}
	count := map[string]int{}
	for _, n := range ns {
		for k := range n.Extra {
			count[k]++
		}
	}
	extra := make([]string, 0, len(count))
	for k := range count {
		extra = append(extra, k)
	}
	sort.Slice(extra, func(i, j int) bool {
		if count[extra[i]] != count[extra[j]] {
			return count[extra[i]] > count[extra[j]]
		}
		return extra[i] < extra[j]
	})
	return append([]string{"title", "tags", "created"}, extra...), nil
}
