# Lua scripting in KatNotes

KatNotes lets you add your own commands, written in Lua. A command is one `.lua` file; drop it in the right folder and it shows up next to the built-in commands.

There are two kinds:

| Kind | Where you type it | Folder | Good for |
|---|---|---|---|
| **`/` commands** | `/name` in the GUI editor | `<config>/slash/` | inserting or rewriting text where you are typing |
| **`:` commands** | `:name [args…]` in the command window (`:` in the TUI, Ctrl+Shift+P in the GUI), or `katnotes run name [args…]` | `<config>/commands/` | actions: showing information, writing notes, jumping to a note, running programs |

`<config>` is the KatNotes config folder:

| System | Usual location |
|---|---|
| Linux | `~/.config/katnotes` (or `$XDG_CONFIG_HOME/katnotes`) |
| macOS | `~/Library/Application Support/katnotes` |
| Windows | `%AppData%\katnotes` |

Set `KATNOTES_CONFIG_DIR` to use a different folder.

The first time the TUI or the GUI starts, it creates both folders and puts a commented `_example.lua` in each. Those examples are the shortest reference to the API, and they stay in sync with the version you have installed.

## Your first command

Create `<config>/slash/sig.lua`:

```lua
return {
  description = "Insert my signature",
  run = function(ctx)
    return "— me, " .. os.date("%Y-%m-%d")
  end,
}
```

In the GUI, run `:reload` from the command palette (Ctrl+Shift+P), then type `/sig` in a note. The menu lists it with a `lua` badge, and choosing it replaces `/sig` with `— me, 2026-11-20`.

Every command file follows this pattern: it **returns a table** with a `description` (shown in menus and help) and a `run(ctx)` function. KatNotes calls `run` with a context table and uses what it returns.

## File rules

- The file name is the command name: `wordcount.lua` becomes `:wordcount`. Names may use only `a-z`, `0-9`, `_` and `-`.
- Files whose names start with `_` are ignored. Use this to switch a command off, or to keep helper code and examples around.
- If a file has the same name as a built-in command (for example `sync.lua` in `commands/`, or `today.lua` in `slash/`), the file is ignored and the built-in command runs. The GUI tells you when this happens.
- A file with a syntax error doesn't hide the others. The GUI lists the problem when it loads the commands.

### Picking up changes

| | GUI | TUI |
|---|---|---|
| `/` commands | `:reload` | (the TUI has no `/` menu) |
| `:` commands | `:reload` | automatic, the next time you type `:` |

## `/` commands (GUI editor)

Typing `/` at the start of a line or after a space opens the command menu. When you pick one of your commands, KatNotes runs the script with the current line and note.

### The context (`ctx`)

| Field | What it holds |
|---|---|
| `ctx.line` | the current line, with the `/name` token taken out |
| `ctx.before` | the text on the line before the token |
| `ctx.after` | the text on the line after the token |
| `ctx.body` | the whole note as Markdown, without frontmatter, including edits that aren't saved yet |
| `ctx.note.kind` | `"page"` or `"journal"` |
| `ctx.note.id` | the file name without `.md`, e.g. `hello-world` or `2026-11-20` |
| `ctx.note.title` | the title as currently typed |
| `ctx.note.tags` | a list of tags, e.g. `{ "idea", "go" }` |
| `ctx.note.path` | the full path of the note's file |
| `ctx.notes_dir` | the notes folder |
| `ctx.config_dir` | the KatNotes config folder |
| `ctx.source` | always `"slash"` here |
| `ctx.app` | always `"gui"` here |

### What `run` can return

| Return value | Effect |
|---|---|
| a string (or number) | replaces `/name` with that text |
| `nil` | removes `/name` and does nothing else |
| `{ text = "…" }` | same as returning a string |
| `{ line = "…" }` | replaces the **whole line**; the cursor goes to the end |
| `{ line = "…", cursor = n }` | replaces the whole line and puts the cursor at column `n` (counting from 0) |

If you edit the line while the script is running, KatNotes doesn't apply the result, so it never overwrites what you typed.

### Time limit

A `/` command is stopped after **2 seconds**, so a slow script can't stall typing. Keep these scripts quick and local. For anything slow, write a `:` command instead.

## `:` commands (command window)

These appear in the command window of both apps, with completion and help, as `:name [args…]`.

