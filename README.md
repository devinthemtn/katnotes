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
| Lua `/` commands (GUI) | `~/.config/katnotes/commands/<name>.lua` |

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
task_done_date = true       # checking a task adds {done:YYYY-MM-DD}

# GUI only
open_with = ""              # app for Ctrl+Shift+O; empty = system default (xdg-open / open / start)
start_page = "none"         # none, last, today, page/<id> or journal/<id>
frontmatter_collapsed = false

[font]                      # GUI fonts; empty families use the system ones
body = ""                   # e.g. "Inter Variable"
mono = ""                   # e.g. "JetBrains Mono"
size = 14                   # base text size in px (8-40)
```

To change these, run `katnotes config set <key> <value>` (use `font.size` etc. for the `[font]` table), open the file with `katnotes config edit`, or use the GUI's Settings dialog. In the GUI, `:reload` in the command palette picks up hand edits.

## Tasks

Markdown tasks (`- [ ] buy milk`, `- [x] done`) are plain GFM, so they work in any Markdown editor. KatNotes adds:

- `contains_task: true` in a note's frontmatter while it has tasks. It is kept up to date on every save; `katnotes tasks sync` backfills notes written by other tools.
- A Tasks tab in the TUI and GUI listing notes with tasks, and a view of every task across notes.

### Task metadata

Due dates, priority and anything else go in one `{key:value …}` block at the end of the task line:

```markdown
- [ ] Call the plumber {due:2026-11-20 pri:high}
- [ ] Draft report #work {due:2026-11-21}
- [x] Pay rent {due:2026-11-01 done:2026-10-30}
- [ ] Book venue {due:2026-12-01 note:"ask about parking"}
```

Other Markdown viewers show the block as plain text. The KatNotes preview shows it as chips, and overdue tasks are marked.

- Only a block at the **end** of the line counts. Braces anywhere else are just text, and so is a block inside `` `code` ``.
- The block holds space-separated `key:value` pairs. Keys are lowercase (`[a-z][a-z0-9_-]*`). A value with spaces goes in double quotes. If any token isn't `key:value`, the braces stay text, so prose like `{x}` is safe.
- Dates are `YYYY-MM-DD`, so they sort and grep well.

| Key | Meaning |
|---|---|
| `due` | due date; tasks sort by it and overdue ones are marked |
| `pri` | `high`, `med` or `low` |
| `done` | date the task was checked. KatNotes adds it when you check a task and removes it when you uncheck one (turn off with `task_done_date = false`) |
| `start` | not actionable before this date; shown dimmed until then |

Any other key (e.g. `project:house`) is kept and can be filtered with `--where`.

Searching is plain grep:

```sh
rg 'due:2026-11' ~/KatNotes        # due this month
rg '\{[^}]*pri:high' ~/KatNotes     # high priority
```

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
katnotes tasks sync [-n]       # set/remove contains_task in every note (-n: dry run)
katnotes tasks list [--overdue] [--due-before fri] [--pri high] [--where key:value] [-a]
katnotes tasks set page/<id>:<line> due tomorrow   # set {due:…}; no value removes the key
katnotes config path|show|edit|set
```

Every command accepts `--json` for scripting.

## TUI keys

| Key | Action |
|---|---|
| `tab` / `1`–`4` | switch between Pages, Journal, Tags and Tasks |
| `n` | new note |
| `enter` / `e` | edit in `$EDITOR` |
| `r` | rename |
| `d` | delete |
| `T` | today's journal |
| `/` | full-text search |
| `t` | filter by tag (`enter` on the Tags tab does the same) |
| `esc` | clear filters |
| `J` / `K` | scroll the preview |
| `:` | command window with completion (`:sync-tasks`, `:tasks`, `:reload`, …) |
| `v` | Tasks tab: switch between notes with tasks and all tasks |
| `a` | Tasks tab: show or hide done tasks |
| `x` / space | Tasks tab: check or uncheck the selected task |
| `@` | all tasks: set the due date (`2026-11-20`, `today`, `tom`, `fri`, `+3d`, `+2w`; empty clears) |
| `p` | all tasks: cycle priority (high → med → low → none) |
| `enter` | Tasks tab: edit the note at the task's line (vim, nano, helix, code and others) |
| `?` / `F1` | help popup (shows the version; scrolls if the window is short) |
| `q` | quit |

