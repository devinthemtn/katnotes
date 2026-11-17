// Package editor resolves and launches the user's external text editor.
package editor

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

// Resolve returns the editor command (program plus args) to use. Order:
// configured value, $VISUAL, $EDITOR, then nano, vim, vi, whichever exists.
func Resolve(configured string) ([]string, error) {
	for _, c := range []string{configured, os.Getenv("VISUAL"), os.Getenv("EDITOR")} {
		if f := strings.Fields(c); len(f) > 0 {
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
	argv, err := Resolve(configured)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(argv[0], append(argv[1:], path)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd, nil
}

// Open runs the editor on path and waits for it to exit.
func Open(configured, path string) error {
	cmd, err := Command(configured, path)
	if err != nil {
		return err
	}
	return cmd.Run()
}
