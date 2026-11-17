package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/BurntSushi/toml"
	"github.com/devinthemtn/katnotes/internal/config"
	"github.com/devinthemtn/katnotes/internal/editor"
	"github.com/devinthemtn/katnotes/internal/notes"
	"github.com/spf13/cobra"
)

func initCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create the notes directory (journal/ and pages/) and default config",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, _ := config.Path()
			if a.jsonOut {
				return a.printJSON(map[string]string{"notesDir": a.Store.Root, "config": p})
			}
			fmt.Fprintf(a.Out, "notes:  %s\nconfig: %s\n", a.Store.Root, p)
			return nil
		},
	}
}

func newCmd(a *App) *cobra.Command {
	var tags []string
	var noEdit bool
	var body string
	cmd := &cobra.Command{
		Use:   "new [title]",
		Short: "Create a page (or a journal entry with -j; title is then an optional YYYY-MM-DD date)",
		RunE: func(cmd *cobra.Command, args []string) error {
			kind := notes.Kind(a.Cfg.DefaultKind)
			if j, _ := cmd.Flags().GetBool("journal"); j {
				kind = notes.KindJournal
			}
			if !kind.Valid() {
				kind = notes.KindPage
			}
			if body == "-" {
				b, err := io.ReadAll(os.Stdin)
				if err != nil {
					return err
				}
				body = string(b)
			}
			n, err := a.Store.Create(kind, strings.Join(args, " "), tags, body)
			if err != nil {
				return err
			}
			if !noEdit && !a.jsonOut {
				if n, err = a.EditNote(n); err != nil {
					return err
				}
			}
			if a.jsonOut {
				return a.printJSON(n)
			}
			fmt.Fprintf(a.Out, "created %s/%s  %s\n", n.Kind, n.ID, n.Path)
			return nil
		},
	}
	cmd.Flags().BoolP("journal", "j", false, "create a journal entry")
	cmd.Flags().StringSliceVarP(&tags, "tags", "t", nil, "comma-separated tags")
	cmd.Flags().BoolVar(&noEdit, "no-edit", false, "don't open the editor")
	cmd.Flags().StringVarP(&body, "message", "m", "", "note body (use - to read stdin)")
	return cmd
}

func lsCmd(a *App) *cobra.Command {
	var tag, sortBy string
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List notes",
		Args:    cobra.NoArgs,
	}
	kind := kindFlags(cmd)
	cmd.Flags().StringVarP(&tag, "tag", "t", "", "only notes with this tag")
	cmd.Flags().StringVarP(&sortBy, "sort", "s", "", "sort by updated, title or id")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		list, err := a.Store.List(kind(), notes.ListOpts{Tag: tag, Sort: notes.SortBy(sortBy)})
		if err != nil {
			return err
		}
		if a.jsonOut {
			for i := range list {
				list[i].Body = ""
			}
			if list == nil {
				list = []notes.Note{}
			}
			return a.printJSON(list)
		}
		tw := tabwriter.NewWriter(a.Out, 0, 2, 2, ' ', 0)
		fmt.Fprintln(tw, "KIND\tID\tTITLE\tTAGS\tUPDATED")
		for _, n := range list {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", n.Kind, n.ID, n.Title, strings.Join(n.Tags, ","), n.Updated.Format("2006-01-02 15:04"))
		}
		return tw.Flush()
	}
	return cmd
}

func showCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Print a note (raw Markdown, or JSON with --json)",
		Args:  cobra.ExactArgs(1),
	}
	kind := kindFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		n, err := a.resolve(args[0], kind())
		if err != nil {
			return err
		}
		if a.jsonOut {
			return a.printJSON(n)
		}
		data, err := os.ReadFile(n.Path)
		if err != nil {
			return err
		}
		_, err = a.Out.Write(data)
		return err
	}
	return cmd
}

func editCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Open a note in your editor",
		Args:  cobra.ExactArgs(1),
	}
	kind := kindFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		n, err := a.resolve(args[0], kind())
		if err != nil {
			return err
		}
		_, err = a.EditNote(n)
		return err
	}
	return cmd
}

