//go:build !windows

package main

// fromExplorer is Windows-only; see explorer_windows.go.
func fromExplorer() bool { return false }
