package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/devinthemtn/katnotes/internal/commands"
	"github.com/devinthemtn/katnotes/internal/notes"
	"github.com/devinthemtn/katnotes/internal/viewstate"
)

// Commands that run the same code as the TUI and GUI ":" command window
// (internal/commands), so the three frontends behave alike.

// env is the commands.Env for a CLI command on note n (nil for none).
func (a *App) env(n *notes.Note) *commands.Env {
	return &commands.Env{Store: a.Store, Note: n, App: commands.AppCLI, Now: time.Now}
}

// exec runs a shared command and prints its result: JSON with --json,
// otherwise the pick list (if any), the clipboard text, then the status.
func (a *App) exec(n *notes.Note, name string, args ...string) (commands.Result, error) {
	return a.execEnv(a.env(n), name, args...)
}

func (a *App) execEnv(env *commands.Env, name string, args ...string) (commands.Result, error) {
	r, err := commands.Exec(env, name, args)
	if err != nil {
		return r, err
	}
	if a.jsonOut {
		if r.Note != nil {
			c := *r.Note
			c.Body = ""
			r.Note = &c
		}
		return r, a.printJSON(r)
	}
	if r.PickTitle != "" && len(r.Pick) > 0 {
		tw := tabwriter.NewWriter(a.Out, 0, 2, 2, ' ', 0)
		for _, c := range r.Pick {
			fmt.Fprintf(tw, "%s/%s\t%s\t%s\n", c.Kind, c.ID, c.Title, c.Detail)
		}
		if err := tw.Flush(); err != nil {
			return r, err
		}
		return r, nil
	}
	if r.Clipboard != "" {
		fmt.Fprintln(a.Out, r.Clipboard)
		return r, nil
	}
	if r.Status != "" {
		fmt.Fprintln(a.Out, r.Status)
	}
	return r, nil
}

// noteCmd is a shared command that acts on the note named by its first
// argument, e.g. "pin <id>".
func noteCmd(a *App, use, short, name string, args cobra.PositionalArgs, split func([]string) []string) *cobra.Command {
	cmd := &cobra.Command{Use: use, Short: short, Args: args}
	kind := kindFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		n, err := a.resolve(args[0], kind())
		if err != nil {
			return err
		}
		rest := args[1:]
		if split != nil {
			rest = split(rest)
		}
		_, err = a.exec(n, name, rest...)
		return err
	}
	return cmd
}

// restAfter keeps the first i values and joins the rest into one.
func restAfter(i int) func([]string) []string {
	return func(v []string) []string {
		if len(v) <= i {
			return v
		}
		return append(append([]string{}, v[:i]...), strings.Join(v[i:], " "))
	}
}

func tagCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{Use: "tag", Short: "Add or remove tags on a note"}
	cmd.AddCommand(
		noteCmd(a, "add <id> <tag…>", "Add tags to a note", "tag add", cobra.MinimumNArgs(2), nil),
		noteCmd(a, "rm <id> <tag…>", "Remove tags from a note", "tag rm", cobra.MinimumNArgs(2), nil),
	)
	return cmd
}

func setCmd(a *App) *cobra.Command {
	return noteCmd(a, "set <id> <field> <value…>", "Set a frontmatter field (adds frontmatter if missing)",
		"set", cobra.MinimumNArgs(3), restAfter(1))
}

func unsetCmd(a *App) *cobra.Command {
	return noteCmd(a, "unset <id> <field>", "Remove a frontmatter field", "unset", cobra.ExactArgs(2), nil)
}

func pinCmd(a *App) *cobra.Command {
	return noteCmd(a, "pin <id>", "Pin a note to the top of the lists", "pin", cobra.ExactArgs(1), nil)
}

func unpinCmd(a *App) *cobra.Command {
	return noteCmd(a, "unpin <id>", "Unpin a note", "unpin", cobra.ExactArgs(1), nil)
}

func linkCmd(a *App) *cobra.Command {
	return noteCmd(a, "link <id>", "Print a Markdown link to a note (the apps' copy-link)", "copy-link", cobra.ExactArgs(1), nil)
}

func journalCmd(a *App) *cobra.Command {
	var noEdit bool
	cmd := &cobra.Command{
		Use:   "journal [date]",
		Short: "Open (creating if needed) a journal day: today, yesterday, tomorrow, +N, -N, YYYY-MM-DD or a date as date_format",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if noEdit || a.jsonOut {
				_, err := a.exec(nil, "journal", args...)
				return err
			}
			r, err := commands.Exec(a.env(nil), "journal", args)
			if err != nil {
				return err
			}
			_, err = a.EditNote(r.Note)
			return err
		},
	}
	cmd.Flags().BoolVar(&noEdit, "no-edit", false, "just say whether it was created")
	return cmd
}

func recentCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "recent",
		Short: "Notes opened lately in the TUI or GUI",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := a.exec(nil, "recent")
			return err
		},
	}
}

func gitCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{Use: "git", Short: "Git commands for the notes folder"}
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Notes changed since the last commit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := a.exec(nil, "git status")
			if err == nil && !a.jsonOut && len(r.Pick) > 0 {
				fmt.Fprintln(a.Out, r.Status)
			}
			return err
		},
	})
	return cmd
}

func themeCmd(a *App) *cobra.Command {
	var app string
	cmd := &cobra.Command{
		Use:   "theme [name]",
		Short: "Show or set the colour theme (auto, light, dark, a file in themes/, or shared)",
		Long: "With no name, says which theme is in use. A name sets the shared `theme`\n" +
			"option, or with --app tui|gui that app's own theme; \"shared\" clears an\n" +
			"app's own theme so it follows the shared one.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env := a.env(nil)
			switch app {
			case "":
			case commands.AppTUI, commands.AppGUI:
				env.App = app
			default:
				return fmt.Errorf("--app is tui or gui, not %q", app)
			}
			_, err := a.execEnv(env, "theme", args...)
			return err
		},
	}
	cmd.Flags().StringVar(&app, "app", "", "set this app's own theme: tui or gui")
	return cmd
}

func savedCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "saved",
		Short: "List saved searches (run one with search --saved <name>)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := viewstate.Path()
			if err != nil {
				return err
			}
			ss := viewstate.Load(p).Searches
			if a.jsonOut {
				return a.printJSON(ss)
			}
			if len(ss) == 0 {
				fmt.Fprintln(a.Out, "no saved searches")
				return nil
			}
			tw := tabwriter.NewWriter(a.Out, 0, 2, 2, ' ', 0)
			for _, s := range ss {
				fmt.Fprintf(tw, "%s\t%s\t%s\n", s.Name, s.Mode, s.Text)
			}
			return tw.Flush()
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "rm <name…>",
		Short: "Delete a saved search",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := a.exec(nil, "search rm", strings.Join(args, " "))
			return err
		},
	})
	return cmd
}
