package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

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
	var body, tmpl string
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
			var n *notes.Note
			var err error
			if tmpl != "" {
				if kind == notes.KindJournal || body != "" {
					return fmt.Errorf("--template makes a page and can't be combined with -j or -m")
				}
				dir, derr := templatesDir()
				if derr != nil {
					return derr
				}
				src, terr := notes.ReadTemplate(dir, tmpl)
				if terr != nil {
					return terr
				}
				n, err = a.Store.CreateFromTemplate(strings.Join(args, " "), src, tags)
			} else {
				n, err = a.Store.Create(kind, strings.Join(args, " "), tags, body)
			}
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
	cmd.Flags().StringVar(&tmpl, "template", "", "create the page from a template in the config folder's templates/")
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
		r, relinked, err := a.Store.Rename(n.Kind, n.ID, strings.Join(args[1:], " "))
		if err != nil {
			return err
		}
		if a.jsonOut {
			return a.printJSON(r)
		}
		fmt.Fprintf(a.Out, "renamed %s/%s -> %s/%s\n", n.Kind, n.ID, r.Kind, r.ID)
		if len(relinked) > 0 {
			fmt.Fprintf(a.Out, "updated links in %d note(s): %s\n", len(relinked), strings.Join(relinked, ", "))
		}
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

func tasksCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{Use: "tasks", Short: "Work with Markdown tasks (- [ ] …) in notes"}
	var dryRun bool
	sync := &cobra.Command{
		Use:   "sync",
		Short: "Update the contains_task frontmatter field in every note",
		Long: "Rescan every note and set contains_task: true in its frontmatter when the\n" +
			"body has a task, or remove the field when it has none. Only notes that need\n" +
			"a change are rewritten, and their updated time is kept.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			changes, err := a.Store.SyncTaskFlags(dryRun)
			if a.jsonOut {
				if changes == nil {
					changes = []notes.TaskFlagChange{}
				}
				if jerr := a.printJSON(changes); jerr != nil {
					return jerr
				}
				return err
			}
			verb := "updated"
			if dryRun {
				verb = "would update"
			}
			for _, c := range changes {
				state := "contains_task: true"
				if !c.ContainsTask {
					state = "contains_task removed"
				}
				fmt.Fprintf(a.Out, "%s %s/%s (%s)\n", verb, c.Kind, c.ID, state)
			}
			if err != nil {
				return err
			}
			if len(changes) == 0 {
				fmt.Fprintln(a.Out, "all notes are up to date")
			} else {
				fmt.Fprintf(a.Out, "%s %d note(s)\n", verb, len(changes))
			}
			return nil
		},
	}
	sync.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "show what would change without writing")
	cmd.AddCommand(sync, tasksListCmd(a), tasksSetCmd(a))
	return cmd
}

