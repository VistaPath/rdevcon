//go:build !windows

package main

func windowsShowMessage(message string) {
}

func windowsIsAdmin() bool {
	return false
}
