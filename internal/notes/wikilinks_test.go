package notes

import (
	"strings"
	"testing"
)

func TestResolveWikiLinks(t *testing.T) {
	s := newStore(t)
	s.Create(KindPage, "Groceries", nil, "# Groceries\n")
	s.Create(KindPage, "Big Plans", nil, "# Big Plans\n")
	src, _ := s.Create(KindPage, "Scratch", nil, strings.Join([]string{
		"see [[groceries]] and [[big-plans|the plan]] and [[Brand New]]",
		"again [[Brand New]] and [[A [b]]] and [[Scratch]] and [[ -- ]]",
		"code `[[Not Me]]`",
		"```",
		"[[Fenced]]",
		"```",
	}, "\n")+"\n")
	n, res, err := s.ResolveWikiLinks(KindPage, src.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"see [groceries](../pages/groceries.md) and [the plan](../pages/big-plans.md) and [Brand New](../pages/brand-new.md)",
		"again [Brand New](../pages/brand-new.md) and [[A [b]]] and [[Scratch]] and [[ -- ]]",
		"code `[[Not Me]]`",
		"```",
		"[[Fenced]]",
		"```",
	}, "\n")
	if !strings.HasPrefix(n.Body, want) {
		t.Errorf("body =\n%s\nwant\n%s", n.Body, want)
	}
	if res.Links != 4 || strings.Join(res.Created, ",") != "page/brand-new" ||
		strings.Join(res.Linked, ",") != "page/groceries,page/big-plans" {
		t.Errorf("result = %+v", res)
	}
	if p, err := s.Get(KindPage, "brand-new"); err != nil || p.Title != "Brand New" {
		t.Errorf("created page = %+v %v", p, err)
	}
	for _, id := range []string{"not-me", "fenced"} {
		if _, err := s.Get(KindPage, id); err == nil {
			t.Errorf("%s was created", id)
		}
	}
	// Nothing left to do: no change.
	if _, res, err := s.ResolveWikiLinks(KindPage, src.ID, false); err != nil || res.Links != 0 {
		t.Errorf("second run = %+v %v", res, err)
	}
}

func TestResolveWikiLinksMove(t *testing.T) {
	s := newStore(t)
	s.Create(KindPage, "Groceries", nil, "# Groceries\n\n- bread\n")
	body := strings.Join([]string{
		"# Shopping",
		"",
		"- [[Groceries]]",
		"  - milk",
		"",
		"    - skim",
		"- [[Hardware]]",
		"\t- nails",
		"- [[Groceries]] later",
		"  - eggs",
		"- [[Groceries]] and [[Hardware]]",
		"  - stays",
		"done",
	}, "\n") + "\n"
	src, _ := s.Create(KindPage, "Shopping", nil, body)

	// Without move the indented lines stay.
	if n, _, _ := s.ResolveWikiLinks(KindPage, src.ID, false); !strings.Contains(n.Body, "  - milk") {
		t.Fatalf("moved without move:\n%s", n.Body)
	}
	s = newStore(t)
	s.Create(KindPage, "Groceries", nil, "# Groceries\n\n- bread\n")
	src, _ = s.Create(KindPage, "Shopping 2", nil, body)
	n, res, err := s.ResolveWikiLinks(KindPage, src.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	wantSrc := strings.Join([]string{
		"# Shopping",
		"",
		"- [Groceries](../pages/groceries.md)",
		"- [Hardware](../pages/hardware.md)",
		"- [Groceries](../pages/groceries.md) later",
		"- [Groceries](../pages/groceries.md) and [Hardware](../pages/hardware.md)",
		"  - stays",
		"done",
	}, "\n")
	if strings.TrimRight(n.Body, "\n") != wantSrc {
		t.Errorf("source =\n%s\nwant\n%s", n.Body, wantSrc)
	}
	if res.Blocks != 3 {
		t.Errorf("blocks = %d", res.Blocks)
	}
	g, _ := s.Get(KindPage, "groceries")
	wantG := "# Groceries\n\n- bread\n\n## From [Shopping 2](../pages/shopping-2.md)\n\n- milk\n\n  - skim\n- eggs\n"
	if g.Body != wantG {
		t.Errorf("groceries =\n%q\nwant\n%q", g.Body, wantG)
	}
	h, _ := s.Get(KindPage, "hardware")
	if h.Body != "# Hardware\n\n- nails\n" {
		t.Errorf("hardware = %q", h.Body)
	}
}

// Lines under a link an earlier run (or the GUI's [[ picker) already wrote
// move too.
func TestResolveWikiLinksMoveConverted(t *testing.T) {
	s := newStore(t)
	s.Create(KindPage, "Groceries", nil, "# Groceries\n")
	src, _ := s.Create(KindPage, "Plan", nil, strings.Join([]string{
		"- [Groceries](../pages/groceries.md)",
		"  - milk",
		"- [Gone](../pages/gone.md)",
		"  - new page body",
		"- [Groceries](../pages/groceries.md) [Gone](../pages/gone.md)",
		"  - two links: stays",
		"- [web](https://example.com)",
		"  - external: stays",
	}, "\n")+"\n")
	n, res, err := s.ResolveWikiLinks(KindPage, src.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Blocks != 2 || strings.Contains(n.Body, "milk") || strings.Contains(n.Body, "new page body") ||
		!strings.Contains(n.Body, "two links: stays") || !strings.Contains(n.Body, "external: stays") {
		t.Fatalf("result %+v body:\n%s", res, n.Body)
	}
	if g, _ := s.Get(KindPage, "groceries"); !strings.HasSuffix(g.Body, "## From [Plan](../pages/plan.md)\n\n- milk\n") {
		t.Errorf("groceries = %q", g.Body)
	}
	if g, err := s.Get(KindPage, "gone"); err != nil || g.Body != "# Gone\n\n- new page body\n" {
		t.Errorf("gone = %+v %v", g, err)
	}
	// Without move, Markdown links are left alone.
	src, _ = s.Create(KindPage, "Plan 2", nil, "- [Groceries](../pages/groceries.md)\n  - eggs\n")
	if _, res, _ := s.ResolveWikiLinks(KindPage, src.ID, false); res.Blocks != 0 || res.Links != 0 {
		t.Errorf("no move: %+v", res)
	}
}
