package notes

import (
	"fmt"
	"slices"
	"strings"
)

// The link graph: notes as nodes and the links between them as edges,
// optionally with tags and not-yet-written link targets as nodes too. Every
// frontend draws it from Store.Graph.

// Node types.
const (
	NodeNote    = "note"
	NodeTag     = "tag"
	NodeMissing = "missing" // a link target that doesn't exist yet
)

// Edge types.
const (
	EdgeLink = "link" // a note links to a note (or missing note)
	EdgeTag  = "tag"  // a note has a tag
)

// GraphOpts chooses what Store.Graph includes.
type GraphOpts struct {
	Center  string // "kind/id" for a local graph around that note; empty for every note
	Depth   int    // local graph: hops from Center, 1-3 (0 means 1)
	Tags    bool   // add tag nodes, joined to the notes that have them
	Missing bool   // add nodes for links to notes that don't exist
	Journal bool   // include journal entries (the center is always kept)
}

// MaxGraphDepth is the furthest a local graph reaches.
const MaxGraphDepth = 3

// GraphNode is a note, tag or missing note. Key is "kind/id" for notes and
// missing notes, "tag:<name>" for tags.
type GraphNode struct {
	Key    string   `json:"key"`
	Type   string   `json:"type"`
	Kind   Kind     `json:"kind,omitempty"`
	ID     string   `json:"id,omitempty"`
	Title  string   `json:"title"`
	Tags   []string `json:"tags,omitempty"`
	Degree int      `json:"degree"` // edges touching the node, in this graph
	Dist   int      `json:"dist"`   // hops from the center (0 in a global graph)
}

// GraphEdge joins two nodes by key. A link edge goes from the linking note;
// Count is how many links it stands for. A tag edge goes from note to tag.
type GraphEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// Graph is a set of nodes and edges, in a stable order (nodes by key, edges
// by from, then to).
type Graph struct {
	Center string      `json:"center,omitempty"`
	Nodes  []GraphNode `json:"nodes"`
	Edges  []GraphEdge `json:"edges"`
}

// TagKey is the graph key of a tag node.
func TagKey(tag string) string { return "tag:" + tag }

// Graph builds the link graph.
func (s *Store) Graph(o GraphOpts) (*Graph, error) {
	if o.Center != "" {
		k, id, err := ParseKey(o.Center)
		if err != nil {
			return nil, err
		}
		if _, err := s.Get(k, id); err != nil {
			return nil, err
		}
	}
	ns, err := s.List("", ListOpts{})
	if err != nil {
		return nil, err
	}
	parseAllLinks(ns)

	all := make(map[string]bool, len(ns))
	for i := range ns {
		all[ns[i].Key()] = true
	}
	nodes := map[string]*GraphNode{}
	keep := func(n *Note) bool { return o.Journal || n.Kind != KindJournal || n.Key() == o.Center }
	for i := range ns {
		n := &ns[i]
		if keep(n) {
			nodes[n.Key()] = &GraphNode{Key: n.Key(), Type: NodeNote, Kind: n.Kind, ID: n.ID, Title: n.Title, Tags: n.Tags}
		}
	}
	type pair struct{ from, to string }
	edges := map[pair]*GraphEdge{}
	add := func(from, to, typ string) {
		p := pair{from, to}
		if e := edges[p]; e != nil {
			e.Count++
			return
		}
		edges[p] = &GraphEdge{From: from, To: to, Type: typ, Count: 1}
	}
	for i := range ns {
		n := &ns[i]
		from := n.Key()
		if nodes[from] == nil {
			continue
		}
		for _, l := range n.links() {
			to := Key(l.Kind, l.ID)
			if to == from {
				continue
			}
			if nodes[to] == nil {
				if all[to] || !o.Missing || (l.Kind == KindJournal && !o.Journal) {
					continue // filtered out, or missing and not wanted
				}
				nodes[to] = &GraphNode{Key: to, Type: NodeMissing, Kind: l.Kind, ID: l.ID, Title: l.Text}
			}
			add(from, to, EdgeLink)
		}
		if o.Tags {
			for _, t := range n.Tags {
				tk := TagKey(t)
				if nodes[tk] == nil {
					nodes[tk] = &GraphNode{Key: tk, Type: NodeTag, Title: "#" + t}
				}
				add(from, tk, EdgeTag)
			}
		}
	}

	if o.Center != "" {
		depth := min(max(o.Depth, 1), MaxGraphDepth)
		adj := map[string][]string{}
		for p := range edges {
			adj[p.from] = append(adj[p.from], p.to)
			adj[p.to] = append(adj[p.to], p.from)
		}
		dist := map[string]int{o.Center: 0}
		frontier := []string{o.Center}
		for d := 1; d <= depth && len(frontier) > 0; d++ {
			var next []string
			for _, k := range frontier {
				if nodes[k].Type == NodeTag && k != o.Center {
					continue // a popular tag would pull in everything
				}
				for _, nb := range adj[k] {
					if _, seen := dist[nb]; !seen {
						dist[nb] = d
						next = append(next, nb)
					}
				}
			}
			frontier = next
		}
		for k, n := range nodes {
			if d, ok := dist[k]; ok {
				n.Dist = d
			} else {
				delete(nodes, k)
			}
		}
		for p := range edges {
			if nodes[p.from] == nil || nodes[p.to] == nil {
				delete(edges, p)
			}
		}
	}

	g := &Graph{Center: o.Center, Nodes: []GraphNode{}, Edges: []GraphEdge{}}
	for _, e := range edges {
		nodes[e.From].Degree++
		nodes[e.To].Degree++
		g.Edges = append(g.Edges, *e)
	}
	for _, n := range nodes {
		g.Nodes = append(g.Nodes, *n)
	}
	slices.SortFunc(g.Nodes, func(a, b GraphNode) int { return strings.Compare(a.Key, b.Key) })
	slices.SortFunc(g.Edges, func(a, b GraphEdge) int {
		if c := strings.Compare(a.From, b.From); c != 0 {
			return c
		}
		return strings.Compare(a.To, b.To)
	})
	return g, nil
}

// Node returns the node with key k, or nil.
func (g *Graph) Node(k string) *GraphNode {
	i, ok := slices.BinarySearchFunc(g.Nodes, k, func(n GraphNode, k string) int { return strings.Compare(n.Key, k) })
	if !ok {
		return nil
	}
	return &g.Nodes[i]
}

// DOT renders the graph in Graphviz's DOT language.
func (g *Graph) DOT() string {
	q := func(s string) string {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s) + `"`
	}
	var b strings.Builder
	b.WriteString("digraph katnotes {\n  node [shape=box, style=rounded];\n")
	for _, n := range g.Nodes {
		attr := "label=" + q(n.Title)
		switch {
		case n.Type == NodeTag:
			attr += ", shape=ellipse, color=steelblue"
		case n.Type == NodeMissing:
			attr += ", style=\"rounded,dashed\", fontcolor=gray50, color=gray50"
		case n.Key == g.Center:
			attr += ", penwidth=2"
		}
		fmt.Fprintf(&b, "  %s [%s];\n", q(n.Key), attr)
	}
	for _, e := range g.Edges {
		attr := ""
		if e.Type == EdgeTag {
			attr = " [style=dotted, arrowhead=none]"
		} else if e.Count > 1 {
			attr = fmt.Sprintf(" [penwidth=%d]", min(e.Count, 5))
		}
		fmt.Fprintf(&b, "  %s -> %s%s;\n", q(e.From), q(e.To), attr)
	}
	b.WriteString("}\n")
	return b.String()
}
