// Devices, sets of devices, and connections.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Connection struct {
	dev       *Device
	cmd       *exec.Cmd
	forwarded bool
}

type Device struct {
	// Note that some fields in the JSON device database are ignored.
	Serial       string `json:"serial"`
	ID           string `json:"id"`
	User         string `json:"user"`
	offset       int
	port         int
	Location     string `json:"allocation"`
	Comment      string `json:"notes"`
	parent       *DeviceSet
	tunnelCmd    *exec.Cmd
	Hidden       bool `json:"hidden"`
	mounted      bool
	vncForwarded bool
}

type DeviceSet struct {
	deviceList          []*Device
	devicesBySerial     map[string]*Device
	tunnelFinish        chan *Device
	connectionFinish    chan *Connection
	connections         map[*Connection]bool
	forwardedConnection *Connection
	unlockHidden        bool
}

func sshVerbose() string {
	if config.Verbose {
		return "-v"
	}
	return ""
}

func (dev *Device) ConnectCommand(addForwards bool) []string {
	forwards := ""
	if addForwards {
		if config.UseLoopbackAddrs {
			forwards += strings.ReplaceAll(config.CommonForwards, "-L", fmt.Sprintf("-L127.0.1.%d:", dev.offset))
		} else {
			forwards = config.CommonForwards
		}
	}

	if !dev.vncForwarded {
		dev.vncForwarded = true
		vncPort := dev.port - config.PortOffset + 5900
		vncForward := ""
		if config.UseLoopbackAddrs {
			vncForward = fmt.Sprintf("127.0.1.%d:%d", dev.offset, vncPort)
		} else {
			vncForward = fmt.Sprintf("localhost:%d", vncPort)
		}
		forwards += fmt.Sprintf(" -L%s:localhost:%d", vncForward, vncPort)
		fmt.Printf("VNC server at %s\n", vncForward)
		fmt.Print(forwards)
	}

	// Pass along AWS env vars, if set. Only implemented in Linux and macOS for now.
	env_vars := ""
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		for _, aws_var := range []string{"AWS_SECRET_ACCESS_KEY", "AWS_ACCESS_KEY_ID", "AWS_SESSION_TOKEN"} {
			if value := os.Getenv(aws_var); value != "" {
				env_vars += fmt.Sprintf(" %s=%s", aws_var, value)
			}
		}
	}

	// Pass along git user and email, if they can be ascertained.
	gitArgs := strings.Fields("git config --global -l")
	cmd := exec.Command(gitArgs[0], gitArgs[1:]...)
	var outBuffer bytes.Buffer
	cmd.Stdout = &outBuffer

	err := cmd.Run()
	if err == nil {
		for _, line := range strings.Split(outBuffer.String(), "\n") {
			if keyValue := strings.SplitN(strings.TrimSpace(line), "=", 2); len(keyValue) == 2 {
				switch keyValue[0] {
				case "user.email":
					email := keyValue[1]
					env_vars += fmt.Sprintf(" GIT_COMMITTER_EMAIL=%s", email)
					env_vars += fmt.Sprintf(" GIT_AUTHOR_EMAIL=%s", email)
				case "user.name":
					name := strings.Replace(keyValue[1], " ", ".", -1)
					env_vars += fmt.Sprintf(" GIT_COMMITTER_NAME=%s", name)
					env_vars += fmt.Sprintf(" GIT_AUTHOR_NAME=%s", name)
				}
			}
		}
	}

	// The ssh command should be the same across all platforms.
	ssh_command := fmt.Sprintf("ssh -A %s -o StrictHostKeychecking=no -o UserKnownHostsFile=/dev/null -t -p %d %s %s@localhost %s bash -l",
		config.sshOptions(), dev.port, forwards, dev.User, env_vars)

	if config.Verbose {
		fmt.Println(ssh_command)
	}

	// Always show sftp access method.
	fmt.Printf("\nFor file transfers to device %s:\nsftp -o StrictHostKeychecking=no -o UserKnownHostsFile=/dev/null -P %d %s@localhost\n",
		dev.Serial, dev.port, dev.User)

	// And the ssh-copy-id command.
	fmt.Printf("\nTo install your default pubkey on device %s:\nssh-copy-id -o StrictHostKeychecking=no -o UserKnownHostsFile=/dev/null -p %d %s@localhost\n\n", dev.Serial, dev.port, dev.User)

	// Return os-specific command to connect to device.
	if runtime.GOOS == "windows" {
		return strings.Fields(fmt.Sprintf("cmd.exe /c start /wait %s", ssh_command))
	} else if runtime.GOOS == "darwin" {
		return darwinConnectCommand(ssh_command)
	} else {
		// Linux
		if _, err := exec.LookPath("xterm"); err == nil {
			return strings.Fields(fmt.Sprintf("xterm -title %s -e %s", dev.Serial, ssh_command))
		} else {
			return strings.Fields(fmt.Sprintf("gnome-terminal --disable-factory -- %s", ssh_command))
		}
	}
}

