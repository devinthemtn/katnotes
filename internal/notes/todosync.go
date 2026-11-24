package notes

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/devinthemtn/katnotes/internal/todotxt"
)

// Linking a task to a todo.txt file (the one katTasks uses):
//
//	note:     - [ ] Call the plumber {due:2026-11-27 pri:high kt:7f3a2c}
//	todo.txt: (A) 2026-11-23 Call the plumber due:2026-11-27 kn:7f3a2c note:page/house
//
// {kt:new} asks for a link: the next save gives it an ID and writes the
// line. Saving a note rewrites its linked lines from the note (text, due,
// start, priority, done). The done state also flows back: a task checked or
// unchecked in todo.txt since the last sync is checked or unchecked in the
// note, unless the note changed too, in which case the note wins.
const (
	MetaLink     = "kt"   // note side: the link ID, or "new"
	TodoIDTag    = "kn"   // todo.txt side: the same ID
	TodoNoteTag  = "note" // todo.txt side: kind/id of the note
	TodoStartTag = "t"    // todo.txt threshold date, from {start:}
	linkNew      = "new"
)

// linkID matches a valid link ID: six lowercase hex digits. Anything else in
// kt: (say "ne", half typed) is ignored rather than linked.
var linkID = regexp.MustCompile(`^[0-9a-f]{6}$`)

var priLetters = map[string]byte{"high": 'A', "med": 'B', "low": 'C'}

// TodoSync links note tasks to lines in a todo.txt file.
type TodoSync struct {
	File      string // todo.txt
	DoneFile  string // where katTasks archives completed tasks
	StatePath string // JSON with the done state of each ID at the last sync

	mu sync.Mutex
}

var errTodoOff = errors.New("todo.txt sync is off: set enabled = true under [todo_txt] in config.toml")

// TodoChange is one thing a sync did, or would do in a dry run.
type TodoChange struct {
	ID     string `json:"id"`
	Note   string `json:"note,omitempty"` // kind/id
	Line   int    `json:"line"`           // 0-based body line, -1 when not in a note
	Text   string `json:"text"`
	Action string `json:"action"`
}

// TodoChange actions.
const (
	TodoLinked    = "linked"    // a new ID was given and a line added
	TodoAdded     = "added"     // a line was added to todo.txt
	TodoUpdated   = "updated"   // a todo.txt line was rewritten from the note
	TodoChecked   = "checked"   // the note task was checked from todo.txt
	TodoUnchecked = "unchecked" // the note task was unchecked from todo.txt
	TodoMissing   = "missing"   // the line was removed from todo.txt
	TodoOrphan    = "orphan"    // a todo.txt line whose note task is gone
)

type todoState struct {
	Done map[string]bool `json:"done"`
}

// todoBatch is the files and state for one sync, written by commit.
type todoBatch struct {
	t            *TodoSync
	todo, done   *todotxt.File
	state        todoState
	stateChanged bool
	now          time.Time
	noDoneDate   bool
	changes      []TodoChange
	reserved     map[string]bool // IDs given out in this batch
}

func (t *TodoSync) begin(now time.Time, noDoneDate bool) (*todoBatch, error) {
	b := &todoBatch{t: t, now: now, noDoneDate: noDoneDate, state: todoState{Done: map[string]bool{}}}
	var err error
	if b.todo, err = todotxt.Load(t.File); err != nil {
		return nil, err
	}
	doneFile := t.DoneFile
	if doneFile == "" {
		doneFile = filepath.Join(filepath.Dir(t.File), "done.txt")
	}
	if b.done, err = todotxt.Load(doneFile); err != nil {
		return nil, err
	}
	if t.StatePath != "" {
		data, err := os.ReadFile(t.StatePath)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		if len(data) > 0 {
			if err := json.Unmarshal(data, &b.state); err != nil {
				return nil, fmt.Errorf("%s: %w", t.StatePath, err)
			}
			if b.state.Done == nil {
				b.state.Done = map[string]bool{}
			}
		}
	}
	return b, nil
}

