package todotxt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		in        string
		done      bool
		pri       byte
		completed string
		created   string
		text      string
	}{
		{"(A) 2026-11-23 call +work due:fri", false, 'A', "", "2026-11-23", "call +work due:fri"},
		{"x 2026-11-24 2026-11-23 done it", true, 0, "2026-11-24", "2026-11-23", "done it"},
		{"x (B) 2026-11-24 odd but fine", true, 'B', "2026-11-24", "", "odd but fine"},
		{"plain", false, 0, "", "", "plain"},
		{"(a) lowercase is text", false, 0, "", "", "(a) lowercase is text"},
		{"2026-1-2 not a date", false, 0, "", "", "2026-1-2 not a date"},
	}
	for _, c := range cases {
		l := Parse(c.in)
		if l.Done != c.done || l.Pri != c.pri || l.Completed != c.completed || l.Created != c.created || l.Text != c.text {
			t.Errorf("Parse(%q) = %+v", c.in, l)
		}
		if l.String() != c.in {
			t.Errorf("String(%q) = %q", c.in, l.String())
		}
	}
	l := Parse("a kn:abc123 url:https://x.y http://z note:page/p")
	if v, _ := l.Tag("kn"); v != "abc123" {
		t.Errorf("kn = %q", v)
	}
	if _, ok := l.Tag("http"); ok {
		t.Error("a URL read as a tag")
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	for name, data := range map[string]string{
		"lf":   "(A)  two  spaces kept\n\n   \nx 2026-01-01 done\n",
		"crlf": "\ufeffone\r\ntwo kn:aaaaaa\r\n",
		"none": "no newline at end",
	} {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte(data), 0o600)
		f, err := Load(p)
		if err != nil {
			t.Fatal(err)
		}
		if f.Changed() {
			t.Errorf("%s: unchanged file reports a change: %q", name, f.Bytes())
		}
		if name == "crlf" {
			f.Find("kn", "aaaaaa").Done = true
			f.Add(&Line{Text: "three"})
			if err := f.Save(); err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(p)
			if string(got) != "\ufeffone\r\nx two kn:aaaaaa\r\nthree\r\n" {
				t.Errorf("crlf saved %q", got)
			}
			if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
				t.Errorf("mode = %v", st.Mode())
			}
		}
	}

	missing := filepath.Join(dir, "sub", "todo.txt")
	f, err := Load(missing)
	if err != nil || f.Changed() {
		t.Fatalf("missing: %v %v", err, f.Changed())
	}
	f.Save()
	if _, err := os.Stat(missing); err == nil {
		t.Error("saving an empty missing file created it")
	}
}