func (dev *Device) tunnelSetup() {
	var err error
	var sshTunnelKeyFile string

	if dev.tunnelCmd != nil {
		return
	}

	// If the key is on S3, make a temporary local copy, with deferred removal.
	if strings.HasPrefix(config.TunnelKeyPath, "s3://") {
		sshTunnelKeyFile = ".tunnel_key"
		if err = s3Download(config.TunnelKeyPath, sshTunnelKeyFile); err != nil {
			return
		}
		os.Chmod(sshTunnelKeyFile, 0600)
		defer os.Remove(sshTunnelKeyFile)
	} else {
		sshTunnelKeyFile = config.TunnelKeyPath
	}

	tunnelArgs := strings.Fields(fmt.Sprintf("ssh -i %s %s -o StrictHostKeyChecking=accept-new -L%d:localhost:%d -N %s",
		sshTunnelKeyFile, config.sshOptions(), dev.port, dev.port, config.TunnelNameAddr))
	fmt.Println(tunnelArgs)

	// Start the tunnel command.
	dev.tunnelCmd = exec.Command(tunnelArgs[0], tunnelArgs[1:]...)
	dev.tunnelCmd.Stderr = os.Stderr

	if err = dev.tunnelCmd.Start(); err != nil {
		fmt.Println(err)
		dev.tunnelCmd = nil
		return
	}

	go func() {
		dev.tunnelCmd.Wait()
		fmt.Println("tunnel exited")
		dev.tunnelCmd = nil
	}()

	// Wait for tunnel port to be available.
	for {
		if conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", dev.port), 1*time.Second); err == nil {
			// Tunnel ready and forwarding.
			conn.Close()
			break
		}
		if dev.tunnelCmd == nil {
			// Tunnel exited for some reason.
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func (dev *Device) connect() {
	var err error

	dev.tunnelSetup()
	if dev.tunnelCmd == nil {
		return
	}

	addForwards := (dev.parent.forwardedConnection == nil)

	if addForwards && config.SpecialPort != "" {
		if config.UseLoopbackAddrs {
			if err = enableLoopbackAddr(fmt.Sprintf("127.0.1.%d", dev.offset)); err != nil {
				fmt.Println(err)
				return
			}
			fmt.Printf("+++ using 127.0.1.%d forwards\n", dev.offset)
		} else if conn, err := net.DialTimeout("tcp", config.SpecialPort, 1*time.Second); err == nil {
			conn.Close()
			fmt.Printf("*** %s already in use, not forwarding\n", config.SpecialPort)
			addForwards = false
		} else {
			fmt.Printf("+++ using localhost forwards\n")
		}
	}

	connectArgs := dev.ConnectCommand(addForwards)

	cmd := exec.Command(connectArgs[0], connectArgs[1:]...)
	if err = cmd.Start(); err != nil {
		fmt.Println(err)
		return
	}

	con := &Connection{dev, cmd, addForwards}

	dev.parent.connections[con] = true

	if dev.parent.forwardedConnection == nil {
		dev.parent.forwardedConnection = con
	}

	go func() {
		cmd.Wait()
		dev.parent.connectionFinish <- con
		dev.vncForwarded = false
	}()
}

// mount sets up an sshfs mount of the remote device filesystem to the local
// system.
func (dev *Device) mount() {
	var err error
	if runtime.GOOS == "windows" {
		return
	}

	if dev.Hidden {
		fmt.Println("sshfs not allowed on hidden devices")
		return
	}

	if dev.mounted {
		fmt.Println("already mounted")
		return
	}

	dev.tunnelSetup()
	if dev.tunnelCmd == nil {
		return
	}

	mountArgs := strings.Fields(fmt.Sprintf("sshfs -f %s -o BatchMode=yes -o StrictHostKeychecking=no -o UserKnownHostsFile=/dev/null -o port=%d %s@127.0.1.%d:/",
		config.sshOptions(), dev.port, dev.User, dev.offset))

	mountPoint := fmt.Sprintf("%s/sshfs/%s", os.Getenv("HOME"), dev.Serial)
	os.MkdirAll(mountPoint, 0700)

	mountArgs = append(mountArgs, mountPoint)

	fmt.Println(mountArgs)

	cmd := exec.Command(mountArgs[0], mountArgs[1:]...)
	var errBuffer bytes.Buffer
	cmd.Stderr = &errBuffer

	if err = cmd.Start(); err != nil {
		fmt.Println(err)
		return
	}

	con := &Connection{dev, cmd, false}

	dev.parent.connections[con] = true

	dev.mounted = true

	go func() {
		cmd.Wait()
		dev.parent.connectionFinish <- con
		dev.mounted = false

		// Dump stderr for diagnostics.
		fmt.Println(errBuffer.String())

		if runtime.GOOS == "darwin" {
			// Some extra cleanup is required.
			exec.Command("diskutil", "umount", mountPoint).Run()
		}
	}()

}

func (dset *DeviceSet) add(d *Device) {
	dset.deviceList = append(dset.deviceList, d)
	dset.devicesBySerial[d.Serial] = d
}

func (dset *DeviceSet) list() {
	fmt.Print("\nAvailable devices:\n")
	fmt.Printf("serial, id, location, (comment)\n")
	for _, device := range dset.deviceList {
		if device.Hidden && !dset.unlockHidden {
			continue
		}
		fmt.Printf("%s, %4d, %s, (%s)\n", device.Serial, device.offset, device.Location, device.Comment)
	}
}

func (dset *DeviceSet) find(s string) *Device {
	for _, device := range dset.deviceList {
		if s == device.Serial || atoi(s) == device.offset {
			if device.Hidden && !dset.unlockHidden {
				return nil
			} else {
				return device
			}
		}
	}

	// If a tunnel port number with "!" was provided, create an anonymous device and return it.
	if s[len(s)-1] == '!' {
		if port, err := strconv.Atoi(s[0 : len(s)-1]); err == nil {
			serial := fmt.Sprintf("anonymous-%d", port)
			if device, ok := dset.devicesBySerial[serial]; ok {
				return device
			} else {
				newDevice := &Device{Serial: serial,
					offset:   port - config.PortOffset,
					port:     port,
					Location: "dev",
					User:     config.AnonUser,
					Comment:  fmt.Sprintf("anonymous device - port %d", port),
					parent:   dset}
				dset.add(newDevice)
				return newDevice
			}
		}
	}

	return nil
}

// Load device database, return a DeviceSet.
func loadDevices() *DeviceSet {
	fmt.Printf("Device database: %s\n", config.DevicesPath)

	if strings.HasPrefix(config.DevicesPath, "s3://") {
		if result, err := s3Get(config.DevicesPath); err == nil {
			device_database = string(result)
		} else {
			fmt.Printf("*** AWS: failed to get %s from S3\n", config.DevicesPath)
			fmt.Println("*** AWS: Try refreshing your credentials and environment variables")
			os.Exit(1)
		}
	}

	dset := &DeviceSet{tunnelFinish: make(chan *Device), connectionFinish: make(chan *Connection),
		devicesBySerial: make(map[string]*Device),
		connections:     make(map[*Connection]bool)}

	var devices []*Device
	err := json.Unmarshal([]byte(device_database), &devices)
	if err != nil {
		fmt.Println("Error parsing JSON:", err)
	} else {
		for _, d := range devices {
			if d.ID != "" {
				d.offset = atoi(d.ID)
				d.port = d.offset + config.PortOffset
				d.parent = dset
				dset.add(d)
			}
		}
	}

	return dset
}
