package notes

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Wiki links: "[[Page]]" or "[[Page|text]]" typed in a note body.
// ResolveWikiLinks turns them into ordinary Markdown links to pages,
// creating pages that don't exist yet.

var wikiLink = regexp.MustCompile(`\[\[([^\[\]\n|]+)(?:\|([^\[\]\n]+))?\]\]`)

// WikiResult reports what ResolveWikiLinks did. Each list holds "kind/id"
// keys, once each.
type WikiResult struct {
	Linked  []string `json:"linked"`  // existing pages linked to
	Created []string `json:"created"` // pages created
	Moved   []string `json:"moved"`   // pages that received indented lines
	Links   int      `json:"links"`   // [[…]] occurrences rewritten
	Blocks  int      `json:"blocks"`  // indented blocks moved
}

// wikiTarget is a note a link resolved to.
type wikiTarget struct {
	kind      Kind
	id, title string
	created   bool
	blocks    [][]string // moved lines, dedented, in order
}

// ResolveWikiLinks rewrites each [[Page]] (or [[Page|text]]) in the body of
// kind/id as a Markdown link to the page whose title is Page (ignoring case)
// or whose id is Page's slug, creating the page when there is none. Links in
// fenced or inline code are left alone, as are links to the note itself.
//
// With move, the lines indented under a line holding exactly one link — a
// wiki link, or a Markdown link to a note (say one an earlier run wrote) —
// move to that note: they become a new page's body, or are appended to an
// existing note under a "## From [this note](…)" heading.
func (s *Store) ResolveWikiLinks(kind Kind, id string, move bool) (*Note, WikiResult, error) {
	var res WikiResult
	n, err := s.Get(kind, id)
	if err != nil {
		return nil, res, err
	}
	pages, err := s.List(KindPage, ListOpts{})
	if err != nil {
		return nil, res, err
	}
	byTitle, byID := map[string]string{}, map[string]string{}
	for _, p := range pages {
		if _, ok := byTitle[strings.ToLower(p.Title)]; !ok {
			byTitle[strings.ToLower(p.Title)] = p.ID
		}
		byID[p.ID] = p.Title
	}
	targets := map[string]*wikiTarget{}
	var order []string
	// target finds kind/tid, creating it (titled name) when it's missing.
	target := func(tk Kind, tid, name string) (*wikiTarget, error) {
		key := Key(tk, tid)
		if t := targets[key]; t != nil {
			return t, nil
		}
		if tk == kind && tid == id {
			return nil, nil // the note itself
		}
		t := &wikiTarget{kind: tk, id: tid}
		if title, ok := byID[tid]; ok && tk == KindPage {
			t.title = title
		} else {
			p, created, err := s.Ensure(tk, tid, name)
			if err != nil {
				return nil, err
			}
			t.title, t.created = p.Title, created
			if tk == KindPage {
				byTitle[strings.ToLower(p.Title)], byID[tid] = tid, p.Title
			}
		}
		targets[key] = t
		order = append(order, key)
		return t, nil
	}
	// lookup finds or creates the page for a wiki link's target text.
	lookup := func(name string) (*wikiTarget, error) {
		pid, ok := byTitle[strings.ToLower(name)]
		if !ok {
			pid = Slugify(name)
		}
		return target(KindPage, pid, name)
	}

	lines := strings.Split(n.Body, "\n")
	out := make([]string, 0, len(lines))
	fence := ""
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			if f := t[:3]; fence == "" {
				fence = f
			} else if f == fence {
				fence = ""
			}
			out = append(out, line)
			continue
		}
		if fence != "" {
			out = append(out, line)
			continue
		}
		// Markdown links to notes already on the line.
		md := ParseLinks(&Note{Kind: kind, Body: line})
		var ms [][]int
		for _, m := range wikiLink.FindAllStringSubmatchIndex(maskCode(line), -1) {
			if hasAlnum(line[m[2]:m[3]]) {
				ms = append(ms, m)
			}
		}
		// Look targets up left to right, then rewrite right to left so
		// earlier offsets stay valid.
		tgs := make([]*wikiTarget, len(ms))
		for j, m := range ms {
			name := strings.TrimSpace(line[m[2]:m[3]])
			if tgs[j], err = lookup(name); err != nil {
				return nil, res, fmt.Errorf("[[%s]]: %w", name, err)
			}
		}
		var only *wikiTarget
		for j := len(ms) - 1; j >= 0; j-- {
			m, tg := ms[j], tgs[j]
			if tg == nil {
				continue
			}
			text := strings.TrimSpace(line[m[2]:m[3]])
			if m[4] >= 0 {
				if alias := strings.TrimSpace(line[m[4]:m[5]]); alias != "" {
					text = alias
				}
			}
			line = line[:m[0]] + LinkMarkdown(KindPage, tg.id, text) + line[m[1]:]
			res.Links++
			if len(ms) == 1 && len(md) == 0 {
				only = tg
			}
		}
		if move && len(ms) == 0 && len(md) == 1 {
			l := md[0]
			if only, err = target(l.Kind, l.ID, l.Text); err != nil {
				return nil, res, fmt.Errorf("%s: %w", Key(l.Kind, l.ID), err)
			}
		}
		out = append(out, line)
		if move && only != nil {
			if end := indentedBlock(lines, i); end > i+1 {
				only.blocks = append(only.blocks, dedent(lines[i+1:end]))
				res.Blocks++
				i = end - 1
			}
		}
	}
	if res.Links == 0 && res.Blocks == 0 {
		return n, res, nil
	}

	// Write moved lines before saving the source, so nothing is lost if a
	// target can't be saved.
	for _, key := range order {
		tg := targets[key]
		if tg.created {
			res.Created = append(res.Created, key)
		} else {
			res.Linked = append(res.Linked, key)
		}
		if len(tg.blocks) == 0 {
			continue
		}
		p, err := s.Get(tg.kind, tg.id)
		if err != nil {
			return nil, res, err
		}
		var add []string
		for _, b := range tg.blocks {
			add = append(add, b...)
		}
		text := strings.Join(add, "\n")
		if !tg.created {
			text = "## From " + LinkMarkdown(n.Kind, n.ID, n.Title) + "\n\n" + text
		}
		body := strings.TrimRight(p.Body, "\n")
		if body != "" {
			body += "\n\n"
		}
		p.Body = body + text + "\n"
		if err := s.Save(p); err != nil {
			return nil, res, fmt.Errorf("%s: %w", p.Key(), err)
		}
		res.Moved = append(res.Moved, p.Key())
	}
	n.Body = strings.Join(out, "\n")
	if err := s.Save(n); err != nil {
		return nil, res, err
	}
	return n, res, nil
}

func hasAlnum(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0
}

// wikiIndent is the width of line's leading whitespace (a tab counts 4),
// or -1 for a blank line.
func wikiIndent(line string) int {
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
	return -1
}

// indentedBlock returns the end (exclusive) of the lines after lines[at]
// that are indented more than it. Blank lines belong to the block only when
// a more-indented line follows them.
func indentedBlock(lines []string, at int) int {
	base := wikiIndent(lines[at])
	end := at + 1
	for i := at + 1; i < len(lines); i++ {
		w := wikiIndent(lines[i])
		if w < 0 {
			continue
		}
		if w <= base {
			break
		}
		end = i + 1
	}
	return end
}

// dedent removes the indentation the non-blank lines share.
func dedent(block []string) []string {
	least := -1
	for _, l := range block {
		if w := wikiIndent(l); w >= 0 && (least < 0 || w < least) {
			least = w
		}
	}
	out := make([]string, len(block))
	for i, l := range block {
		w, j := 0, 0
		for j < len(l) && w < least && (l[j] == ' ' || l[j] == '\t') {
			if l[j] == '\t' {
				w += 4
			} else {
				w++
			}
			j++
		}
		out[i] = strings.TrimRight(l[j:], " \t")
	}
	return out
}
