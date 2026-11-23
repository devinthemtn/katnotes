// Package cli implements the katnotes command line. Commands are thin
// wrappers over notes.Store.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/devinthemtn/katnotes/internal/commands"
	"github.com/devinthemtn/katnotes/internal/config"
	"github.com/devinthemtn/katnotes/internal/editor"
	"github.com/devinthemtn/katnotes/internal/notes"
	"github.com/devinthemtn/katnotes/internal/recent"
	"github.com/spf13/cobra"
)

// App is the state shared by all commands.
type App struct {
	Cfg   config.Config
	Store *notes.Store
	Out   io.Writer
	// Version of the running binary.
	Version string
	// OpenNote is the note (kind/id) the TUI starts on (--open), over
	// start_page.
	OpenNote string

	jsonOut bool
	dir     string
}

// TUIFunc launches the TUI; set by main so the cli package does not import it.
type TUIFunc func(app *App) error

// NewRoot builds the root command. With no subcommand it runs tui.
func NewRoot(version string, tui TUIFunc) *cobra.Command {
	app := &App{Out: os.Stdout, Version: version}
	root := &cobra.Command{
		Use:           "katnotes",
		Short:         "Personal Markdown notes: pages and a daily journal",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			app.Out = cmd.OutOrStdout()
			config.Install() //nolint:errcheck // defaults work without the file
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if app.dir != "" {
				cfg.NotesDir = config.ExpandHome(app.dir)
			}
			app.Cfg = cfg
			app.Store = commands.NewStore(cfg)
			app.Store.OnTodoError = func(err error) { fmt.Fprintln(cmd.ErrOrStderr(), "katnotes:", err) }
			return app.Store.Init()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if tui == nil {
				return cmd.Help()
			}
			return tui(app)
		},
	}
	root.PersistentFlags().BoolVar(&app.jsonOut, "json", false, "machine-readable JSON output")
	root.PersistentFlags().StringVar(&app.dir, "dir", "", "notes directory (overrides config and $KATNOTES_DIR)")
	root.Flags().StringVar(&app.OpenNote, "open", "", "start the TUI on this note (kind/id, e.g. page/house; katTasks' open_note uses it)")

	root.AddCommand(
		initCmd(app), newCmd(app), lsCmd(app), showCmd(app), editCmd(app),
		rmCmd(app), todayCmd(app), searchCmd(app), tagsCmd(app), mvCmd(app),
		configCmd(app), tasksCmd(app), linksCmd(app), backlinksCmd(app),
		todoCmd(app), syncCmd(app),
		tagCmd(app), setCmd(app), unsetCmd(app), pinCmd(app), unpinCmd(app),
		linkCmd(app), journalCmd(app), recentCmd(app), gitCmd(app),
		themeCmd(app), savedCmd(app), wikilinksCmd(app), graphCmd(app), runCmd(app),
		attachCmd(app), assetsCmd(app),
	)
	return root
}

// Execute runs the CLI and prints errors to stderr.
func Execute(version string, tui TUIFunc) int {
	if err := NewRoot(version, tui).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "katnotes:", err)
		return 1
	}
	return 0
}

func (a *App) printJSON(v any) error {
	enc := json.NewEncoder(a.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// EditNote opens n in the external editor and, if the file changed, reloads
// and re-saves it so frontmatter is normalized and `updated` is bumped.
func (a *App) EditNote(n *notes.Note) (*notes.Note, error) {
	before := a.Store.ModTime(n.Kind, n.ID)
	if err := editor.Open(a.Cfg.Editor, n.Path); err != nil {
		return nil, fmt.Errorf("editor: %w", err)
	}
	return a.AfterEdit(n, before)
}

// AfterEdit re-saves a note if its file changed since before.
func (a *App) AfterEdit(n *notes.Note, before time.Time) (*notes.Note, error) {
	if a.Store.ModTime(n.Kind, n.ID).Equal(before) {
		return n, nil
	}
	fresh, err := a.Store.Get(n.Kind, n.ID)
	if err != nil {
		return nil, err
	}
	return fresh, a.Store.Save(fresh)
}

// kindFlags adds -j/-p and returns a resolver for them.
func kindFlags(cmd *cobra.Command) func() notes.Kind {
	var j, p bool
	cmd.Flags().BoolVarP(&j, "journal", "j", false, "journal entries")
	cmd.Flags().BoolVarP(&p, "page", "p", false, "pages")
	return func() notes.Kind {
		switch {
		case j && !p:
			return notes.KindJournal
		case p && !j:
			return notes.KindPage
		}
		return ""
	}
}

// resolve finds a note by id. "today" and "yesterday" are journal aliases;
// failing an id or slug, it takes what the TUI and GUI accept: "kind/id", an
// exact title, or a title prefix or substring only one note has.
func (a *App) resolve(id string, kind notes.Kind) (*notes.Note, error) {
	switch strings.ToLower(id) {
	case "today":
		id, kind = time.Now().Format(notes.JournalIDLayout), notes.KindJournal
	case "yesterday":
		id, kind = time.Now().AddDate(0, 0, -1).Format(notes.JournalIDLayout), notes.KindJournal
	}
	if kind != "" {
		return a.Store.Get(kind, id)
	}
	found, err := a.Store.FindByID(id)
	if err != nil {
		if !errors.Is(err, notes.ErrNotFound) {
			return nil, err
		}
		// Allow passing a page title instead of its slug.
		if n, err2 := a.Store.Get(notes.KindPage, notes.Slugify(id)); err2 == nil {
			return n, nil
		}
		if n, err2 := commands.ResolveNote(a.Store, id); err2 == nil {
			return n, nil
		}
		return nil, err
	}
	if len(found) > 1 {
		return nil, fmt.Errorf("%q matches both a page and a journal entry; use -p or -j", id)
	}
	return found[0], nil
}

// forgetRecent drops n from the recently-opened list the TUI and GUI share.
func (a *App) forgetRecent(n *notes.Note) {
	if p, err := recent.Path(); err == nil {
		recent.Forget(p, n.Key()) //nolint:errcheck // a convenience
	}
}

// renameRecent follows a rename in the recently-opened list.
func (a *App) renameRecent(from, to *notes.Note) {
	if p, err := recent.Path(); err == nil {
		recent.Rename(p, from.Key(), to.Key()) //nolint:errcheck // a convenience
	}
}
