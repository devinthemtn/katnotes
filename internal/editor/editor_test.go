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
