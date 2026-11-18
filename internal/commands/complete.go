package commands

import (
	"fmt"
	"sort"
	"strings"

	"github.com/devinthemtn/katnotes/internal/config"
	"github.com/devinthemtn/katnotes/internal/editor"
	"github.com/devinthemtn/katnotes/internal/notes"
	"github.com/devinthemtn/katnotes/internal/recent"
	"github.com/devinthemtn/katnotes/internal/theme"
	"github.com/devinthemtn/katnotes/internal/viewstate"
)

// Suggestion is one completion. Accepting it replaces Completion.Fragment
// (the end of the input) with Value, so the new input is Keep + Value.
type Suggestion struct {
	Value  string `json:"value"`
	Label  string `json:"label"`
	Detail string `json:"detail"`
	Recent bool   `json:"recent,omitempty"` // a history entry
}

// Completion is what the window should offer for the current input.
type Completion struct {
	Phase    string       `json:"phase"`    // "command", "arg" or "none"
	Keep     string       `json:"keep"`     // input before the part being completed
	Fragment string       `json:"fragment"` // the part being completed
	Items    []Suggestion `json:"items"`
	Exact    bool         `json:"exact"` // command phase: input already names a command
	Usage    string       `json:"usage"` // the matched command's usage, if any
	Desc     string       `json:"desc"`
	Arg      string       `json:"arg"` // name of the argument being typed
}

// maxItems caps a completion list.
const maxItems = 40

// recentShown is how many history entries an empty prompt offers.
const recentShown = 5

// Complete returns completions for input (the cursor is at its end).
func Complete(env *Env, input string) Completion {
	toks := tokenize(input)
	spec, n := match(env.App, toks)
	trailing := strings.HasSuffix(input, " ") || strings.HasSuffix(input, "\t")
	if spec == nil || (len(toks) == n && !trailing) {
		return completeCommand(env, input, spec)
	}
	return completeArg(env, input, spec, toks[n:], trailing)
}

func completeCommand(env *Env, input string, exact *Spec) Completion {
	lead := len(input) - len(strings.TrimLeft(input, " \t"))
	q := strings.ToLower(input[lead:])
	c := Completion{Phase: "command", Keep: input[:lead], Fragment: input[lead:], Exact: exact != nil}
	if exact != nil {
		c.Usage, c.Desc = exact.Usage(), exact.Desc
	}
	if strings.TrimSpace(q) == "" {
		for i, h := range env.History {
			if i == recentShown {
				break
			}
			c.Items = append(c.Items, Suggestion{Value: h, Label: h, Detail: "recent", Recent: true})
		}
	}
	for _, s := range Rank(env.App, q) {
		v := s.Name
		if len(s.Args) > 0 {
			v += " "
		}
		c.Items = append(c.Items, Suggestion{Value: v, Label: s.Usage(), Detail: s.Desc})
	}
	return c
}

// Rank orders app's commands for query q: name prefix matches first, then
// subsequence and description matches. An empty q lists every command.
func Rank(app, q string) []*Spec {
	q = strings.ToLower(strings.TrimSpace(q))
	all := Catalog(app)
	if q == "" {
		return all
	}
	var prefix, rest []*Spec
	for _, s := range all {
		switch {
		case strings.HasPrefix(s.Name, q):
			prefix = append(prefix, s)
		case isSubseq(q, s.Name) || strings.Contains(strings.ToLower(s.Desc), q):
			rest = append(rest, s)
		}
	}
	return append(prefix, rest...)
}

func isSubseq(needle, hay string) bool {
	i := 0
	for j := 0; j < len(hay) && i < len(needle); j++ {
		if hay[j] == needle[i] {
			i++
		}
	}
	return i == len(needle)
}

func completeArg(env *Env, input string, spec *Spec, rest []token, trailing bool) Completion {
	c := Completion{Phase: "arg", Usage: spec.Usage(), Desc: spec.Desc}
	done := rest
	frag, from := "", len(input)
	if !trailing && len(rest) > 0 {
		last := rest[len(rest)-1]
		done, frag, from = rest[:len(rest)-1], input[last.start:], last.start
	}

	// Which argument is being typed?
	var arg *Arg
	var prev []string
	i := 0
	for ai := range spec.Args {
		a := &spec.Args[ai]
		if a.Rest {
			arg = a
			if i < len(rest) {
				from = rest[i].start
				frag = input[from:]
			}
			break
		}
		if a.Many || i >= len(done) {
			arg = a
			break
		}
		prev = append(prev, done[i].text)
		i++
	}
	if arg == nil {
		c.Phase = "none"
		c.Keep, c.Fragment = input, ""
		return c
	}
	if arg.Many {
		for _, t := range done[i:] {
			prev = append(prev, t.text)
		}
	}
	c.Arg = arg.Name
	c.Keep, c.Fragment = input[:from], input[from:]

	q := strings.ToLower(strings.TrimSpace(strings.Trim(frag, `"`)))
	var matched []Suggestion
	if arg.Kind == ArgNote && arg.complete == nil {
		matched = noteSuggestions(env, q) // fuzzy, already ranked
	} else {
		matched = filter(candidates(env, spec, arg, prev), q)
	}
	for _, s := range matched {
		if !arg.Rest {
			if strings.ContainsAny(s.Value, " \t") {
				s.Value = `"` + s.Value + `"`
			}
			s.Value += " "
		}
		c.Items = append(c.Items, s)
	}
	return c
}