func tasksListCmd(a *App) *cobra.Command {
	var f notes.TaskFilter
	var dueBefore, pri, kind string
	var where []string
	var overdue bool
	c := &cobra.Command{
		Use:   "list",
		Short: "List tasks across notes, sorted by due date and priority",
		Long: "List tasks from every note. Task metadata is a {key:value …} block at the\n" +
			"end of the task line, e.g. \"- [ ] call {due:2026-11-20 pri:high}\".\n" +
			"Dates for --due-before accept YYYY-MM-DD, today, tomorrow, +3d, fri, ….",
		Example: "  katnotes tasks list --overdue\n" +
			"  katnotes tasks list --due-before fri --pri high\n" +
			"  katnotes tasks list --where project:house --all",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			now := time.Now()
			if overdue {
				dueBefore = "yesterday"
			}
			if dueBefore != "" {
				d, err := notes.NormalizeDate(dueBefore, now)
				if err != nil {
					return err
				}
				f.DueBefore = d
			}
			if pri != "" {
				p, err := notes.NormalizePriority(pri)
				if err != nil {
					return err
				}
				f.Pri = p
			}
			if kind != "" {
				k, err := notes.ParseKind(kind)
				if err != nil {
					return err
				}
				f.Kind = k
			}
			for _, w := range where {
				k, v, _ := strings.Cut(w, ":")
				if f.Has == nil {
					f.Has = map[string]string{}
				}
				f.Has[strings.ToLower(k)] = v
			}
			ts, err := a.Store.AllTasks(f)
			if err != nil {
				return err
			}
			if a.jsonOut {
				if ts == nil {
					ts = []notes.TaskRef{}
				}
				for i := range ts {
					ts[i].Note.Body = ""
				}
				return a.printJSON(ts)
			}
			today := now.Format(notes.DateLayout)
			tw := tabwriter.NewWriter(a.Out, 0, 2, 2, ' ', 0)
			for _, r := range ts {
				box := "[ ]"
				if r.Task.Done {
					box = "[x]"
				}
				due := r.Task.Meta[notes.MetaDue]
				if due != "" && due < today && !r.Task.Done {
					due += "!"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s/%s:%d\n", box, dash(due), dash(r.Task.Meta[notes.MetaPri]),
					r.Task.Text, r.Note.Kind, r.Note.ID, r.Task.Line+1)
			}
			return tw.Flush()
		},
	}
	c.Flags().StringVar(&dueBefore, "due-before", "", "only tasks due on or before this date")
	c.Flags().BoolVar(&overdue, "overdue", false, "only tasks due before today")
	c.Flags().StringVar(&pri, "pri", "", "only tasks with this priority (high, med, low)")
	c.Flags().StringArrayVar(&where, "where", nil, "only tasks with metadata key:value (or just key); repeatable")
	c.Flags().BoolVarP(&f.IncludeDone, "all", "a", false, "include done tasks")
	c.Flags().StringVar(&kind, "kind", "", "only page or journal notes")
	return c
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func tasksSetCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "set <kind/id:line> <key> [value]",
		Short: "Set or remove (no value) a metadata key on one task",
		Long: "Set a {key:value} entry on the task at a 1-based body line, as printed by\n" +
			"\"tasks list\". Leave out the value to remove the key. due/start/done accept\n" +
			"YYYY-MM-DD, today, tomorrow, +3d, fri, …; pri accepts high, med, low (h/m/l).",
		Example: "  katnotes tasks set page/groceries:3 due fri\n  katnotes tasks set page/groceries:3 pri",
		Args:    cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			i := strings.LastIndex(args[0], ":")
			ok := i > 0
			ref, lineStr := args[0], ""
			if ok {
				ref, lineStr = args[0][:i], args[0][i+1:]
			}
			kindStr, id, ok2 := strings.Cut(ref, "/")
			line, err := strconv.Atoi(lineStr)
			if !ok || !ok2 || err != nil || line < 1 {
				return fmt.Errorf("want <kind>/<id>:<line>, got %q", args[0])
			}
			k, err := notes.ParseKind(kindStr)
			if err != nil {
				return err
			}
			val := ""
			if len(args) == 3 {
				val = args[2]
			}
			n, err := a.Store.SetTaskMeta(k, id, line-1, args[1], val)
			if err != nil {
				return err
			}
			fmt.Fprintln(a.Out, strings.Split(n.Body, "\n")[line-1])
			return nil
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

func linksCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "links <id>",
		Short: "List the notes a note links to ([text](../pages/<id>.md) links)",
		Args:  cobra.ExactArgs(1),
	}
	kind := kindFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		n, err := a.resolve(args[0], kind())
		if err != nil {
			return err
		}
		ls, err := a.Store.Links(n.Kind, n.ID)
		if err != nil {
			return err
		}
		if a.jsonOut {
			return a.printJSON(ls)
		}
		tw := tabwriter.NewWriter(a.Out, 0, 2, 2, ' ', 0)
		for _, l := range ls {
			state := ""
			if !l.Exists {
				state = "(missing)"
			}
			fmt.Fprintf(tw, "%s/%s\t%s\t%s\tline %d\n", l.Kind, l.ID, l.Title, state, l.Line+1)
		}
		return tw.Flush()
	}
	return cmd
}

func backlinksCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backlinks <id>",
		Short: "List the notes that link to a note, with the linking lines",
		Args:  cobra.ExactArgs(1),
	}
	kind := kindFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		n, err := a.resolve(args[0], kind())
		if err != nil {
			return err
		}
		bl, err := a.Store.Backlinks(n.Kind, n.ID)
		if err != nil {
			return err
		}
		if a.jsonOut {
			return a.printJSON(bl)
		}
		for _, b := range bl {
			fmt.Fprintf(a.Out, "%s/%s  %s\n", b.Note.Kind, b.Note.ID, b.Note.Title)
			for _, l := range b.Lines {
				fmt.Fprintf(a.Out, "  %d: %s\n", l.Line, l.Text)
			}
		}
		if len(bl) == 0 {
			fmt.Fprintln(a.Out, "no notes link here")
		}
		return nil
	}
	return cmd
}
