//go:build !unix

package editor

import "os/exec"

func detach(cmd *exec.Cmd) {}
