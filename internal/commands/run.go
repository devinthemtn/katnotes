package commands

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/devinthemtn/katnotes/internal/notes"
	"github.com/devinthemtn/katnotes/internal/recent"
)

// Run parses and runs a command line. UI commands come back with Result.UI
// set for the frontend to carry out.
func Run(env *Env, input string) (Result, error) {
	p, err := Parse(env.App, input)
	if err != nil {
		return Result{}, err
	}
	if p.Spec.NeedsNote && env.Note == nil {
		return Result{}, fmt.Errorf("%s needs a note: select or open one first", p.Spec.Name)
	}
	if p.Spec.UI || p.Spec.run == nil {
		return Result{Command: p.Spec.Name, UI: p.Spec.Name, Args: p.Args}, nil
	}
	r, err := p.Spec.run(env, p.Args)
	r.Command = p.Spec.Name
	if r.Args == nil {
		r.Args = p.Args
	}
	return r, err
}

// Exec runs the store command name with args already split, one value per
// argument (a Rest argument is a single string). The CLI uses it so cobra's
// argument handling and quoting apply instead of Parse's.
func Exec(env *Env, name string, args []string) (Result, error) {
	for _, s := range catalog {
		if s.Name != name || s.run == nil || !s.inApp(env.App) {
			continue
		}
		if s.NeedsNote && env.Note == nil {
			return Result{}, fmt.Errorf("%s needs a note", s.Name)
		}
		r, err := s.run(env, args)
		r.Command = s.Name
		if r.Args == nil {
			r.Args = args
		}
		return r, err
	}
	return Result{}, fmt.Errorf("no command %q", name)
}

// ResolveNote finds a note from what was typed: "kind/id", an exact title
// (pages before journal entries), an id, or a title prefix or substring that
// only one note has.
func ResolveNote(st *notes.Store, q string) (*notes.Note, error) {
	q = strings.TrimSpace(strings.Trim(strings.TrimSpace(q), `"`))
	if q == "" {
		return nil, fmt.Errorf("which note?")
	}
	if k, id, ok := strings.Cut(q, "/"); ok {
		if kind, err := notes.ParseKind(k); err == nil {
			if n, err := st.Get(kind, id); err == nil {
				return n, nil
			}
		}
	}
	ns, err := st.List("", notes.ListOpts{})
	if err != nil {
		return nil, err
	}
	lq := strings.ToLower(q)
	pick := func(ok func(n *notes.Note) bool) []*notes.Note {
		var out []*notes.Note
		for _, kind := range []notes.Kind{notes.KindPage, notes.KindJournal} {
			for i := range ns {
				if ns[i].Kind == kind && ok(&ns[i]) {
					out = append(out, &ns[i])
				}
			}
		}
		return out
	}
	for _, try := range []func(n *notes.Note) bool{
		func(n *notes.Note) bool { return strings.EqualFold(n.Title, q) },
		func(n *notes.Note) bool { return strings.EqualFold(n.ID, q) },
		func(n *notes.Note) bool { return strings.HasPrefix(strings.ToLower(n.Title), lq) },
		func(n *notes.Note) bool { return strings.Contains(strings.ToLower(n.Title), lq) },
	} {
		found := pick(try)
		switch {
		case len(found) == 1, len(found) > 1 && strings.EqualFold(found[0].Title, q):
			return st.Get(found[0].Kind, found[0].ID)
		case len(found) > 1:
			names := []string{}
			for i, n := range found {
				if i == 3 {
					names = append(names, "…")
					break
				}
				names = append(names, fmt.Sprintf("%q", n.Title))
			}
			return nil, fmt.Errorf("%q matches %d notes: %s", q, len(found), strings.Join(names, ", "))
		}
	}
	return nil, fmt.Errorf("no note matches %q", q)
}

// ParseDay reads a journal day: "", today, yesterday, tomorrow, +N / -N
// (days from today), a 2006-01-02 date or a date in format (the date_format
// setting, e.g. dd/mm/yyyy).
func ParseDay(now time.Time, s, format string) (time.Time, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", "today":
		return now, nil
	case "yesterday":
		return now.AddDate(0, 0, -1), nil
	case "tomorrow":
		return now.AddDate(0, 0, 1), nil
	}
	if (s[0] == '+' || s[0] == '-') && len(s) > 1 {
		if d, err := strconv.Atoi(s); err == nil {
			return now.AddDate(0, 0, d), nil
		}
	}
	if t, err := notes.ParseDate(format, s, now.Location()); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("journal: can't read %q as a day (today, yesterday, tomorrow, +N, -N, %s or YYYY-MM-DD)", s, notes.DatePattern(format))
}

func runOpen(env *Env, args []string) (Result, error) {
	n, err := ResolveNote(env.Store, args[0])
	if err != nil {
		return Result{}, err
	}
	return Result{Note: n, Status: "opened " + n.Key()}, nil
}

