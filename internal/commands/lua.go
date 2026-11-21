package commands

import (
	"fmt"
	"strings"

	"github.com/devinthemtn/katnotes/internal/config"
	"github.com/devinthemtn/katnotes/internal/luacmd"
)

// LuaDir returns the folder of the user's ":" Lua commands (see
// internal/luacmd), or "" for none. Every <name>.lua there is a
// ":name [args…]" command. ("/" editor commands have their own folder.)
// Tests replace it.
var LuaDir = func() string {
	p, _ := config.File(luacmd.CommandDirName) // "" when there is no config directory
	return p
}

// luaSpecs lists the Lua commands as catalog entries. A script whose name is
// the first word of a built-in command is left out: the built-in wins.
func luaSpecs() []*Spec {
	dir := LuaDir()
	if dir == "" {
		return nil
	}
	cmds, _ := luacmd.List(dir) // a broken file is left out of the list
	var out []*Spec
	for _, c := range cmds {
		if builtinWord(c.Name) {
			continue
		}
		desc := c.Description
		if desc == "" {
			desc = "Lua command (" + c.Name + ".lua)"
		}
		out = append(out, &Spec{
			Name: c.Name,
			Desc: desc,
			Args: []Arg{{Name: "args", Kind: ArgText, Optional: true, Rest: true}},
			Long: c.Long,
			Lua:  true,
			run:  runLua(dir, c.Name),
		})
	}
	return out
}

func builtinWord(name string) bool {
	for _, s := range catalog {
		if first, _, _ := strings.Cut(s.Name, " "); strings.EqualFold(first, name) {
			return true
		}
	}
	for a := range aliases {
		if a == name {
			return true
		}
	}
	return false
}

// runLua runs dir/name.lua for the command window.
func runLua(dir, name string) func(env *Env, args []string) (Result, error) {
	return func(env *Env, args []string) (Result, error) {
		arg := strings.Join(args, " ")
		ctx := luacmd.Context{Args: arg, Source: luacmd.SourceCommand, App: env.App}
		if env.Store != nil {
			ctx.NotesDir = env.Store.Root
		}
		if d, err := config.Dir(); err == nil {
			ctx.ConfigDir = d
		}
		if n := env.Note; n != nil {
			ctx.Body = n.Body
			ctx.Note = luacmd.NoteInfo{Kind: string(n.Kind), ID: n.ID, Title: n.Title, Tags: n.Tags}
			if env.Store != nil {
				ctx.Note.Path = env.Store.Path(n.Kind, n.ID)
			}
		}
		lr, err := luacmd.Run(dir, name, ctx)
		// The script may have written notes, so lists reload either way.
		r := Result{Changed: true}
		if err != nil {
			return r, err
		}
		r.Status = firstNonEmpty(lr.Status, lr.Text, lr.Line)
		if r.Status == "" {
			r.Status = name + ": done"
		}
		if lr.Open != "" && env.Store != nil {
			n, err := ResolveNote(env.Store, lr.Open)
			if err != nil {
				return r, fmt.Errorf("%s: open %q: %w", name, lr.Open, err)
			}
			r.Note = n
		}
		return r, nil
	}
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