// filter keeps candidates matching q: prefix matches first, then substring.
func filter(cands []Suggestion, q string) []Suggestion {
	if q == "" {
		if len(cands) > maxItems {
			cands = cands[:maxItems]
		}
		return cands
	}
	var prefix, sub []Suggestion
	for _, s := range cands {
		l := strings.ToLower(s.Value)
		switch {
		case strings.HasPrefix(l, q):
			prefix = append(prefix, s)
		case strings.Contains(l, q):
			sub = append(sub, s)
		}
	}
	out := append(prefix, sub...)
	if len(out) > maxItems {
		out = out[:maxItems]
	}
	return out
}

func candidates(env *Env, spec *Spec, arg *Arg, prev []string) []Suggestion {
	if arg.complete != nil {
		return arg.complete(env, prev)
	}
	st := env.Store
	used := map[string]bool{}
	for _, p := range prev {
		used[strings.ToLower(strings.TrimPrefix(p, "#"))] = true
	}
	switch arg.Kind {
	case ArgSaved:
		var out []Suggestion
		for _, x := range viewstate.Load(env.viewPath()).Searches {
			out = append(out, Suggestion{Value: x.Name, Label: x.Name, Detail: x.Mode + ": " + x.Text})
		}
		return out
	case ArgTag:
		tags, err := st.SortedTags()
		if err != nil {
			return nil
		}
		var out []Suggestion
		for _, t := range tags {
			if used[t.Tag] || (env.Note != nil && env.Note.HasTag(t.Tag)) {
				continue
			}
			out = append(out, Suggestion{Value: t.Tag, Label: "#" + t.Tag, Detail: fmt.Sprintf("%d note(s)", t.Count)})
		}
		return out
	case ArgNoteTag:
		if env.Note == nil {
			return nil
		}
		var out []Suggestion
		for _, t := range env.Note.Tags {
			if !used[t] {
				out = append(out, Suggestion{Value: t, Label: "#" + t, Detail: "on this note"})
			}
		}
		return out
	case ArgField:
		keys, err := st.FieldKeys()
		if err != nil {
			return nil
		}
		var out []Suggestion
		for _, k := range keys {
			if k == "contains_task" {
				continue
			}
			d := ""
			if env.Note != nil {
				d = FieldText(env.Note, k)
			}
			out = append(out, Suggestion{Value: k, Label: k, Detail: d})
		}
		return out
	case ArgTemplate:
		var out []Suggestion
		for _, t := range notes.ListTemplates(env.TemplatesPath()) {
			out = append(out, Suggestion{Value: t, Label: t, Detail: "template"})
		}
		return out
	case ArgChoice:
		var out []Suggestion
		for _, c := range arg.Choices {
			if !used[c] {
				out = append(out, Suggestion{Value: c, Label: c})
			}
		}
		return out
	case ArgDate:
		now := env.now()
		out := []Suggestion{
			{Value: "today", Label: "today", Detail: now.Format(notes.JournalIDLayout)},
			{Value: "yesterday", Label: "yesterday", Detail: now.AddDate(0, 0, -1).Format(notes.JournalIDLayout)},
			{Value: "tomorrow", Label: "tomorrow", Detail: now.AddDate(0, 0, 1).Format(notes.JournalIDLayout)},
		}
		ns, err := st.List(notes.KindJournal, notes.ListOpts{Sort: notes.SortID})
		if err == nil {
			for _, n := range ns {
				out = append(out, Suggestion{Value: n.ID, Label: n.ID, Detail: n.Title})
			}
		}
		return out
	}
	return nil
}

// FieldText is a short display of a note's frontmatter field ("" if unset).
func FieldText(n *notes.Note, key string) string {
	switch key {
	case "title":
		return n.Title
	case "tags":
		if len(n.Tags) == 0 {
			return ""
		}
		return "#" + strings.Join(n.Tags, " #")
	case "created":
		return n.Created.Format("2006-01-02 15:04")
	}
	if v, ok := n.Extra[key]; ok {
		return valueText(v)
	}
	return ""
}

