package commands

import (
	"fmt"
	"strings"

	"github.com/devinthemtn/katnotes/internal/config"
	"github.com/devinthemtn/katnotes/internal/gitsync"
	"github.com/devinthemtn/katnotes/internal/notes"
	"github.com/devinthemtn/katnotes/internal/recent"
	"github.com/devinthemtn/katnotes/internal/theme"
	"github.com/devinthemtn/katnotes/internal/viewstate"
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

// themeShared is the ":theme" value that clears this app's own theme, so it
// follows the shared `theme` option again.
const themeShared = "shared"

// appLabel names env's app for messages.
func appLabel(app string) string {
	switch app {
	case AppTUI:
		return "TUI"
	case AppGUI:
		return "GUI"
	case AppCLI:
		return "shared"
	}
	return "app"
}

// themeStatus describes the theme in effect for app.
func themeStatus(cfg config.Config, app string) string {
	own := map[string]string{AppTUI: cfg.TUITheme, AppGUI: cfg.GUITheme}
	if v, ok := own[app]; ok && v == "" {
		return appLabel(app) + " theme: " + cfg.Theme + " (shared)"
	}
	return appLabel(app) + " theme: " + cfg.ThemeFor(app)
}

// runTheme sets this app's theme (tui_theme / gui_theme in config.toml);
// "shared" clears it so the app follows `theme`. The frontend then applies
// it (Result.UI "theme", Args the option now in effect). With no name it
// says which theme is in use.
func runTheme(env *Env, args []string) (Result, error) {
	dir := env.ThemesPath()
	avail := "auto, " + strings.Join(theme.List(dir), ", ")
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		cfg, err := config.Load()
		if err != nil {
			return Result{}, err
		}
		return Result{Status: themeStatus(cfg, env.App) + " · available: " + avail}, nil
	}
	name := strings.TrimSuffix(strings.TrimSpace(args[0]), ".toml")
	switch {
	case strings.EqualFold(name, themeShared):
		name = ""
	case strings.EqualFold(name, theme.Auto):
		name = theme.Auto
	case !theme.Exists(dir, name):
		return Result{}, fmt.Errorf("no theme %q: available are %s, or shared", name, avail)
	default:
		if _, err := theme.Load(dir, name); err != nil {
			return Result{}, err
		}
	}
	if err := config.Set(config.ThemeKey(env.App), name); err != nil {
		return Result{}, err
	}
	cfg, err := config.Load()
	if err != nil {
		return Result{}, err
	}
	return Result{Status: themeStatus(cfg, env.App), UI: "theme", Args: []string{cfg.ThemeFor(env.App)}}, nil
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

// runSearch returns a saved search for the app to show: Result.UI is
// "search" and Args are its mode and text.
func runSearch(env *Env, args []string) (Result, error) {
	x, ok := viewstate.Load(env.viewPath()).FindSearch(args[0])
	if !ok {
		return Result{}, fmt.Errorf("no saved search %q (search save <name> keeps the current one)", args[0])
	}
	return Result{UI: "search", Args: []string{x.Mode, x.Text}, Status: fmt.Sprintf("search %q: %s %s", x.Name, x.Mode, x.Text)}, nil
}

func runSearchSave(env *Env, args []string) (Result, error) {
	mode := env.Search.Mode
	if mode == "" {
		mode = string(notes.SearchName)
	}
	if _, err := viewstate.SaveSearch(env.viewPath(), viewstate.Search{Name: args[0], Mode: mode, Text: env.Search.Text}); err != nil {
		return Result{}, err
	}
	return Result{Changed: true, Status: fmt.Sprintf("saved search %q (%s: %s)", args[0], mode, env.Search.Text)}, nil
}

func runSearchRm(env *Env, args []string) (Result, error) {
	if _, err := viewstate.DeleteSearch(env.viewPath(), args[0]); err != nil {
		return Result{}, err
	}
	return Result{Changed: true, Status: fmt.Sprintf("deleted saved search %q", args[0])}, nil
}
