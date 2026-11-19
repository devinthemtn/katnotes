package notes

import (
	"flag"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

// A generated notes folder the size of a long-used one: pages and journal
// days with tags, tasks (some with metadata, some nested or quoted) and links
// between notes in every form KatNotes understands, plus look-alikes inside
// code and images that must be ignored. corpusInfo records what a correct
// Store should report, so the scale test can check it.

var scaleOut = flag.String("scale.out", "", "write a 10k-note test corpus to this folder and exit (for trying the apps by hand)")

type corpusInfo struct {
	pages, days []string            // ids
	titles      map[string]string   // key -> title
	backlinks   map[string][]string // target key -> sorted source keys
	outLinks    map[string]int      // source key -> links to notes (missing ones too)
	open, done  int
	tags        map[string]int
	marker      int // notes containing the word "zephyrine"
	phrase      int // notes containing "quarterly harvest review"
}

var (
	corpusWords = strings.Fields(`the a of to and in note plan idea draft meeting week project garden
		budget travel book read write call email list review fix ship design sketch kitchen
		morning evening walk river mountain coffee tea market school music film code server`)
	corpusTags = []string{"work", "home", "idea", "go", "garden", "travel", "books", "health",
		"money", "music", "family", "errand", "draft", "meeting", "reading", "café"}
	corpusPri = []string{"high", "med", "low"}
)

// genCorpus writes pages and journal days straight into s.Root. Files are
// written directly (not through Store) so building 10k notes is quick and
// the reading side is what gets exercised.
func genCorpus(tb testing.TB, s *Store, pages, days int) corpusInfo {
	tb.Helper()
	if err := s.Init(); err != nil {
		tb.Fatal(err)
	}
	r := rand.New(rand.NewSource(42))
	info := corpusInfo{titles: map[string]string{}, outLinks: map[string]int{}, tags: map[string]int{}}
	back := map[string]map[string]bool{}

	for i := 0; i < pages; i++ {
		w := corpusWords[r.Intn(len(corpusWords))]
		title := fmt.Sprintf("%s %s %d", strings.ToUpper(w[:1])+w[1:], corpusWords[r.Intn(len(corpusWords))], i)
		if i == 7 {
			title = "Café Notes" // a Unicode id, linked both raw and %-escaped
		}
		id := Slugify(title)
		info.pages = append(info.pages, id)
		info.titles[Key(KindPage, id)] = title
	}
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local)
	for i := 0; i < days; i++ {
		id := start.AddDate(0, 0, i).Format(JournalIDLayout)
		info.days = append(info.days, id)
		info.titles[Key(KindJournal, id)] = id
	}

	pick := func() (Kind, string) {
		if r.Intn(4) == 0 {
			return KindJournal, info.days[r.Intn(len(info.days))]
		}
		return KindPage, info.pages[r.Intn(len(info.pages))]
	}
	dest := func(from, to Kind, id string) string {
		if r.Intn(3) == 0 && strings.ContainsRune(id, 'é') {
			id = url.PathEscape(id)
		}
		switch r.Intn(4) {
		case 1:
			if from == to {
				return id + ".md" // same folder
			}
		case 2:
			return "<" + LinkDest(to, id) + ">"
		case 3:
			return LinkDest(to, id) + "#details"
		}
		return LinkDest(to, id)
	}

	write := func(kind Kind, id string, idx int) {
		key := Key(kind, id)
		var body strings.Builder
		if kind == KindJournal {
			body.WriteString("# " + id + "\n\n")
		} else {
			body.WriteString("# " + info.titles[key] + "\n\n")
		}
		// Tasks.
		ntasks := 0
		if r.Intn(3) > 0 {
			ntasks = r.Intn(9)
		}
		for t := 0; t < ntasks; t++ {
			done := r.Intn(3) == 0
			box := "[ ]"
			if done {
				box = "[x]"
				info.done++
			} else {
				info.open++
			}
			prefix := []string{"- ", "  - ", "> - ", "1. ", "* "}[r.Intn(5)]
			meta := ""
			switch r.Intn(4) {
			case 1:
				meta = fmt.Sprintf(" {due:2026-%02d-%02d pri:%s}", 1+r.Intn(12), 1+r.Intn(28), corpusPri[r.Intn(3)])
			case 2:
				meta = fmt.Sprintf(` {due:2026-11-%02d note:"ask %s"}`, 1+r.Intn(28), corpusWords[r.Intn(len(corpusWords))])
			}
			fmt.Fprintf(&body, "%s%s %s task %d%s\n", prefix, box, corpusWords[r.Intn(len(corpusWords))], t, meta)
		}
		if ntasks > 0 {
			body.WriteString("- [ ]\n-[ ] not a task\n\n") // look-alikes
		}
		// Paragraphs with links, about 5 KB in all.
		linked := map[string]bool{}
		nlinks := r.Intn(11)
		for body.Len() < 4500 {
			for w := 0; w < 40; w++ {
				body.WriteString(corpusWords[r.Intn(len(corpusWords))] + " ")
			}
			if nlinks > 0 {
				nlinks--
				tk, tid := pick()
				tkey := Key(tk, tid)
				if r.Intn(15) == 0 {
					tk, tid, tkey = KindPage, fmt.Sprintf("missing-%d", r.Intn(50)), ""
				}
				if tkey == key {
					tk, tid, tkey = KindPage, "missing-self", ""
				}
				fmt.Fprintf(&body, "see [%s](%s) ", info.titles[tkey], dest(kind, tk, tid))
				info.outLinks[key]++
				if tkey != "" {
					linked[tkey] = true
				}
			}
			body.WriteString("\n\n")
		}
		if idx%50 == 0 {
			body.WriteString("A zephyrine morning.\n")
			info.marker++
		}
		if idx%70 == 0 {
			body.WriteString("Notes from the Quarterly Harvest Review.\n")
			info.phrase++
		}
		// Look-alikes that must not count as links or tasks.
		other := info.pages[r.Intn(len(info.pages))]
		fmt.Fprintf(&body, "Inline `[code](%s)` and ![img](%s).\n\n```\n- [ ] fenced task\n[fenced](%s)\n```\n",
			LinkDest(KindPage, other), LinkDest(KindPage, other), LinkDest(KindPage, other))
		for t := range linked {
			if back[t] == nil {
				back[t] = map[string]bool{}
			}
			back[t][key] = true
		}

		// Frontmatter.
		var tags []string
		for _, i := range r.Perm(len(corpusTags))[:r.Intn(4)] {
			tags = append(tags, corpusTags[i])
			info.tags[corpusTags[i]]++
		}
		ts := start.Add(time.Duration(idx) * time.Hour).Format(time.RFC3339)
		var fm strings.Builder
		fmt.Fprintf(&fm, "---\ntitle: %q\ntags: [%s]\ncreated: %q\nupdated: %q\n", info.titles[key], strings.Join(tags, ", "), ts, ts)
		if ntasks > 0 {
			fm.WriteString("contains_task: true\n")
		}
		if r.Intn(40) == 0 {
			fm.WriteString("pinned: true\n")
		}
		if r.Intn(5) == 0 {
			fm.WriteString("status: draft\naliases: [a, b]\n")
		}
		fm.WriteString("---\n")
		if err := os.WriteFile(s.Path(kind, id), []byte(fm.String()+body.String()), 0o644); err != nil {
			tb.Fatal(err)
		}
	}
	for i, id := range info.pages {
		write(KindPage, id, i)
	}
	for i, id := range info.days {
		write(KindJournal, id, pages+i)
	}

	info.backlinks = map[string][]string{}
	for t, srcs := range back {
		for k := range srcs {
			info.backlinks[t] = append(info.backlinks[t], k)
		}
		sort.Strings(info.backlinks[t])
	}
	return info
}

