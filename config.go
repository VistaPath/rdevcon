// Configuration for rdevcon

package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed config.json
var config_json string

type Config struct {
	DevicesPath      string
	TunnelKeyPath    string
	TunnelNameAddr   string
	SelfUpdatePath   string
	PortOffset       int
	CommonForwards   string
	SpecialPort      string
	Verbose          bool
	SshOptionList    []string
	UseLoopbackAddrs bool
}

var config *Config

func ConfigLoad() *Config {
	config = &Config{}

	// Embedded config.json is loaded first.
	err := json.Unmarshal([]byte(config_json), config)
	if err != nil {
		fmt.Println("embedded config error:", err)
	}

	// If a local config.json is found, it can override
	// any or all values
	if _, err := os.Stat("config.json"); err == nil {
		if data, err := os.ReadFile("config.json"); err == nil {
			if err := json.Unmarshal(data, config); err != nil {
				fmt.Println("local config error:", err)
			}
		} else {
			fmt.Println("local config read:", err)
		}
	}

	// Replace placeholders in SelfUpdatePath
	config.SelfUpdatePath = strings.Replace(config.SelfUpdatePath,
		"$platform",
		runtime.GOOS+"-"+runtime.GOARCH,
		1)

	executable, _ := os.Executable()
	config.SelfUpdatePath = strings.Replace(config.SelfUpdatePath,
		"$argv0",
		filepath.Base(executable),
		1)

	setLoopback(config.UseLoopbackAddrs)

	return config
}

func (config *Config) sshOptions() string {
	options := ""
	if config.Verbose {
		options += " -v"
	}

	for _, option := range config.SshOptionList {
		options += " " + option
	}

	return options
}
