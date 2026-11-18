// Package commands is the ":" command window shared by the TUI and the GUI:
// one catalog of commands with typed arguments, parsing, completion,
// history, and the actions that only touch the note store. Actions that are
// about a frontend's own UI (zoom, help, a confirm dialog…) are listed here
// too, so both apps show the same names and descriptions, but Run hands them
// back to the frontend as Result.UI.
package commands

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/devinthemtn/katnotes/internal/config"
	"github.com/devinthemtn/katnotes/internal/notes"
	"github.com/devinthemtn/katnotes/internal/recent"
	"github.com/devinthemtn/katnotes/internal/viewstate"
)

// App names, for commands that only exist in one frontend.
const (
	AppTUI = "tui"
	AppGUI = "gui"
)

// ArgKind selects how an argument is completed.
type ArgKind string

const (
	ArgText     ArgKind = "text"     // free text
	ArgNote     ArgKind = "note"     // any note, by title
	ArgTag      ArgKind = "tag"      // any tag in use
	ArgNoteTag  ArgKind = "note-tag" // a tag on the current note
	ArgField    ArgKind = "field"    // a frontmatter key
	ArgDate     ArgKind = "date"     // today, yesterday, tomorrow or a journal date
	ArgTemplate ArgKind = "template" // a page template name
	ArgChoice   ArgKind = "choice"   // one of Arg.Choices
	ArgSaved    ArgKind = "saved"    // a saved search
)

// Arg describes one argument.
type Arg struct {
	Name     string   `json:"name"`
	Kind     ArgKind  `json:"kind"`
	Optional bool     `json:"optional,omitempty"`
	Rest     bool     `json:"rest,omitempty"`    // takes the rest of the line, spaces and all
	Many     bool     `json:"many,omitempty"`    // repeats: one value per word
	Choices  []string `json:"choices,omitempty"` // ArgChoice: the allowed values

	// complete overrides the kind's completer. args are the values typed
	// before this one.
	complete func(env *Env, args []string) []Suggestion
}

// Spec is one command.
type Spec struct {
	Name      string   `json:"name"`
	Desc      string   `json:"desc"`
	Args      []Arg    `json:"args"`
	NeedsNote bool     `json:"needsNote,omitempty"`
	UI        bool     `json:"ui,omitempty"`   // the frontend performs it
	Long      bool     `json:"long,omitempty"` // slow (network): run it in the background
	Apps      []string `json:"apps,omitempty"` // empty: every app

	run func(env *Env, args []string) (Result, error)
}

// Usage is the name followed by its arguments, e.g. "tag add <tag…>".
func (s *Spec) Usage() string {
	var b strings.Builder
	b.WriteString(s.Name)
	for _, a := range s.Args {
		name := a.Name
		if a.Many || a.Rest {
			name += "…"
		}
		if a.Optional {
			b.WriteString(" [" + name + "]")
		} else {
			b.WriteString(" <" + name + ">")
		}
	}
	return b.String()
}

func (s *Spec) inApp(app string) bool {
	if len(s.Apps) == 0 || app == "" {
		return true
	}
	for _, a := range s.Apps {
		if a == app {
			return true
		}
	}
	return false
}

// Env is what a command runs against.
type Env struct {
	Store   *notes.Store
	Note    *notes.Note // the selected / open note, or nil
	App     string      // AppTUI or AppGUI
	History []string    // recent command lines, newest first
	Now     func() time.Time

	// Files in the config directory; empty means the default location.
	RecentPath   string // recently opened notes (internal/recent)
	TemplatesDir string // page templates
	ViewPath     string // sort orders and saved searches (internal/viewstate)

	// Search is the app's current search, for "search save".
	Search CurrentSearch
}

// CurrentSearch is the search an app is showing.
type CurrentSearch struct {
	Mode string
	Text string
}

func (e *Env) viewPath() string {
	if e.ViewPath != "" {
		return e.ViewPath
	}
	p, _ := viewstate.Path()
	return p
}

func (e *Env) recentPath() string {
	if e.RecentPath != "" {
		return e.RecentPath
	}
	p, _ := recent.Path()
	return p
}

// TemplatesPath is the page templates folder for e.
func (e *Env) TemplatesPath() string {
	if e.TemplatesDir != "" {
		return e.TemplatesDir
	}
	d, err := config.Dir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, notes.TemplatesDirName)
}

func (e *Env) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// Choice is one entry of a pick list (links, backlinks).
type Choice struct {
	Kind   notes.Kind `json:"kind"`
	ID     string     `json:"id"`
	Title  string     `json:"title"`
	Detail string     `json:"detail"`
	Exists bool       `json:"exists"`
}

// Result is what running a command produced. The frontend applies it:
// show Status, reload when Changed, show / select Note (and open it in the
// editor when Edit), copy Clipboard, list Pick, and run its own action for UI.
type Result struct {
	Command   string      `json:"command"`
	Status    string      `json:"status"`
	Changed   bool        `json:"changed"` // notes on disk changed: reload lists
	Note      *notes.Note `json:"note"`
	Edit      bool        `json:"edit"`
	Relinked  []string    `json:"relinked"`
	Clipboard string      `json:"clipboard"`
	PickTitle string      `json:"pickTitle"` // set when Pick is a list to show (it may be empty)
	Pick      []Choice    `json:"pick"`
	UI        string      `json:"ui"` // UI action name for the frontend, or ""
	Args      []string    `json:"args"`
}

