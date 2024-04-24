// Text-mode user interface (TUI) for remote device connections.

package main

import (
	"bufio"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

var config *Config

//go:embed devices.json
var device_database string

func systemOk() bool {
	var err error

	if runtime.GOOS == "windows" {
		if err = exec.Command("ssh", "-V").Run(); err != nil {
			fmt.Println("Please install OpenSSH client: https://www.hawaii.edu/askus/1874")
			return false
		}
	}

	return true
}

func checkExitConditions(done *bool) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		cmd := exec.Command("sh", "-c", "lsof | grep sshfs/")

		output, _ := cmd.Output()

		if string(output) == "" {
			*done = true
		} else {
			fmt.Printf("Some processes are holding sshfs references, please close them:\n%s\n", output)
		}
	} else if runtime.GOOS == "windows" {
		// sshfs not supported on Windows, so nothing to check.
		*done = true
	}
}

func help() {
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("123 - connect to device with port offset 123")
	fmt.Println("23080123T - connect to device with serial 23080123T")
	fmt.Println("22123! - connect to device with tunnel port 22123 (for unlisted devices)")
	fmt.Println("list - list devices")
	fmt.Println("help - this help")
	fmt.Println("exit - exit program")
	fmt.Println("exit!- exit program even if clean exit conditions aren't met (also ctrl-d or ctrl-z)")
}

func main() {
	config = ConfigLoad()

	// Pass certain args along to ssh commands.
	optNext := ""
	for _, arg := range os.Args {
		if arg == "-v" {
			config.Verbose = true
		} else if len(arg) == 2 && arg[0:1] == "-" && strings.Index("iIo", arg[1:]) >= 0 {
			optNext = arg
			continue
		} else if optNext != "" {
			config.SshOptionList = append(config.SshOptionList, fmt.Sprintf("%s %s\n", optNext, arg))
		}
		optNext = ""
	}

	awsSetup()

	checkForUpdates()

	if !systemOk() {
		fmt.Print("Press Enter to continue...")
		fmt.Scanln()
		return
	}

	allDevices := loadDevices()
	allDevices.list()
	help()
	fmt.Print("> ")

	command := make(chan string)

	go func() {
		// Command-line input
		scanner := bufio.NewScanner(os.Stdin)
		for {
			if !scanner.Scan() {
				// eof
				fmt.Println("")
				// We've lost stdin at this point, so we can't retry
				// if there are problems
				command <- "exit!"
				break
			}

			input := strings.TrimSpace(scanner.Text())
			command <- input
		}
	}()

	handleCommand := func(input string, done *bool) {
		ilen := len(input)
		if ilen == 0 {
		} else if input == "exit" {
			checkExitConditions(done)
		} else if input == "exit!" {
			*done = true
		} else if input == "list" {
			allDevices.list()
		} else if input == "unlock-hidden" {
			allDevices.unlockHidden = true
		} else if input == "lock-hidden" {
			allDevices.unlockHidden = false
		} else if input == "help" {
			help()
		} else if input[ilen-1:] == "~" {
			if dev := allDevices.find(input[:ilen-1]); dev != nil {
				dev.mount()
			}
		} else if dev := allDevices.find(input); dev != nil {
			dev.connect()
		}
		if !*done {
			fmt.Print("> ")
		}
	}

	for {
		// Main loop servicing command-line (and TBD http) requests.
		// To avoid race conditions, limit state changes to synchronous
		// function calls from this loop.
		done := false
		select {
		case input := <-command:
			handleCommand(input, &done)
		case _ = <-time.After(1 * time.Second):
			// fmt.Println("timeout")
		case dev := <-allDevices.tunnelFinish:
			// Explicitly close/kill all connections supported by tunnel.
			// TBD for now, needs testing.
			dev = dev
		case con := <-allDevices.connectionFinish:
			// Clear forwardedConnection if it matches.
			if con == allDevices.forwardedConnection {
				allDevices.forwardedConnection = nil
				fmt.Println("forwards available again!")
			}
			// Remove from connection set.
			delete(allDevices.connections, con)
		}

		if done {
			break
		}
	}

	for _, dev := range allDevices.deviceList {
		if dev.tunnelCmd != nil {
			dev.tunnelCmd.Process.Kill()
		}
	}
}