func runNew(env *Env, args []string) (Result, error) {
	n, err := env.Store.Create(notes.KindPage, args[0], nil, "")
	if err != nil {
		return Result{}, err
	}
	return Result{Note: n, Changed: true, Edit: true, Status: "created " + n.Key()}, nil
}

func runJournal(env *Env, args []string) (Result, error) {
	day := ""
	if len(args) > 0 {
		day = args[0]
	}
	d, err := ParseDay(env.now(), day, env.Store.DateFormat)
	if err != nil {
		return Result{}, err
	}
	n, created, err := env.Store.Journal(d)
	if err != nil {
		return Result{}, err
	}
	r := Result{Note: n, Changed: created, Edit: created, Status: "opened " + n.Key()}
	if created {
		r.Status = "created " + n.Key()
	}
	return r, nil
}

func hashTags(tags []string) string { return "#" + strings.Join(tags, " #") }

func runTagAdd(env *Env, args []string) (Result, error) {
	n, err := env.Store.Get(env.Note.Kind, env.Note.ID)
	if err != nil {
		return Result{}, err
	}
	add := notes.NormalizeTags(args)
	var added []string
	for _, t := range add {
		if !n.HasTag(t) {
			added = append(added, t)
		}
	}
	if len(added) == 0 {
		return Result{Note: n, Status: n.Key() + " already has " + hashTags(add)}, nil
	}
	n.Tags = append(n.Tags, added...)
	if err := env.Store.Save(n); err != nil {
		return Result{}, err
	}
	return Result{Note: n, Changed: true, Status: "tagged " + n.Key() + " " + hashTags(added)}, nil
}

func runTagRm(env *Env, args []string) (Result, error) {
	n, err := env.Store.Get(env.Note.Kind, env.Note.ID)
	if err != nil {
		return Result{}, err
	}
	rm := map[string]bool{}
	for _, t := range notes.NormalizeTags(args) {
		rm[t] = true
	}
	var keep, removed []string
	for _, t := range n.Tags {
		if rm[t] {
			removed = append(removed, t)
		} else {
			keep = append(keep, t)
		}
	}
	if len(removed) == 0 {
		return Result{}, fmt.Errorf("%s has no tag %s", n.Key(), hashTags(notes.NormalizeTags(args)))
	}
	if keep == nil {
		keep = []string{}
	}
	n.Tags = keep
	if err := env.Store.Save(n); err != nil {
		return Result{}, err
	}
	return Result{Note: n, Changed: true, Status: "removed " + hashTags(removed) + " from " + n.Key()}, nil
}

func runSet(env *Env, args []string) (Result, error) {
	field, value := args[0], args[1]
	n, relinked, err := env.Store.SetField(env.Note, field, value)
	if err != nil {
		return Result{}, err
	}
	r := Result{Note: n, Changed: true, Relinked: relinked,
		Status: fmt.Sprintf("set %s = %s on %s", field, FieldText(n, field), n.Key())}
	if field == "title" && n.Key() != env.Note.Key() {
		r.Status = "renamed to " + n.Key()
		recent.Rename(env.recentPath(), env.Note.Key(), n.Key()) //nolint:errcheck // a convenience
	}
	r.Status += relinkedText(relinked)
	return r, nil
}

func runUnset(env *Env, args []string) (Result, error) {
	n, err := env.Store.UnsetField(env.Note, args[0])
	if err != nil {
		return Result{}, err
	}
	return Result{Note: n, Changed: true, Status: "removed " + args[0] + " from " + n.Key()}, nil
}

func relinkedText(relinked []string) string {
	if len(relinked) == 0 {
		return ""
	}
	return fmt.Sprintf(" · updated links in %d note(s)", len(relinked))
}

func runRename(env *Env, args []string) (Result, error) {
	n, relinked, err := env.Store.Rename(env.Note.Kind, env.Note.ID, args[0])
	if err != nil {
		return Result{}, err
	}
	recent.Rename(env.recentPath(), env.Note.Key(), n.Key()) //nolint:errcheck // a convenience
	return Result{Note: n, Changed: true, Relinked: relinked,
		Status: "renamed to " + n.Key() + relinkedText(relinked)}, nil
}

