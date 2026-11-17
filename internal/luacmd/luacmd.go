// Package luacmd runs user-defined editor commands written in Lua
// (gopher-lua). Each command is one file, <dir>/<name>.lua, that returns a
// table with a description and a run(ctx) function. See ExampleScript.
package luacmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// DirName is the commands folder inside the KatNotes config directory.
const DirName = "commands"

// Timeout bounds loading or running one script.
var Timeout = 2 * time.Second

var validName = regexp.MustCompile(`^[a-z0-9_-]+$`)

// Command is a loaded user command.
type Command struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
}

// NoteInfo describes the open note.
type NoteInfo struct {
	Kind  string   `json:"kind"`
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Tags  []string `json:"tags"`
	Path  string   `json:"path"`
}

// Context is passed to run(ctx) as a Lua table with the same field names.
type Context struct {
	Line      string   `json:"line"`   // current line without the /name token
	Before    string   `json:"before"` // line text before the token
	After     string   `json:"after"`  // line text after the token
	Body      string   `json:"body"`   // the whole note
	Note      NoteInfo `json:"note"`
	NotesDir  string   `json:"notes_dir"`
	ConfigDir string   `json:"config_dir"`
}

// Result is what run returned. Kind "text" replaces the /name token with
// Text; Kind "line" replaces the whole line with Line and puts the cursor at
// Cursor (-1 means end of line).
type Result struct {
	Kind   string `json:"kind"`
	Text   string `json:"text"`
	Line   string `json:"line"`
	Cursor int    `json:"cursor"`
}

type cached struct {
	mod  time.Time
	size int64
	cmd  Command
	err  error
}

var (
	cacheMu sync.Mutex
	cache   = map[string]cached{}
)

// List loads every command in dir. Files starting with "_" and files whose
// name isn't [a-z0-9_-] are skipped. A broken file yields an error but does
// not hide the others. A missing dir is not an error.
func List(dir string) ([]Command, []error) {
	paths, _ := filepath.Glob(filepath.Join(dir, "*.lua"))
	sort.Strings(paths)
	var cmds []Command
	var errs []error
	for _, p := range paths {
		base := filepath.Base(p)
		name := strings.ToLower(strings.TrimSuffix(base, ".lua"))
		if strings.HasPrefix(base, "_") {
			continue
		}
		if !validName.MatchString(name) {
			errs = append(errs, fmt.Errorf("%s: command names may only use a-z, 0-9, _ and -", base))
			continue
		}
		c, err := loadCached(p, name)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		cmds = append(cmds, c)
	}
	return cmds, errs
}

func loadCached(path, name string) (Command, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return Command{}, err
	}
	cacheMu.Lock()
	c, ok := cache[path]
	cacheMu.Unlock()
	if ok && c.mod.Equal(fi.ModTime()) && c.size == fi.Size() {
		return c.cmd, c.err
	}
	cmd, err := load(path, name)
	cacheMu.Lock()
	cache[path] = cached{mod: fi.ModTime(), size: fi.Size(), cmd: cmd, err: err}
	cacheMu.Unlock()
	return cmd, err
}

// newState returns a Lua state with the standard libraries and a deadline.
func newState() (*lua.LState, context.CancelFunc) {
	L := lua.NewState()
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	L.SetContext(ctx)
	return L, func() { cancel(); L.Close() }
}

// module runs the file and returns the table it returns.
func module(L *lua.LState, path string) (*lua.LTable, error) {
	if err := L.DoFile(path); err != nil {
		return nil, scriptErr(path, err)
	}
	tbl, ok := L.Get(-1).(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("%s: must return a table like { description = \"…\", run = function(ctx) … end }", filepath.Base(path))
	}
	if _, ok := tbl.RawGetString("run").(*lua.LFunction); !ok {
		return nil, fmt.Errorf("%s: the returned table needs a run(ctx) function", filepath.Base(path))
	}
	return tbl, nil
}

func load(path, name string) (Command, error) {
	L, done := newState()
	defer done()
	tbl, err := module(L, path)
	if err != nil {
		return Command{}, err
	}
	desc := ""
	if d, ok := tbl.RawGetString("description").(lua.LString); ok {
		desc = string(d)
	}
	return Command{Name: name, Description: desc, Path: path}, nil
}

