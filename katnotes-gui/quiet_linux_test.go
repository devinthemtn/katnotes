//go:build linux

package main

import (
	"strings"
	"testing"
)

func TestFilterNoise(t *testing.T) {
	in := "Overriding existing handler for signal 10. Set JSC_SIGNAL_FOR_GC if you want WebKit to use a different signal\n" +
		"libEGL warning: DRI3 error: Could not get DRI3 device\n" +
		"real error: something broke\n" +
		"libEGL warning: Ensure your X server supports DRI3 to get accelerated rendering\n" +
		"last line without newline"
	var out strings.Builder
	filterNoise(strings.NewReader(in), &out)
	want := "real error: something broke\nlast line without newline"
	if out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
}
