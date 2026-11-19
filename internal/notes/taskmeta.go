package notes

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Task metadata lives in one {key:value …} block at the end of a task line:
//
//	- [ ] Call the plumber {due:2026-11-20 pri:high}
//	- [x] Pay rent {due:2026-11-01 done:2026-10-30}
//	- [ ] Book venue {note:"ask about parking"}
//
// The block is plain text, so the note still reads well in any Markdown
// viewer and is easy to grep (rg 'due:2026-11'). Rules:
//   - only a block at the very end of the line counts; braces elsewhere are text
//   - the block holds whitespace-separated key:value pairs, keys [a-z][a-z0-9_-]*,
//     values without spaces unless double-quoted
//   - if any token is not a valid pair, the braces are ordinary text
//   - a block inside an inline code span is ignored
//
// Unknown keys are kept. The frontend's tasks.ts mirrors these rules.

// Well-known metadata keys.
const (
	MetaDue   = "due"   // YYYY-MM-DD
	MetaPri   = "pri"   // high | med | low
	MetaDone  = "done"  // YYYY-MM-DD, stamped when a task is checked
	MetaStart = "start" // YYYY-MM-DD, the task is not actionable before this
)

// DateLayout is the layout of date values in task metadata.
const DateLayout = "2006-01-02"

// Priorities, highest first.
var Priorities = []string{"high", "med", "low"}

var (
	metaBlock = regexp.MustCompile(`\{([^{}]*)\}\s*$`)
	metaPair  = regexp.MustCompile(`^([a-z][a-z0-9_-]*):(?:"([^"]*)"|([^\s"{}]+))`)
	metaKey   = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
)

// MetaPair is one key:value entry, in file order.
type MetaPair struct {
	Key   string
	Value string
}

// parseMetaBody parses the inside of a {…} block. ok is false when it is not
// a metadata block (empty, or any token that isn't key:value).
func parseMetaBody(s string) (pairs []MetaPair, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	for s != "" {
		m := metaPair.FindStringSubmatch(s)
		if m == nil {
			return nil, false
		}
		v := m[3]
		if m[3] == "" {
			v = m[2]
		}
		pairs = append(pairs, MetaPair{m[1], v})
		rest := s[len(m[0]):]
		if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
			return nil, false // e.g. due:x"y" or a:b:c… glued together
		}
		s = strings.TrimLeft(rest, " \t")
	}
	return pairs, true
}

// splitMeta finds the metadata block at the end of text. It returns the text
// before the block (right-trimmed), the pairs, and the byte offset where the
// block starts (-1 when there is none).
func splitMeta(text string) (plain string, pairs []MetaPair, at int) {
	loc := metaBlock.FindStringSubmatchIndex(text)
	if loc == nil {
		return text, nil, -1
	}
	before := text[:loc[0]]
	if strings.Count(before, "`")%2 == 1 {
		return text, nil, -1 // inside an inline code span
	}
	pairs, ok := parseMetaBody(text[loc[2]:loc[3]])
	if !ok {
		return text, nil, -1
	}
	return strings.TrimRight(before, " \t"), pairs, loc[0]
}

// ParseMeta returns the task text without its metadata block, and the
// metadata as a map (nil when there is none). Later duplicate keys win.
func ParseMeta(text string) (string, map[string]string) {
	plain, pairs, _ := splitMeta(text)
	if pairs == nil {
		return text, nil
	}
	m := make(map[string]string, len(pairs))
	for _, p := range pairs {
		m[p.Key] = p.Value
	}
	return plain, m
}

// FormatMeta renders pairs as a {…} block, quoting values that need it.
func FormatMeta(pairs []MetaPair) string {
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		v := p.Value
		if v == "" || strings.ContainsAny(v, " \t{}\"") {
			v = `"` + strings.ReplaceAll(v, `"`, "'") + `"`
		}
		parts = append(parts, p.Key+":"+v)
	}
	return "{" + strings.Join(parts, " ") + "}"
}

// withMeta sets key to value in the task text's block (value "" deletes the
// key) and returns the new text. Other keys keep their order and spelling;
// the block is created or removed as needed.
func withMeta(text, key, value string) string {
	plain, pairs, at := splitMeta(text)
	if at < 0 {
		plain = strings.TrimRight(text, " \t")
	}
	out := make([]MetaPair, 0, len(pairs)+1)
	found := false
	for _, p := range pairs {
		if p.Key != key {
			out = append(out, p)
			continue
		}
		if !found && value != "" {
			out = append(out, MetaPair{key, value})
		}
		found = true
	}
	if !found && value != "" {
		out = append(out, MetaPair{key, value})
	}
	if len(out) == 0 {
		return plain
	}
	if plain == "" {
		return FormatMeta(out)
	}
	return plain + " " + FormatMeta(out)
}

// SetLineMeta sets (or with value "" removes) one metadata key on a task line.
// ok is false when line is not a task.
func SetLineMeta(line, key, value string) (string, bool) {
	loc := taskLine.FindStringSubmatchIndex(line)
	if loc == nil {
		return line, false
	}
	// Group 3 is the text after the box and its whitespace.
	return line[:loc[6]] + withMeta(line[loc[6]:loc[7]], key, value), true
}

