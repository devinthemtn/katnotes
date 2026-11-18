package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/devinthemtn/katnotes/internal/editor"
)

// start launches cmd detached from the GUI (in its own session, so it
// outlives the window) and reaps it in the background.
func start(cmd *exec.Cmd) error { return editor.StartDetached(cmd) }

// openExternal opens path with cmdline (open_with, e.g. "code -n"), just
// as ":open-with" would, or with the system default app when it is empty.
func openExternal(cmdline, path string) error {
	if strings.TrimSpace(cmdline) != "" {
		return openWith(cmdline, path)
	}
	return start(editor.SystemOpenCommand(path))
}

// openTerminal opens a terminal window whose working directory is dir.
func openTerminal(dir string) error { return runInTerminal(dir, nil) }

// openWith opens path once with cmdline (":open-with"). Terminal programs
// KatNotes knows (vim, hx…) run in a new terminal window; anything else is
// started detached, as the GUI has no terminal of its own.
func openWith(cmdline, path string) error {
	argv, detached, err := editor.ParseOpenWith(cmdline)
	if err != nil {
		return err
	}
	if _, err := exec.LookPath(argv[0]); err != nil {
		return err
	}
	argv = append(argv, path)
	if term, _ := editor.InTerminal(argv[0]); term && !detached {
		return runInTerminal(filepath.Dir(path), argv)
	}
	return start(exec.Command(argv[0], argv[1:]...))
}

// runInTerminal opens a terminal window in dir running argv, or the user's
// shell when argv is empty.
func runInTerminal(dir string, argv []string) error {
	switch runtime.GOOS {
	case "darwin":
		if len(argv) == 0 {
			return start(exec.Command("open", "-a", "Terminal", dir))
		}
		script := "cd " + shellQuote(dir) + " && exec"
		for _, a := range argv {
			script += " " + shellQuote(a)
		}
		return start(exec.Command("osascript",
			"-e", `tell application "Terminal" to do script "`+appleQuote(script)+`"`,
			"-e", `tell application "Terminal" to activate`))
	case "windows":
		if _, err := exec.LookPath("wt"); err == nil {
			return start(exec.Command("wt", append([]string{"-d", dir}, argv...)...))
		}
		if len(argv) == 0 {
			argv = []string{"cmd"}
		}
		cmd := exec.Command("cmd", append([]string{"/c", "start", ""}, argv...)...)
		cmd.Dir = dir // avoids quoting trouble with spaces in dir
		return start(cmd)
	}

	candidates := []string{os.Getenv("TERMINAL"), "x-terminal-emulator",
		"kitty", "alacritty", "wezterm", "foot", "ghostty",
		"gnome-terminal", "konsole", "xfce4-terminal", "xterm"}
	for _, c := range candidates {
		f := strings.Fields(c)
		if len(f) == 0 {
			continue
		}
		p, err := exec.LookPath(f[0])
		if err != nil {
			continue
		}
		bin, name := f[0], filepath.Base(f[0])
		// x-terminal-emulator is an alternatives symlink (often to
		// gnome-terminal.wrapper); use the real terminal so its cwd flag applies.
		if real, err := filepath.EvalSymlinks(p); err == nil {
			realName := strings.TrimSuffix(filepath.Base(real), ".wrapper")
			if terminalDirArgs(realName, dir) != nil {
				if rp, err := exec.LookPath(realName); err == nil {
					bin, name = rp, realName
				}
			}
		}
		args := append(f[1:], terminalDirArgs(name, dir)...)
		args = append(args, terminalExecArgs(name, argv)...)
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		return start(cmd)
	}
	return errors.New("no terminal found: set $TERMINAL")
}

// terminalExecArgs returns the arguments that make terminal `name` run argv.
func terminalExecArgs(name string, argv []string) []string {
	if len(argv) == 0 {
		return nil
	}
	switch name {
	case "kitty", "foot":
		return argv
	case "gnome-terminal", "wezterm":
		return append([]string{"--"}, argv...)
	case "xfce4-terminal":
		return append([]string{"-x"}, argv...)
	}
	return append([]string{"-e"}, argv...) // xterm, alacritty, konsole, ghostty, urxvt…
}

// shellQuote quotes s for a POSIX shell.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// appleQuote escapes s for an AppleScript string literal.
func appleQuote(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// terminalDirArgs returns flags for terminals that ignore the process cwd.
func terminalDirArgs(name, dir string) []string {
	switch name {
	case "gnome-terminal", "xfce4-terminal":
		return []string{"--working-directory=" + dir}
	case "konsole":
		return []string{"--workdir", dir}
	case "wezterm":
		return []string{"start", "--cwd", dir}
	}
	return nil
}

// noteDir picks the folder to open a terminal in.
func noteDir(path, root string) string {
	if path == "" {
		return root
	}
	if fi, err := os.Stat(filepath.Dir(path)); err == nil && fi.IsDir() {
		return filepath.Dir(path)
	}
	return root
}