func (b *todoBatch) commit() error {
	if err := b.todo.Save(); err != nil {
		return err
	}
	if !b.stateChanged || b.t.StatePath == "" {
		return nil
	}
	data, err := json.MarshalIndent(b.state, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(b.t.StatePath, append(data, '\n'))
}

func (b *todoBatch) setState(id string, done bool) {
	if v, ok := b.state.Done[id]; !ok || v != done {
		b.state.Done[id] = done
		b.stateChanged = true
	}
}

// find returns id's line in todo.txt, else in done.txt (archived is true).
func (b *todoBatch) find(id string) (l *todotxt.Line, archived bool) {
	if l := b.todo.Find(TodoIDTag, id); l != nil {
		return l, false
	}
	if l := b.done.Find(TodoIDTag, id); l != nil {
		return l, true
	}
	return nil, false
}

func (b *todoBatch) newID() (string, error) {
	for range 100 {
		var buf [3]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return "", err
		}
		id := hex.EncodeToString(buf[:])
		if _, used := b.state.Done[id]; used || b.reserved[id] {
			continue
		}
		if l, _ := b.find(id); l != nil {
			continue
		}
		return id, nil
	}
	return "", errors.New("todo.txt: could not pick an unused task ID")
}

// HasTodoLinks reports whether body has a task with a kt: link (or kt:new).
// hasLinkedTodo reports whether a task has a todo.txt ID ({kt:new} doesn't).
func hasLinkedTodo(tasks []Task) bool {
	for _, t := range tasks {
		if v := linkValue(t); v != "" && v != linkNew {
			return true
		}
	}
	return false
}

func HasTodoLinks(body string) bool {
	if !strings.Contains(body, MetaLink+":") {
		return false
	}
	for _, t := range ParseTasks(body) {
		if linkValue(t) != "" {
			return true
		}
	}
	return false
}

// countTodoLinks counts ts's tasks linked to todo.txt (kt:new included),
// and those that could be: the linked ones plus the open ones.
func countTodoLinks(ts []Task) (linked, linkable int) {
	for _, t := range ts {
		l := linkValue(t) != ""
		if l {
			linked++
		}
		if l || !t.Done {
			linkable++
		}
	}
	return linked, linkable
}

// linkValue is the task's kt: value if it is a valid ID or "new".
func linkValue(t Task) string {
	v := t.Meta[MetaLink]
	if v == linkNew || linkID.MatchString(v) {
		return v
	}
	return ""
}

// prepare runs before n is written: it gives kt:new tasks an ID and takes
// done-state changes made in todo.txt since the last sync into the body.
// It reports whether the body changed.
func (b *todoBatch) prepare(n *Note) (bool, error) {
	lines := strings.Split(n.Body, "\n")
	changed := false
	for _, t := range ParseTasks(n.Body) {
		v := linkValue(t)
		if v == "" {
			continue
		}
		l := strings.TrimSuffix(lines[t.Line], "\r")
		cr := len(l) != len(lines[t.Line])
		if v == linkNew {
			id, err := b.newID()
			if err != nil {
				return changed, err
			}
			if b.reserved == nil {
				b.reserved = map[string]bool{}
			}
			b.reserved[id] = true
			l, _ = SetLineMeta(l, MetaLink, id)
			b.changes = append(b.changes, TodoChange{ID: id, Note: noteKey(n), Line: t.Line, Text: t.Text, Action: TodoLinked})
		} else {
			line, _ := b.find(v)
			last, known := b.state.Done[v]
			if line == nil || !known || line.Done == last || t.Done != last {
				continue // nothing changed in todo.txt, or the note changed too
			}
			l = markTaskLine(l, line.Done, line.Completed, b.now, b.noDoneDate)
			act := TodoUnchecked
			if line.Done {
				act = TodoChecked
			}
			b.changes = append(b.changes, TodoChange{ID: v, Note: noteKey(n), Line: t.Line, Text: t.Text, Action: act})
		}
		if cr {
			l += "\r"
		}
		if l != lines[t.Line] {
			lines[t.Line] = l
			changed = true
		}
	}
	if changed {
		n.Body = strings.Join(lines, "\n")
	}
	return changed, nil
}