// runTags replaces the note's tags with a comma or space separated list;
// an empty one clears them.
func runTags(env *Env, args []string) (Result, error) {
	n, err := env.Store.Get(env.Note.Kind, env.Note.ID)
	if err != nil {
		return Result{}, err
	}
	var raw []string
	if len(args) > 0 {
		raw = strings.FieldsFunc(args[0], func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
	}
	tags := notes.NormalizeTags(raw)
	if slices.Equal(tags, notes.NormalizeTags(n.Tags)) {
		return Result{Note: n, Status: "tags unchanged"}, nil
	}
	n.Tags = tags
	if err := env.Store.Save(n); err != nil {
		return Result{}, err
	}
	status := "cleared the tags of " + n.Key()
	if len(tags) > 0 {
		status = "tags for " + n.Key() + ": " + hashTags(tags)
	}
	return Result{Note: n, Changed: true, Status: status}, nil
}

// runLink adds a link to another note under the note's "## Links" section.
func runLink(env *Env, args []string) (Result, error) {
	to, err := ResolveNote(env.Store, args[0])
	if err != nil {
		return Result{}, err
	}
	n, added, err := env.Store.AddLink(env.Note.Kind, env.Note.ID, to.Kind, to.ID, to.Title)
	if err != nil {
		return Result{}, err
	}
	if !added {
		return Result{Note: n, Status: n.Key() + " already links to " + to.Key()}, nil
	}
	return Result{Note: n, Changed: true, Status: "linked " + n.Key() + " → " + to.Key()}, nil
}

// runWikiLinks turns the note's [[page]] text into links ("move" also moves
// the lines indented under each one to its page).
func runWikiLinks(env *Env, args []string) (Result, error) {
	move := false
	for _, a := range args {
		switch strings.ToLower(strings.TrimSpace(a)) {
		case "move":
			move = true
		case "":
		default:
			return Result{}, fmt.Errorf("wikilinks: unknown option %q (want move)", a)
		}
	}
	n, res, err := env.Store.ResolveWikiLinks(env.Note.Kind, env.Note.ID, move)
	if err != nil {
		return Result{}, err
	}
	if res.Links == 0 && res.Blocks == 0 {
		msg := "no [[links]] in " + n.Key()
		if move {
			msg = "no [[links]] or indented lines under a link in " + n.Key()
		}
		return Result{Note: n, Status: msg}, nil
	}
	var parts []string
	if res.Links > 0 {
		parts = append(parts, fmt.Sprintf("%d link(s)", res.Links))
	}
	if len(res.Created) > 0 {
		parts = append(parts, "created "+strings.Join(res.Created, ", "))
	}
	if res.Blocks > 0 {
		parts = append(parts, fmt.Sprintf("moved %d block(s) to %s", res.Blocks, strings.Join(res.Moved, ", ")))
	}
	status := "wikilinks " + n.Key() + ": " + strings.Join(parts, " · ")
	return Result{Note: n, Changed: true, Status: status}, nil
}

func runCopyLink(env *Env, _ []string) (Result, error) {
	n := env.Note
	link := notes.LinkMarkdown(n.Kind, n.ID, n.Title)
	return Result{Clipboard: link, Status: "copied " + link}, nil
}

func runLinks(env *Env, _ []string) (Result, error) {
	refs, err := env.Store.Links(env.Note.Kind, env.Note.ID)
	if err != nil {
		return Result{}, err
	}
	r := Result{UI: "links", PickTitle: "Links from " + env.Note.Title, Pick: []Choice{}}
	seen := map[string]bool{}
	for _, l := range refs {
		k := notes.Key(l.Kind, l.ID)
		if seen[k] {
			continue
		}
		seen[k] = true
		d := k
		if !l.Exists {
			d += " · missing"
		}
		r.Pick = append(r.Pick, Choice{Kind: l.Kind, ID: l.ID, Title: l.Title, Detail: d, Exists: l.Exists})
	}
	r.Status = fmt.Sprintf("%d link(s) from %s", len(r.Pick), env.Note.Key())
	return r, nil
}

func runBacklinks(env *Env, _ []string) (Result, error) {
	bl, err := env.Store.Backlinks(env.Note.Kind, env.Note.ID)
	if err != nil {
		return Result{}, err
	}
	r := Result{UI: "backlinks", PickTitle: "Links to " + env.Note.Title, Pick: []Choice{}}
	for _, b := range bl {
		r.Pick = append(r.Pick, Choice{Kind: b.Note.Kind, ID: b.Note.ID, Title: b.Note.Title,
			Detail: fmt.Sprintf("%s · %d line(s)", b.Note.Key(), len(b.Lines)), Exists: true})
	}
	r.Status = fmt.Sprintf("%d note(s) link to %s", len(r.Pick), env.Note.Key())
	return r, nil
}

func runSyncTasks(env *Env, _ []string) (Result, error) {
	changes, err := env.Store.SyncTaskFlags(false)
	if err != nil {
		return Result{}, err
	}
	if len(changes) == 0 {
		return Result{Status: "sync-tasks: all notes are up to date"}, nil
	}
	ids := make([]string, 0, len(changes))
	for _, c := range changes {
		ids = append(ids, notes.Key(c.Kind, c.ID))
	}
	return Result{Changed: true, Status: fmt.Sprintf("sync-tasks: updated %d note(s): %s", len(changes), strings.Join(ids, ", "))}, nil
}
