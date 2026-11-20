# KatNotes

Personal notes in plain Markdown with YAML frontmatter. KatNotes has one Go core and three ways to use it:

- **`katnotes`** is a CLI and, run with no arguments, a TUI (Bubble Tea). It is a pure Go build with no CGO.
- **`katnotes-gui`** is a desktop app built with Wails v3 and Svelte.

Every frontend goes through the same `internal/notes` package, so they all read and write the same files.

## Installing on Windows

Run `KatNotes-<version>-windows-amd64-setup.exe`. It installs for your user only and doesn't need admin rights.

The installer isn't code-signed yet, so Windows warns you before running it:

- **"Windows protected your PC"** (SmartScreen): click **More info**, then **Run anyway**.
- **"Smart App Control blocked an app that may be unsafe"**: this screen has no way past it. The only option is to turn Smart App Control off in Settings > Privacy & security > Windows Security > App & browser control > Smart App Control. Windows only lets you turn it back on by resetting or reinstalling Windows.

## Where things live

| What | Where |
|---|---|
| Notes | `~/KatNotes/pages/<slug>.md` and `~/KatNotes/journal/YYYY-MM-DD.md` |
| Options | `$XDG_CONFIG_HOME/katnotes/config.toml` (usually `~/.config/katnotes/config.toml`) |
| Lua `/` commands (GUI editor) | `~/.config/katnotes/slash/<name>.lua` |
| Lua `:` commands (TUI and GUI command window) | `~/.config/katnotes/commands/<name>.lua` |
| Page templates | `~/.config/katnotes/templates/<name>.md` (an example `meeting.md` is written on first run) |
| Colour themes | `~/.config/katnotes/themes/<name>.toml` (`light.toml` and `dark.toml` are written on first run), shared by the TUI and the GUI |
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
editor = ""                 # falls back to $VISUAL, then $EDITOR, then nano/vim/vi; quotes work as in a shell, e.g. "emacsclient -nw -a ''"
previewer = ""              # TUI "o": Markdown viewer, e.g. "katdown" or "glow -p"; empty tries katdown, glow, frogmouth, mdcat
previewer_mode = "auto"     # auto (glow/mdcat/bat/less… use the terminal, others open a window), terminal or window
default_kind = "page"       # what `katnotes new` makes
journal_template = "# {{.Date}}\n\n"   # fields: .Date .ID .Weekday .Time
date_format = "yyyy-mm-dd"  # journal titles, and dates typed for new entries: dd/mm/yyyy, mm/dd/yyyy, dddd d mmmm yyyy… (or a Go layout)
task_done_date = true       # checking a task adds {done:YYYY-MM-DD}

start_page = "none"         # none, last, today, page/<id> or journal/<id> (last: the latest note opened in either app)
frontmatter_collapsed = false  # GUI table / TUI preview start folded (TUI: F toggles)
theme = "auto"              # auto (light or dark, following the system / terminal) or a file name in themes/
tui_theme = ""              # the TUI's own theme; empty uses theme
gui_theme = ""              # the GUI's own theme; empty uses theme
open_with = ""              # :external (GUI: Ctrl+Shift+O), e.g. "emacsclient -c -a ''"; empty = system default app
tui_open_with = ""          # the TUI's own :external program, e.g. "emacsclient -nw -a ''"; empty uses open_with