func backlinkKeys(t *testing.T, s *Store, kind Kind, id string) []string {
	t.Helper()
	bl, err := s.Backlinks(kind, id)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, b := range bl {
		out = append(out, b.Note.Key())
	}
	sort.Strings(out)
	return out
}

func TestScaleCorpus(t *testing.T) {
	if *scaleOut != "" {
		s := New(*scaleOut)
		genCorpus(t, s, 8000, 2000)
		t.Logf("wrote 10000 notes to %s", *scaleOut)
		return
	}
	s := newStore(t)
	info := genCorpus(t, s, 1600, 400)

	all, err := s.List("", ListOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2000 {
		t.Fatalf("listed %d notes, want 2000", len(all))
	}
	for _, n := range all {
		if n.Problem != "" {
			t.Fatalf("%s: %s", n.Key(), n.Problem)
		}
	}

	// Backlinks: every 10th note, the café page (linked raw and %-escaped)
	// and the 20 most linked-to.
	keys := append(prefixed(KindPage, info.pages), prefixed(KindJournal, info.days)...)
	sample := []string{Key(KindPage, "café-notes")}
	for i := 0; i < len(keys); i += 10 {
		sample = append(sample, keys[i])
	}
	byLinks := slices.Clone(keys)
	sort.SliceStable(byLinks, func(i, j int) bool { return len(info.backlinks[byLinks[i]]) > len(info.backlinks[byLinks[j]]) })
	sample = append(sample, byLinks[:20]...)
	for _, key := range sample {
		k, id, _ := ParseKey(key)
		got, want := backlinkKeys(t, s, k, id), info.backlinks[key]
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Fatalf("backlinks of %s:\n got %v\nwant %v", key, got, want)
		}
	}
	// Outgoing links for a sample.
	for _, id := range info.pages[:100] {
		ls, err := s.Links(KindPage, id)
		if err != nil {
			t.Fatal(err)
		}
		if len(ls) != info.outLinks[Key(KindPage, id)] {
			t.Fatalf("links of %s: %d, want %d", id, len(ls), info.outLinks[Key(KindPage, id)])
		}
	}

	// Tasks.
	nts, err := s.Tasks("")
	if err != nil {
		t.Fatal(err)
	}
	open, done := 0, 0
	for _, nt := range nts {
		open += nt.Open
		done += nt.Done
	}
	if open != info.open || done != info.done {
		t.Errorf("Tasks: %d open %d done, want %d %d", open, done, info.open, info.done)
	}
	if refs, _ := s.AllTasks(TaskFilter{}); len(refs) != info.open {
		t.Errorf("AllTasks: %d, want %d open", len(refs), info.open)
	}
	if refs, _ := s.AllTasks(TaskFilter{IncludeDone: true}); len(refs) != info.open+info.done {
		t.Errorf("AllTasks with done: %d, want %d", len(refs), info.open+info.done)
	}
	if ch, _ := s.SyncTaskFlags(true); len(ch) != 0 {
		t.Errorf("SyncTaskFlags wants to change %d notes, e.g. %v", len(ch), ch[0])
	}

	// Tags and search.
	tags, _ := s.Tags()
	for tag, c := range info.tags {
		if tags[tag] != c {
			t.Errorf("tag %s: %d, want %d", tag, tags[tag], c)
		}
	}
	if ms, _ := s.Find(FindQuery{Mode: SearchText, Text: "zephyrine"}); len(ms) != info.marker {
		t.Errorf("text search: %d, want %d", len(ms), info.marker)
	}
	if ms, _ := s.Find(FindQuery{Mode: SearchText, Text: `"quarterly harvest review"`}); len(ms) != info.phrase {
		t.Errorf("phrase search: %d, want %d", len(ms), info.phrase)
	}
	if ms, _ := s.Find(FindQuery{Mode: SearchTags, Text: "work"}); len(ms) != info.tags["work"] {
		t.Errorf("tag search: %d, want %d", len(ms), info.tags["work"])
	}
	if ms, _ := s.Find(FindQuery{Mode: SearchName, Text: "café"}); len(ms) == 0 || ms[0].Note.ID != "café-notes" {
		t.Errorf("name search for café: %v", ms)
	}

	// Rename the most linked-to page: every link follows it.
	var target string
	for _, id := range info.pages {
		if len(info.backlinks[Key(KindPage, id)]) > len(info.backlinks[Key(KindPage, target)]) {
			target = id
		}
	}
	srcs := info.backlinks[Key(KindPage, target)]
	n, changed, err := s.Rename(KindPage, target, "Renamed Hub Page")
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != len(srcs) {
		t.Errorf("rename changed %d notes, want %d", len(changed), len(srcs))
	}
	if got := backlinkKeys(t, s, KindPage, n.ID); strings.Join(got, " ") != strings.Join(srcs, " ") {
		t.Errorf("backlinks after rename:\n got %v\nwant %v", got, srcs)
	}
	if got := backlinkKeys(t, s, KindPage, target); len(got) != 0 {
		t.Errorf("links still point at the old id: %v", got)
	}
}

