package notes

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/sahilm/fuzzy"
)

// SearchMode picks what a search looks at. The TUI's "/" bar and the GUI's
// search box switch between these; the CLI's search takes --name / --tags.
type SearchMode string

const (
	SearchName SearchMode = "name" // fuzzy over title and filename
	SearchTags SearchMode = "tags" // notes carrying every listed tag
	SearchText SearchMode = "text" // full text: every word / "phrase"
)

// SearchModes lists the modes in the order the UIs cycle them.
var SearchModes = []SearchMode{SearchName, SearchTags, SearchText}

// ParseSearchMode reads a mode name ("" is name).
func ParseSearchMode(s string) (SearchMode, error) {
	switch SearchMode(strings.ToLower(strings.TrimSpace(s))) {
	case "", SearchName:
		return SearchName, nil
	case SearchTags, "tag":
		return SearchTags, nil
	case SearchText, "full", "body":
		return SearchText, nil
	}
	return "", fmt.Errorf("unknown search mode %q (want name, tags or text)", s)
}

// NextSearchMode is the mode after m, wrapping around (d = -1 goes back).
func NextSearchMode(m SearchMode, d int) SearchMode {
	for i, x := range SearchModes {
		if x == m {
			return SearchModes[(i+d+len(SearchModes))%len(SearchModes)]
		}
	}
	return SearchName
}

// FindQuery is one search.
type FindQuery struct {
	Mode SearchMode `json:"mode"`
	Text string     `json:"text"`
	Kind Kind       `json:"kind"` // "" searches every kind
	Tag  string     `json:"tag"`  // only notes with this tag ("" for any)
	Sort SortBy     `json:"sort"` // order for tags mode and empty queries
}

// Find runs a search. An empty Text lists the notes (filtered by Kind and
// Tag, in Sort order), so a search box can be cleared live.
//
//   - name: fuzzy match against each note's title and filename, best first.
//     Match.NameHits gives the matched title characters for highlighting.
//   - tags: every word is a tag the note must have ("#" optional, "-tag"
//     excludes); the last word also matches tag prefixes while typing.
//   - text: every word and "quoted phrase" must appear in the title, tags or
//     body, case-insensitively. Match.Lines holds the matching lines.
func (s *Store) Find(q FindQuery) ([]Match, error) {
	all, err := s.List(q.Kind, ListOpts{Tag: q.Tag, Sort: q.Sort})
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(q.Text)
	if text == "" {
		out := make([]Match, len(all))
		for i, n := range all {
			out[i] = Match{Note: n, Lines: []LineMatch{}}
		}
		return out, nil
	}
	switch q.Mode {
	case SearchTags:
		return findTags(all, q.Text), nil
	case SearchText:
		return findText(all, text), nil
	default:
		return findName(all, text), nil
	}
}

func findName(all []Note, pattern string) []Match {
	titles := make([]string, len(all))
	ids := make([]string, len(all))
	for i, n := range all {
		titles[i], ids[i] = n.Title, n.ID
	}
	type hit struct {
		score int
		runes []int
	}
	best := map[int]hit{}
	for _, m := range fuzzy.FindNoSort(pattern, titles) {
		best[m.Index] = hit{m.Score, byteToRune(m.Str, m.MatchedIndexes)}
	}
	for _, m := range fuzzy.FindNoSort(pattern, ids) {
		if h, ok := best[m.Index]; !ok || m.Score > h.score {
			best[m.Index] = hit{score: m.Score} // matched the filename: nothing to mark in the title
		}
	}
	out := make([]Match, 0, len(best))
	for i, h := range best {
		out = append(out, Match{Note: all[i], TitleMatch: true, Lines: []LineMatch{}, NameHits: h.runes, rank: h.score})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].rank != out[j].rank {
			return out[i].rank > out[j].rank
		}
		return out[i].Note.Updated.After(out[j].Note.Updated)
	})
	return out
}

// byteToRune turns byte offsets in s into rune offsets.
func byteToRune(s string, idx []int) []int {
	out := make([]int, 0, len(idx))
	for _, b := range idx {
		out = append(out, utf8.RuneCountInString(s[:b]))
	}
	return out
}

func findTags(all []Note, text string) []Match {
	words := strings.Fields(strings.ToLower(text))
	prefixLast := !strings.HasSuffix(text, " ")
	var out []Match
	for _, n := range all {
		ok := true
		for i, w := range words {
			neg := strings.HasPrefix(w, "-")
			w = strings.TrimPrefix(strings.TrimPrefix(w, "-"), "#")
			if w == "" {
				continue
			}
			has := false
			for _, t := range n.Tags {
				if t == w || (prefixLast && i == len(words)-1 && !neg && strings.HasPrefix(t, w)) {
					has = true
					break
				}
			}
			if has == neg {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, Match{Note: n, TagMatch: true, Lines: []LineMatch{}})
		}
	}
	return out
}

// searchTerms splits text into lowercase words and "quoted phrases".
func searchTerms(text string) []string {
	var terms []string
	text = strings.ToLower(text)
	for {
		i := strings.IndexByte(text, '"')
		if i < 0 {
			break
		}
		terms = append(terms, strings.Fields(text[:i])...)
		rest := text[i+1:]
		j := strings.IndexByte(rest, '"')
		if j < 0 {
			text = rest
			break
		}
		if p := strings.TrimSpace(rest[:j]); p != "" {
			terms = append(terms, p)
		}
		text = rest[j+1:]
	}
	return append(terms, strings.Fields(text)...)
}

func findText(all []Note, text string) []Match {
	terms := searchTerms(text)
	if len(terms) == 0 {
		return nil
	}
	var out []Match
	for _, n := range all {
		title := strings.ToLower(n.Title)
		tags := strings.Join(n.Tags, " ")
		body := strings.ToLower(n.Body)
		ok := true
		for _, t := range terms {
			tt := strings.TrimPrefix(t, "#")
			if !strings.Contains(title, t) && !strings.Contains(tags, tt) && !strings.Contains(body, t) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		m := Match{Note: n, Lines: []LineMatch{}}
		for _, t := range terms {
			m.TitleMatch = m.TitleMatch || strings.Contains(title, t)
			m.TagMatch = m.TagMatch || strings.Contains(tags, strings.TrimPrefix(t, "#"))
		}
		for i, line := range strings.Split(n.Body, "\n") {
			l := strings.ToLower(line)
			for _, t := range terms {
				if strings.Contains(l, t) {
					if len(m.Lines) < maxLinesPerMatch {
						m.Lines = append(m.Lines, LineMatch{Line: i + 1, Text: strings.TrimSpace(line)})
					}
					m.rank++
					break
				}
			}
		}
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].score() > out[j].score() })
	return out
}
