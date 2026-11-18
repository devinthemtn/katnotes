// Package editor resolves and launches the user's external text editor.
package editor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Resolve returns the editor command (program plus args) to use. Order:
// configured value, $VISUAL, $EDITOR, then nano, vim, vi, whichever exists.
// Values are split with shell-style quoting (SplitArgs): two single quotes
// are an empty argument, as emacsclient -a wants.
func Resolve(configured string) ([]string, error) {
	for _, c := range []string{configured, os.Getenv("VISUAL"), os.Getenv("EDITOR")} {
		f, err := SplitArgs(c)
		if err != nil {
			return nil, fmt.Errorf("editor %q: %w", c, err)
		}
		if len(f) > 0 {
			return f, nil
		}
	}
	for _, c := range []string{"nano", "vim", "vi"} {
		if p, err := exec.LookPath(c); err == nil {
			return []string{p}, nil
		}
	}
	return nil, errors.New("no editor found: set editor in the config or $EDITOR")
}

// Command builds an *exec.Cmd that opens path in the editor, wired to the
// current terminal.
func Command(configured, path string) (*exec.Cmd, error) {
	return CommandAt(configured, path, 0)
}

// Open runs the editor on path and waits for it to exit.
func Open(configured, path string) error {
	cmd, err := Command(configured, path)
	if err != nil {
		return err
	}
	return cmd.Run()
}

// CommandAt is like Command but asks the editor to open at a 1-based line
// when it is one that understands a line argument. Unknown editors just open
// the file.
func CommandAt(configured, path string, line int) (*exec.Cmd, error) {
	argv, err := Resolve(configured)
	if err != nil {
		return nil, err
	}
	args := append([]string{}, argv[1:]...)
	if line > 0 {
		switch strings.TrimSuffix(filepath.Base(argv[0]), ".exe") {
		case "vi", "vim", "nvim", "gvim", "nano", "micro", "emacs", "emacsclient", "kak", "joe", "mg", "ne", "jed":
			args = append(args, fmt.Sprintf("+%d", line))
		case "hx", "helix", "subl", "zed":
			path = fmt.Sprintf("%s:%d", path, line)
		case "code", "codium", "code-insiders", "cursor":
			args = append(args, "-g")
			path = fmt.Sprintf("%s:%d", path, line)
		case "kate":
			args = append(args, "-l", fmt.Sprint(line))
		}
	}
	cmd := exec.Command(argv[0], append(args, path)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd, nil
}
