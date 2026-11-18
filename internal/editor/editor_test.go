package editor

import (
	"reflect"
	"strings"
	"testing"
)

func TestCommandAt(t *testing.T) {
	for configured, want := range map[string]string{
		"nvim":        "nvim +7 /n.md",
		"vim -p":      "vim -p +7 /n.md",
		"hx":          "hx /n.md:7",
		"code --wait": "code --wait -g /n.md:7",
		"gedit":       "gedit /n.md",
	} {
		c, err := CommandAt(configured, "/n.md", 7)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(c.Args, " "); got != want {
			t.Errorf("%s: %q, want %q", configured, got, want)
		}
	}
}

func TestPreviewCommand(t *testing.T) {
	cases := []struct {
		configured, mode, want string
		term                   bool
	}{
		{"glow -p", "", "glow -p /n.md", true},
		{"katdown", "auto", "katdown /n.md", false},
		{"/usr/bin/mdcat", "", "/usr/bin/mdcat /n.md", true},
		{"katdown", "terminal", "katdown /n.md", true},
		{"glow", "window", "glow /n.md", false},
	}
	for _, c := range cases {
		cmd, term, err := PreviewCommand(c.configured, c.mode, "/n.md")
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(cmd.Args, " "); got != c.want || term != c.term {
			t.Errorf("%s/%s: %q term=%v, want %q term=%v", c.configured, c.mode, got, term, c.want, c.term)
		}
	}
	if _, _, err := PreviewCommand("glow", "sideways", "/n.md"); err == nil {
		t.Error("bad mode accepted")
	}
}

func TestOpenWithCommand(t *testing.T) {
	// "sh" is on every unix PATH; it stands in for an unknown program.
	cases := []struct {
		cmdline, want string
		term          bool
	}{
		{"sh", "sh /n.md", true},
		{"sh -x &", "sh -x /n.md", false},
	}
	for _, c := range cases {
		cmd, term, err := OpenWithCommand(c.cmdline, "/n.md")
		if err != nil {
			t.Skip(err)
		}
		if got := strings.Join(cmd.Args, " "); got != c.want || term != c.term {
			t.Errorf("%s: %q term=%v, want %q term=%v", c.cmdline, got, term, c.want, c.term)
		}
	}
	if _, _, err := OpenWithCommand("  & ", "/n.md"); err == nil {
		t.Error("empty program accepted")
	}
	if _, _, err := OpenWithCommand("no-such-program-katnotes", "/n.md"); err == nil {
		t.Error("missing program accepted")
	}
}

func TestInTerminal(t *testing.T) {
	for prog, want := range map[string][2]bool{
		"nvim": {true, true}, "/usr/bin/vim": {true, true}, "code": {false, true},
		"gedit": {false, true}, "glow": {true, true}, "mystery": {false, false},
	} {
		if term, known := InTerminal(prog); term != want[0] || known != want[1] {
			t.Errorf("%s: term=%v known=%v, want %v", prog, term, known, want)
		}
	}
}

func TestSplitArgs(t *testing.T) {
	for in, want := range map[string][]string{
		`emacsclient -nw -a ''`: {"emacsclient", "-nw", "-a", ""},
		`code  --wait`:          {"code", "--wait"},
		`vim "-c" "set ft=md"`:  {"vim", "-c", "set ft=md"},
		`a 'it''s' b\ c "q\"x"`: {"a", "its", "b c", `q"x`},
		``:                      nil,
	} {
		got, err := SplitArgs(in)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("SplitArgs(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{`a 'b`, `a "b`} {
		if _, err := SplitArgs(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