### The context (`ctx`)

| Field | What it holds |
|---|---|
| `ctx.args` | everything typed after the name, as one string (`""` if nothing) |
| `ctx.app` | `"tui"`, `"gui"` or `"cli"` (`katnotes run`) |
| `ctx.source` | always `"command"` here |
| `ctx.body` | the selected (TUI) or open (GUI) note's Markdown, without frontmatter |
| `ctx.note` | `{ kind, id, title, tags, path }` for that note |
| `ctx.notes_dir` | the notes folder |
| `ctx.config_dir` | the KatNotes config folder |

If no note is selected or open, `ctx.body` and the `ctx.note` fields are empty strings and `ctx.note.tags` is an empty table, so check before you rely on them:

```lua
if ctx.note.id == "" then
  return "open a note first"
end
```

`ctx.line`, `ctx.before` and `ctx.after` are empty for `:` commands.

### What `run` can return

| Return value | Effect |
|---|---|
| a string, or `{ status = "…" }` | shows that message |
| `{ open = "…" }` | goes to that note: `"page/<id>"`, `"journal/2026-11-20"`, or a title |
| `{ open = "…", status = "…" }` | both |
| `nil` | shows `<name>: done` |

After every run (even a failed one), KatNotes reloads the note list. This means scripts can create or change files in `ctx.notes_dir` and the apps pick up the changes.

### From the command line

`katnotes run` lists your `:` commands, and `katnotes run name [args…]` runs one. It prints the status the script returns and, for `{ open = … }`, the note's key and path. `--note <id>` passes a note in as `ctx.note` and `ctx.body`, and `--json` prints the whole result. Everything after the command name goes to the script, flags included, so `katnotes run todo --urgent` gives `ctx.args = "--urgent"`.

`ctx.app` is `"cli"` here. The terminal shows a multi-line status in full, while the TUI's status bar has room for one line only, so a script can return more detail when `ctx.app == "cli"`.

### Options

Add these next to `description` and `run`:

| Field | Default | Effect |
|---|---|---|
| `timeout = 60` | 2 | seconds the script may run, at most 300 |
| `long = true` | `false` | runs the command in the background, like `:sync`, so the app stays usable |

`timeout` only applies to `:` commands. A `/` command always gets 2 seconds.

## Notes on disk

Scripts that read or write notes work with plain files:

- Pages: `<notes_dir>/pages/<slug>.md`
- Journal days: `<notes_dir>/journal/YYYY-MM-DD.md`

A note starts with YAML frontmatter:

```markdown
---
title: Hello World
tags: [idea, go]
---
# Hello World

Body text…
```

Frontmatter is optional. A file without it still works: KatNotes takes the title from the first `# heading`, or from the file name. Tasks are ordinary Markdown checkboxes, optionally followed by one `{key:value …}` block:

```markdown
- [ ] Call the plumber {due:2026-11-20 pri:high}
```

Forward slashes work in paths on every system, Windows included, so `ctx.notes_dir .. "/pages/inbox.md"` is fine.

## Examples

### `/` — quote the current line

```lua
-- slash/quote.lua: turn the line into a blockquote
return {
  description = "Make this line a quote",
  run = function(ctx)
    return { line = "> " .. ctx.line }
  end,
}
```

### `/` — make the line a task due in a week

```lua
-- slash/week.lua
return {
  description = "Task due one week from today",
  run = function(ctx)
    -- drop any existing "- [ ] " or "- " prefix and surrounding spaces
    local text = ctx.line:gsub("^%s*[-*]%s+%[[ xX]%]%s*", "")
    text = text:gsub("^%s*[-*]%s+", ""):gsub("^%s+", ""):gsub("%s+$", "")
    local due = os.date("%Y-%m-%d", os.time() + 7 * 24 * 60 * 60)
    local line = "- [ ] " .. text
    -- put the cursor after the task text, before the {…} block
    return { line = line .. " {due:" .. due .. "}", cursor = #line }
  end,
}
```

### `/` — insert the note's tags as hashtags

```lua
-- slash/hashtags.lua
return {
  description = "Insert this note's tags as #hashtags",
  run = function(ctx)
    local out = {}
    for _, t in ipairs(ctx.note.tags) do
      table.insert(out, "#" .. t)
    end
    return table.concat(out, " ")
  end,
}
```