// Run executes the named command from dir.
func Run(dir, name string, c Context) (Result, error) {
	if !validName.MatchString(name) {
		return Result{}, fmt.Errorf("invalid command name %q", name)
	}
	path := filepath.Join(dir, name+".lua")
	if _, err := os.Stat(path); err != nil {
		return Result{}, fmt.Errorf("no command %q in %s", name, dir)
	}
	L, done := newState()
	defer done()
	tbl, err := module(L, path)
	if err != nil {
		return Result{}, err
	}
	if err := L.CallByParam(lua.P{Fn: tbl.RawGetString("run"), NRet: 1, Protect: true}, contextTable(L, c)); err != nil {
		return Result{}, scriptErr(path, err)
	}
	ret := L.Get(-1)
	switch v := ret.(type) {
	case *lua.LNilType:
		return Result{Kind: "text"}, nil
	case lua.LString, lua.LNumber, lua.LBool:
		return Result{Kind: "text", Text: lua.LVAsString(v)}, nil
	case *lua.LTable:
		if l := v.RawGetString("line"); l != lua.LNil {
			r := Result{Kind: "line", Line: lua.LVAsString(l), Cursor: -1}
			if n, ok := v.RawGetString("cursor").(lua.LNumber); ok {
				r.Cursor = int(n)
			}
			return r, nil
		}
		return Result{Kind: "text", Text: lua.LVAsString(v.RawGetString("text"))}, nil
	}
	return Result{}, fmt.Errorf("%s: run returned a %s; return a string, nil, {text=…} or {line=…, cursor=…}", filepath.Base(path), ret.Type())
}

func contextTable(L *lua.LState, c Context) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("line", lua.LString(c.Line))
	t.RawSetString("before", lua.LString(c.Before))
	t.RawSetString("after", lua.LString(c.After))
	t.RawSetString("body", lua.LString(c.Body))
	t.RawSetString("notes_dir", lua.LString(c.NotesDir))
	t.RawSetString("config_dir", lua.LString(c.ConfigDir))
	n := L.NewTable()
	n.RawSetString("kind", lua.LString(c.Note.Kind))
	n.RawSetString("id", lua.LString(c.Note.ID))
	n.RawSetString("title", lua.LString(c.Note.Title))
	n.RawSetString("path", lua.LString(c.Note.Path))
	tags := L.NewTable()
	for _, tg := range c.Note.Tags {
		tags.Append(lua.LString(tg))
	}
	n.RawSetString("tags", tags)
	t.RawSetString("note", n)
	return t
}

func scriptErr(path string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "context deadline exceeded") {
		return fmt.Errorf("%s: timed out after %s", filepath.Base(path), Timeout)
	}
	var ae *lua.ApiError
	if errors.As(err, &ae) {
		msg := strings.TrimSpace(ae.Object.String())
		return fmt.Errorf("%s", strings.TrimPrefix(msg, filepath.Dir(path)+string(filepath.Separator)))
	}
	return fmt.Errorf("%s: %w", filepath.Base(path), err)
}

// WriteExample creates dir and a commented _example.lua if it is missing.
func WriteExample(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	p := filepath.Join(dir, "_example.lua")
	if _, err := os.Stat(p); err == nil {
		return nil
	}
	return os.WriteFile(p, []byte(ExampleScript), 0o644)
}

// ExampleScript documents the command contract.
const ExampleScript = `-- KatNotes slash command example.
--
-- Each <name>.lua file in this folder becomes a "/name" command in the GUI
-- editor. Names may use a-z, 0-9, _ and -. Files starting with "_" (like this
-- one) are ignored: copy it to e.g. sig.lua to try it. After adding or editing
-- a file, run :reload from the command palette (Ctrl+Shift+P).
--
-- The file returns a table with a description (shown in the / menu) and a
-- run(ctx) function. ctx has:
--   ctx.line        the current line, with the /name token removed
--   ctx.before      text on the line before the token
--   ctx.after       text on the line after the token
--   ctx.body        the whole note (Markdown, without frontmatter)
--   ctx.note        { kind, id, title, tags = {…}, path }
--   ctx.notes_dir   the notes folder
--   ctx.config_dir  the KatNotes config folder
--
-- run can return:
--   "text"                        replaces /name with text
--   nil                           just removes /name
--   { text = "…" }                same as returning a string
--   { line = "…", cursor = n }    replaces the whole line; cursor is a 0-based
--                                 column (optional, defaults to end of line)
--
-- The Lua standard library is available (os.date, string, table, io, …).
-- Scripts are stopped after 2 seconds.

return {
  description = "Insert a signature with today's date",
  run = function(ctx)
    return "— " .. (os.getenv("USER") or "me") .. ", " .. os.date("%Y-%m-%d")
  end,
}
`
