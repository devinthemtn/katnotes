package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/devinthemtn/katnotes/internal/commands"
	"github.com/devinthemtn/katnotes/internal/config"
	"github.com/devinthemtn/katnotes/internal/luacmd"
	"github.com/devinthemtn/katnotes/internal/notes"
	"github.com/devinthemtn/katnotes/internal/recent"
	"github.com/devinthemtn/katnotes/internal/theme"
	"github.com/devinthemtn/katnotes/internal/viewstate"
)

// NotesService exposes the shared notes.Store to the Svelte frontend. Every
// exported method becomes a generated TS binding.
type NotesService struct {
	mu    sync.Mutex
	cfg   config.Config
	store *notes.Store
}

// Info describes where notes and options live.
type Info struct {
	NotesDir   string `json:"notesDir"`
	ConfigPath string `json:"configPath"`
	Version    string `json:"version"`
}

// NewNotesService loads the config and prepares the notes directory.
func NewNotesService() (*NotesService, error) {
	s := &NotesService{}
	if d, err := config.Dir(); err == nil {
		luacmd.WriteExamples(d) //nolint:errcheck // the examples are a convenience
	}
	if dir := commands.TemplatesDir(); dir != "" {
		notes.WriteExampleTemplate(dir) //nolint:errcheck // the example is a convenience
	}
	if dir := theme.Dir(); dir != "" {
		theme.WriteDefaults(dir) //nolint:errcheck // the files are starting points
	}
	return s, s.reload()
}

func (s *NotesService) reload() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	st := commands.NewStore(cfg)
	if err := st.Init(); err != nil {
		return err
	}
	s.cfg, s.store = cfg, st
	return nil
}

func (s *NotesService) lock() *notes.Store {
	s.mu.Lock()
	return s.store
}

func parseKind(k string) (notes.Kind, error) {
	if k == "" {
		return "", nil
	}
	return notes.ParseKind(k)
}

// Info returns paths and the app version.
func (s *NotesService) Info() Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, _ := config.Path()
	return Info{NotesDir: s.store.Root, ConfigPath: p, Version: version}
}

// List returns notes of a kind ("page", "journal", or "" for all), optionally
// filtered by tag. Bodies are omitted; use Get for the full note.
func (s *NotesService) List(kind, tag string) ([]notes.Note, error) {
	st := s.lock()
	defer s.mu.Unlock()
	k, err := parseKind(kind)
	if err != nil {
		return nil, err
	}
	ns, err := st.List(k, notes.ListOpts{Tag: tag})
	for i := range ns {
		ns[i].Body = ""
	}
	if ns == nil {
		ns = []notes.Note{}
	}
	return ns, err
}

// Get returns one note with its body.
func (s *NotesService) Get(kind, id string) (*notes.Note, error) {
	st := s.lock()
	defer s.mu.Unlock()
	k, err := notes.ParseKind(kind)
	if err != nil {
		return nil, err
	}
	return st.Get(k, id)
}

// Create makes a new note. For journal entries title is an optional
// YYYY-MM-DD date.
func (s *NotesService) Create(kind, title string, tags []string) (*notes.Note, error) {
	st := s.lock()
	defer s.mu.Unlock()
	k, err := notes.ParseKind(kind)
	if err != nil {
		return nil, err
	}
	return st.Create(k, title, tags, "")
}

// ErrConflict means the file changed on disk (e.g. from the CLI or TUI) since
// the frontend loaded it.
var ErrConflict = errors.New("this note was changed outside the app; reload it before saving")

// Save writes tags and body. baseStamp is the note's Stamp as the frontend
// last saw it; if the file has changed since, ErrConflict is returned unless
// force is set. (The stamp, not `updated`, because an updated: field in the
// frontmatter stays put when another program edits the file.)
func (s *NotesService) Save(kind, id string, tags []string, body, baseStamp string, force bool) (*notes.Note, error) {
	st := s.lock()
	defer s.mu.Unlock()
	k, err := notes.ParseKind(kind)
	if err != nil {
		return nil, err
	}
	n, err := st.Get(k, id)
	if err != nil {
		return nil, err
	}
	if !force && baseStamp != "" && n.Stamp != baseStamp {
		return nil, ErrConflict
	}
	n.Tags = tags
	n.Body = body
	return n, st.Save(n)
}

