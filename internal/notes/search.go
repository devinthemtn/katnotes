package notes

import (
	"sort"
	"strings"
)

// LineMatch is a body line that matched a search.
type LineMatch struct {
	Line int    `json:"line"` // 1-based line number within the body
	Text string `json:"text"`
}

// Match is a note that matched a search.
type Match struct {
	Note       Note        `json:"note"`
	TitleMatch bool        `json:"titleMatch"`
	TagMatch   bool        `json:"tagMatch"`
	Lines      []LineMatch `json:"lines"`
}

func (m Match) score() int {
	s := len(m.Lines)
	if m.TagMatch {
		s += 50
	}
	if m.TitleMatch {
		s += 100
	}
	return s
}

const maxLinesPerMatch = 5

// Search does a case-insensitive substring search over titles, tags and
// bodies. An empty kind searches everything. Results are ranked with title
// and tag hits first.
func (s *Store) Search(query string, kind Kind) ([]Match, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil, nil
	}
	all, err := s.List(kind, ListOpts{})
	if err != nil {
		return nil, err
	}
	var out []Match
	for _, n := range all {
		m := Match{Note: n, Lines: []LineMatch{}}
		m.TitleMatch = strings.Contains(strings.ToLower(n.Title), q)
		for _, t := range n.Tags {
			if strings.Contains(t, strings.TrimPrefix(q, "#")) {
				m.TagMatch = true
			}
		}
		for i, line := range strings.Split(n.Body, "\n") {
			if strings.Contains(strings.ToLower(line), q) {
				if len(m.Lines) < maxLinesPerMatch {
					m.Lines = append(m.Lines, LineMatch{Line: i + 1, Text: strings.TrimSpace(line)})
				}
			}
		}
		if m.TitleMatch || m.TagMatch || len(m.Lines) > 0 {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].score() > out[j].score() })
	return out, nil
}

// Tags returns every tag in use with the number of notes carrying it.
func (s *Store) Tags() (map[string]int, error) {
	all, err := s.List("", ListOpts{})
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, n := range all {
		for _, t := range n.Tags {
			out[t]++
		}
	}
	return out, nil
}

// TagCount is a tag with its usage count.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// SortedTags returns Tags ordered by count (desc) then name.
func (s *Store) SortedTags() ([]TagCount, error) {
	m, err := s.Tags()
	if err != nil {
		return nil, err
	}
	out := make([]TagCount, 0, len(m))
	for t, c := range m {
		out = append(out, TagCount{t, c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})
	return out, nil
}
