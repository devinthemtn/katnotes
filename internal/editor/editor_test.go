package editor

import (
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
