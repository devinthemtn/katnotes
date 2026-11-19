package main

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/inconshreveable/mousetrap"
	"github.com/spf13/cobra"
)

// fromExplorer handles katnotes.exe being started from Explorer (a
// double-click, or "katnotes" typed into Run or Start, since the installer
// puts its folder on PATH) rather than from a terminal. With no arguments it
// starts katnotes-gui.exe from the same folder and reports true, so main
// exits and the console window Windows opened goes away. Without the GUI it
// lets the TUI run in that console instead of cobra's "This is a command line
// tool" message. With arguments cobra's message stays, as the output would
// vanish with the window.
func fromExplorer() bool {
	if !mousetrap.StartedByExplorer() || len(os.Args) > 1 {
		return false
	}
	if exe, err := os.Executable(); err == nil {
		gui := filepath.Join(filepath.Dir(exe), "katnotes-gui.exe")
		if _, err := os.Stat(gui); err == nil {
			cmd := exec.Command(gui)
			cmd.Dir = filepath.Dir(exe)
			if cmd.Start() == nil {
				cmd.Process.Release() //nolint:errcheck // nothing to clean up
				return true
			}
		}
	}
	cobra.MousetrapHelpText = ""
	return false
}