// push writes n's linked tasks to todo.txt (in memory until commit).
func (b *todoBatch) push(n *Note) {
	for _, t := range ParseTasks(n.Body) {
		id := linkValue(t)
		if id == "" || id == linkNew {
			continue
		}
		line, archived := b.find(id)
		_, known := b.state.Done[id]
		ch := TodoChange{ID: id, Note: noteKey(n), Line: t.Line, Text: t.Text}
		switch {
		case line != nil && !archived:
			before := line.String()
			b.fill(line, n, t)
			if line.String() != before {
				ch.Action = TodoUpdated
			}
		case line != nil && archived && t.Done:
			// Archived and still done: leave done.txt alone.
		case line == nil && known:
			// Removed from todo.txt by hand: don't bring it back.
			// "katnotes tasks push" does.
			if !b.reported(id, TodoMissing) {
				ch.Action = TodoMissing
			}
		default:
			// New, or unchecked in the note after katTasks archived it.
			nl := &todotxt.Line{Created: b.now.Format(DateLayout)}
			if line != nil && line.Created != "" {
				nl.Created = line.Created
			}
			b.fill(nl, n, t)
			b.todo.Add(nl)
			if !b.reported(id, TodoLinked) {
				ch.Action = TodoAdded
			}
		}
		if ch.Action != "" {
			b.changes = append(b.changes, ch)
		}
		b.setState(id, t.Done)
	}
}

func (b *todoBatch) reported(id, action string) bool {
	for _, c := range b.changes {
		if c.ID == id && c.Action == action {
			return true
		}
	}
	return false
}

// managedTags are the todo.txt tags push writes from the note.
var managedTags = map[string]bool{MetaDue: true, TodoStartTag: true, MetaPri: true, TodoIDTag: true, TodoNoteTag: true}

// fill sets line from the note task: done state, priority and text. Words
// added in katTasks (+projects, @contexts, other tags) that the note text
// doesn't have are kept.
func (b *todoBatch) fill(line *todotxt.Line, n *Note, t Task) {
	pri := priLetters[t.Meta[MetaPri]]
	// Keep katTasks' own letter when it means the same (D…Z are low).
	old := line.Pri
	if v, ok := line.Tag(MetaPri); ok && len(v) == 1 && old == 0 {
		old = v[0]
	}
	if old != 0 && priName(old) == t.Meta[MetaPri] {
		pri = old
	}
	line.Done = t.Done
	line.Pri = 0
	if t.Done {
		if line.Completed == "" || (t.Meta[MetaDone] != "" && t.Meta[MetaDone] != line.Completed) {
			line.Completed = t.Meta[MetaDone]
		}
		if line.Completed == "" {
			line.Completed = b.now.Format(DateLayout)
		}
	} else {
		line.Completed = ""
		line.Pri = pri
	}

	words := strings.Fields(t.Text)
	have := map[string]bool{}
	for _, w := range words {
		have[w] = true
	}
	for _, w := range strings.Fields(line.Text) {
		k, isTag := todotxt.IsTag(w)
		keep := (isTag && !managedTags[k]) || (!isTag && len(w) > 1 && (w[0] == '+' || w[0] == '@'))
		if keep && !have[w] {
			words = append(words, w)
			have[w] = true
		}
	}
	if d := t.Meta[MetaDue]; d != "" {
		words = append(words, MetaDue+":"+d)
	}
	if d := t.Meta[MetaStart]; d != "" {
		words = append(words, TodoStartTag+":"+d)
	}
	if t.Done && pri != 0 {
		// Like todo.sh, a done task keeps its priority as pri:X.
		words = append(words, MetaPri+":"+string(pri))
	}
	words = append(words, TodoIDTag+":"+linkValue(t), TodoNoteTag+":"+noteKey(n))
	if !sameWords(words, strings.Fields(line.Text)) {
		line.Text = strings.Join(words, " ") // else keep the order katTasks wrote
	}
}