[font]                      # GUI fonts; empty families use the system ones
body = ""                   # e.g. "Inter Variable"
mono = ""                   # e.g. "JetBrains Mono"
size = 14                   # base text size in px (8-40)
```

To change these, run `katnotes config set <key> <value>` (use `font.size` etc. for the `[font]` table), open the file with `katnotes config edit`, or use the GUI's Settings dialog, which covers every option. In the GUI, `:reload` in the command palette picks up hand edits.

### Themes

Each `.toml` file in `~/.config/katnotes/themes/` is a colour theme, named after the file. The first time either app runs, it writes `light.toml` and `dark.toml` there: every colour, each with a comment saying what it colours. Edit them, or copy one to `ocean.toml` and change what you like. Colours you leave out come from the built-in light or dark theme, whichever `dark = true/false` picks, so a theme can be just a few lines:

```toml
dark = true
[colors]
accent = "#2ec4b6"
heading = "#2ec4b6"
```

`theme = "auto"` uses the themes named `light` and `dark`, following the system setting in the GUI and the terminal background in the TUI. Both apps use `theme` unless `tui_theme` or `gui_theme` gives one its own. `:theme <name>` sets the theme of the app you type it in (its `tui_theme` / `gui_theme`), and `:theme shared` clears that so the app follows `theme` again. You can also use `katnotes config set theme <name>` (or `tui_theme` / `gui_theme`), or the shared, GUI and TUI pickers in the GUI's Settings. The GUI re-reads theme files when its window regains focus, so edits show straight away; the TUI applies them on `:theme` or the next start. A theme file with a mistake still loads (bad values keep the built-in colour) and the app says what's wrong. The TUI uses the theme's accent, muted, border, heading and status colours, and keeps your terminal's own background and text colours.

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

Notes are cached by file time and size, along with their parsed links and tasks, so lists, searches, backlinks and the task views stay quick with thousands of files (about 20–60 ms at 10,000 notes). Edits made outside the apps are picked up.

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
- **Quick link:** press `L` (TUI or GUI; Ctrl+Shift+L while typing in the GUI) or run `:link <note>`, pick a note, and `- [Title](…)` is added under a `## Links` section at the end of the note, created the first time.
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
katnotes journal [day]         # a journal day: tomorrow, -3, +1, 2026-11-20 (--no-edit)
katnotes ls [-p|-j] [-t tag] [-s updated|title|id]
katnotes show <id>             # also accepts: today, yesterday, page/<id>, or a (unique part of a) title
katnotes edit <id>
katnotes mv <id> "New title"   # pages move to the new slug; links to them are updated
katnotes rm <id> [-f]
katnotes search <query> [-p|-j] [--name|--tags]   # full text by default; --name fuzzy, --tags all must match
katnotes search <query> --save <name>              # also keep it as a saved search (TUI and GUI see it too)
katnotes search --saved <name>                     # run a saved search
katnotes saved [rm <name>]     # list or delete saved searches
katnotes tags
katnotes tag add|rm <id> <tag…>
katnotes set <id> <field> <value>   # frontmatter, as :set; unset <id> <field> removes one
katnotes pin|unpin <id>
katnotes link <id>             # print a Markdown link to the note
katnotes recent                # notes opened lately in the TUI or GUI
katnotes git status            # notes changed since the last commit
katnotes theme [name] [--app tui|gui]   # show or set the shared (or one app's) theme
katnotes tasks sync [-n]       # set/remove contains_task in every note (-n: dry run)
katnotes tasks list [--overdue] [--due-before fri] [--pri high] [--where key:value] [-a]
katnotes tasks set page/<id>:<line> due tomorrow   # set {due:…}; no value removes the key
katnotes links <id>            # notes this note links to (marks missing ones)
katnotes backlinks <id>        # notes that link here, with the lines
katnotes config path|show|edit|set
```

Every command accepts `--json` for scripting. The commands shared with the command window below (`tag`, `set`, `pin`, `journal`, `todo`, `recent`, `git status`, `theme`, saved searches) run the same code as in the TUI and GUI, and `mv` and `rm` keep the apps' recent-notes list up to date.

## TUI keys

| Key | Action |
|---|---|
| `j` / `k` | move the selection |
| `gg` / `G` | first / last item |
| `ctrl+d` / `ctrl+u` | half a page down / up the list |
| `ctrl+f` / `ctrl+b` | next / previous page of the list |
| `tab` / `1`–`4` / `gt` / `gT` | switch between Pages, Journal, Tags and Tasks (`h` / `l` too) |
| `ctrl+e` / `ctrl+y` | scroll the preview a line (`pgdn` / `pgup`: half a page) |
| `n` | new note |
| `enter` / `e` | edit in `$EDITOR` |
| `p` / `gx` | open in the Markdown previewer (`previewer` in the config, e.g. katdown or glow) |
| `r` | rename |
| `T` | edit the note's tags in one prompt, prefilled with the current ones (`work, ideas`; empty clears) |
| `L` | link the note to another: type part of its name, `enter` adds `- [Title](…)` under a `## Links` section (created if needed) |
| `dd` | delete (asks first) |
| `J` / `ctrl+j` | today's journal |
| `/` | live search: Tab switches Name / Tags / Text, Enter keeps it, Esc clears |
| `ctrl+p` | quick open a note by name |
| `s` | sort Pages / Journal: modified, created, title |
| `t` | filter by tag (`enter` on the Tags tab does the same) |
| `esc` | clear filters |
| `R` | reload notes from disk (same as `:reload`) |
| `:` | command window: `:open`, `:tag add`, `:set`, `:frontmatter`, … with completion (see [Command window](#command-window)) |
| `v` | Tasks tab: switch between notes with tasks and all tasks |
| `a` | all tasks: cycle open → done → all |
| `w` | all tasks: cycle any date → overdue → due this week |
| `F` | show or collapse the frontmatter above the preview |
| `x` / space | Tasks tab: check or uncheck the selected task |
| `@` | all tasks: set the due date (`2026-11-20`, `today`, `tom`, `fri`, `+3d`, `+2w`; empty clears) |
| `p` | all tasks: cycle priority (high → med → low → none) |
| `enter` | Tasks tab: edit the note at the task's line (vim, nano, helix, code and others) |
| `gb` | notes that link to the selected note (esc closes) |
| `gf` | notes the selected note links to; `enter` on a (missing) one creates it |
| `?` / `F1` | help popup (shows the version; scrolls if the window is short) |
| `q` / `ZZ` / `:q` | quit |

`g`, `d` and `Z` start two-key sequences, as in vim; the status bar shows the pending key, and `esc` cancels it. The command window also takes `:q`, `:q!` and `:h`.

## GUI

The window has a sidebar (Pages, Journal, Tags and Tasks tabs, plus search) and an editor with edit, split and preview modes. Notes autosave as you type. A collapsible table above the editor shows the frontmatter, and task boxes in the preview can be clicked.

In the editor, `/` at the start of a line or after a space opens a command menu: `/todo`, `/due`, `/pri`, `/start` (make the line a task and jump into its `{…}` block with value suggestions), `/today`, `/tomorrow`, `/yesterday`, `/time`, plus your own Lua commands (below).

If a note is changed outside the app (by the CLI, the TUI or another editor), the GUI says so instead of overwriting it. Changes made elsewhere are picked up when the window regains focus.

| Shortcut | Action |
|---|---|
| Ctrl+N | new note |
| Ctrl+J (or `J`) | today's journal |
| F2 | rename: put the cursor in the title field |
| Ctrl+F / Ctrl+K (or `/`) | search (Tab in the box switches Name / Tags / Text) |
| Ctrl+P | quick open a note by name |
| Ctrl+S | save |
| Ctrl+1 … Ctrl+4 | Pages, Journal, Tags and Tasks tabs |
| Ctrl+E | cycle edit, split and preview |
| `[[` | link to a page or journal day (picker; saved as a Markdown link) |
| `L` / Ctrl+Shift+L | link the note to another: pick it, and `- [Title](…)` is added under `## Links` (`L` outside text fields) |
| `T` / Ctrl+Shift+G | edit the note's tags, comma separated; Enter saves (`T` outside text fields) |
| Ctrl+Enter | make the line a task, or check/uncheck it |
| Ctrl+Shift+K | delete the line (or every line in the selection) |
| Enter | on a list item or task: continue the list below, with the same bullet, the next number, or a new unchecked box (Shift+Enter for a plain new line); on an empty item: remove it |
| `{` on a task line | start a `{key:value}` block: suggests `due`, `pri`, `start` and keys you've used, then values (dates like `today`, `fri`, `+3d` become `YYYY-MM-DD`) |
| Ctrl+Space | on a task line: open those suggestions, adding the `{}` block if needed |
| Ctrl+Shift+P (or `:`) | command palette, the same commands as the TUI's `:` (see [Command window](#command-window)) |
| Ctrl+Shift+T | all tasks from every note: filter by due date, sort by due, set the due date and priority per task |
| Ctrl+Shift+O | open the note in the default app (`open_with`) |
| Ctrl+Shift+Z | zoom: hide the sidebar and top bar |
| Ctrl+= / Ctrl+- / Ctrl+0 | larger, smaller, reset text size |
| `?` / F1 | help popup with the app version |

The keys in backticks (`/`, `:`, `J`, `?`) are the TUI's, and work when you're not typing in a field.

### Lua commands

KatNotes runs your own commands written in Lua (gopher-lua, standard library available). The full guide, with more examples, is in [docs/lua-scripting.md](docs/lua-scripting.md). There are two kinds, each in its own folder of the config directory, and each folder gets a commented `_example.lua` on first launch. Files starting with `_` are ignored, and names may use `a-z`, `0-9`, `_` and `-`.

**`/` commands** (`~/.config/katnotes/slash/<name>.lua`) work in the GUI editor: typing `/name` runs the script and puts what it returns into the note. They are stopped after 2 s, so typing never stalls. After editing one, run `:reload`.

```lua
-- ~/.config/katnotes/slash/sig.lua
return {
  description = "Insert my signature",
  run = function(ctx)   -- ctx: line, before, after, body, note{kind,id,title,tags,path}, notes_dir, config_dir
    return "— me, " .. os.date("%Y-%m-%d")   -- text replaces /sig; nil removes it;
  end,                                          -- { line = "…", cursor = n } replaces the line
}
```

**`:` commands** (`~/.config/katnotes/commands/<name>.lua`) join the built-in commands in the command window of both apps, as `:name [args…]`, with completion and help. The script gets the typed text as `ctx.args`, plus `app`, `body`, `note`, `notes_dir` and `config_dir` for the selected or open note. Returned text (or `{ status = "…" }`) is shown as a message, `{ open = "page/id" }` goes to that note, and the note list reloads afterwards, so scripts can write notes. They are stopped after 2 s unless they set `timeout = <seconds>` (up to 300); `long = true` runs them in the background like `:sync`. A file named like a built-in command (`sync.lua`) is ignored.

```lua
-- ~/.config/katnotes/commands/wordcount.lua
return {
  description = "Count the words in the note",
  run = function(ctx)
    local n = 0
    for _ in ctx.body:gmatch("%S+") do n = n + 1 end
    return ctx.note.title .. ": " .. n .. " words"
  end,
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
| `tags <tag, …>` | replace all of the note's tags (commas or spaces between them; no tags clears them) |
| `set <field> <value>` | set a frontmatter field, adding frontmatter to files that have none. Values are YAML, so `true`, `3` and `[a, b]` keep their types; `set title …` renames, `set tags …` replaces the tags |
| `unset <field>` | remove a frontmatter field |
| `rename <title>` | rename the note and update links to it |
| `frontmatter` | edit all of the note's frontmatter as YAML (in `$EDITOR` in the TUI, in a dialog in the GUI) |
| `open-with <program>` | open the note once in another program, e.g. `open-with code` or `open-with nvim -R`; Tab suggests the editors you have installed. Terminal programs (vim, hx, nano, glow…) take over the TUI's screen, or get a new terminal window in the GUI; window programs (code, gedit, typora…) start alongside. Quotes work as in a shell (`-a ''` is an empty argument). In the TUI a program KatNotes doesn't know (emacs, emacsclient…) gets the terminal and the GUI starts it on its own, so give it the options for each: `-nw` in the TUI, `-c` in the GUI; end the line with `&` to start it in the background instead. The GUI then watches the file while the note stays open, so changes saved in the other program appear right away; if you have unsaved edits in the GUI too, it shows the conflict bar instead of overwriting either side. `external` does the same |
| `external` | open the note in `open_with`, or in the TUI `tui_open_with` if set (handled like `open-with`); with neither, the system's default app |
| `copy-link` | copy a Markdown link to the note |
| `link <note>` | link this note to another: adds `- [Title](../pages/id.md)` under its `## Links` heading, creating the heading at the end of the note the first time (nothing happens if the link is already there) |
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
| `theme [name]` | set this app's colour theme (`auto`, `light`, `dark`, your own, or `shared` to follow the `theme` option; see Themes); with no name, show the current one |
| `sync-tasks`, `terminal`, `reload`, `help` | as in each app; the GUI also has `zoom`, the TUI `quit` |

Tab completes the command and then each argument: note titles, tags in use (tags the note already has are left out), frontmatter fields and values already used in other notes, and journal days. ↑↓ choose a suggestion, and Enter runs the line. If you picked a suggestion with ↑↓, Enter uses it.

Lists that commands return (`recent`, `git status`, `links`, `backlinks`) open in the palette in the GUI and in the list pane in the TUI. Enter opens a note, and Esc closes the list.

On an empty prompt, ↑ steps back through earlier commands and the most recent ones are listed first. The history is shared by the TUI and the GUI and is kept in `command_history` in the config directory.

### Page templates

Every `.md` file in `~/.config/katnotes/templates/` is a template for `new from <name> <title>`, the GUI's new-page dialog and `katnotes new --template`. Templates are Go text/templates with `{{.Title}}`, `{{.Date}}`, `{{.ID}}` (today as YYYY-MM-DD), `{{.Weekday}}` and `{{.Time}}`. Frontmatter in a template (tags, `status: draft`…) is copied to the new page. The full guide, covering page templates and `journal_template`, is in [docs/templates.md](docs/templates.md).

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
task install      # both binaries into $GOPATH/bin; in WSL, the Windows versions too
task build-windows    # bin/katnotes.exe and katnotes-gui/bin/katnotes-gui.exe
task install-windows  # from WSL: the Windows part of task install on its own
task build-win-installer  # bin/KatNotes-<version>-windows-amd64-setup.exe, to hand out
```

### Windows installer

`task build-win-installer` builds an installer for Windows users without any dev tools (it needs `makensis`: `sudo apt install nsis`). It installs for the current user without asking for admin rights, into `%LOCALAPPDATA%\Programs\katnotes`:

- `katnotes-gui.exe` with Start Menu and desktop shortcuts, and `katnotes.exe` on the user's PATH;
- the WebView2 runtime, if Windows doesn't already have it;
- an uninstaller under Settings > Apps, which removes the programs and the PATH entry but leaves notes and settings.

It also works silently: `setup.exe /S`, or `/S /D=C:\path` for another folder. The installer isn't code-signed, so Windows SmartScreen warns on first run ("More info" > "Run anyway").

### Windows from WSL

In WSL, `task install` also cross-compiles both apps for Windows (no extra toolchain needed) and runs `install-windows`, which:

- copies `katnotes.exe` and `katnotes-gui.exe` to `%LOCALAPPDATA%\Programs\katnotes`;
- adds a **KatNotes** Start Menu shortcut, and puts that folder on your Windows PATH so `katnotes` works in PowerShell and cmd;
- points the Windows `notes_dir` at your WSL notes (`\\wsl.localhost\<distro>\…`), so both versions share one set of notes. This only happens while the Windows config still uses the default folder and that folder has no notes; the Windows app otherwise keeps its own config in `%APPDATA%\katnotes`, since Linux settings like `emacsclient` don't work there;
- installs `katnotes-win` into `$GOPATH/bin`, which starts the Windows GUI from the WSL terminal without holding the shell.

Set `WIN_INSTALL_DIR` to install somewhere else.

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
