package commands

import (
	"fmt"
	"strings"

	"github.com/devinthemtn/katnotes/internal/gitsync"
	"github.com/devinthemtn/katnotes/internal/notes"
	"github.com/devinthemtn/katnotes/internal/recent"
)

func runNewFrom(env *Env, args []string) (Result, error) {
	src, err := notes.ReadTemplate(env.TemplatesPath(), args[0])
	if err != nil {
		return Result{}, err
	}
	n, err := env.Store.CreateFromTemplate(args[1], src, nil)
	if err != nil {
		return Result{}, err
	}
	return Result{Note: n, Changed: true, Edit: true, Status: "created " + key(n) + " from " + args[0]}, nil
}

func runRecent(env *Env, _ []string) (Result, error) {
	r := Result{PickTitle: "Recent notes", Pick: []Choice{}}
	for _, k := range recent.Load(env.recentPath()) {
		kind, id, ok := strings.Cut(k, "/")
		if !ok {
			continue
		}
		pk, err := notes.ParseKind(kind)
		if err != nil {
			continue
		}
		n, err := env.Store.Get(pk, id)
		if err != nil {
			continue // deleted or renamed outside the apps
		}
		r.Pick = append(r.Pick, Choice{Kind: n.Kind, ID: n.ID, Title: n.Title, Exists: true,
			Detail: k + " · m: " + n.Updated.Format("Jan 02 2006")})
	}
	r.Status = fmt.Sprintf("%d recent note(s)", len(r.Pick))
	return r, nil
}

func addTask(env *Env, n *notes.Note, text string) (Result, error) {
	got, line, err := env.Store.AddTask(n.Kind, n.ID, text)
	if err != nil {
		return Result{}, err
	}
	return Result{Note: got, Changed: true, Status: fmt.Sprintf("added task to %s:%d", key(got), env.Store.FileLine(got.Kind, got.ID, line))}, nil
}

func runTodo(env *Env, args []string) (Result, error) {
	n, _, err := env.Store.Journal(env.now())
	if err != nil {
		return Result{}, err
	}
	r, err := addTask(env, n, args[0])
	r.Note = nil // stay where you are; the task went to the journal
	return r, err
}

func runTodoHere(env *Env, args []string) (Result, error) { return addTask(env, env.Note, args[0]) }

func runPin(env *Env, _ []string) (Result, error) {
	if env.Note.Pinned() {
		return Result{Status: key(env.Note) + " is already pinned"}, nil
	}
	n, _, err := env.Store.SetField(env.Note, notes.PinnedKey, "true")
	if err != nil {
		return Result{}, err
	}
	return Result{Note: n, Changed: true, Status: "pinned " + key(n)}, nil
}

func runUnpin(env *Env, _ []string) (Result, error) {
	if !env.Note.Pinned() {
		return Result{Status: key(env.Note) + " isn't pinned"}, nil
	}
	n, err := env.Store.UnsetField(env.Note, notes.PinnedKey)
	if err != nil {
		return Result{}, err
	}
	return Result{Note: n, Changed: true, Status: "unpinned " + key(n)}, nil
}

func runSync(env *Env, _ []string) (Result, error) {
	repo, err := gitsync.Open(env.Store.Root)
	if err != nil {
		return Result{}, err
	}
	s, err := repo.Sync(env.now())
	if err != nil {
		return Result{Changed: s.Committed > 0}, err
	}
	return Result{Changed: s.Pulled, Status: s.String()}, nil
}

// noteAt maps a path relative to the notes folder to a note, if it is one.
func noteAt(path string) (notes.Kind, string, bool) {
	dir, file, ok := strings.Cut(path, "/")
	if !ok || strings.Contains(file, "/") || !strings.HasSuffix(file, ".md") {
		return "", "", false
	}
	for _, k := range []notes.Kind{notes.KindPage, notes.KindJournal} {
		if k.Dir() == dir {
			return k, strings.TrimSuffix(file, ".md"), true
		}
	}
	return "", "", false
}

var gitCodes = map[byte]string{'M': "modified", 'A': "added", 'D': "deleted", 'R': "renamed", '?': "new", 'C': "copied", 'U': "conflict"}

func runGitStatus(env *Env, _ []string) (Result, error) {
	repo, err := gitsync.Open(env.Store.Root)
	if err != nil {
		return Result{}, err
	}
	cs, err := repo.Status()
	if err != nil {
		return Result{}, err
	}
	r := Result{PickTitle: "Changed since the last commit", Pick: []Choice{}}
	var other []string
	for _, c := range cs {
		what := gitCodes[c.Code[1]]
		if what == "" {
			what = gitCodes[c.Code[0]]
		}
		kind, id, ok := noteAt(c.Path)
		n, err := env.Store.Get(kind, id)
		if !ok || err != nil {
			other = append(other, what+" "+c.Path)
			continue
		}
		r.Pick = append(r.Pick, Choice{Kind: kind, ID: id, Title: n.Title, Exists: true, Detail: what + " · " + c.Path})
	}
	switch {
	case len(cs) == 0:
		r.Status = "git: nothing changed since the last commit"
	default:
		r.Status = fmt.Sprintf("git: %d change(s)", len(cs))
		if len(other) > 0 {
			r.Status += " · " + strings.Join(other, ", ")
		}
	}
	return r, nil
}