### `:` — word count

```lua
-- commands/wordcount.lua
return {
  description = "Count the words in the note",
  run = function(ctx)
    if ctx.note.id == "" then return "no note selected" end
    local n = 0
    for _ in ctx.body:gmatch("%S+") do n = n + 1 end
    return string.format("%s: %d words", ctx.note.title, n)
  end,
}
```

### `:` — open tasks in this note

```lua
-- commands/opentasks.lua
return {
  description = "Count unchecked tasks in the note",
  run = function(ctx)
    local open, done = 0, 0
    for line in (ctx.body .. "\n"):gmatch("(.-)\n") do
      if line:match("^%s*[-*]%s+%[ %]") then open = open + 1
      elseif line:match("^%s*[-*]%s+%[[xX]%]") then done = done + 1 end
    end
    return string.format("%d open, %d done", open, done)
  end,
}
```

### `:` — capture to an inbox page

`:inbox buy stamps` adds a task to `pages/inbox.md`, creating the page if needed, and then opens it.

```lua
-- commands/inbox.lua
return {
  description = "Add a task to the Inbox page",
  run = function(ctx)
    if ctx.args == "" then return "usage: :inbox <task>" end
    local path = ctx.notes_dir .. "/pages/inbox.md"

    local exists = io.open(path, "r")
    if exists then exists:close() end

    local f = assert(io.open(path, "a"))
    if not exists then
      f:write("---\ntitle: Inbox\ntags: [inbox]\n---\n# Inbox\n\n")
    end
    f:write("- [ ] " .. ctx.args .. "\n")
    f:close()

    return { status = "added to Inbox", open = "page/inbox" }
  end,
}
```

### `:` — jump to a journal day

```lua
-- commands/day.lua: ":day 2026-11-01"
return {
  description = "Go to a journal day (YYYY-MM-DD)",
  run = function(ctx)
    local d = ctx.args:match("^%s*(%d%d%d%d%-%d%d%-%d%d)%s*$")
    if not d then return "usage: :day YYYY-MM-DD" end
    return { open = "journal/" .. d }
  end,
}
```

Opening a journal day that doesn't exist yet fails with an error message. To create it, write the file first, as in the inbox example.

### `:` — run an external program

Slow or networked work should set `timeout` and `long`:

```lua
-- commands/weather.lua
return {
  description = "Show the weather",
  timeout = 20,
  long = true,
  run = function(ctx)
    local p = assert(io.popen("curl -s 'https://wttr.in/?format=3'"))
    local out = p:read("*a")
    p:close()
    return (out:gsub("%s+$", ""))  -- parentheses keep only the string
  end,
}
```

## Errors and debugging

- Call `error("message")` to stop and show `message`. Runtime errors are shown the same way, with the file name and line number.
- A script that runs too long is stopped with `<name>.lua: timed out after <limit>`, e.g. `after 2s`, or `after 1m0s` for a `:` command with `timeout = 60`.
- `print` output isn't shown anywhere. To inspect a value, return it:

  ```lua
  return { status = "args=[" .. ctx.args .. "] id=" .. ctx.note.id }
  ```

- The TUI is the quickest place to try a `:` command: it reloads the folder every time you type `:`, so you can edit, save and rerun without restarting.

## Things to know

- **Lua version.** KatNotes uses [gopher-lua](https://github.com/yuin/gopher-lua), which implements **Lua 5.1**. There is no `//` operator, no bitwise operators and no `utf8` library. `#s` and `string.sub` count bytes, not characters, which matters for non-ASCII text and `cursor` positions.
- **Standard library.** `string`, `table`, `math`, `os` and `io` are all available, so scripts can read and write files and run programs. Scripts run with your own permissions, so only use scripts you trust.
- **No shared state between runs.** Each run gets a new Lua state, so globals are gone when the next run starts. To remember something, write a file under `ctx.config_dir`.
- **Keep the top level free of side effects.** The file is executed both when KatNotes lists commands and every time it runs one. Put all real work inside `run`.
- **`require`.** Lua looks for modules relative to the app's working directory, not your config folder. To share helpers, put them in a file starting with `_` (so it isn't a command) and load it with `dofile(ctx.config_dir .. "/commands/_util.lua")` inside `run`.