// sameWords reports whether a and b say the same: the same plain words in
// the same order, and the same tags, +projects and @contexts in any order.
func sameWords(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	movable := func(w string) bool {
		_, tag := todotxt.IsTag(w)
		return tag || (len(w) > 1 && (w[0] == '+' || w[0] == '@'))
	}
	var pa, pb []string
	count := map[string]int{}
	for _, w := range a {
		if movable(w) {
			count[w]++
		} else {
			pa = append(pa, w)
		}
	}
	for _, w := range b {
		if !movable(w) {
			pb = append(pb, w)
		} else if count[w]--; count[w] < 0 {
			return false
		}
	}
	return slices.Equal(pa, pb)
}

func noteKey(n *Note) string { return string(n.Kind) + "/" + n.ID }

// markTaskLine checks or unchecks a task line, stamping done:<date> (date,
// or today when empty) when checking and removing it when unchecking,
// unless noDate is set.
func markTaskLine(l string, done bool, date string, now time.Time, noDate bool) string {
	loc := taskLine.FindStringSubmatchIndex(l)
	if loc == nil {
		return l
	}
	mark := " "
	if done {
		mark = "x"
	}
	// loc[4]:loc[5] is the mark inside the box.
	l = l[:loc[4]] + mark + l[loc[5]:]
	if noDate {
		return l
	}
	stamp := ""
	if done {
		stamp = date
		if stamp == "" {
			stamp = now.Format(DateLayout)
		}
	}
	l, _ = SetLineMeta(l, MetaDone, stamp)
	return l
}

// saveLinked writes n with write, syncing its linked tasks around it. The
// first error is write's; the second is the sync's, which never stops the
// note being written.
func (t *TodoSync) saveLinked(n *Note, now time.Time, noDoneDate bool, write func() error) (error, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	b, err := t.begin(now, noDoneDate)
	if err != nil {
		return write(), err
	}
	if _, err := b.prepare(n); err != nil {
		return write(), err
	}
	if err := write(); err != nil {
		return err, nil
	}
	b.push(n)
	return nil, b.commit()
}

// SyncTodo syncs every note's linked tasks with todo.txt: done-state
// changes made in todo.txt are taken into the notes, notes edited outside
// KatNotes are pushed, and lines whose note task is gone are reported. With
// dryRun nothing is written.
func (s *Store) SyncTodo(dryRun bool) ([]TodoChange, error) {
	if s.Todo == nil {
		return nil, errTodoOff
	}
	t := s.Todo
	t.mu.Lock()
	defer t.mu.Unlock()
	ns, err := s.List("", ListOpts{})
	if err != nil {
		return nil, err
	}
	b, err := t.begin(s.now(), s.NoDoneDate)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for i := range ns {
		n := &ns[i]
		if !HasTodoLinks(n.Body) {
			continue
		}
		full, err := s.Get(n.Kind, n.ID)
		if err != nil {
			return b.changes, err
		}
		changed, err := b.prepare(full)
		if err != nil {
			return b.changes, err
		}
		if changed && !dryRun {
			full.Updated = s.now()
			if err := s.write(full, false); err != nil {
				return b.changes, fmt.Errorf("%s: %w", noteKey(full), err)
			}
		}
		b.push(full)
		for _, tk := range ParseTasks(full.Body) {
			if v := linkValue(tk); v != "" {
				seen[v] = true
			}
		}
	}
	for _, l := range b.todo.Lines {
		id, ok := l.Tag(TodoIDTag)
		if !ok || seen[id] || !linkID.MatchString(id) {
			continue
		}
		note, _ := l.Tag(TodoNoteTag)
		b.changes = append(b.changes, TodoChange{ID: id, Note: note, Line: -1, Text: l.String(), Action: TodoOrphan})
	}
	sort.SliceStable(b.changes, func(i, j int) bool { return b.changes[i].Note < b.changes[j].Note })
	if dryRun {
		return b.changes, nil
	}
	return b.changes, b.commit()
}