func rmCmd(a *App) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:     "rm <id>",
		Aliases: []string{"delete"},
		Short:   "Delete a note",
		Args:    cobra.ExactArgs(1),
	}
	kind := kindFlags(cmd)
	cmd.Flags().BoolVarP(&force, "force", "f", false, "don't ask for confirmation")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		n, err := a.resolve(args[0], kind())
		if err != nil {
			return err
		}
		if !force {
			fmt.Fprintf(os.Stderr, "delete %s/%s (%q)? [y/N] ", n.Kind, n.ID, n.Title)
			line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
				return fmt.Errorf("aborted")
			}
		}
		if err := a.Store.Delete(n.Kind, n.ID); err != nil {
			return err
		}
		if a.jsonOut {
			return a.printJSON(map[string]string{"deleted": string(n.Kind) + "/" + n.ID})
		}
		fmt.Fprintf(a.Out, "deleted %s/%s\n", n.Kind, n.ID)
		return nil
	}
	return cmd
}

func mvCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "mv <id> <new title>",
		Aliases: []string{"rename"},
		Short:   "Rename a note (pages also move to the new slug)",
		Args:    cobra.MinimumNArgs(2),
	}
	kind := kindFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		n, err := a.resolve(args[0], kind())
		if err != nil {
			return err
		}
		r, err := a.Store.Rename(n.Kind, n.ID, strings.Join(args[1:], " "))
		if err != nil {
			return err
		}
		if a.jsonOut {
			return a.printJSON(r)
		}
		fmt.Fprintf(a.Out, "renamed %s/%s -> %s/%s\n", n.Kind, n.ID, r.Kind, r.ID)
		return nil
	}
	return cmd
}

func todayCmd(a *App) *cobra.Command {
	var noEdit bool
	cmd := &cobra.Command{
		Use:   "today",
		Short: "Open (creating if needed) today's journal entry",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			n, created, err := a.Store.Today()
			if err != nil {
				return err
			}
			if a.jsonOut {
				return a.printJSON(n)
			}
			if noEdit {
				verb := "exists"
				if created {
					verb = "created"
				}
				fmt.Fprintf(a.Out, "%s %s\n", verb, n.Path)
				return nil
			}
			_, err = a.EditNote(n)
			return err
		},
	}
	cmd.Flags().BoolVar(&noEdit, "no-edit", false, "just print the path")
	return cmd
}

func searchCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Full-text search over titles, tags and bodies",
		Args:  cobra.MinimumNArgs(1),
	}
	kind := kindFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		res, err := a.Store.Search(strings.Join(args, " "), kind())
		if err != nil {
			return err
		}
		if a.jsonOut {
			for i := range res {
				res[i].Note.Body = ""
			}
			if res == nil {
				res = []notes.Match{}
			}
			return a.printJSON(res)
		}
		if len(res) == 0 {
			fmt.Fprintln(a.Out, "no matches")
			return nil
		}
		for _, m := range res {
			fmt.Fprintf(a.Out, "%s/%s  %s", m.Note.Kind, m.Note.ID, m.Note.Title)
			if len(m.Note.Tags) > 0 {
				fmt.Fprintf(a.Out, "  [%s]", strings.Join(m.Note.Tags, ", "))
			}
			fmt.Fprintln(a.Out)
			for _, l := range m.Lines {
				fmt.Fprintf(a.Out, "  %4d: %s\n", l.Line, l.Text)
			}
		}
		return nil
	}
	return cmd
}

func tagsCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "tags",
		Short: "List tags with note counts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			tags, err := a.Store.SortedTags()
			if err != nil {
				return err
			}
			if a.jsonOut {
				return a.printJSON(tags)
			}
			tw := tabwriter.NewWriter(a.Out, 0, 2, 2, ' ', 0)
			for _, t := range tags {
				fmt.Fprintf(tw, "%s\t%d\n", t.Tag, t.Count)
			}
			return tw.Flush()
		},
	}
}

func configCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Show or change options"}
	cmd.AddCommand(
		&cobra.Command{
			Use: "path", Short: "Print the config file path", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				p, err := config.Path()
				fmt.Fprintln(a.Out, p)
				return err
			},
		},
		&cobra.Command{
			Use: "show", Short: "Print the effective config", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				if a.jsonOut {
					return a.printJSON(a.Cfg)
				}
				return toml.NewEncoder(a.Out).Encode(a.Cfg)
			},
		},
		&cobra.Command{
			Use: "edit", Short: "Open the config file in your editor", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				p, err := config.Path()
				if err != nil {
					return err
				}
				return editor.Open(a.Cfg.Editor, p)
			},
		},
		&cobra.Command{
			Use:   "set <key> <value>",
			Short: "Set an option (" + strings.Join(config.Keys, ", ") + ")",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				return config.Set(args[0], args[1])
			},
		},
	)
	return cmd
}
