// Loopback device configuration

package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os/exec"
	"runtime"
)

var loopbackAliases = []string{}

func enableLoopbackAddr(addr string) error {
	// First validate the IP address.
	ip := net.ParseIP(addr)
	if ip == nil {
		return errors.New("Invalid IP address")
	}

	_, subnet, _ := net.ParseCIDR("127.0.0.0/8")
	if !subnet.Contains(ip) {
		return errors.New("Invalid loopback IP address")
	}

	// Add the loopback alias or address.
	if runtime.GOOS == "linux" {
		// Nothing to do! Loopback aliases work automatically on Linux.
	}

	if runtime.GOOS == "darwin" {
		rc := exec.Command("sh", "-c", fmt.Sprintf("ifconfig lo0 | grep 'inet %s '", addr)).Run()
		if rc == nil {
			return nil
		}

		// Add the loopback.
		err := darwinSudoCommand(" to add loopback alias", []string{"ifconfig", "lo0", "alias", addr})
		if err != nil {
			return err
		}

	}

	if runtime.GOOS == "windows" {
		cmd := exec.Command("netsh", "interface", "ipv4", "add", "address", "Microsoft Loopback Adapter", addr, "255.0.0.0")
		err := cmd.Run()
		if err != nil {
			return err
		}
	}

	loopbackAliases = append(loopbackAliases, addr)

	return nil
}

// Clean up resources allocated at runtime.
func loopbackCleanup() {
	for _, addr := range loopbackAliases {
		if runtime.GOOS == "darwin" {
			fmt.Printf("removing lo0 alias %s\n", addr)
			darwinSudoCommand(" to remove loopback alias", []string{"ifconfig", "lo0", "-alias", addr})
		}

		if runtime.GOOS == "windows" {
			fmt.Printf("removing loopback address %s\n", addr)
			cmd := exec.Command("netsh", "interface", "ipv4", "delete", "address", "Microsoft Loopback Adapter", addr)
			err := cmd.Run()
			if err != nil {
				log.Println("Failed to remove loopback address:", err)
			}
		}
	}
}
