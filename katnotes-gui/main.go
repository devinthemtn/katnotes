// Command katnotes-gui is the Wails desktop frontend for KatNotes. It shares
// the notes core with the katnotes CLI/TUI.
package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"runtime/debug"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/devinthemtn/katnotes/internal/config"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

// version is set at build time with -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	if version == "dev" {
		if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			version = bi.Main.Version
		}
	}
	for _, arg := range os.Args[1:] {
		if arg == "-v" || arg == "--version" {
			fmt.Printf("katnotes-gui %s\n", version)
			return
		}
	}

	quietStartup()

	// Without the file the app runs on the defaults, so a failure isn't fatal.
	config.Install() //nolint:errcheck

	svc, err := NewNotesService()
	if err != nil {
		log.Fatal(err)
	}

	app := application.New(application.Options{
		Name:        "katnotes-gui",
		Description: "Personal Markdown notes: pages and a daily journal",
		Icon:        icon,
		Services: []application.Service{
			application.NewService(svc),
		},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: svc.assetMiddleware(),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		Linux: application.LinuxOptions{
			ProgramName: "katnotes-gui",
		},
	})

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "KatNotes",
		Width:            1200,
		Height:           780,
		MinWidth:         720,
		MinHeight:        480,
		BackgroundColour: application.NewRGB(24, 24, 32),
		URL:              "/",
		// Without this Wails blocks files dropped from the desktop. With it,
		// Linux and macOS hand the drop to Go as file paths (see below), not
		// to the page; elements need data-file-drop-target to receive one.
		EnableFileDrop: true,
		// Wails on Linux grabs Ctrl+Shift+Z (redo) before the page sees it,
		// so zoom has to be bound here and handed to the frontend.
		KeyBindings: map[string]func(window application.Window){
			"CmdOrCtrl+Shift+Z": func(w application.Window) {
				w.ExecJS(`window.dispatchEvent(new CustomEvent("katnotes:toggle-zoom"))`)
			},
		},
	})

	// Files dropped on the editor: the page imports the images by path.
	win.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		ctx := e.Context()
		drop := struct {
			Files []string `json:"files"`
			X     int      `json:"x"`
			Y     int      `json:"y"`
		}{Files: ctx.DroppedFiles()}
		if t := ctx.DropTargetDetails(); t != nil {
			drop.X, drop.Y = t.X, t.Y
		}
		detail, err := json.Marshal(drop)
		if err != nil || len(drop.Files) == 0 {
			return
		}
		win.ExecJS(fmt.Sprintf(`window.dispatchEvent(new CustomEvent("katnotes:files-dropped", {detail: %s}))`, detail))
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
