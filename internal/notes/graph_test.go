package notes

import (
	"strings"
	"testing"
)

// graphStore: a → b (twice), b → c, c → missing, journal → a; a and d
// share the tag "x".
func graphStore(t *testing.T) *Store {
	t.Helper()
	s := newStore(t)
	s.Create(KindPage, "A", []string{"x"}, "[B](../pages/b.md) [b again](b.md) [self](a.md)\n")
	s.Create(KindPage, "B", nil, "[C](../pages/c.md)\n")
	s.Create(KindPage, "C", nil, "[Nope](../pages/nope.md)\n")
	s.Create(KindPage, "D", []string{"x"}, "alone\n")
	j, _, _ := s.Journal(fixed)
	j.Body += "[A](../pages/a.md)\n"
	s.Save(j)
	return s
}

func keys(g *Graph) string {
	var ks []string
	for _, n := range g.Nodes {
		ks = append(ks, n.Key)
	}
	return strings.Join(ks, ",")
}

func edges(g *Graph) string {
	var es []string
	for _, e := range g.Edges {
		es = append(es, e.From+">"+e.To)
	}
	return strings.Join(es, ",")
}

func TestGraphGlobal(t *testing.T) {
	s := graphStore(t)
	g, err := s.Graph(GraphOpts{Journal: true})
	if err != nil {
		t.Fatal(err)
	}
	if k := keys(g); k != "journal/2026-11-16,page/a,page/b,page/c,page/d" {
		t.Errorf("nodes = %s", k)
	}
	if e := edges(g); e != "journal/2026-11-16>page/a,page/a>page/b,page/b>page/c" {
		t.Errorf("edges = %s", e)
	}
	if g.Edges[1].Count != 2 || g.Node("page/b").Degree != 2 || g.Node("page/d").Degree != 0 {
		t.Errorf("count/degree wrong: %+v", g)
	}

	// No journal, with missing notes and tags.
	g, _ = s.Graph(GraphOpts{Missing: true, Tags: true})
	if k := keys(g); k != "page/a,page/b,page/c,page/d,page/nope,tag:x" {
		t.Errorf("nodes = %s", k)
	}
	if n := g.Node("page/nope"); n.Type != NodeMissing || n.Title != "Nope" {
		t.Errorf("missing node = %+v", n)
	}
	if e := edges(g); e != "page/a>page/b,page/a>tag:x,page/b>page/c,page/c>page/nope,page/d>tag:x" {
		t.Errorf("edges = %s", e)
	}
}

func TestGraphLocal(t *testing.T) {
	s := graphStore(t)
	g, err := s.Graph(GraphOpts{Center: "page/b", Depth: 1, Journal: true})
	if err != nil || keys(g) != "page/a,page/b,page/c" {
		t.Fatalf("depth 1 = %s %v", keys(g), err)
	}
	g, _ = s.Graph(GraphOpts{Center: "page/b", Depth: 2, Journal: true, Missing: true})
	if keys(g) != "journal/2026-11-16,page/a,page/b,page/c,page/nope" || g.Node("page/nope").Dist != 2 || g.Node("page/b").Dist != 0 {
		t.Errorf("depth 2 = %s", keys(g))
	}
	// Tags are one hop, but the search doesn't go through them: d is only
	// reachable via tag:x.
	g, _ = s.Graph(GraphOpts{Center: "page/a", Depth: 3, Tags: true})
	if strings.Contains(keys(g), "page/d") || !strings.Contains(keys(g), "tag:x") {
		t.Errorf("tag hop = %s", keys(g))
	}
	// Centred on a tag, its notes are found.
	g, _ = s.Graph(GraphOpts{Center: "page/d", Depth: 1, Tags: true})
	if keys(g) != "page/d,tag:x" {
		t.Errorf("tag neighbour = %s", keys(g))
	}
	// A journal center is kept even with journal entries hidden.
	g, _ = s.Graph(GraphOpts{Center: "journal/2026-11-16"})
	if keys(g) != "journal/2026-11-16,page/a" {
		t.Errorf("journal center = %s", keys(g))
	}
	if _, err := s.Graph(GraphOpts{Center: "page/zzz"}); err == nil {
		t.Error("missing center should fail")
	}
	if _, err := s.Graph(GraphOpts{Center: "bogus"}); err == nil {
		t.Error("bad key should fail")
	}
}

func TestGraphDOT(t *testing.T) {
	s := graphStore(t)
	g, _ := s.Graph(GraphOpts{Center: "page/a", Tags: true, Missing: true})
	dot := g.DOT()
	for _, want := range []string{
		"digraph katnotes {",
		`"page/a" [label="A", penwidth=2];`,
		`"tag:x" [label="#x", shape=ellipse`,
		`"page/a" -> "page/b" [penwidth=2];`,
		`"page/a" -> "tag:x" [style=dotted, arrowhead=none];`,
	} {
		if !strings.Contains(dot, want) {
			t.Errorf("DOT missing %q:\n%s", want, dot)
		}
	}
}
