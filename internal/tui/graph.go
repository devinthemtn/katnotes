package tui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/list"

	"github.com/devinthemtn/katnotes/internal/notes"
)

// The graph view (:graph, gn): the GUI's graph as text. The list holds the
// notes around one note (local) or every note by how linked it is (all);
// the preview draws the selected note's neighbourhood as a tree, depth
// links deep (+ / - change it).

type graphView struct {
	of    *notes.Note // center; nil lists every note
	depth int
}

// openGraph switches the list to the graph view, around n (nil: every note).
func (m *model) openGraph(n *notes.Note) {
	m.clearViews()
	g := &graphView{depth: 1}
	if n != nil {
		c := *n
		g.of = &c
		g.depth = 2
	}
	m.graph = g
	m.list.Select(0)
	m.reload()
	m.graphStatus()
}

func (m *model) graphStatus() {
	what := "every note, most linked first"
	if m.graph.of != nil {
		what = "around " + m.graph.of.Title
	}
	m.setStatus(fmt.Sprintf("graph: %s · depth %d (+/-) · enter opens · esc closes", what, m.graph.depth))
}

// graphDepth changes the depth by d (1 to notes.MaxGraphDepth).
func (m *model) graphDepth(d int) {
	nd := min(max(m.graph.depth+d, 1), notes.MaxGraphDepth)
	if nd == m.graph.depth {
		return
	}
	m.graph.depth = nd
	if m.graph.of != nil {
		m.reload()
	}
	m.preview = ""
	m.updatePreview()
	m.graphStatus()
}

// graphItems lists the graph view's notes: by distance from the center,
// or (every note) by number of links.
func (m *model) graphItems() ([]list.Item, error) {
	o := notes.GraphOpts{Missing: true, Journal: true}
	if m.graph.of != nil {
		o.Center, o.Depth = m.graph.of.Key(), m.graph.depth
	}
	g, err := m.app.Store.Graph(o)
	if err != nil {
		return nil, err
	}
	ns := slices.Clone(g.Nodes)
	slices.SortStableFunc(ns, func(a, b notes.GraphNode) int {
		if m.graph.of != nil {
			if c := cmp.Compare(a.Dist, b.Dist); c != 0 {
				return c
			}
		} else if c := cmp.Compare(b.Degree, a.Degree); c != 0 {
			return c
		}
		return cmp.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
	})
	var items []list.Item
	for _, gn := range ns {
		if gn.Type == notes.NodeMissing {
			items = append(items, missingItem{notes.LinkRef{Link: notes.Link{Kind: gn.Kind, ID: gn.ID, Text: gn.Title}, Title: gn.Title}})
			continue
		}
		n, err := m.app.Store.Get(gn.Kind, gn.ID)
		if err != nil {
			continue
		}
		detail := fmt.Sprintf("%d link(s)", gn.Degree)
		if m.graph.of != nil {
			switch gn.Dist {
			case 0:
				detail = "center · " + detail
			case 1:
				detail = "1 link away · " + detail
			default:
				detail = fmt.Sprintf("%d links away · %s", gn.Dist, detail)
			}
		}
		items = append(items, noteItem{n: *n, match: detail})
	}
	return items, nil
}

// graphTree draws the notes within depth links of key as a tree: → for a
// link out, ← for a link in. Each note shows once, where it's nearest.
func graphTree(st *notes.Store, key string, depth int) (string, error) {
	g, err := st.Graph(notes.GraphOpts{Center: key, Depth: depth, Missing: true, Journal: true})
	if err != nil {
		return "", err
	}
	type step struct {
		to  string
		out bool
	}
	adj := map[string][]step{}
	for _, e := range g.Edges {
		adj[e.From] = append(adj[e.From], step{e.To, true})
		adj[e.To] = append(adj[e.To], step{e.From, false})
	}
	title := func(k string) string {
		n := g.Node(k)
		if n == nil {
			return k
		}
		if n.Type == notes.NodeMissing {
			return n.Title + statusStyle.Render(" (missing)")
		}
		return n.Title
	}
	// Breadth first, so each note hangs under the parent nearest the center.
	children := map[string][]step{}
	seen := map[string]bool{key: true}
	frontier := []string{key}
	for d := 0; d < depth && len(frontier) > 0; d++ {
		var next []string
		for _, k := range frontier {
			steps := slices.Clone(adj[k])
			slices.SortStableFunc(steps, func(a, b step) int {
				if a.out != b.out {
					if a.out {
						return -1
					}
					return 1
				}
				return cmp.Compare(strings.ToLower(title(a.to)), strings.ToLower(title(b.to)))
			})
			for _, s := range steps {
				if seen[s.to] {
					continue
				}
				seen[s.to] = true
				children[k] = append(children[k], s)
				next = append(next, s.to)
			}
		}
		frontier = next
	}

	var b strings.Builder
	b.WriteString("\n  " + filterStyle.Render(title(key)) + "\n")
	var walk func(k, indent string)
	walk = func(k, indent string) {
		cs := children[k]
		for i, c := range cs {
			branch, more := "├─", "│   "
			if i == len(cs)-1 {
				branch, more = "└─", "    "
			}
			arrow := "→ "
			if !c.out {
				arrow = "← "
			}
			b.WriteString("  " + indent + statusStyle.Render(branch+arrow) + title(c.to) + "\n")
			walk(c.to, indent+more)
		}
	}
	walk(key, "")
	if len(children[key]) == 0 {
		b.WriteString(statusStyle.Render("  no links to or from this note") + "\n")
	}
	b.WriteString("\n" + statusStyle.Render(fmt.Sprintf("  → links to · ← linked from · depth %d (+/-)", depth)) + "\n")
	return b.String(), nil
}

func (m *model) graphTitle() string {
	if m.graph.of == nil {
		return "graph · every note"
	}
	return "graph around " + m.graph.of.Title
}
