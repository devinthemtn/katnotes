package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/devinthemtn/katnotes/internal/gitsync"
)

// The CLI side of the features the TUI and GUI expose as ":" commands.

func todoCmd(a *App) *cobra.Command {
	var here string
	cmd := &cobra.Command{
		Use:   "todo <task…>",
		Short: "Add a task to today's journal (or --here <id>), e.g. todo call Sam {due:fri}",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			task := strings.Join(args, " ")
			if here == "" {
				_, err := a.exec(nil, "todo", task)
				return err
			}
			n, err := a.resolve(here, "")
			if err != nil {
				return err
			}
			_, err = a.exec(n, "todo here", task)
			return err
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
