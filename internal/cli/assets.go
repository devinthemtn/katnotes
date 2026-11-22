package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/devinthemtn/katnotes/internal/notes"
)

func attachCmd(a *App) *cobra.Command {
	var alt string
	cmd := &cobra.Command{
		Use:   "attach <file> [note]",
		Short: "Copy an image into assets/ and print its Markdown (or append it to a note)",
		Long: "Copies the image into the assets/ folder of the notes directory and prints\n" +
			"![](../assets/<file>). Given a note, the Markdown is added to the end of it instead.\n" +
			"A file with the same contents already in assets/ is reused.",
		Args: cobra.RangeArgs(1, 2),
	}
	cmd.Flags().StringVar(&alt, "alt", "", "alt text for the image")
	kind := kindFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		var n *notes.Note
		if len(args) == 2 {
			var err error
			if n, err = a.resolve(args[1], kind()); err != nil {
				return err
			}
		}
		var rel string
		var err error
		if n != nil {
			rel, err = a.Store.AttachImage(n, args[0], alt)
		} else {
			rel, err = a.Store.ImportAsset(args[0])
		}
		if err != nil {
			return err
		}
		md := notes.ImageMarkdown(alt, rel)
		if a.jsonOut {
			out := map[string]string{"path": rel, "markdown": md}
			if n != nil {
				out["note"] = n.Key()
			}
			return a.printJSON(out)
		}
		if n != nil {
			fmt.Fprintf(a.Out, "added %s to %s\n", rel, n.Key())
			return nil
		}
		fmt.Fprintln(a.Out, md)
		return nil
	}
	return cmd
}

func assetsCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{Use: "assets", Short: "Images and other files in the notes' assets/ folder"}
	ls := &cobra.Command{
		Use: "ls", Short: "List the files in assets/", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			files, err := a.Store.Assets()
			if err != nil {
				return err
			}
			if a.jsonOut {
				return a.printJSON(nonNil(files))
			}
			for _, f := range files {
				fmt.Fprintln(a.Out, f)
			}
			return nil
		},
	}
	var del bool
	orphans := &cobra.Command{
		Use:   "orphans",
		Short: "List files in assets/ that no note refers to (--delete removes them)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			files, err := a.Store.OrphanAssets()
			if err != nil {
				return err
			}
			if del {
				for _, f := range files {
					p, ok := a.Store.AssetPath(f)
					if !ok {
						continue
					}
					if err := os.Remove(p); err != nil {
						return err
					}
				}
			}
			if a.jsonOut {
				return a.printJSON(nonNil(files))
			}
			for _, f := range files {
				if del {
					fmt.Fprintln(a.Out, "deleted", f)
				} else {
					fmt.Fprintln(a.Out, f)
				}
			}
			return nil
		},
	}
	orphans.Flags().BoolVar(&del, "delete", false, "delete the unused files")
	cmd.AddCommand(ls, orphans)
	return cmd
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
