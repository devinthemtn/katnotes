# Templates in KatNotes

Templates fill in a new note for you. There are two kinds:

| Kind | Used when | Lives in |
|---|---|---|
| **Page templates** | you create a page and pick a template | one `.md` file each, in `<config>/templates/` |
| **The journal template** | any new journal day is created | the `journal_template` option in `config.toml` |

`<config>` is the KatNotes config folder: usually `~/.config/katnotes` on Linux, `~/Library/Application Support/katnotes` on macOS, and `%AppData%\katnotes` on Windows (or whatever `KATNOTES_CONFIG_DIR` points to).

Both kinds use [Go templates](https://pkg.go.dev/text/template): plain text with fields like `{{.Title}}` that KatNotes replaces when the note is created.

## Page templates

### Making one

Each Markdown file in `<config>/templates/` is a template, named after the file: `meeting.md` is the template `meeting`. Names may use letters, digits, `_` and `-`. The first time the TUI or GUI starts, it writes an example, `meeting.md`:

```markdown
---
tags: [meeting]
---
# {{.Title}}

{{.Weekday}} {{.Date}} · {{.Time}}

## Attendees

## Notes

## Actions

```

New and edited templates are picked up right away, with no reload needed.

### Using one

| Where | How |
|---|---|
| Command window (TUI `:`, GUI Ctrl+Shift+P) | `new from meeting Weekly sync` (Tab completes template names) |
| GUI | Ctrl+N, then choose it from the **Template** list in the new-page dialog |
| CLI | `katnotes new --template meeting "Weekly sync"` (add `-t a,b` for extra tags) |

Templates are for pages only. Journal days use the journal template (below).

### Fields

| Field | Value | Example |
|---|---|---|
| `{{.Title}}` | the title you gave the new page | `Weekly sync` |
| `{{.Date}}` | today, in your `date_format` | `2026-11-20`, or `Friday 20 November 2026` |
| `{{.ID}}` | today as `YYYY-MM-DD`, whatever your `date_format` | `2026-11-20` |
| `{{.Weekday}}` | today's day of the week, in English | `Friday` |
| `{{.Time}}` | the current time, 24-hour | `09:05` |

A misspelled field such as `{{.Titel}}` comes out empty.

### Frontmatter

A template can start with frontmatter. Everything in it is copied to the new page, with a few exceptions:

- **`tags`** are added to the page, along with any tags you typed when creating it.
- **`title`, `created` and `updated`** are ignored. The page always gets your title and the current time.
- **`contains_task`** is ignored. KatNotes sets it on save, based on whether the body has tasks.
- **Any other key** (`status`, `project`, `due`…) is copied as is.

Fields work in frontmatter too:

```markdown
---
tags: [meeting]
status: draft
meeting_date: "{{.ID}}"
---
```

**Quote dates in frontmatter.** An unquoted `meeting_date: {{.ID}}` is read as a timestamp and saved as `2026-11-20T00:00:00Z`. Writing `"{{.ID}}"` keeps it as `2026-11-20`.

### The body

The body is everything after the frontmatter. It's written to the new page exactly as rendered, so it's usually a good idea to start with `# {{.Title}}`. If the body is empty, for example in a template that only sets tags, the page gets `# <title>` as usual.

Tasks in a template work like tasks anywhere else, so a checklist template is just a list of tasks:

```markdown
- [ ] Book the room
- [ ] Send the agenda {due:fri}
```

## The journal template

Every new journal day starts from `journal_template`. This applies however the day gets created: `J` / Ctrl+J, `katnotes today`, `katnotes todo`, opening a day from a link, and so on. The default is just a heading:

```toml
journal_template = "# {{.Date}}\n\n"
```

### Fields

| Field | Value |
|---|---|
| `{{.Date}}` | the journal day, in your `date_format` |
| `{{.ID}}` | the journal day as `YYYY-MM-DD` |
| `{{.Weekday}}` | the journal day's day of the week, in English |
| `{{.Time}}` | the time the entry was created, 24-hour |

The date fields describe the **journal day**, not today. Opening next Monday's entry gives `Monday` and next Monday's date.

There is no `{{.Title}}`: a journal day's title is its date. A misspelled field comes out as `<no value>`, not empty as it does in page templates.

### Setting it

Pick whichever is easiest:

- **GUI:** Settings → **Journal template** (a multi-line box).
- **CLI:** `katnotes config set journal_template "# {{.Date}}\n\n## Today\n\n"`. Write `\n` for each new line.
- **By hand:** `katnotes config edit`. TOML's triple quotes let you write it over several lines:

  ```toml
  journal_template = """
  # {{.Weekday}} {{.Date}}

  ## Plan

  - [ ] 

  ## Log

  """
  ```

  A newline straight after the opening `"""` is dropped, so this starts with the heading. In the GUI, run `:reload` after editing the file.

### No frontmatter

The journal template is **body only**. Frontmatter in it isn't read, so it ends up as text in the body. Journal days get their title, `created` and `updated` automatically. To tag a day, add tags after creating it (`:tag add …`, or `T` in the TUI).

Templates apply only when a note is **created**. Editing a template doesn't change notes made from it earlier.

## Going further with Go templates

The fields cover most uses, but Go templates can also make choices. These work in both kinds of template.

**Something only on certain days:**

```
# {{.Weekday}} {{.Date}}
{{if eq .Weekday "Monday"}}
## Plan for the week
{{end}}
{{- if or (eq .Weekday "Saturday") (eq .Weekday "Sunday")}}
Weekend — no standup.
{{end}}
```

**Trimming blank lines.** A `-` inside the braces removes the whitespace and newlines on that side: `{{- …}}` trims before, and `{{… -}}` trims after. Use it to stop `{{if}}` / `{{end}}` lines from leaving gaps in the note.

**Comments** that don't appear in the note:

```
{{/* Remember to fill in the attendees */}}
```

**Shortening and formatting** with `printf`:

```
{{printf "%.3s" .Weekday}}        → Fri
{{printf "Week of %s" .Date}}     → Week of 2026-11-20
```

**Literal braces.** To get `{{` into the note itself, write `{{"{{"}}`.

There's no date arithmetic (such as "next Friday"), and fields can't come from outside the template. For those, write a [Lua `:` command](lua-scripting.md) that creates the note.

## Troubleshooting

| Problem | Cause |
|---|---|
| `template: … unexpected …` when creating | a `{{` without a matching `}}`, or an `{{if}}` with no `{{end}}` |
| `no template "x" (have: …)` | the file isn't `<config>/templates/x.md`, or its name uses characters other than letters, digits, `_` and `-` |
| a date in frontmatter shows up as `…T00:00:00Z` | put the value in quotes: `"{{.ID}}"` |
| `<no value>` in a journal day | a misspelled field, e.g. `{{.Title}}`, which journals don't have |
| `---` and keys in a journal's body | journal templates can't set frontmatter (see above) |

## Examples

### Project page

`templates/project.md`:

```markdown
---
tags: [project]
status: active
started: "{{.ID}}"
---
# {{.Title}}

## Goal

## Next steps

- [ ] 

## Log

- {{.Date}}: started
```

### Book notes

`templates/book.md`:

```markdown
---
tags: [book, reading]
author: ""
rating: ""
---
# {{.Title}}

Started {{.Weekday}} {{.Date}}.

## Summary

## Quotes

## Thoughts
```

### Daily journal with a weekly review

In `config.toml`:

```toml
journal_template = """
# {{.Weekday}} {{.Date}}

## Tasks

- [ ] 

## Notes
{{if eq .Weekday "Friday"}}
## Weekly review

- What went well?
- What didn't?
- Next week's focus:
{{end}}
"""
```