// PushTask links the task on body line `line` to todo.txt, or for a task
// already linked whose line was removed from todo.txt, writes it again.
func (s *Store) PushTask(kind Kind, id string, line int) (*Note, error) {
	if s.Todo == nil {
		return nil, errTodoOff
	}
	n, err := s.Get(kind, id)
	if err != nil {
		return nil, err
	}
	var task *Task
	for _, t := range ParseTasks(n.Body) {
		if t.Line == line {
			task = &t
			break
		}
	}
	if task == nil {
		return nil, ErrTaskMoved
	}
	v := linkValue(*task)
	if v == linkNew {
		// Saved as {kt:new} while sync was off: saving now links it.
		if err := s.Save(n); err != nil {
			return nil, err
		}
		return n, nil
	}
	if v != "" {
		// Forget the last sync so push writes the line again if it is gone.
		if err := s.Todo.forget(v); err != nil {
			return nil, err
		}
		if err := s.Save(n); err != nil {
			return nil, err
		}
		return n, nil
	}
	return s.SetTaskMeta(kind, id, line, MetaLink, linkNew)
}

// PushAllTasks links every open task in the note that isn't linked to
// todo.txt yet, in one save; done tasks are left alone. It returns how many
// tasks it linked.
func (s *Store) PushAllTasks(kind Kind, id string) (*Note, int, error) {
	if s.Todo == nil {
		return nil, 0, errTodoOff
	}
	n, err := s.Get(kind, id)
	if err != nil {
		return nil, 0, err
	}
	lines := strings.Split(n.Body, "\n")
	count := 0
	for _, t := range ParseTasks(n.Body) {
		switch linkValue(t) {
		case "":
			if t.Done {
				continue
			}
			cr := strings.HasSuffix(lines[t.Line], "\r")
			l, _ := SetLineMeta(strings.TrimSuffix(lines[t.Line], "\r"), MetaLink, linkNew)
			if cr {
				l += "\r"
			}
			lines[t.Line] = l
			count++
		case linkNew:
			// Saved as {kt:new} while sync was off: saving now links it.
			count++
		}
	}
	if count == 0 {
		return n, 0, nil
	}
	n.Body = strings.Join(lines, "\n")
	if err := s.Save(n); err != nil {
		return nil, 0, err
	}
	return n, count, nil
}

func (t *TodoSync) forget(id string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	b, err := t.begin(time.Now(), false)
	if err != nil {
		return err
	}
	if _, ok := b.state.Done[id]; ok {
		delete(b.state.Done, id)
		b.stateChanged = true
	}
	return b.commit()
}

// writeFileAtomic replaces path with data through a temporary file.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// TodoLine is an open todo.txt line that isn't linked to a note yet.
type TodoLine struct {
	Raw  string `json:"raw"`  // the line as it reads in todo.txt
	Text string `json:"text"` // what the note task would say
	Line int    `json:"line"` // 1-based line in todo.txt
	Due  string `json:"due,omitempty"`
	Pri  string `json:"pri,omitempty"` // high, med or low
}

// importedTags are the todo.txt tags an import turns into note metadata
// (or that would be wrong to copy into the note's text).
var importedTags = map[string]bool{MetaDue: true, TodoStartTag: true, MetaPri: true, TodoIDTag: true, TodoNoteTag: true}

// todoTask works out the note task for a todo.txt line: its text (the
// line without key:value tags) and metadata.
func todoTask(l *todotxt.Line) (string, []MetaPair) {
	var words []string
	for _, w := range strings.Fields(l.Text) {
		if _, tag := todotxt.IsTag(w); !tag {
			words = append(words, w)
		}
	}
	var meta []MetaPair
	if d, ok := l.Tag(MetaDue); ok {
		if _, err := time.Parse(DateLayout, d); err == nil {
			meta = append(meta, MetaPair{MetaDue, d})
		}
	}
	if d, ok := l.Tag(TodoStartTag); ok {
		if _, err := time.Parse(DateLayout, d); err == nil {
			meta = append(meta, MetaPair{MetaStart, d})
		}
	}
	if p := priName(l.Pri); p != "" {
		meta = append(meta, MetaPair{MetaPri, p})
	}
	return strings.Join(words, " "), meta
}