// RenameResult is a renamed note and the notes whose links were updated.
type RenameResult struct {
	Note    *notes.Note `json:"note"`
	Updated []string    `json:"updated"`
}

// Rename changes a note's title. Pages also move to the new slug, and links
// to them in other notes are rewritten.
func (s *NotesService) Rename(kind, id, title string) (*RenameResult, error) {
	st := s.lock()
	defer s.mu.Unlock()
	k, err := notes.ParseKind(kind)
	if err != nil {
		return nil, err
	}
	n, updated, err := st.Rename(k, id, title)
	if updated == nil {
		updated = []string{}
	}
	if n == nil {
		return nil, err
	}
	renameRecent(kind+"/"+id, n.Key())
	return &RenameResult{Note: n, Updated: updated}, err
}

// Backlinks lists the notes that link to a note, with the linking lines.
func (s *NotesService) Backlinks(kind, id string) ([]notes.Backlink, error) {
	st := s.lock()
	defer s.mu.Unlock()
	k, err := notes.ParseKind(kind)
	if err != nil {
		return nil, err
	}
	return st.Backlinks(k, id)
}

// Delete removes a note.
func (s *NotesService) Delete(kind, id string) error {
	st := s.lock()
	defer s.mu.Unlock()
	k, err := notes.ParseKind(kind)
	if err != nil {
		return err
	}
	if err := st.Delete(k, id); err != nil {
		return err
	}
	forgetRecent(kind + "/" + id)
	return nil
}

// Today returns today's journal entry, creating it if needed.
func (s *NotesService) Today() (*notes.Note, error) {
	st := s.lock()
	defer s.mu.Unlock()
	n, _, err := st.Today()
	return n, err
}

// Find runs a search in one of the modes "name" (fuzzy title/filename),
// "tags" (every tag must match) or "text" (full text). An empty text lists
// the kind's notes in sortBy order ("" for the default). Bodies are left
// out; matching lines and title hits are included.
func (s *NotesService) Find(mode, text, kind, tag, sortBy string) ([]notes.Match, error) {
	st := s.lock()
	defer s.mu.Unlock()
	k, err := parseKind(kind)
	if err != nil {
		return nil, err
	}
	md, err := notes.ParseSearchMode(mode)
	if err != nil {
		return nil, err
	}
	sb, err := notes.ParseSort(sortBy)
	if err != nil {
		return nil, err
	}
	res, err := st.Find(notes.FindQuery{Mode: md, Text: text, Kind: k, Tag: tag, Sort: sb})
	for i := range res {
		res[i].Note.Body = ""
	}
	if res == nil {
		res = []notes.Match{}
	}
	return res, err
}

// ViewState is the shared sort orders and saved searches (view.json).
func (s *NotesService) ViewState() viewstate.State {
	p, err := viewstate.Path()
	if err != nil {
		return viewstate.State{Sort: map[string]string{}, Searches: []viewstate.Search{}}
	}
	return viewstate.Load(p)
}

// SetSort remembers a tab's sort order (shared with the TUI).
func (s *NotesService) SetSort(kind, sortBy string) error {
	if _, err := notes.ParseSort(sortBy); err != nil {
		return err
	}
	p, err := viewstate.Path()
	if err != nil {
		return err
	}
	_, err = viewstate.SetSort(p, kind, sortBy)
	return err
}

// Tags lists tags with counts, most used first.
func (s *NotesService) Tags() ([]notes.TagCount, error) {
	st := s.lock()
	defer s.mu.Unlock()
	return st.SortedTags()
}

// GetConfig returns the effective options.
func (s *NotesService) GetConfig() config.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// SetOption changes one option in config.toml and reloads.
func (s *NotesService) SetOption(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := config.Set(key, value); err != nil {
		return err
	}
	return s.reload()
}

// ChooseNotesDir opens a folder picker and, if a folder is chosen, makes it
// the notes directory. It returns the chosen path ("" if cancelled).
func (s *NotesService) ChooseNotesDir() (string, error) {
	app := application.Get()
	if app == nil {
		return "", fmt.Errorf("no application")
	}
	dir, err := app.Dialog.OpenFile().
		SetTitle("Choose KatNotes folder").
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		SetDirectory(s.Info().NotesDir).
		PromptForSingleSelection()
	if err != nil || strings.TrimSpace(dir) == "" {
		return "", err
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("not a folder: %s", dir)
	}
	if os.Getenv(config.EnvNotesDir) != "" {
		return "", fmt.Errorf("$%s is set and overrides the config", config.EnvNotesDir)
	}
	return dir, s.SetOption("notes_dir", dir)
}

