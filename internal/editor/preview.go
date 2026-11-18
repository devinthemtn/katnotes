package editor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Previewer modes (config previewer_mode).
const (
	PreviewAuto     = "auto"     // terminal for known terminal viewers, otherwise a window
	PreviewTerminal = "terminal" // take over the terminal until the viewer exits (glow, mdcat…)
	PreviewWindow   = "window"   // start a separate window and return at once (katdown…)
)

// defaultPreviewers are tried in order when no previewer is configured.
var defaultPreviewers = []string{"katdown", "glow", "frogmouth", "mdcat"}

// terminalViewers run inside the terminal, so the TUI must hand it over.
var terminalViewers = map[string]bool{
	"glow": true, "mdcat": true, "mdless": true, "mdv": true, "frogmouth": true,
	"bat": true, "batcat": true, "less": true, "more": true, "most": true, "lowdown": true,
	"vim": true, "nvim": true, "view": true,
}

// ResolvePreviewer returns the previewer command (program plus args): the
// configured one, else the first of katdown, glow, frogmouth, mdcat on PATH.
func ResolvePreviewer(configured string) ([]string, error) {
	if f := strings.Fields(configured); len(f) > 0 {
		return f, nil
	}
	for _, c := range defaultPreviewers {
		if p, err := exec.LookPath(c); err == nil {
			return []string{p}, nil
		}
	}
	return nil, errors.New("no Markdown previewer found: set previewer in the config (e.g. katdown or glow)")
}

// NormalizePreviewMode validates a previewer_mode value ("" means auto).
func NormalizePreviewMode(mode string) (string, error) {
	switch m := strings.ToLower(strings.TrimSpace(mode)); m {
	case "", PreviewAuto:
		return PreviewAuto, nil
	case PreviewTerminal, "term", "tty":
		return PreviewTerminal, nil
	case PreviewWindow, "gui", "detach", "detached":
		return PreviewWindow, nil
	}
	return "", fmt.Errorf("previewer_mode must be auto, terminal or window, got %q", mode)
}

// PreviewCommand builds the command that previews path. inTerminal reports
// whether it needs the terminal (run it in the foreground and wait); otherwise
// start it detached with StartDetached.
func PreviewCommand(configured, mode, path string) (cmd *exec.Cmd, inTerminal bool, err error) {
	argv, err := ResolvePreviewer(configured)
	if err != nil {
		return nil, false, err
	}
	mode, err = NormalizePreviewMode(mode)
	if err != nil {
		return nil, false, err
	}
	switch mode {
	case PreviewTerminal:
		inTerminal = true
	case PreviewAuto:
		inTerminal = terminalViewers[strings.TrimSuffix(filepath.Base(argv[0]), ".exe")]
	}
	cmd = exec.Command(argv[0], append(argv[1:], path)...)
	if inTerminal {
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	}
	return cmd, inTerminal, nil
}

// StartDetached starts a windowed previewer in its own session (so closing
// the terminal doesn't take it down), with no terminal I/O, and reaps it in
// the background.
func StartDetached(cmd *exec.Cmd) error {
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() //nolint:errcheck
	return nil
}