// Priority ranks the task's pri value: high 3, med 2, low 1, otherwise 0.
func (t Task) Priority() int { return PriorityRank(t.Meta[MetaPri]) }

// PriorityRank ranks a pri value: high 3, med 2, low 1, otherwise 0.
func PriorityRank(p string) int {
	for i, name := range Priorities {
		if p == name {
			return len(Priorities) - i
		}
	}
	return 0
}

// NormalizePriority accepts high/med/low and the short forms h/m/l,
// medium, 1–3 (1 is highest) and A–C.
func NormalizePriority(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "high", "h", "1", "a", "hi":
		return "high", nil
	case "med", "m", "medium", "2", "b", "mid":
		return "med", nil
	case "low", "l", "3", "c", "lo":
		return "low", nil
	case "", "none", "-":
		return "", nil
	}
	return "", fmt.Errorf("priority must be high, med or low, got %q", s)
}

var weekdays = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

var relDays = regexp.MustCompile(`^\+(\d+)([dw])$`)

// NormalizeDate turns a date as typed into YYYY-MM-DD. It accepts ISO dates,
// today, tomorrow (tom), yesterday, +Nd / +Nw, and weekday names (the next
// such day, never today). "" and "none" return "".
func NormalizeDate(s string, now time.Time) (string, error) {
	v := strings.ToLower(strings.TrimSpace(s))
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch v {
	case "", "none", "-":
		return "", nil
	case "today", "tod":
		return day.Format(DateLayout), nil
	case "tomorrow", "tom", "tmr":
		return day.AddDate(0, 0, 1).Format(DateLayout), nil
	case "yesterday":
		return day.AddDate(0, 0, -1).Format(DateLayout), nil
	}
	if m := relDays.FindStringSubmatch(v); m != nil {
		var n int
		fmt.Sscan(m[1], &n)
		if m[2] == "w" {
			n *= 7
		}
		return day.AddDate(0, 0, n).Format(DateLayout), nil
	}
	if len(v) >= 3 {
		if wd, ok := weekdays[v[:3]]; ok {
			d := (int(wd) - int(day.Weekday()) + 7) % 7
			if d == 0 {
				d = 7
			}
			return day.AddDate(0, 0, d).Format(DateLayout), nil
		}
	}
	if d, err := time.Parse(DateLayout, v); err == nil {
		return d.Format(DateLayout), nil
	}
	return "", fmt.Errorf("date must be YYYY-MM-DD, today, tomorrow, +3d, +2w or a weekday, got %q", s)
}

// NormalizeMeta validates and normalizes a value for a well-known key. Other
// keys are only checked for a valid name.
func NormalizeMeta(key, value string, now time.Time) (string, error) {
	key = strings.ToLower(strings.TrimSpace(key))
	if !metaKey.MatchString(key) {
		return "", fmt.Errorf("invalid metadata key %q (want lowercase letters, digits, - or _)", key)
	}
	switch key {
	case MetaDue, MetaDone, MetaStart:
		return NormalizeDate(value, now)
	case MetaPri:
		return NormalizePriority(value)
	}
	return strings.TrimSpace(value), nil
}

// MetaValueStat is how often one value is used for a metadata key.
type MetaValueStat struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// MetaKeyStat is how often a metadata key is used across tasks, with its most
// common values (left empty for date keys, whose values aren't worth suggesting).
type MetaKeyStat struct {
	Key    string          `json:"key"`
	Count  int             `json:"count"`
	Values []MetaValueStat `json:"values"`
}

// maxMetaValues caps the values kept per key in MetaStats.
const maxMetaValues = 20

// MetaStats counts the metadata keys and values used by tasks in every note,
// most used first. Editors use it to suggest keys and values.
func (s *Store) MetaStats() ([]MetaKeyStat, error) {
	ns, err := s.List("", ListOpts{})
	if err != nil {
		return nil, err
	}
	keys := map[string]int{}
	vals := map[string]map[string]int{}
	for _, n := range ns {
		for _, t := range n.tasks() {
			for k, v := range t.Meta {
				keys[k]++
				if k == MetaDue || k == MetaStart || k == MetaDone {
					continue
				}
				if vals[k] == nil {
					vals[k] = map[string]int{}
				}
				vals[k][v]++
			}
		}
	}
	out := make([]MetaKeyStat, 0, len(keys))
	for k, c := range keys {
		st := MetaKeyStat{Key: k, Count: c, Values: []MetaValueStat{}}
		for v, vc := range vals[k] {
			st.Values = append(st.Values, MetaValueStat{v, vc})
		}
		sort.Slice(st.Values, func(i, j int) bool {
			a, b := st.Values[i], st.Values[j]
			if a.Count != b.Count {
				return a.Count > b.Count
			}
			return a.Value < b.Value
		})
		if len(st.Values) > maxMetaValues {
			st.Values = st.Values[:maxMetaValues]
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	return out, nil
}
