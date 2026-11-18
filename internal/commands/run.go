package commands

import (
	"fmt"
	"strconv"
	"strings"
	"time"

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

func key(n *notes.Note) string { return string(n.Kind) + "/" + n.ID }

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
// (days from today) or a 2006-01-02 date.
func ParseDay(now time.Time, s string) (time.Time, error) {
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
	if t, err := time.ParseInLocation(notes.JournalIDLayout, s, now.Location()); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("journal: can't read %q as a day (today, yesterday, tomorrow, +N, -N or YYYY-MM-DD)", s)
}

func runOpen(env *Env, args []string) (Result, error) {
	n, err := ResolveNote(env.Store, args[0])
	if err != nil {
		return Result{}, err
	}
	return Result{Note: n, Status: "opened " + key(n)}, nil
}

func runNew(env *Env, args []string) (Result, error) {
	n, err := env.Store.Create(notes.KindPage, args[0], nil, "")
	if err != nil {
		return Result{}, err
	}
	return Result{Note: n, Changed: true, Edit: true, Status: "created " + key(n)}, nil
}

func runJournal(env *Env, args []string) (Result, error) {
	day := ""
	if len(args) > 0 {
		day = args[0]
	}
	d, err := ParseDay(env.now(), day)
	if err != nil {
		return Result{}, err
	}
	n, created, err := env.Store.Journal(d)
	if err != nil {
		return Result{}, err
	}
	r := Result{Note: n, Changed: created, Edit: created, Status: "opened " + key(n)}
	if created {
		r.Status = "created " + key(n)
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
		return Result{Note: n, Status: key(n) + " already has " + hashTags(add)}, nil
	}
	n.Tags = append(n.Tags, added...)
	if err := env.Store.Save(n); err != nil {
		return Result{}, err
	}
	return Result{Note: n, Changed: true, Status: "tagged " + key(n) + " " + hashTags(added)}, nil
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
		return Result{}, fmt.Errorf("%s has no tag %s", key(n), hashTags(notes.NormalizeTags(args)))
	}
	if keep == nil {
		keep = []string{}
	}
	n.Tags = keep
	if err := env.Store.Save(n); err != nil {
		return Result{}, err
	}
	return Result{Note: n, Changed: true, Status: "removed " + hashTags(removed) + " from " + key(n)}, nil
}

func runSet(env *Env, args []string) (Result, error) {
	field, value := args[0], args[1]
	n, relinked, err := env.Store.SetField(env.Note, field, value)
	if err != nil {
		return Result{}, err
	}
	r := Result{Note: n, Changed: true, Relinked: relinked,
		Status: fmt.Sprintf("set %s = %s on %s", field, FieldText(n, field), key(n))}
	if field == "title" && key(n) != key(env.Note) {
		r.Status = "renamed to " + key(n)
		recent.Rename(env.recentPath(), key(env.Note), key(n)) //nolint:errcheck // a convenience
	}
	r.Status += relinkedText(relinked)
	return r, nil
}

func runUnset(env *Env, args []string) (Result, error) {
	n, err := env.Store.UnsetField(env.Note, args[0])
	if err != nil {
		return Result{}, err
	}
	return Result{Note: n, Changed: true, Status: "removed " + args[0] + " from " + key(n)}, nil
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
	recent.Rename(env.recentPath(), key(env.Note), key(n)) //nolint:errcheck // a convenience
	return Result{Note: n, Changed: true, Relinked: relinked,
		Status: "renamed to " + key(n) + relinkedText(relinked)}, nil
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
		k := string(l.Kind) + "/" + l.ID
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
	r.Status = fmt.Sprintf("%d link(s) from %s", len(r.Pick), key(env.Note))
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
			Detail: fmt.Sprintf("%s · %d line(s)", key(&b.Note), len(b.Lines)), Exists: true})
	}
	r.Status = fmt.Sprintf("%d note(s) link to %s", len(r.Pick), key(env.Note))
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
		ids = append(ids, string(c.Kind)+"/"+c.ID)
	}
	return Result{Changed: true, Status: fmt.Sprintf("sync-tasks: updated %d note(s): %s", len(changes), strings.Join(ids, ", "))}, nil
}