// OpenExternal opens a note's file with the open_with option (like
// OpenWith), or the system default app when it is empty.
func (s *NotesService) OpenExternal(kind, id string) error {
	st := s.lock()
	k, err := notes.ParseKind(kind)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	path, cmdline := st.Path(k, id), s.cfg.OpenWithFor(commands.AppGUI)
	s.mu.Unlock()
	if _, err := os.Stat(path); err != nil {
		return err
	}
	return openExternal(cmdline, path)
}

// FileStamp identifies the current version of a note's file (its Stamp),
// so the frontend can notice edits made by an external editor. It is ""
// when the file is missing.
func (s *NotesService) FileStamp(kind, id string) string {
	st := s.lock()
	defer s.mu.Unlock()
	k, err := notes.ParseKind(kind)
	if err != nil {
		return ""
	}
	return st.Stamp(k, id)
}

// OpenWith opens a note's file once with cmdline (":open-with code").
func (s *NotesService) OpenWith(kind, id, cmdline string) error {
	st := s.lock()
	k, err := notes.ParseKind(kind)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	path := st.Path(k, id)
	s.mu.Unlock()
	if _, err := os.Stat(path); err != nil {
		return err
	}
	return openWith(cmdline, path)
}

// OpenTerminal opens a terminal in the note's folder, or the notes folder
// when kind/id are empty.
func (s *NotesService) OpenTerminal(kind, id string) error {
	st := s.lock()
	root, path := st.Root, ""
	if kind != "" && id != "" {
		if k, err := notes.ParseKind(kind); err == nil {
			path = st.Path(k, id)
		}
	}
	s.mu.Unlock()
	return openTerminal(noteDir(path, root))
}

// ThemeInfo is the colour theme to show, from the `theme` option.
type ThemeInfo struct {
	Theme  theme.Theme `json:"theme"`
	Option string      `json:"option"` // the option in effect: "auto" or a theme name
	Names  []string    `json:"names"`  // every theme there is
	Dir    string      `json:"dir"`    // the themes folder
	Error  string      `json:"error"`  // a problem in the theme file (the theme still works)
}

// Theme resolves the GUI's theme (gui_theme, else theme); systemDark picks light or dark for
// "auto". Theme files are re-read each call, so edits show without a restart.
func (s *NotesService) Theme(systemDark bool) ThemeInfo {
	s.mu.Lock()
	option := s.cfg.ThemeFor(commands.AppGUI)
	s.mu.Unlock()
	dir := theme.Dir()
	t, err := theme.Resolve(dir, option, systemDark)
	info := ThemeInfo{Theme: t, Option: option, Names: theme.List(dir), Dir: dir}
	if err != nil {
		info.Error = err.Error()
	}
	return info
}

// ReloadConfig re-reads config.toml, picking up hand edits without a restart.
func (s *NotesService) ReloadConfig() (config.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reload(); err != nil {
		return s.cfg, err
	}
	return s.cfg, nil
}

// slashDir is the folder of the "/" editor commands. (":" commands are
// found by internal/commands.)
func slashDir() (string, error) {
	d, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, luacmd.SlashDirName), nil
}

// SlashList is the user's Lua commands plus any files that failed to load.
type SlashList struct {
	Commands []luacmd.Command `json:"commands"`
	Errors   []string         `json:"errors"`
}

// SlashCommands lists the user's Lua commands. A broken file is reported in
// Errors without hiding the commands that loaded.
func (s *NotesService) SlashCommands() (SlashList, error) {
	out := SlashList{Commands: []luacmd.Command{}, Errors: []string{}}
	dir, err := slashDir()
	if err != nil {
		return out, err
	}
	cmds, errs := luacmd.List(dir)
	out.Commands = append(out.Commands, cmds...)
	for _, e := range errs {
		out.Errors = append(out.Errors, e.Error())
	}
	return out, nil
}

