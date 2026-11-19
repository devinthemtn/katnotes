// Command katnotes is the KatNotes CLI. Run with no arguments for the TUI.
package main

import (
	"os"
	"runtime/debug"

	"github.com/devinthemtn/katnotes/internal/cli"
	"github.com/devinthemtn/katnotes/internal/tui"
)

// version is set at build time with -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	if fromExplorer() {
		return
	}
	if version == "dev" {
		if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			version = bi.Main.Version
		}
	}
	os.Exit(cli.Execute(version, func(app *cli.App) error {
		return tui.Run(app)
	}))
}
