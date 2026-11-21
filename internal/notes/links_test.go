package notes

import (
	"os"
	"strings"
	"testing"
)

func TestParseLinks(t *testing.T) {
	n := &Note{Kind: KindPage, Body: strings.Join([]string{
		"see [Groceries](../pages/groceries.md) and [Day](../journal/2026-11-17.md)",
		"same dir [Other](other.md#part) and <angle> [Sp](<../pages/sp.md> \"t\")",
		"external [x](https://example.com) [m](mailto:a@b.c) img ![p](../pages/pic.md)",
		"code `[no](../pages/no.md)` but [yes](../pages/yes.md)",
		"```",
		"[fenced](../pages/fenced.md)",
		"```",
		"bad [j](../journal/notadate.md) [up](../../x.md) [esc \\] br](../pages/esc.md)",
		"[enc](../pages/caf%C3%A9.md)",
	}, "\n")}
	var got []string
	for _, l := range ParseLinks(n) {
		got = append(got, l.Text+"="+string(l.Kind)+"/"+l.ID)
	}
	want := "Groceries=page/groceries,Day=journal/2026-11-17,Other=page/other,Sp=page/sp,yes=page/yes,esc ] br=page/esc,enc=page/café"
	if strings.Join(got, ",") != want {
		t.Errorf("links =\n%s\nwant\n%s", strings.Join(got, ","), want)
	}
	// From a journal entry, a bare "x.md" is another journal file.
	j := &Note{Kind: KindJournal, Body: "[y](2026-11-16.md) [p](../pages/a.md)"}
	if ls := ParseLinks(j); len(ls) != 2 || ls[0].Kind != KindJournal || ls[1].ID != "a" {
		t.Errorf("journal links = %+v", ls)
	}
	if md := LinkMarkdown(KindPage, "a-b", "A [b]"); md != `[A \[b\]](../pages/a-b.md)` {
		t.Errorf("LinkMarkdown = %s", md)
	}
}

func TestLinksBacklinksAndRename(t *testing.T) {
	s := newStore(t)
	s.Create(KindPage, "Groceries", nil, "list\n")
	s.Create(KindPage, "Plan", nil, "buy [Groceries](../pages/groceries.md)\nalso [the list](groceries.md#top) and [Missing](../pages/missing.md)\n[Groceries](../pages/groceries.md) twice [Groceries](../pages/groceries.md)\n")
	j, _, _ := s.Journal(fixed)
	j.Body += "shop [Groceries](../pages/groceries.md)\n"
	s.Save(j)

	ls, err := s.Links(KindPage, "plan")
	if err != nil || len(ls) != 5 || !ls[0].Exists || ls[2].Exists || ls[2].Title != "Missing" {
		t.Fatalf("links = %+v %v", ls, err)
	}
	bl, err := s.Backlinks(KindPage, "groceries")
	if err != nil || len(bl) != 2 {
		t.Fatalf("backlinks = %+v %v", bl, err)
	}
	var plan Backlink
	for _, b := range bl {
		if b.Note.ID == "plan" {
			plan = b
		}
	}
	if len(plan.Lines) != 3 || plan.Lines[0].Line != 1 || plan.Lines[2].Line != 3 {
		t.Errorf("plan lines = %+v", plan.Lines)
	}

	r, changed, err := s.Rename(KindPage, "groceries", "Shopping List")
	if err != nil || r.ID != "shopping-list" || len(changed) != 2 {
		t.Fatalf("rename = %v %v %v", r, changed, err)
	}
	p, _ := s.Get(KindPage, "plan")
	want := "buy [Shopping List](../pages/shopping-list.md)\nalso [the list](shopping-list.md#top) and [Missing](../pages/missing.md)\n[Shopping List](../pages/shopping-list.md) twice [Shopping List](../pages/shopping-list.md)\n"
	if p.Body != want {
		t.Errorf("plan body =\n%q\nwant\n%q", p.Body, want)
	}
	if raw, _ := os.ReadFile(j.Path); !strings.Contains(string(raw), "[Shopping List](../pages/shopping-list.md)") {
		t.Errorf("journal not rewritten: %s", raw)
	}
	if bl, _ := s.Backlinks(KindPage, "shopping-list"); len(bl) != 2 {
		t.Errorf("backlinks after rename = %d", len(bl))
	}
}

func TestEnsure(t *testing.T) {
	s := newStore(t)
	n, created, err := s.Ensure(KindPage, "ideas-2", "Ideas")
	if err != nil || !created || n.ID != "ideas-2" || n.Title != "Ideas" {
		t.Fatalf("ensure page = %+v %v %v", n, created, err)
	}
	if _, created, _ := s.Ensure(KindPage, "ideas-2", "Other"); created {
		t.Error("created twice")
	}
	if j, created, err := s.Ensure(KindJournal, "2026-01-02", ""); err != nil || !created || j.ID != "2026-01-02" {
		t.Errorf("ensure journal = %+v %v %v", j, created, err)
	}
	if _, _, err := s.Ensure(KindJournal, "nope", ""); err == nil {
		t.Error("bad journal id accepted")
	}
}