// RunSlash runs one user Lua command for the open note.
func (s *NotesService) RunSlash(name string, ctx luacmd.Context) (luacmd.Result, error) {
	dir, err := slashDir()
	if err != nil {
		return luacmd.Result{}, err
	}
	// "/name" always runs as a slash command, with the short time limit.
	ctx.Source, ctx.App, ctx.Args = luacmd.SourceSlash, commands.AppGUI, ""
	s.mu.Lock()
	ctx.NotesDir = s.store.Root
	if k, err := notes.ParseKind(ctx.Note.Kind); err == nil && ctx.Note.ID != "" {
		ctx.Note.Path = s.store.Path(k, ctx.Note.ID)
	}
	s.mu.Unlock()
	if d, err := config.Dir(); err == nil {
		ctx.ConfigDir = d
	}
	return luacmd.Run(dir, name, ctx)
}

// Tasks lists every note that contains tasks, with its tasks (bodies omitted).
func (s *NotesService) Tasks() ([]notes.NoteTasks, error) {
	st := s.lock()
	defer s.mu.Unlock()
	ts, err := st.Tasks("")
	for i := range ts {
		ts[i].Note.Body = ""
	}
	if ts == nil {
		ts = []notes.NoteTasks{}
	}
	return ts, err
}

// SetTask checks or unchecks the task on a note's body line and saves it.
func (s *NotesService) SetTask(kind, id string, line int, done bool) (*notes.Note, error) {
	st := s.lock()
	defer s.mu.Unlock()
	k, err := notes.ParseKind(kind)
	if err != nil {
		return nil, err
	}
	return st.SetTask(k, id, line, done)
}

// SetTaskMeta sets one {key:value} metadata entry on the task at a note's
// body line and saves it; an empty value removes the key. Dates accept
// "tomorrow", "fri", "+3d" and the like.
func (s *NotesService) SetTaskMeta(kind, id string, line int, key, value string) (*notes.Note, error) {
	st := s.lock()
	defer s.mu.Unlock()
	k, err := notes.ParseKind(kind)
	if err != nil {
		return nil, err
	}
	return st.SetTaskMeta(k, id, line, key, value)
}

// TaskMetaStats lists the task metadata keys in use and their common values,
// for the editor's {…} autocomplete.
func (s *NotesService) TaskMetaStats() ([]notes.MetaKeyStat, error) {
	st := s.lock()
	defer s.mu.Unlock()
	out, err := st.MetaStats()
	if out == nil {
		out = []notes.MetaKeyStat{}
	}
	return out, err
}

// EnsureNote returns a note, creating it first if it doesn't exist (used when
// following a link to a missing note). New pages keep exactly this id.
func (s *NotesService) EnsureNote(kind, id, title string) (*notes.Note, error) {
	st := s.lock()
	defer s.mu.Unlock()
	k, err := notes.ParseKind(kind)
	if err != nil {
		return nil, err
	}
	n, _, err := st.Ensure(k, id, title)
	return n, err
}

// ---- ":" command palette (shared catalog in internal/commands) ----

// cmdEnv builds the command environment for the open note (kind/id may be
// empty). The caller holds s.mu.
func (s *NotesService) cmdEnv(st *notes.Store, kind, id string) *commands.Env {
	env := &commands.Env{Store: st, App: commands.AppGUI}
	if p, err := commands.HistoryPath(); err == nil {
		env.History = commands.LoadHistory(p)
	}
	if kind != "" && id != "" {
		if k, err := notes.ParseKind(kind); err == nil {
			if n, err := st.Get(k, id); err == nil {
				env.Note = n
			}
		}
	}
	return env
}

// Commands lists the palette's commands for the GUI, in display order.
func (s *NotesService) Commands() []*commands.Spec { return commands.Catalog(commands.AppGUI) }

// CompleteCommand returns completions for a partly typed command line, with
// kind/id the open note (empty when none).
func (s *NotesService) CompleteCommand(input, kind, id string) commands.Completion {
	st := s.lock()
	defer s.mu.Unlock()
	c := commands.Complete(s.cmdEnv(st, kind, id), input)
	if c.Items == nil {
		c.Items = []commands.Suggestion{}
	}
	return c
}