func prefixed(k Kind, ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = Key(k, id)
	}
	return out
}

// BenchmarkScale times the whole-folder operations on 10k notes (8k pages,
// 2k journal days, ~5 KB each). Warm runs reuse the parse cache, as the apps
// do; Cold runs start from a new Store each time.
func BenchmarkScale(b *testing.B) {
	root := b.TempDir()
	s := New(root)
	info := genCorpus(b, s, 8000, 2000)
	s.List("", ListOpts{})
	hub := info.pages[0]
	for _, id := range info.pages {
		if len(info.backlinks[Key(KindPage, id)]) > len(info.backlinks[Key(KindPage, hub)]) {
			hub = id
		}
	}
	run := func(name string, f func()) {
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				f()
			}
		})
	}
	run("ListCold", func() { New(root).List("", ListOpts{}) })
	run("List", func() { s.List("", ListOpts{}) })
	run("FindName", func() { s.Find(FindQuery{Mode: SearchName, Text: "garden 12", Kind: KindPage}) })
	run("FindText", func() { s.Find(FindQuery{Mode: SearchText, Text: "zephyrine"}) })
	// Worst case: words in nearly every note, so every note is scanned line by line.
	run("FindTextCommon", func() { s.Find(FindQuery{Mode: SearchText, Text: `coffee "river walk"`}) })
	run("FindTags", func() { s.Find(FindQuery{Mode: SearchTags, Text: "work -home"}) })
	run("Backlinks", func() { s.Backlinks(KindPage, hub) })
	run("Links", func() { s.Links(KindPage, hub) })
	run("Tasks", func() { s.Tasks("") })
	run("AllTasks", func() { s.AllTasks(TaskFilter{}) })
	run("MetaStats", func() { s.MetaStats() })
	run("Tags", func() { s.SortedTags() })
	cur, other := hub, "Renamed Hub Page"
	run("Rename", func() {
		n, _, err := s.Rename(KindPage, cur, other)
		if err != nil {
			b.Fatal(err)
		}
		cur, other = n.ID, info.titles[Key(KindPage, hub)]
		if n.ID == hub {
			other = "Renamed Hub Page"
		}
	})
}

