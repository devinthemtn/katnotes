//go:build linux

package main

import (
	"bufio"
	"io"
	"log"
	"os"
	"runtime/debug"
	"strings"
	"syscall"
)

// noise is the start of lines WebKitGTK and Mesa print on every start
// on some systems (WSL, some VMs and drivers). They are harmless and only
// confuse people who start the app from a terminal.
var noise = []string{
	"Overriding existing handler for signal ",
	"libEGL warning: ",
}

// quietStartup hides those lines. Set KATNOTES_GUI_VERBOSE=1 to see them.
//
// The C libraries write straight to file descriptor 2, so fd 2 becomes a
// pipe that drops the noise and passes everything else through. Go's own
// output (os.Stderr, log, crash reports) goes to the real stderr directly,
// so a panic is never lost in the pipe.
func quietStartup() {
	if os.Getenv("KATNOTES_GUI_VERBOSE") != "" {
		return
	}
	if os.Getenv("EGL_LOG_LEVEL") == "" {
		_ = os.Setenv("EGL_LOG_LEVEL", "fatal") // Mesa: only fatal EGL errors
	}

	realFd, err := syscall.Dup(2)
	if err != nil {
		return
	}
	syscall.CloseOnExec(realFd)
	real := os.NewFile(uintptr(realFd), "/dev/stderr")

	r, w, err := os.Pipe()
	if err != nil {
		real.Close()
		return
	}
	if err := syscall.Dup3(int(w.Fd()), 2, 0); err != nil {
		r.Close()
		w.Close()
		real.Close()
		return
	}
	w.Close() // fd 2 now holds the write end

	os.Stderr = real
	log.SetOutput(real)
	_ = debug.SetCrashOutput(real, debug.CrashOptions{})

	go filterNoise(r, real)
}

func filterNoise(r io.Reader, out io.Writer) {
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if line != "" && !isNoise(line) {
			_, _ = io.WriteString(out, line)
		}
		if err != nil {
			return
		}
	}
}

func isNoise(line string) bool {
	for _, p := range noise {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return false
}