// priName maps a todo.txt priority to a note one: A high, B med, C and
// below low.
func priName(p byte) string {
	switch {
	case p == 'A':
		return "high"
	case p == 'B':
		return "med"
	case p >= 'C' && p <= 'Z':
		return "low"
	}
	return ""
}

// UnlinkedTodos lists the open lines in todo.txt that have no kn: link, in
// file order.
func (s *Store) UnlinkedTodos() ([]TodoLine, error) {
	if s.Todo == nil {
		return nil, errTodoOff
	}
	s.Todo.mu.Lock()
	defer s.Todo.mu.Unlock()
	f, err := todotxt.Load(s.Todo.File)
	if err != nil {
		return nil, err
	}
	var out []TodoLine
	for i, l := range f.Lines {
		if l.Blank() || l.Done {
			continue
		}
		if _, linked := l.Tag(TodoIDTag); linked {
			continue
		}
		text, meta := todoTask(l)
		if text == "" {
			continue
		}
		tl := TodoLine{Raw: l.String(), Text: text, Line: i + 1}
		for _, p := range meta {
			switch p.Key {
			case MetaDue:
				tl.Due = p.Value
			case MetaPri:
				tl.Pri = p.Value
			}
		}
		out = append(out, tl)
	}
	return out, nil
}

// ImportTodo links the open, unlinked todo.txt line raw (as UnlinkedTodos
// gives it; failing an exact match, the one line whose text contains raw)
// to a new task at the end of the note's task list. The line gets kn: and
// note: tags; the rest of it is left as it was. It returns the note and the
// task's 0-based body line.
func (s *Store) ImportTodo(kind Kind, id, raw string) (*Note, int, error) {
	if s.Todo == nil {
		return nil, 0, errTodoOff
	}
	n, err := s.Get(kind, id)
	if err != nil {
		return nil, 0, err
	}
	text, meta, linkID, err := s.Todo.claim(n, strings.TrimSpace(raw), s.now())
	if err != nil {
		return nil, 0, err
	}
	meta = append(meta, MetaPair{MetaLink, linkID})
	return s.AddTask(kind, id, text+" "+FormatMeta(meta))
}

// claim tags the matching todo.txt line with a new ID for note n.
func (t *TodoSync) claim(n *Note, raw string, now time.Time) (string, []MetaPair, string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	b, err := t.begin(now, false)
	if err != nil {
		return "", nil, "", err
	}
	open := func(l *todotxt.Line) bool {
		_, linked := l.Tag(TodoIDTag)
		return !l.Blank() && !l.Done && !linked
	}
	var line *todotxt.Line
	for _, l := range b.todo.Lines {
		if open(l) && l.String() == raw {
			line = l
			break
		}
	}
	if line == nil && raw != "" {
		q := strings.ToLower(raw)
		var hits []*todotxt.Line
		for _, l := range b.todo.Lines {
			if open(l) && strings.Contains(strings.ToLower(l.String()), q) {
				hits = append(hits, l)
			}
		}
		switch len(hits) {
		case 0:
		case 1:
			line = hits[0]
		default:
			return "", nil, "", fmt.Errorf("%d open todo.txt tasks match %q: pick one from the list", len(hits), raw)
		}
	}
	if line == nil {
		return "", nil, "", fmt.Errorf("no open, unlinked task in %s matches %q", t.File, raw)
	}
	text, meta := todoTask(line)
	if text == "" {
		return "", nil, "", fmt.Errorf("that todo.txt task has no text to import")
	}
	id, err := b.newID()
	if err != nil {
		return "", nil, "", err
	}
	line.Text = strings.TrimSpace(line.Text + " " + TodoIDTag + ":" + id + " " + TodoNoteTag + ":" + noteKey(n))
	b.setState(id, false)
	if err := b.commit(); err != nil {
		return "", nil, "", err
	}
	return text, meta, id, nil
}
