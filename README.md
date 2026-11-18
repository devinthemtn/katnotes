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
| Page templates | `~/.config/katnotes/templates/<name>.md` (an example `meeting.md` is written on first run) |
| Command history, recent notes | `~/.config/katnotes/command_history` and `~/.config/katnotes/recent`, shared by the TUI and the GUI |
| Sort orders, saved searches | `~/.config/katnotes/view.json`, shared by the TUI and the GUI |

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
previewer = ""              # TUI "o": Markdown viewer, e.g. "katdown" or "glow -p"; empty tries katdown, glow, frogmouth, mdcat
previewer_mode = "auto"     # auto (glow/mdcat/bat/less… use the terminal, others open a window), terminal or window
default_kind = "page"       # what `katnotes new` makes
journal_template = "# {{.Date}}\n\n"   # fields: .Date .ID .Weekday .Time
date_format = "2006-01-02"  # Go time layout for journal titles
task_done_date = true       # checking a task adds {done:YYYY-MM-DD}

start_page = "none"         # none, last, today, page/<id> or journal/<id> (last: the latest note opened in either app)
frontmatter_collapsed = false  # GUI table / TUI preview start folded (TUI: F toggles)

# GUI only
open_with = ""              # app for Ctrl+Shift+O; empty = system default (xdg-open / open / start)