func valueText(v any) string {
	switch t := v.(type) {
	case []any:
		parts := make([]string, len(t))
		for i, x := range t {
			parts[i] = fmt.Sprint(x)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case nil:
		return ""
	}
	return fmt.Sprint(v)
}

// fieldValues suggests values already used for the field in prev[0].
func fieldValues(env *Env, prev []string) []Suggestion {
	if len(prev) == 0 {
		return nil
	}
	key := prev[0]
	switch key {
	case "title":
		return currentTitle(env, nil)
	case "created":
		now := env.now()
		return []Suggestion{{Value: now.Format(notes.JournalIDLayout), Label: now.Format(notes.JournalIDLayout), Detail: "today"}}
	}
	ns, err := env.Store.List("", notes.ListOpts{})
	if err != nil {
		return nil
	}
	count := map[string]int{}
	for i := range ns {
		if s := FieldText(&ns[i], key); s != "" && key != "tags" {
			count[s]++
		}
	}
	vals := make([]string, 0, len(count))
	for v := range count {
		vals = append(vals, v)
	}
	sort.Slice(vals, func(i, j int) bool {
		if count[vals[i]] != count[vals[j]] {
			return count[vals[i]] > count[vals[j]]
		}
		return vals[i] < vals[j]
	})
	out := make([]Suggestion, 0, len(vals))
	for _, v := range vals {
		out = append(out, Suggestion{Value: v, Label: v, Detail: fmt.Sprintf("%d note(s)", count[v])})
	}
	return out
}

// noteFields suggests the fields the current note has that can be removed.
func noteFields(env *Env, _ []string) []Suggestion {
	if env.Note == nil {
		return nil
	}
	var out []Suggestion
	if len(env.Note.Tags) > 0 {
		out = append(out, Suggestion{Value: "tags", Label: "tags", Detail: FieldText(env.Note, "tags")})
	}
	keys := make([]string, 0, len(env.Note.Extra))
	for k := range env.Note.Extra {
		if k != "contains_task" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, Suggestion{Value: k, Label: k, Detail: FieldText(env.Note, k)})
	}
	return out
}

// themes suggests "auto", the theme names in the themes folder and
// "shared".
func themes(env *Env, _ []string) []Suggestion {
	cur, shared := "", ""
	if cfg, err := config.Load(); err == nil {
		cur, shared = cfg.ThemeFor(env.App), cfg.Theme
	}
	out := []Suggestion{{Value: theme.Auto, Label: theme.Auto, Detail: "light or dark, following the system"}}
	for _, n := range theme.List(env.ThemesPath()) {
		out = append(out, Suggestion{Value: n, Label: n, Detail: "theme"})
	}
	for i := range out {
		if out[i].Value == cur {
			out[i].Detail = "current"
		}
	}
	if env.App == AppTUI || env.App == AppGUI {
		out = append(out, Suggestion{Value: themeShared, Label: themeShared, Detail: "follow the theme option (now " + shared + "), like the other app"})
	}
	return out
}

// programs suggests the editors and viewers installed on this machine, for
// open-with.
func programs(_ *Env, _ []string) []Suggestion {
	var out []Suggestion
	for _, p := range editor.KnownPrograms() {
		detail := "opens a window"
		switch {
		case p.Either:
			detail = "terminal or window, by its options (e.g. -nw)"
		case p.InTerminal:
			detail = "runs in a terminal"
		}
		out = append(out, Suggestion{Value: p.Name, Label: p.Name, Detail: detail})
	}
	return out
}

func currentTitle(env *Env, _ []string) []Suggestion {
	if env.Note == nil {
		return nil
	}
	return []Suggestion{{Value: env.Note.Title, Label: env.Note.Title, Detail: "current title"}}
}

// noteSuggestions completes a note title: a fuzzy name search (title or
// filename), or, for an empty query, the recently opened notes first and
// then the most recently modified ones. This is also the quick-open list.
func noteSuggestions(env *Env, q string) []Suggestion {
	ms, err := env.Store.Find(notes.FindQuery{Mode: notes.SearchName, Text: q})
	if err != nil {
		return nil
	}
	sug := func(n *notes.Note, detail string) Suggestion {
		return Suggestion{Value: n.Title, Label: n.Title, Detail: detail}
	}
	var out []Suggestion
	if q == "" {
		byKey := map[string]*notes.Note{}
		for i := range ms {
			byKey[ms[i].Note.Key()] = &ms[i].Note
		}
		seen := map[string]bool{}
		for _, k := range recent.Load(env.recentPath()) {
			if n, ok := byKey[k]; ok && !seen[k] {
				seen[k] = true
				out = append(out, sug(n, k+" · recent"))
			}
		}
		sort.SliceStable(ms, func(i, j int) bool { return ms[i].Note.Updated.After(ms[j].Note.Updated) })
		for i := range ms {
			if k := ms[i].Note.Key(); !seen[k] {
				out = append(out, sug(&ms[i].Note, k))
			}
		}
	} else {
		for i := range ms {
			out = append(out, sug(&ms[i].Note, ms[i].Note.Key()))
		}
	}
	if len(out) > maxItems {
		out = out[:maxItems]
	}
	return out
}
