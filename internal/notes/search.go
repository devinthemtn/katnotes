package notes

import (
	"sort"
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
	// NameHits are the title characters (rune offsets) a name search
	// matched, for highlighting.
	NameHits []int `json:"nameHits,omitempty"`

	rank int // name: fuzzy score; text: number of matching lines
}

func (m Match) score() int {
	s := m.rank
	if m.TagMatch {
		s += 50
	}
	if m.TitleMatch {
		s += 100
	}
	return s
}

const maxLinesPerMatch = 5

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
