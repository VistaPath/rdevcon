// Stub versions of windows-only functions to allow cross-platform builds.
//go:build !windows

package main

func windowsShowMessage(message string) {
}

func windowsIsAdmin() bool {
	return false
}