// Catalog lists the commands available in app ("" lists all), in display
// order.
func Catalog(app string) []*Spec {
	out := make([]*Spec, 0, len(catalog))
	for _, s := range catalog {
		if s.inApp(app) {
			out = append(out, s)
		}
	}
	return out
}

var catalog []*Spec

func init() {
	note := Arg{Name: "note", Kind: ArgNote, Rest: true}
	catalog = []*Spec{
		{Name: "open", Desc: "Go to a note by title", Args: []Arg{note}, run: runOpen},
		{Name: "new", Desc: "Create a page with this title", Args: []Arg{{Name: "title", Kind: ArgText, Rest: true}}, run: runNew},
		{Name: "new from", Desc: "Create a page from a template", Args: []Arg{
			{Name: "template", Kind: ArgTemplate},
			{Name: "title", Kind: ArgText, Rest: true},
		}, run: runNewFrom},
		{Name: "recent", Desc: "Notes you opened lately (in either app)", run: runRecent},
		{Name: "search", Desc: "Show a saved search", Args: []Arg{{Name: "saved search", Kind: ArgSaved, Rest: true}}, run: runSearch},
		{Name: "search save", Desc: "Save the current search under a name", Args: []Arg{{Name: "name", Kind: ArgText, Rest: true}}, run: runSearchSave},
		{Name: "search rm", Desc: "Delete a saved search", Args: []Arg{{Name: "saved search", Kind: ArgSaved, Rest: true}}, run: runSearchRm},
		{Name: "todo", Desc: "Add a task to today's journal, e.g. todo call Sam {due:fri}", Args: []Arg{{Name: "task", Kind: ArgText, Rest: true}}, run: runTodo},
		{Name: "todo here", Desc: "Add a task to this note", NeedsNote: true, Args: []Arg{{Name: "task", Kind: ArgText, Rest: true}}, run: runTodoHere},
		{Name: "journal", Desc: "Open a journal day (default today)", Args: []Arg{{Name: "date", Kind: ArgDate, Optional: true}}, run: runJournal},
		{Name: "tag add", Desc: "Add tags to the note", NeedsNote: true, Args: []Arg{{Name: "tag", Kind: ArgTag, Many: true}}, run: runTagAdd},
		{Name: "tag rm", Desc: "Remove tags from the note", NeedsNote: true, Args: []Arg{{Name: "tag", Kind: ArgNoteTag, Many: true}}, run: runTagRm},
		{Name: "set", Desc: "Set a frontmatter field (adds frontmatter if missing)", NeedsNote: true, Args: []Arg{
			{Name: "field", Kind: ArgField},
			{Name: "value", Kind: ArgText, Rest: true, complete: fieldValues},
		}, run: runSet},
		{Name: "unset", Desc: "Remove a frontmatter field", NeedsNote: true, Args: []Arg{
			{Name: "field", Kind: ArgField, complete: noteFields},
		}, run: runUnset},
		{Name: "rename", Desc: "Rename the note (updates links to it)", NeedsNote: true, Args: []Arg{
			{Name: "title", Kind: ArgText, Rest: true, complete: currentTitle},
		}, run: runRename},
		{Name: "pin", Desc: "Pin the note to the top of the list", NeedsNote: true, run: runPin},
		{Name: "unpin", Desc: "Unpin the note", NeedsNote: true, run: runUnpin},
		{Name: "copy-link", Desc: "Copy a Markdown link to the note", NeedsNote: true, run: runCopyLink},
		{Name: "links", Desc: "Notes this note links to", NeedsNote: true, run: runLinks},
		{Name: "backlinks", Desc: "Notes that link to this note", NeedsNote: true, run: runBacklinks},
		{Name: "frontmatter", Desc: "Edit the note's frontmatter as YAML (adds one if missing)", NeedsNote: true, UI: true},
		{Name: "delete", Desc: "Delete the note (asks first)", NeedsNote: true, UI: true},
		{Name: "tasks", Desc: "Show tasks from every note: open|done|all, any|overdue|week", UI: true, Args: []Arg{
			{Name: "filter", Kind: ArgChoice, Optional: true, Many: true, Choices: []string{"open", "done", "all", "any", "overdue", "week"}},
		}},
		{Name: "sync-tasks", Desc: "Update contains_task in every note's frontmatter", run: runSyncTasks},
		{Name: "sync", Desc: "Git: commit all notes, then pull and push", Long: true, run: runSync},
		{Name: "git status", Desc: "Git: notes changed since the last commit", run: runGitStatus},
		{Name: "terminal", Desc: "Open a terminal in the note's folder", UI: true},
		{Name: "external", Desc: "Open the note in the system's default app", NeedsNote: true, UI: true, Apps: []string{AppGUI}},
		{Name: "zoom", Desc: "Toggle focus mode (hide sidebar and top bar)", UI: true, Apps: []string{AppGUI}},
		{Name: "reload", Desc: "Reload notes from disk", UI: true, Apps: []string{AppTUI}},
		{Name: "reload", Desc: "Reload config.toml and Lua / commands", UI: true, Apps: []string{AppGUI}},
		{Name: "help", Desc: "Show keyboard help", UI: true},
		{Name: "quit", Desc: "Quit KatNotes", UI: true, Apps: []string{AppTUI}},
	}
}
