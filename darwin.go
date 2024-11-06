// Special support for Darwin clients.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Basic AppleScript to launch our ssh session.
var terminalTemplate = `
tell app "Terminal" to do script "%s; exit"
`

// This has to be a separate block of AppleScript, because the "create
// window with default profile" will fail if iTerm is not installed.
var itermTemplate = `
tell application "iTerm"
	activate
	if (count of windows) = 0 then
		set newWindow to (create window with default profile)
		tell current session of newWindow
			write text "%s; exit"
		end tell
	else
		tell current window
			tell current session of current tab
				set newPane to (split vertically with default profile)
				tell newPane
					write text "%s; exit"
				end tell
			end tell
		end tell
	end if
end tell
`

var launchScriptTemplate = `#!/bin/bash
tempdir=$1
cd $tempdir
mkfifo done
osascript %s
cat done # This will block until the dd command writes to the fifo.
cd /tmp
rm -rf ${tempdir}
`

// darwinConnectCommand returns a temporary bash script that wraps two
// other scripts in a temporary folder. The order of invocation goes
// like this,
//
//	launch-and-wait.sh
//	 `- osascript terminal.script
//	     `- connect.sh
//
// It has to be done like this because the osascript command that
// launches the terminal runs asynchronously, but we want the ssh
// command to block. The cat/dd commands on the fifo handle the
// synchronization.
func darwinConnectCommand(ssh_command string) []string {
	tempdir, _ := os.MkdirTemp("/tmp", "rdevcon-*")

	connectScriptPath := tempdir + "/" + "connect.sh"
	connectScriptContents := fmt.Sprintf("#!/bin/bash\n%s\ndd if=/dev/null of=%s/done\n",
		ssh_command, tempdir)
	os.WriteFile(connectScriptPath, []byte(connectScriptContents), 0700)

	var terminalScript string
	matches, _ := filepath.Glob("/Applications/iTerm*.app")
	if len(matches) > 0 {
		terminalScript = fmt.Sprintf(itermTemplate, connectScriptPath, connectScriptPath)
	} else {
		terminalScript = fmt.Sprintf(terminalTemplate, connectScriptPath)
	}

	terminalScriptPath := tempdir + "/" + "terminal.scpt"
	os.WriteFile(terminalScriptPath, []byte(terminalScript), 0700)

	launchScriptPath := tempdir + "/" + "launch-and-wait.sh"
	os.WriteFile(launchScriptPath, []byte(fmt.Sprintf(launchScriptTemplate, terminalScriptPath)), 0700)

	return []string{launchScriptPath, tempdir}
}

// sudo askpass program, created in a temp folder that is deleted after use
var askpassScript = `#!/bin/bash
osascript -e 'Tell application "System Events" to display dialog "Authentication requried to add loopback address:" default answer "" with hidden answer buttons {"OK"} default button 1' -e 'text returned of result'
`

func darwinSudoCommand(args []string) error {
	tempdir, _ := os.MkdirTemp("/tmp", "rdevcon-sudo-*")
	defer os.RemoveAll(tempdir)

	askpassScriptPath := tempdir + "/" + "askpass.sh"
	os.WriteFile(askpassScriptPath, []byte(askpassScript), 0700)

	cmd := exec.Command("sudo", "-A")
	cmd.Args = append(cmd.Args, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "SUDO_ASKPASS="+askpassScriptPath)

	return cmd.Run()
}
