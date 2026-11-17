package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// start launches cmd detached from the GUI and reaps it in the background.
func start(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() //nolint:errcheck
	return nil
}

// openExternal opens path with cmdline (e.g. "code -n"), or with the system
// default app when cmdline is empty.
func openExternal(cmdline, path string) error {
	if f := strings.Fields(cmdline); len(f) > 0 {
		return start(exec.Command(f[0], append(f[1:], path)...))
	}
	switch runtime.GOOS {
	case "darwin":
		return start(exec.Command("open", path))
	case "windows":
		return start(exec.Command("cmd", "/c", "start", "", path))
	default:
		return start(exec.Command("xdg-open", path))
	}
}

// openTerminal opens a terminal window whose working directory is dir.
func openTerminal(dir string) error {
	switch runtime.GOOS {
	case "darwin":
		return start(exec.Command("open", "-a", "Terminal", dir))
	case "windows":
		if _, err := exec.LookPath("wt"); err == nil {
			return start(exec.Command("wt", "-d", dir))
		}
		cmd := exec.Command("cmd", "/c", "start", "", "cmd")
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
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		return start(cmd)
	}
	return errors.New("no terminal found: set $TERMINAL")
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