func TestAddLink(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	mk := func(title, body string) *Note {
		n, err := s.Create(KindPage, title, nil, body)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	mk("Groceries", "milk\n")
	mk("Ideas", "x\n")
	a := mk("Plan", "# Plan\n\nsome text\n")
	body := func() string {
		n, err := s.Get(KindPage, a.ID)
		if err != nil {
			t.Fatal(err)
		}
		return n.Body
	}

	if _, added, err := s.AddLink(KindPage, "plan", KindPage, "groceries", "Groceries"); err != nil || !added {
		t.Fatalf("first link: added=%v err=%v", added, err)
	}
	want := "# Plan\n\nsome text\n\n## Links\n\n- [Groceries](../pages/groceries.md)\n"
	if got := body(); got != want {
		t.Errorf("new section:\n%q\nwant\n%q", got, want)
	}
	s.AddLink(KindPage, "plan", KindPage, "ideas", "Ideas")
	want = "# Plan\n\nsome text\n\n## Links\n\n- [Groceries](../pages/groceries.md)\n- [Ideas](../pages/ideas.md)\n"
	if got := body(); got != want {
		t.Errorf("second link:\n%q\nwant\n%q", got, want)
	}
	if _, added, err := s.AddLink(KindPage, "plan", KindPage, "ideas", "Ideas"); err != nil || added {
		t.Errorf("duplicate: added=%v err=%v", added, err)
	}
	if _, _, err := s.AddLink(KindPage, "plan", KindPage, "plan", "Plan"); err == nil {
		t.Error("self-link was allowed")
	}
	bl, err := s.Backlinks(KindPage, "ideas")
	if err != nil || len(bl) != 1 || bl[0].Note.ID != "plan" {
		t.Errorf("backlinks = %+v, %v", bl, err)
	}
}

func TestAddLinkLine(t *testing.T) {
	item := "- [B](../pages/b.md)"
	for _, c := range []struct{ name, in, want string }{
		{"empty", "", "## Links\n\n" + item + "\n"},
		{"section then heading", "# T\n\n## Links\n\n- [A](../pages/a.md)\n\n## Later\n\ntext\n",
			"# T\n\n## Links\n\n- [A](../pages/a.md)\n" + item + "\n\n## Later\n\ntext\n"},
		{"empty section then heading", "## Links\n## Later\n", "## Links\n\n" + item + "\n\n## Later\n"},
		{"subheading stays inside", "## Links\n\n### Work\n- [A](../pages/a.md)\n", "## Links\n\n### Work\n- [A](../pages/a.md)\n" + item + "\n"},
		{"heading in a fence", "```\n## Links\n```\n", "```\n## Links\n```\n\n## Links\n\n" + item + "\n"},
	} {
		if got := addLinkLine(c.in, item); got != c.want {
			t.Errorf("%s:\n%q\nwant\n%q", c.name, got, c.want)
		}
	}
}

func TestBodyLinks(t *testing.T) {
	n := &Note{Kind: KindPage, Body: strings.Join([]string{
		"See [Groceries](../pages/groceries.md) and [Ideas](ideas.md).",
		"Docs at https://go.dev/doc. and [the spec](<https://go.dev/ref/spec> \"spec\").",
		"![a cat](cat.png) [top](#top) `[code](x.md)` [mail](mailto:a@b.c) [pdf](../files/a.pdf)",
		"```",
		"[fenced](fenced.md) https://hidden.example",
		"```",
	}, "\n")}
	got := BodyLinks(n)
	type row struct {
		text, dest string
		note       bool
		id         string
	}
	want := []row{
		{"Groceries", "../pages/groceries.md", true, "groceries"},
		{"Ideas", "ideas.md", true, "ideas"},
		{"https://go.dev/doc", "https://go.dev/doc", false, ""},
		{"the spec", "https://go.dev/ref/spec", false, ""},
		{"mail", "mailto:a@b.c", false, ""},
		{"pdf", "../files/a.pdf", false, ""},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d links: %+v", len(got), got)
	}
	lines := strings.Split(n.Body, "\n")
	for i, w := range want {
		g := got[i]
		if g.Text != w.text || g.Dest != w.dest || g.Note != w.note || g.ID != w.id {
			t.Errorf("link %d = %+v, want %+v", i, g, w)
		}
		if lines[g.Line][g.TextStart:g.TextEnd] != w.text {
			t.Errorf("link %d offsets cover %q", i, lines[g.Line][g.TextStart:g.TextEnd])
		}
	}
}

func TestPlainLinks(t *testing.T) {
	in := `see [Beta](../pages/beta.md), [a \[b\]](<x y.md> "t") and ![pic](p.png)`
	if got, want := PlainLinks(in), `see Beta, a [b] and ![pic](p.png)`; got != want {
		t.Errorf("PlainLinks = %q, want %q", got, want)
	}
}

func TestBodyLinkDestOffsets(t *testing.T) {
	n := &Note{Kind: KindPage, Body: "go [Beta](<../pages/beta.md>) or https://x.org\n"}
	ls := BodyLinks(n)
	if len(ls) != 2 {
		t.Fatalf("links: %+v", ls)
	}
	line := strings.Split(n.Body, "\n")[0]
	if got := line[ls[0].DestStart:ls[0].DestEnd]; got != "<../pages/beta.md>" {
		t.Errorf("dest span = %q", got)
	}
	if ls[1].DestEnd != 0 {
		t.Errorf("a bare URL has no dest span: %+v", ls[1])
	}
}
