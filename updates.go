// Self-update code.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	//"time"
)

func checkForUpdates() {
	if config.SelfUpdatePath == "" {
		return
	}

	defer fmt.Println("")

	executable, _ := os.Executable()

	fmt.Printf("Checking for new version at %s\n", config.SelfUpdatePath)
	key := "X-Sha1"
	aws_sha1, err := s3Metadata(config.SelfUpdatePath, key)
	if err != nil {
		fmt.Println(err)
		return
	}

	if aws_sha1 == "" {
		fmt.Printf("key %s is empty or not set on %s\n", key, config.SelfUpdatePath)
		return
	}

	exe_sha1 := sha1string(executable)

	if exe_sha1 == aws_sha1 {
		fmt.Println("Have latest version.")
		return
	}

	fmt.Println("Checksum mismatch! Getting new version...")

	for _, f := range []string{"main.go", "rdevcon/main.go"} {
		if _, err = os.Stat(f); err == nil {
			fmt.Println("*** Not updating inside development tree.")
			return
		}
	}

	newBinary, err := s3Get(config.SelfUpdatePath)
	if err != nil {
		fmt.Println("error getting %s\n", config.SelfUpdatePath)
		return
	}

	// Rename current to old
	backupName := executable + ".old"

	os.Remove(backupName)

	err = os.Rename(executable, backupName)
	if err != nil {
		fmt.Println("error renaming %s -> %s\n", executable, backupName)
		return
	}

	// Save new to current
	err = os.WriteFile(executable, newBinary, 0700)
	if err != nil {
		fmt.Println("error writing new %s\n", executable)
		return
	}

	fmt.Println("Restarting...\n")

	if runtime.GOOS == "windows" {
		// Windows doesn't have exec(), so the best we can do is
		// start the new version and silently wait for it to exit.
		cmd := exec.Command(executable, os.Args[1:]...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if cmd.Start() != nil {
			fmt.Println("Error re-starting")
		} else {
			cmd.Wait()
			os.Exit(0)
		}
	} else {
		// Linux and macOS.
		if err := syscall.Exec(executable, os.Args, os.Environ()); err != nil {
			fmt.Println("Error re-executing program:", err)
		}
	}

}