[font]                      # GUI fonts; empty families use the system ones
body = ""                   # e.g. "Inter Variable"
mono = ""                   # e.g. "JetBrains Mono"
size = 14                   # base text size in px (8-40)
```

To change these, run `katnotes config set <key> <value>` (use `font.size` etc. for the `[font]` table), open the file with `katnotes config edit`, or use the GUI's Settings dialog. In the GUI, `:reload` in the command palette picks up hand edits.

## Finding notes

Both apps have the same search with three modes. In the TUI, `/` opens a live search bar and Tab switches the mode. In the GUI, use the Name / Tags / Text buttons over the search box, or press Tab in the box.

| Mode | Matches | Example |
|---|---|---|
| Name | fuzzy title or filename, best match first | `stdup` finds *Weekly Standup* |
| Tags | notes with every tag listed; the last word also matches the start of a tag as you type; `-tag` excludes | `work meet`, `home -done` |
| Text | every word and `"quoted phrase"` somewhere in the title, tags or body, showing the matching line | `budget "next week"` |

- **Quick open** (`ctrl+p` in the TUI, Ctrl+P in the GUI) jumps to any note by name. Recent notes come first.
- **Sort** each list by modified, created or title: `s` in the TUI, or the Sort menu in the GUI. Date orders add month headers, with pinned notes on top. The choice is remembered per tab.
- **Saved searches:** `:search save <name>` keeps the current search. It appears with 🔎 at the top of the Tags tab, and `:search <name>` runs it again.
- **Problem files:** a file whose frontmatter can't be read (often an imported one) is still listed, marked ⚠, with the whole file as its body. `:frontmatter` fixes it.

Notes are cached by file time and size, so lists and searches stay quick with thousands of files. Edits made outside the apps are picked up.

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

## Links

Links between notes are ordinary relative Markdown links, so katdown, GitHub and other viewers can follow them:

```markdown
Call about [Groceries](../pages/groceries.md) — see [2026-11-17](../journal/2026-11-17.md)
```

- **GUI:** type `[[` in the editor to open a picker of pages and journal days. It also reads dates, so `[[today` or `[[fri` gives that journal day. Pick a note, or choose **New page "…"**, and the link is written for you.
- **Preview:** click a link to open the note. Links to notes that don't exist yet are shown dashed, and clicking one creates the note.
- **Backlinks:** a **Linked from** panel under each note (GUI) and a footer in the TUI preview list the notes that link to it. Each entry shows the linking line.
- **Renaming** a page rewrites the links to it in every other note. Link text that matched the old title becomes the new title.
- KatNotes writes the `../pages/<id>.md` / `../journal/<date>.md` form, which works from both folders. Any relative link that resolves to a note counts, e.g. a hand-written `other.md`. Links inside code are ignored.

Find links with grep: `rg '\(\.\./pages/groceries\.md' ~/KatNotes`.

## CLI

```
katnotes                       # TUI
katnotes new "Title" -t a,b    # new page, opens $EDITOR (--no-edit, -m "body", -m - for stdin)
katnotes new -j [YYYY-MM-DD]   # new journal entry
katnotes new --template meeting "Standup"   # new page from ~/.config/katnotes/templates/meeting.md
katnotes todo call Sam {due:fri}            # add a task to today's journal (--here <id> for another note)
katnotes sync                  # git: commit the notes folder, then pull --rebase and push
katnotes today                 # open or create today's journal entry
katnotes ls [-p|-j] [-t tag] [-s updated|title|id]
katnotes show <id>             # also accepts: today, yesterday, or a page title
katnotes edit <id>
katnotes mv <id> "New title"   # pages move to the new slug; links to them are updated
katnotes rm <id> [-f]
katnotes search <query> [-p|-j] [--name|--tags]   # full text by default; --name fuzzy, --tags all must match
katnotes tags
katnotes tasks sync [-n]       # set/remove contains_task in every note (-n: dry run)
katnotes tasks list [--overdue] [--due-before fri] [--pri high] [--where key:value] [-a]
katnotes tasks set page/<id>:<line> due tomorrow   # set {due:…}; no value removes the key
katnotes links <id>            # notes this note links to (marks missing ones)
katnotes backlinks <id>        # notes that link here, with the lines
katnotes config path|show|edit|set
```

Every command accepts `--json` for scripting.

## TUI keys

| Key | Action |
|---|---|
| `tab` / `1`–`4` | switch between Pages, Journal, Tags and Tasks |
| `n` | new note |
| `enter` / `e` | edit in `$EDITOR` |
| `o` | open in the Markdown previewer (`previewer` in the config, e.g. katdown or glow) |
| `r` | rename |
| `d` | delete |
| `T` | today's journal |
| `/` | live search: Tab switches Name / Tags / Text, Enter keeps it, Esc clears |
| `ctrl+p` | quick open a note by name |
| `s` | sort Pages / Journal: modified, created, title |
| `t` | filter by tag (`enter` on the Tags tab does the same) |
| `esc` | clear filters |
| `J` / `K` | scroll the preview |
| `:` | command window: `:open`, `:tag add`, `:set`, `:frontmatter`, … with completion (see [Command window](#command-window)) |
| `v` | Tasks tab: switch between notes with tasks and all tasks |
| `a` | all tasks: cycle open → done → all |
| `w` | all tasks: cycle any date → overdue → due this week |
| `F` | show or collapse the frontmatter above the preview |
| `x` / space | Tasks tab: check or uncheck the selected task |
| `@` | all tasks: set the due date (`2026-11-20`, `today`, `tom`, `fri`, `+3d`, `+2w`; empty clears) |
| `p` | all tasks: cycle priority (high → med → low → none) |
| `enter` | Tasks tab: edit the note at the task's line (vim, nano, helix, code and others) |
| `b` | notes that link to the selected note (esc closes) |
| `f` | notes the selected note links to; `enter` on a (missing) one creates it |
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
| Ctrl+K | search (Tab in the box switches Name / Tags / Text) |
| Ctrl+P | quick open a note by name |
| Ctrl+S | save |
| Ctrl+E | cycle edit, split and preview |
| `[[` | link to a page or journal day (picker; saved as a Markdown link) |
| Ctrl+Enter | make the line a task, or check/uncheck it |
| `{` on a task line | start a `{key:value}` block: suggests `due`, `pri`, `start` and keys you've used, then values (dates like `today`, `fri`, `+3d` become `YYYY-MM-DD`) |
| Ctrl+Space | on a task line: open those suggestions, adding the `{}` block if needed |
| Ctrl+Shift+P / Ctrl+Shift+: | command palette, the same commands as the TUI's `:` (see [Command window](#command-window)) |
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

## Command window

`:` in the TUI and Ctrl+Shift+P in the GUI open the same command window. Both apps share the command list, argument completion and history. Commands that act on a note use the selected note (TUI) or the open one (GUI).

| Command | What it does |
|---|---|
| `open <note>` | go to a note by title (a unique part of the title is enough; `page/id` works too) |
| `new <title>` | create a page and start editing it |
| `journal [day]` | open a journal day: `today` (default), `yesterday`, `tomorrow`, `-3`, `+1` or `2026-11-20` |
| `tag add <tag…>` / `tag rm <tag…>` | add or remove tags |
| `set <field> <value>` | set a frontmatter field, adding frontmatter to files that have none. Values are YAML, so `true`, `3` and `[a, b]` keep their types; `set title …` renames, `set tags …` replaces the tags |
| `unset <field>` | remove a frontmatter field |
| `rename <title>` | rename the note and update links to it |
| `frontmatter` | edit all of the note's frontmatter as YAML (in `$EDITOR` in the TUI, in a dialog in the GUI) |
| `copy-link` | copy a Markdown link to the note |
| `links` / `backlinks` | notes this note links to, or that link to it |
| `delete` | delete the note (asks first) |
| `todo <task>` / `todo here <task>` | add `- [ ] task` to today's journal (or this note) without opening an editor; `{due:fri pri:h}` works like in the editor |
| `pin` / `unpin` | keep the note at the top of the list (`pinned: true` in its frontmatter; shown with 📌) |
| `new from <template> <title>` | create a page from a template (see below) |
| `recent` | the notes you opened lately, in either app |
| `search save <name>` / `search <name>` / `search rm <name>` | keep the current search, run a saved one, delete one |
| `tasks [open\|done\|all] [any\|overdue\|week]` | the all-tasks view with those filters |
| `sync` | git: commit every change in the notes folder, then pull (rebase) and push if the branch has an upstream |
| `git status` | git: the notes changed since the last commit |
| `sync-tasks`, `terminal`, `reload`, `help` | as in each app; the GUI also has `zoom` and `external`, the TUI `quit` |

Tab completes the command and then each argument: note titles, tags in use (tags the note already has are left out), frontmatter fields and values already used in other notes, and journal days. ↑↓ choose a suggestion, and Enter runs the line. If you picked a suggestion with ↑↓, Enter uses it.

Lists that commands return (`recent`, `git status`, `links`, `backlinks`) open in the palette in the GUI and in the list pane in the TUI. Enter opens a note, and Esc closes the list.

On an empty prompt, ↑ steps back through earlier commands and the most recent ones are listed first. The history is shared by the TUI and the GUI and is kept in `command_history` in the config directory.

### Page templates

Every `.md` file in `~/.config/katnotes/templates/` is a template for `new from <name> <title>`, the GUI's new-page dialog and `katnotes new --template`. Templates are Go text/templates with `{{.Title}}`, `{{.Date}}`, `{{.ID}}` (today as YYYY-MM-DD), `{{.Weekday}}` and `{{.Time}}`. Frontmatter in a template (tags, `status: draft`…) is copied to the new page:

```markdown
---
tags: [meeting]
---
# {{.Title}}

{{.Weekday}} {{.Date}} · {{.Time}}

## Notes
```

### Git sync

If the notes folder is in a git repository, `:sync` (or `katnotes sync`) commits everything in it as `katnotes sync <date time>`. When the branch has an upstream, it then pulls with `--rebase --autostash` and pushes. Only the notes folder is committed, even when it sits inside a bigger repository. Git never prompts for a password, so use an SSH key or a credential helper. Run `git push -u <remote> <branch>` once to set up the upstream.

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
