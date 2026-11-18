package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/devinthemtn/katnotes/internal/config"
	"github.com/devinthemtn/katnotes/internal/gitsync"
	"github.com/devinthemtn/katnotes/internal/notes"
)

// The CLI side of the features the TUI and GUI expose as ":" commands.

func todoCmd(a *App) *cobra.Command {
	var here string
	cmd := &cobra.Command{
		Use:   "todo <task…>",
		Short: "Add a task to today's journal (or --here <id>), e.g. todo call Sam {due:fri}",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var n *notes.Note
			var err error
			if here != "" {
				n, err = a.resolve(here, "")
			} else {
				n, _, err = a.Store.Journal(time.Now())
			}
			if err != nil {
				return err
			}
			got, line, err := a.Store.AddTask(n.Kind, n.ID, strings.Join(args, " "))
			if err != nil {
				return err
			}
			if a.jsonOut {
				return a.printJSON(got)
			}
			fmt.Fprintf(a.Out, "added task to %s/%s:%d\n", got.Kind, got.ID, a.Store.FileLine(got.Kind, got.ID, line))
			return nil
		},
	}
	cmd.Flags().StringVar(&here, "here", "", "add the task to this note instead of today's journal")
	return cmd
}

func syncCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Git: commit every change in the notes folder, then pull and push",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := gitsync.Open(a.Store.Root)
			if err != nil {
				return err
			}
			s, err := repo.Sync(time.Now())
			if err != nil {
				return err
			}
			if a.jsonOut {
				return a.printJSON(s)
			}
			fmt.Fprintln(a.Out, s.String())
			return nil
		},
	}
}

// templatesDir is the page templates folder in the config directory.
func templatesDir() (string, error) {
	d, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, notes.TemplatesDirName), nil
}
