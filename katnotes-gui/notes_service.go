package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/devinthemtn/katnotes/internal/cli"
	"github.com/devinthemtn/katnotes/internal/config"
	"github.com/devinthemtn/katnotes/internal/luacmd"
	"github.com/devinthemtn/katnotes/internal/notes"
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
	if dir, err := commandsDir(); err == nil {
		luacmd.WriteExample(dir) //nolint:errcheck // the example is a convenience
	}
	return s, s.reload()
}

func (s *NotesService) reload() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	st := cli.NewStore(cfg)
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

// Save writes tags and body. baseUpdated is the note's `updated` value as the
// frontend last saw it; if the file has moved on since, ErrConflict is
// returned unless force is set.
func (s *NotesService) Save(kind, id string, tags []string, body, baseUpdated string, force bool) (*notes.Note, error) {
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
	if !force && baseUpdated != "" {
		if base, err := time.Parse(time.RFC3339Nano, baseUpdated); err == nil && !n.Updated.Equal(base) {
			return nil, ErrConflict
		}
	}
	n.Tags = tags
	n.Body = body
	return n, st.Save(n)
}

// Rename changes a note's title (pages also move to the new slug).
func (s *NotesService) Rename(kind, id, title string) (*notes.Note, error) {
	st := s.lock()
	defer s.mu.Unlock()
	k, err := notes.ParseKind(kind)
	if err != nil {
		return nil, err
	}
	return st.Rename(k, id, title)
}

// Delete removes a note.
func (s *NotesService) Delete(kind, id string) error {
	st := s.lock()
	defer s.mu.Unlock()
	k, err := notes.ParseKind(kind)
	if err != nil {
		return err
	}
	return st.Delete(k, id)
}

// Today returns today's journal entry, creating it if needed.
func (s *NotesService) Today() (*notes.Note, error) {
	st := s.lock()
	defer s.mu.Unlock()
	n, _, err := st.Today()
	return n, err
}

// Search does full-text search; kind may be "".
func (s *NotesService) Search(query, kind string) ([]notes.Match, error) {
	st := s.lock()
	defer s.mu.Unlock()
	k, err := parseKind(kind)
	if err != nil {
		return nil, err
	}
	res, err := st.Search(query, k)
	for i := range res {
		res[i].Note.Body = ""
	}
	if res == nil {
		res = []notes.Match{}
	}
	return res, err
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

// OpenExternal opens a note's file with the open_with option, or the system
// default app when it is empty.
func (s *NotesService) OpenExternal(kind, id string) error {
	st := s.lock()
	k, err := notes.ParseKind(kind)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	path, cmdline := st.Path(k, id), s.cfg.OpenWith
	s.mu.Unlock()
	if _, err := os.Stat(path); err != nil {
		return err
	}
	return openExternal(cmdline, path)
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

// ReloadConfig re-reads config.toml, picking up hand edits without a restart.
func (s *NotesService) ReloadConfig() (config.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reload(); err != nil {
		return s.cfg, err
	}
	return s.cfg, nil
}

func commandsDir() (string, error) {
	d, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, luacmd.DirName), nil
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
	dir, err := commandsDir()
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
	dir, err := commandsDir()
	if err != nil {
		return luacmd.Result{}, err
	}
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
