# KatNotes

Personal notes in plain Markdown with YAML frontmatter. KatNotes has one Go core and three ways to use it:

- **`katnotes`** is a CLI and, run with no arguments, a TUI (Bubble Tea). It is a pure Go build with no CGO.
- **`katnotes-gui`** is a desktop app built with Wails v3 and Svelte.

Every frontend goes through the same `internal/notes` package, so they all read and write the same files.

## Where things live

| What | Where |
|---|---|
| Notes | `~/KatNotes/pages/<slug>.md` and `~/KatNotes/journal/YYYY-MM-DD.md` |
| Options | `$XDG_CONFIG_HOME/katnotes/config.toml` (usually `~/.config/katnotes/config.toml`) |

You can override the notes folder with `notes_dir` in the config, `$KATNOTES_DIR`, or `--dir`.

A note looks like this:

```markdown
---
title: Hello World
tags: [idea, go]
created: "2026-11-16T20:37:44-07:00"
updated: "2026-11-16T20:37:44-07:00"
---
# Hello World

Body text…
```

Frontmatter keys that KatNotes doesn't know are kept when a note is saved. A file without frontmatter still works: the title comes from its first `# heading` or, failing that, the filename.

## Config

```toml
notes_dir = "~/KatNotes"
editor = ""                 # falls back to $VISUAL, then $EDITOR, then nano/vim/vi
default_kind = "page"       # what `katnotes new` makes
journal_template = "# {{.Date}}\n\n"   # fields: .Date .ID .Weekday .Time
date_format = "2006-01-02"  # Go time layout for journal titles
```

To change these, run `katnotes config set <key> <value>`, open the file with `katnotes config edit`, or use the GUI's Settings dialog.

## CLI

```
katnotes                       # TUI
katnotes new "Title" -t a,b    # new page, opens $EDITOR (--no-edit, -m "body", -m - for stdin)
katnotes new -j [YYYY-MM-DD]   # new journal entry
katnotes today                 # open or create today's journal entry
katnotes ls [-p|-j] [-t tag] [-s updated|title|id]
katnotes show <id>             # also accepts: today, yesterday, or a page title
katnotes edit <id>
katnotes mv <id> "New title"   # pages move to the new slug
katnotes rm <id> [-f]
katnotes search <query> [-p|-j]
katnotes tags
katnotes config path|show|edit|set
```

Every command accepts `--json` for scripting.

## TUI keys

| Key | Action |
|---|---|
| `tab` / `1` `2` `3` | switch between Pages, Journal and Tags |
| `n` | new note |
| `enter` / `e` | edit in `$EDITOR` |
| `r` | rename |
| `d` | delete |
| `T` | today's journal |
| `/` | full-text search |
| `t` | filter by tag (`enter` on the Tags tab does the same) |
| `esc` | clear filters |
| `J` / `K` | scroll the preview |
| `q` | quit |

## GUI

The window has a sidebar (Pages, Journal and Tags tabs, plus search) and an editor with edit, split and preview modes. Notes autosave as you type.

If a note is changed outside the app (by the CLI, the TUI or another editor), the GUI says so instead of overwriting it. Changes made elsewhere are picked up when the window regains focus.

| Shortcut | Action |
|---|---|
| Ctrl+N | new note |
| Ctrl+J | today's journal |
| Ctrl+K | search |
| Ctrl+S | save |
| Ctrl+E | cycle edit, split and preview |

## Building

You need Go 1.26+ and [go-task](https://taskfile.dev). The GUI also needs `wails3` and Node. On Linux it needs GTK3 and webkit2gtk-4.1; the Taskfiles pass `-tags gtk3`.

```
task build        # bin/katnotes
task build-gui    # katnotes-gui/bin/katnotes-gui
task dev-gui      # GUI with hot reload
task test         # go test -tags gtk3 ./...
task install      # both binaries into $GOPATH/bin
```

## Layout

```
cmd/katnotes/       CLI entry point (no args → TUI)
internal/notes/     core: Store, frontmatter, search, tags
internal/config/    config.toml load/save
internal/editor/    $EDITOR resolution
internal/cli/       cobra commands
internal/tui/       Bubble Tea app
katnotes-gui/       Wails v3 app (NotesService + Svelte frontend)
```
