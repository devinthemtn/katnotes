//go:build !linux

package main

// quietStartup does nothing: the noisy WebKitGTK lines are Linux-only.
func quietStartup() {}
