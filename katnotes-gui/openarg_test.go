package main

import "testing"

func TestOpenArg(t *testing.T) {
	cases := map[string][]string{
		"page/house":         {"--open", "page/house"},
		"journal/2026-11-23": {"-x", "--open=journal/2026-11-23"},
		"":                   {"--open"},
	}
	for want, args := range cases {
		if got := openArg(args); got != want {
			t.Errorf("openArg(%q) = %q, want %q", args, got, want)
		}
	}
	if openArg(nil) != "" {
		t.Error("no args")
	}
}
