// Windows specific stuff

//go:build windows

package main

import (
	"log"
	"os/exec"
	"runtime"
	"syscall"
)

func windowsIsAdmin() bool {
	// Load shell32.dll
	modShell32 := syscall.NewLazyDLL("shell32.dll")
	procIsUserAnAdmin := modShell32.NewProc("IsUserAnAdmin")

	// Call the IsUserAnAdmin function
	r1, _, _ := procIsUserAnAdmin.Call()

	// If r1 is 1, the user is an admin
	return r1 != 0
}

func windowsShowMessage(message string) {
	if runtime.GOOS == "windows" {
		// PowerShell script to display the message box
		psScript := `Add-Type -AssemblyName "System.Windows.Forms"; [System.Windows.Forms.MessageBox]::Show('` + message + `', 'Information', [System.Windows.Forms.MessageBoxButtons]::OK)`

		cmd := exec.Command("powershell", "-Command", psScript)
		err := cmd.Run()
		if err != nil {
			log.Println("Failed to show message box:", err)
		}
	}
}