## GUI

The window has a sidebar (Pages, Journal, Tags and Tasks tabs, plus search) and an editor with edit, split and preview modes. Notes autosave as you type. A collapsible table above the editor shows the frontmatter, and task boxes in the preview can be clicked.

In the editor, `/` at the start of a line or after a space opens a command menu: `/todo`, `/due`, `/pri`, `/start` (make the line a task and jump into its `{…}` block with value suggestions), `/today`, `/tomorrow`, `/yesterday`, `/time`, plus your own Lua commands (below).

If a note is changed outside the app (by the CLI, the TUI or another editor), the GUI says so instead of overwriting it. Changes made elsewhere are picked up when the window regains focus.

| Shortcut | Action |
|---|---|
| Ctrl+N | new note |
| Ctrl+J | today's journal |
| Ctrl+K | search |
| Ctrl+S | save |
| Ctrl+E | cycle edit, split and preview |
| Ctrl+Enter | make the line a task, or check/uncheck it |
| `{` on a task line | start a `{key:value}` block: suggests `due`, `pri`, `start` and keys you've used, then values (dates like `today`, `fri`, `+3d` become `YYYY-MM-DD`) |
| Ctrl+Space | on a task line: open those suggestions, adding the `{}` block if needed |
| Ctrl+Shift+P / Ctrl+Shift+: | command palette (zoom, open, terminal, tasks, reload) |
| Ctrl+Shift+T | all tasks from every note: filter by due date, sort by due, set the due date and priority per task |
| Ctrl+Shift+O | open the note in the default app (`open_with`) |
| Ctrl+Shift+Z | zoom: hide the sidebar and top bar |
| Ctrl+= / Ctrl+- / Ctrl+0 | larger, smaller, reset text size |
| `?` / F1 | help popup with the app version (`?` works when you're not typing in a field) |

### Lua commands

Each `<name>.lua` file in `~/.config/katnotes/commands/` becomes a `/name` command in the GUI editor (gopher-lua, standard library available, 2 s limit per run). A commented `_example.lua` is created there on first launch; files starting with `_` are ignored.

```lua
-- ~/.config/katnotes/commands/sig.lua
return {
  description = "Insert my signature",
  run = function(ctx)   -- ctx: line, before, after, body, note{kind,id,title,tags,path}, notes_dir, config_dir
    return "— me, " .. os.date("%Y-%m-%d")   -- text replaces /sig; nil removes it;
  end,                                          -- { line = "…", cursor = n } replaces the line
}
```

## Building

You need Go 1.26+ and [go-task](https://taskfile.dev). The GUI also needs `wails3` and Node. On Linux it needs GTK3 and webkit2gtk-4.1; the Taskfiles pass `-tags gtk3`.

```
task build        # bin/katnotes
task build-gui    # katnotes-gui/bin/katnotes-gui
task dev-gui      # GUI with hot reload
task test         # go test -tags gtk3 ./... and the frontend unit tests (npm test)
task install      # both binaries into $GOPATH/bin
```

## Layout

```
cmd/katnotes/       CLI entry point (no args → TUI)
internal/notes/     core: Store, frontmatter, search, tags
internal/config/    config.toml load/save
internal/editor/    $EDITOR resolution (and open-at-line)
internal/luacmd/    user Lua slash commands
internal/cli/       cobra commands
internal/tui/       Bubble Tea app
katnotes-gui/       Wails v3 app (NotesService + Svelte frontend)
```