// RunCommand runs a command line against the open note and records it in
// the shared history. searchMode/searchText are the sidebar's current search
// (for "search save"). UI commands come back with Result.UI set. Slow
// commands (Spec.Long: git sync) run without holding the store lock, so the
// window stays usable meanwhile.
func (s *NotesService) RunCommand(input, kind, id, searchMode, searchText string) (commands.Result, error) {
	st := s.lock()
	env := s.cmdEnv(st, kind, id)
	env.Search = commands.CurrentSearch{Mode: searchMode, Text: searchText}
	if p, err := commands.Parse(commands.AppGUI, input); err == nil && p.Spec.Long {
		s.mu.Unlock()
	} else {
		defer s.mu.Unlock()
	}
	r, err := commands.Run(env, input)
	if err != nil {
		return r, err
	}
	if p, perr := commands.HistoryPath(); perr == nil {
		commands.AddHistory(p, input) //nolint:errcheck // history is a convenience
	}
	if r.Pick == nil {
		r.Pick = []commands.Choice{}
	}
	if r.Relinked == nil {
		r.Relinked = []string{}
	}
	if r.Args == nil {
		r.Args = []string{}
	}
	return r, nil
}

// CommandHistory returns recent command lines, newest first.
func (s *NotesService) CommandHistory() []string {
	p, err := commands.HistoryPath()
	if err != nil {
		return []string{}
	}
	return commands.LoadHistory(p)
}

// FrontmatterDoc is a note's frontmatter as editable YAML.
type FrontmatterDoc struct {
	YAML     string `json:"yaml"`
	Inferred bool   `json:"inferred"` // the file has no frontmatter; values were worked out from it
}

// FrontmatterText returns a note's frontmatter as YAML (without --- lines).
func (s *NotesService) FrontmatterText(kind, id string) (FrontmatterDoc, error) {
	st := s.lock()
	defer s.mu.Unlock()
	k, err := notes.ParseKind(kind)
	if err != nil {
		return FrontmatterDoc{}, err
	}
	n, err := st.Get(k, id)
	if err != nil {
		return FrontmatterDoc{}, err
	}
	has, err := st.HasFrontmatter(k, id)
	if err != nil {
		return FrontmatterDoc{}, err
	}
	y, err := notes.FrontmatterYAML(n)
	return FrontmatterDoc{YAML: string(y), Inferred: !has}, err
}

// ApplyFrontmatter replaces a note's frontmatter with the given YAML. A new
// title renames the note, as with Rename.
func (s *NotesService) ApplyFrontmatter(kind, id, text string) (*RenameResult, error) {
	st := s.lock()
	defer s.mu.Unlock()
	k, err := notes.ParseKind(kind)
	if err != nil {
		return nil, err
	}
	n, err := st.Get(k, id)
	if err != nil {
		return nil, err
	}
	r, relinked, err := st.ApplyFrontmatter(n, []byte(text))
	if err != nil {
		return nil, err
	}
	renameRecent(kind+"/"+id, r.Key())
	if relinked == nil {
		relinked = []string{}
	}
	return &RenameResult{Note: r, Updated: relinked}, nil
}

// ---- recent notes (shared with the TUI) and page templates ----

func forgetRecent(key string) {
	if p, err := recent.Path(); err == nil {
		recent.Forget(p, key) //nolint:errcheck // a convenience
	}
}

func renameRecent(from, to string) {
	if from == to {
		return
	}
	if p, err := recent.Path(); err == nil {
		recent.Rename(p, from, to) //nolint:errcheck // a convenience
	}
}

// TouchRecent records a note as just opened, for :recent and
// start_page = "last" in both apps.
func (s *NotesService) TouchRecent(kind, id string) {
	if p, err := recent.Path(); err == nil {
		recent.Touch(p, kind+"/"+id) //nolint:errcheck // a convenience
	}
}

// LastNote is the most recently opened note that still exists, as
// "kind/id", or "".
func (s *NotesService) LastNote() string {
	p, err := recent.Path()
	if err != nil {
		return ""
	}
	st := s.lock()
	defer s.mu.Unlock()
	for _, k := range recent.Load(p) {
		if _, err := st.GetKey(k); err == nil {
			return k
		}
	}
	return ""
}

// Templates lists the page templates in the config folder's templates/.
func (s *NotesService) Templates() []string {
	return notes.ListTemplates(commands.TemplatesDir())
}

// CreateFromTemplate creates a page titled title from template name.
func (s *NotesService) CreateFromTemplate(name, title string, tags []string) (*notes.Note, error) {
	src, err := notes.ReadTemplate(commands.TemplatesDir(), name)
	if err != nil {
		return nil, err
	}
	st := s.lock()
	defer s.mu.Unlock()
	return st.CreateFromTemplate(title, src, tags)
}
