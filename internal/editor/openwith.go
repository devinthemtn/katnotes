package editor

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ":open-with" opens a note once in a program other than the configured
// editor. Terminal programs (vim, hx…) need a terminal: the TUI hands over
// its own, the GUI starts a terminal window. Window programs (code, gedit…)
// start detached.

// terminalEditors are editors that run inside a terminal, on top of the
// terminal previewers in terminalViewers.
var terminalEditors = map[string]bool{
	"vi": true, "vim": true, "nvim": true, "nano": true, "micro": true, "hx": true, "helix": true,
	"kak": true, "joe": true, "mg": true, "ne": true, "jed": true, "emacs": true, "ed": true,
	"pico": true, "amp": true, "dte": true, "mcedit": true, "tilde": true,
}

// windowEditors are programs that open their own window.
var windowEditors = map[string]bool{
	"code": true, "codium": true, "code-insiders": true, "cursor": true, "zed": true, "subl": true,
	"gedit": true, "gnome-text-editor": true, "kate": true, "kwrite": true, "mousepad": true,
	"pluma": true, "xed": true, "geany": true, "gvim": true, "emacsclient": true, "typora": true,
	"obsidian": true, "marktext": true, "ghostwriter": true, "apostrophe": true, "retext": true,
	"notepad": true, "notepad++": true, "katdown": true, "xdg-open": true, "open": true,
	"libreoffice": true, "lowriter": true, "atom": true, "lapce": true,
}

// suggestOrder is the order KnownPrograms lists programs in.
var suggestOrder = []string{
	"nvim", "vim", "vi", "hx", "nano", "micro", "emacs", "kak",
	"code", "codium", "cursor", "zed", "subl", "gvim", "kate", "gedit", "gnome-text-editor",
	"mousepad", "xed", "geany", "typora", "marktext", "ghostwriter", "apostrophe", "obsidian",
	"glow", "mdcat", "bat", "less", "katdown", "xdg-open",
}

// ProgramName is argv0 without its folder or .exe suffix.
func ProgramName(argv0 string) string {
	return strings.TrimSuffix(filepath.Base(argv0), ".exe")
}

// InTerminal reports whether prog (a program name or path) runs inside a
// terminal, and whether KatNotes knows the program at all.
func InTerminal(prog string) (inTerminal, known bool) {
	name := ProgramName(prog)
	switch {
	case terminalEditors[name] || terminalViewers[name]:
		return true, true
	case windowEditors[name]:
		return false, true
	}
	return false, false
}

// KnownProgram is an editor or viewer found on PATH.
type KnownProgram struct {
	Name       string
	InTerminal bool
}

// KnownPrograms lists the editors and viewers KatNotes knows that are
// installed, for completing ":open-with".
func KnownPrograms() []KnownProgram {
	var out []KnownProgram
	for _, name := range suggestOrder {
		if _, err := exec.LookPath(name); err == nil {
			term, _ := InTerminal(name)
			out = append(out, KnownProgram{Name: name, InTerminal: term})
		}
	}
	return out
}

// ParseOpenWith splits an ":open-with" command line into the program and
// its arguments. A trailing "&" asks for the program to be started detached
// (in its own window) even when KatNotes doesn't know it.
func ParseOpenWith(cmdline string) (argv []string, detached bool, err error) {
	s := strings.TrimSpace(cmdline)
	if strings.HasSuffix(s, "&") {
		s, detached = strings.TrimSpace(strings.TrimSuffix(s, "&")), true
	}
	argv = strings.Fields(s)
	if len(argv) == 0 {
		return nil, false, errors.New("open with what? e.g. open-with nvim")
	}
	return argv, detached, nil
}

// OpenWithCommand builds the command that opens path with cmdline from a
// terminal app. inTerminal reports whether to hand it the terminal (run it
// in the foreground and wait); otherwise start it with StartDetached.
// Programs KatNotes doesn't know get the terminal, unless cmdline ends in
// "&": a window program then just keeps the terminal until it closes, where
// a detached terminal program would have no screen at all.
func OpenWithCommand(cmdline, path string) (cmd *exec.Cmd, inTerminal bool, err error) {
	argv, detached, err := ParseOpenWith(cmdline)
	if err != nil {
		return nil, false, err
	}
	if _, err := exec.LookPath(argv[0]); err != nil {
		return nil, false, err
	}
	inTerminal = !detached
	if term, known := InTerminal(argv[0]); known && !detached {
		inTerminal = term
	}
	cmd = exec.Command(argv[0], append(argv[1:], path)...)
	if inTerminal {
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	}
	return cmd, inTerminal, nil
}
