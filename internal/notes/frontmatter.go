package notes

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const fmDelim = "---"

// splitFrontmatter separates a leading YAML block delimited by --- lines from
// the body. ok is false when the document has no frontmatter.
func splitFrontmatter(data []byte) (header, body []byte, ok bool) {
	s := string(data)
	s = strings.TrimPrefix(s, "\uFEFF")
	if !strings.HasPrefix(s, fmDelim+"\n") && !strings.HasPrefix(s, fmDelim+"\r\n") {
		return nil, data, false
	}
	rest := s[strings.Index(s, "\n")+1:]
	// Closing delimiter: a line that is exactly ---.
	pos := 0
	for {
		nl := strings.Index(rest[pos:], "\n")
		var line string
		if nl < 0 {
			line = rest[pos:]
		} else {
			line = rest[pos : pos+nl]
		}
		if strings.TrimRight(line, "\r") == fmDelim {
			header := rest[:pos]
			var b string
			if nl >= 0 {
				b = rest[pos+nl+1:]
			}
			return []byte(header), []byte(b), true
		}
		if nl < 0 {
			return nil, data, false
		}
		pos += nl + 1
	}
}

// parseNote decodes raw file contents into n (Kind, ID and Path are left to
// the caller).
func parseNote(data []byte, n *Note) error {
	header, body, ok := splitFrontmatter(data)
	n.Body = string(body)
	n.Extra = nil
	if !ok {
		return nil
	}
	var m map[string]any
	if err := yaml.Unmarshal(header, &m); err != nil {
		return fmt.Errorf("frontmatter: %w", err)
	}
	for k, v := range m {
		switch k {
		case "title":
			n.Title = fmt.Sprint(v)
		case "tags":
			n.Tags = toTags(v)
		case "created":
			if t, ok := toTime(v); ok {
				n.Created = t
			} else {
				n.extra()[k] = v
			}
		case "updated":
			if t, ok := toTime(v); ok {
				n.Updated = t
			} else {
				n.extra()[k] = v
			}
		default:
			n.extra()[k] = v
		}
	}
	return nil
}

func (n *Note) extra() map[string]any {
	if n.Extra == nil {
		n.Extra = map[string]any{}
	}
	return n.Extra
}

func toTags(v any) []string {
	var raw []string
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			raw = append(raw, fmt.Sprint(x))
		}
	case string:
		raw = strings.FieldsFunc(t, func(r rune) bool { return r == ',' || r == ' ' })
	case nil:
	default:
		raw = []string{fmt.Sprint(t)}
	}
	return NormalizeTags(raw)
}

// NormalizeTags lowercases, trims, strips a leading #, and de-duplicates tags
// while keeping their order.
func NormalizeTags(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range in {
		t = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), "#")))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

func toTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case string:
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04", "2006-01-02"} {
			if tm, err := time.ParseInLocation(layout, t, time.Local); err == nil {
				return tm, true
			}
		}
	}
	return time.Time{}, false
}

// encodeNote renders n as a Markdown document with YAML frontmatter.
func encodeNote(n *Note) ([]byte, error) {
	header, err := encodeFrontmatter(n)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteString(fmDelim + "\n")
	buf.Write(header)
	buf.WriteString(fmDelim + "\n")
	buf.WriteString(n.Body)
	return buf.Bytes(), nil
}

// encodeFrontmatter renders n's frontmatter as YAML, without the --- lines.
// Known keys come first in a fixed order, followed by unknown keys sorted by
// name.
func encodeFrontmatter(n *Note) ([]byte, error) {
	doc := &yaml.Node{Kind: yaml.MappingNode}
	add := func(k string, v any) error {
		var val yaml.Node
		if err := val.Encode(v); err != nil {
			return err
		}
		doc.Content = append(doc.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: k}, &val)
		return nil
	}
	tags := n.Tags
	if tags == nil {
		tags = []string{}
	}
	if err := add("title", n.Title); err != nil {
		return nil, err
	}
	if err := add("tags", tags); err != nil {
		return nil, err
	}
	doc.Content[3].Style = yaml.FlowStyle
	if err := add("created", n.Created.Format(time.RFC3339)); err != nil {
		return nil, err
	}
	if err := add("updated", n.Updated.Format(time.RFC3339)); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(n.Extra))
	for k := range n.Extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := add(k, n.Extra[k]); err != nil {
			return nil, err
		}
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	enc.Close()
	return buf.Bytes(), nil
}