// The cached links and tasks follow the file: an outside edit is seen, and a
// caller editing a listed note's Body doesn't get the old parse.
func TestDerivedFollowsEdits(t *testing.T) {
	s := newStore(t)
	s.Create(KindPage, "Hub", nil, "")
	src, _ := s.Create(KindPage, "Src", nil, "# Src\n\nnothing yet\n")
	if bl, _ := s.Backlinks(KindPage, "hub"); len(bl) != 0 {
		t.Fatalf("backlinks before: %v", bl)
	}
	if ts, _ := s.Tasks(""); len(ts) != 0 {
		t.Fatalf("tasks before: %v", ts)
	}
	// Another program adds a link and a task.
	data, _ := os.ReadFile(src.Path)
	data = append(data, "see [Hub](hub.md)\n- [ ] new task\n"...)
	os.WriteFile(src.Path, data, 0o644)
	later := time.Now().Add(time.Minute)
	os.Chtimes(src.Path, later, later)
	if bl, _ := s.Backlinks(KindPage, "hub"); len(bl) != 1 {
		t.Errorf("outside edit: backlinks %v", bl)
	}
	if ts, _ := s.Tasks(""); len(ts) != 1 || ts[0].Open != 1 {
		t.Errorf("outside edit: tasks %v", ts)
	}
	// A listed note with its body changed in memory is parsed afresh.
	ns, _ := s.List(KindPage, ListOpts{})
	for i := range ns {
		if ns[i].ID == "src" {
			ns[i].Body = "- [ ] a\n- [ ] b\n"
			if got := len(ns[i].tasks()); got != 2 {
				t.Errorf("edited body: %d tasks, want 2", got)
			}
			if got := len(ns[i].links()); got != 0 {
				t.Errorf("edited body: %d links, want 0", got)
			}
		}
	}
}
