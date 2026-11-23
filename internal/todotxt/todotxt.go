// Package todotxt reads and writes todo.txt files
// (https://github.com/todotxt/todo.txt), the format katTasks uses. It only
// rewrites the lines it changes: every other line, including blank lines,
// is written back exactly as it was read.
package todotxt

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DateLayout is the date format used in todo.txt.
const DateLayout = "2006-01-02"

const bom = "\ufeff"

// Line is one line of a todo.txt file.
type Line struct {
	Done      bool
	Pri       byte   // 'A'..'Z', or 0 for none
	Completed string // YYYY-MM-DD, done lines only
	Created   string // YYYY-MM-DD
	Text      string // description, with +projects, @contexts and key:value tags

	orig, origForm string
	blank          bool
}

// Parse parses one todo.txt line. It never fails: anything it doesn't
// recognize stays in Text.
func Parse(s string) *Line {
	s = strings.TrimRight(s, "\r\n")
	l := &Line{orig: s}
	defer func() { l.origForm = l.String() }()
	if strings.TrimSpace(s) == "" {
		l.blank = true
		return l
	}
	rest := s
	if strings.HasPrefix(rest, "x ") {
		l.Done = true
		rest = rest[2:]
	}
	if len(rest) >= 4 && rest[0] == '(' && rest[1] >= 'A' && rest[1] <= 'Z' && rest[2] == ')' && rest[3] == ' ' {
		l.Pri, rest = rest[1], rest[4:]
	}
	if d, r, ok := cutDate(rest); ok {
		if l.Done {
			l.Completed, rest = d, r
			if d2, r2, ok := cutDate(rest); ok {
				l.Created, rest = d2, r2
			}
		} else {
			l.Created, rest = d, r
		}
	}
	l.Text = rest
	return l
}

func cutDate(s string) (string, string, bool) {
	if len(s) < 10 || (len(s) > 10 && s[10] != ' ') {
		return "", s, false
	}
	d := s[:10]
	for i, c := range d {
		if (i == 4 || i == 7) != (c == '-') || (c != '-' && (c < '0' || c > '9')) {
			return "", s, false
		}
	}
	if len(s) == 10 {
		return d, "", true
	}
	return d, s[11:], true
}

// String renders the line in todo.txt form.
func (l *Line) String() string {
	if l.blank {
		return l.orig
	}
	var b strings.Builder
	if l.Done {
		b.WriteString("x ")
	}
	if l.Pri != 0 {
		b.WriteByte('(')
		b.WriteByte(l.Pri)
		b.WriteString(") ")
	}
	if l.Done && l.Completed != "" {
		b.WriteString(l.Completed + " ")
	}
	if l.Created != "" {
		b.WriteString(l.Created + " ")
	}
	b.WriteString(strings.TrimRight(l.Text, " \t"))
	return b.String()
}

// raw is what goes in the file: the line as read unless it changed.
func (l *Line) raw() string {
	if l.String() == l.origForm {
		return l.orig
	}
	return l.String()
}

// Blank reports whether the line is empty or only whitespace.
func (l *Line) Blank() bool { return l.blank }

func splitTag(w string) (string, string, bool) {
	k, v, ok := strings.Cut(w, ":")
	if !ok || k == "" || v == "" || strings.ContainsAny(k, "+@") || strings.HasPrefix(v, "//") {
		return "", "", false
	}
	return k, v, true
}

// Tag returns the value of the first key:value tag with key.
func (l *Line) Tag(key string) (string, bool) {
	for _, w := range strings.Fields(l.Text) {
		if k, v, ok := splitTag(w); ok && k == key {
			return v, true
		}
	}
	return "", false
}

// IsTag reports whether the word w is a key:value tag, and returns its key.
func IsTag(w string) (string, bool) {
	k, _, ok := splitTag(w)
	return k, ok
}

// File is a todo.txt file as read, so it can be written back unchanged
// except for the lines that were edited or added.
type File struct {
	Path  string
	Lines []*Line

	eol      string
	bom      bool
	missing  bool
	original []byte
	// noEOL is the last line read when the file didn't end in a newline;
	// it stays without one while it is still last.
	noEOL *Line
}

// Load reads path. A missing file is an empty File.
func Load(path string) (*File, error) {
	f := &File{Path: path, eol: "\n"}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		f.missing = true
		return f, nil
	}
	if err != nil {
		return nil, err
	}
	f.original = data
	if bytes.HasPrefix(data, []byte(bom)) {
		f.bom = true
		data = data[len(bom):]
	}
	if i := bytes.IndexByte(data, '\n'); i > 0 && data[i-1] == '\r' {
		f.eol = "\r\n"
	}
	s, eol := strings.CutSuffix(string(data), "\n")
	if s == "" {
		return f, nil
	}
	for _, raw := range strings.Split(s, "\n") {
		f.Lines = append(f.Lines, Parse(raw))
	}
	if !eol {
		f.noEOL = f.Lines[len(f.Lines)-1]
	}
	return f, nil
}

// Find returns the first line whose key tag is value, or nil.
func (f *File) Find(key, value string) *Line {
	for _, l := range f.Lines {
		if v, ok := l.Tag(key); ok && v == value && !l.blank {
			return l
		}
	}
	return nil
}

// Add appends a line.
func (f *File) Add(l *Line) { f.Lines = append(f.Lines, l) }

// Bytes renders the file.
func (f *File) Bytes() []byte {
	var b bytes.Buffer
	if f.bom {
		b.WriteString(bom)
	}
	for i, l := range f.Lines {
		b.WriteString(l.raw())
		if i < len(f.Lines)-1 || l != f.noEOL {
			b.WriteString(f.eol)
		}
	}
	return b.Bytes()
}

// Changed reports whether Bytes differs from the file as read.
func (f *File) Changed() bool {
	if f.missing {
		return len(f.Lines) > 0
	}
	return !bytes.Equal(f.Bytes(), f.original)
}

// Save writes the file in one step (a temporary file, then a rename) if it
// changed. A symlinked file is written through the link.
func (f *File) Save() error {
	if !f.Changed() {
		return nil
	}
	path := f.Path
	if p, err := filepath.EvalSymlinks(path); err == nil {
		path = p
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	mode := fs.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	data := f.Bytes()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	f.original, f.missing = data, false
	return nil
}
