# rdevcon

<!--- Use "mdtoc" to refresh table of contents -->
<!---toc start-->

* [rdevcon](#rdevcon)
  * [Overview](#overview)
  * [Design](#design)
    * [Hub and devices](#hub-and-devices)
    * [Hub and workstations](#hub-and-workstations)
    * [Workstations and devices](#workstations-and-devices)
  * [Usage and features](#usage-and-features)
    * [The command prompt](#the-command-prompt)
    * [Configuration files](#configuration-files)
      * [config.json](#configjson)
      * [Device database](#device-database)
    * [TCP port forwarding](#tcp-port-forwarding)
    * [AWS environment variable forwarding](#aws-environment-variable-forwarding)
    * [Git credential forwarding](#git-credential-forwarding)
    * [Sshfs mounts](#sshfs-mounts)
  * [Hub server setup](#hub-server-setup)
  * [Building](#building)
  * [Notes](#notes)
    * [SSH host key options](#ssh-host-key-options)
    * [S3 resources](#s3-resources)
    * [Securing the deployment](#securing-the-deployment)

<!---toc end-->

## Overview

`rdevcon` automates making ssh connections from workstations to a
fleet of similarly configured devices via a central hub server.  The
name is simply derived from **r**emote **dev**ice **con**nections.

`rdevcon` tries to extend certain elements of a developer's local
workstation environment to remote devices, in a convenient and
transparent manner.

Features include,

  * Device inventory stored in a JSON file, either compiled-in or external.
  * Connections run in platform-native command windows (xterm, Terminal, cmd.exe).
  * TCP port forwarding.
  * AWS environment variable forwarding.
  * Git credential forwarding.
  * Sshfs mounts.
  * Builds for Linux, macOS, and Windows.
  * Compiled-in configuration facilitating distribution as a single binary (per platform), in conjunction with resources on S3.
  * Automatic in-place updates.

`rdevcon` has a simple interactive command prompt interface. A typical session
can be summarized like this,

```
$ rdevcon
> 123
[ a new terminal appears, with a login prompt for device 123]
```

## Design

There are 2+N ssh connections involved in connecting to a device,
as illustrated in the following sections.


### Hub and devices

There is a central server or hub where all the connections meet.

Every device in the fleet makes an ssh connection to the hub, with a
unique reverse port to its local sshd port,

```
                           ssh device@hub -R22001:localhost:22
┌─────────────┐         ┌───────────┐         ┌───────────┐
│             │         │         22│<--------│.          │
│             │         │    22001-'│         │ `->22     │
│ workstation │         │    hub    │         │   device  │
└─────────────┘         └───────────┘         └───────────┘

```

Setting up these connections is outside the purview of `rdevcon`, and
can be automated however you like. For instance,

  * [autossh](https://github.com/Autossh/autossh) running as a service
  * A script that monitors a URL and starts the ssh connection on-demand

A useful convention is to allocate the reverse ports using an offset
into a range of rarely used ports, with the offset serving as a
shorthand identifier for a device.  For example 22000..22999
provides a range of 1000 ports allowing for 1000 devices in a
deployment, and in the example above device `1` would connect to the
hub with reverse port 22001.


### Hub and workstations

Workstations (desktop, laptop, VM, etc.) make one "tunnel" connection
to the server for each device they want to connect to. By convention,
the same reverse port that the device uses to connect to the server is
also used as a local forward port on the workstation, like this,

```
    ssh support@hub -L22001:localhost:22001
┌─────────────┐         ┌───────────┐         ┌───────────┐
│            .│-------->│22       22│<--------│.          │
│     22001-' │         │    22001-'│         │ `->22     │
│ workstation │         │    hub    │         │   device  │
└─────────────┘         └───────────┘         └───────────┘
```
`rdevcon` automatically creates these "tunnel" connections as needed.

### Workstations and devices

Once a tunnel connection is established, the workstation can make any
number of connections to the device via the forwarded localhost port,

```
┌─────────────┐         ┌───────────┐         ┌───────────┐
│            .│-------->│22       22│<--------│.          │
│     22001-' │         │    22001-'│         │ `->22     │
│ workstation │-------- │    hub    │ ------->│   device  │
│             │-------- │           │ ------->│           │
│             │   ...   │           │   ...   │           │
└─────────────┘         └───────────┘         └───────────┘
                 ssh -p 22001 user@localhost
                 sftp -P 22001 user@localhost
                            ...

```
Port forwardings to the device can be added at this level, for example to reach
a web server and VNC server on device 1,

```
ssh -p 22001 -L8080:localhost:80 -L5999:localhost:5900 user@localhost
```

It is worth emphasizing that the hub server does not need to know
about any of these additional forwardings. All it does is
make device sshd ports available to connected workstations.


## Usage and features

### The command prompt

`rdevcon` has a simple command prompt interface. At startup, the set
of available devices is presented, followed by a list of available
commands. These can be repeated at any time with `list` and `help`
respectively.

```
Available devices:
serial, port, location, (comment)
LAB-00000001,   1, Device 1, ()
LAB-00000010,  10, Device 10, ()
LAB-00000123, 123, Device 123, ()

Commands:
LAB-00000123 - connect to device with serial LAB-00000123
123 - connect to device with port offset 123
123~ - sshfs mount device with port 123 at `$HOME/sshfs/LAB-00000123/`
22123! - connect to device with tunnel port 22123 (for unlisted devices)
list - list devices
help - this help
exit - exit program
exit!- exit program even if clean exit conditions aren't met (also ctrl-d or ctrl-z)
>
```

If everything is configured properly, and device LAB-00000123 is up
and connected to the hub server, then typing `123` should launch a
connection to the device. Suggested commands for using `sftp` and
`ssh-copy-id` are also printed out.

```
> 123
For file transfers to device LAB-00000123:
sftp -o StrictHostKeychecking=no -o UpdateHostKeys=no -P 22123 user@localhost

To install your default pubkey on device LAB-00000123:
ssh-copy-id -o StrictHostKeychecking=no -o UpdateHostKeys=no -p 22123 user@localhost

```

Each connection is launched in a separate window on the workstation,
using a platform-native terminal emulator or console.

  * Windows: cmd.exe
  * Linux: xterm if installed, or gnome-terminal as a fallback
  * MacOS: Iterm2 if installed, or Terminal as a fallback

Exiting `rdevcon` should kill all active tunnels and connections.


### Configuration files

Two files are compiled into `rdevcon` at build time using `//go@embed`.

#### config.json

A file `config.json` must be created before building `rdevcon`. It should be a single
depth JSON object containing the following keys,

  * DevicesPath: Path to a JSON file containing your device inventory, as described below. DevicesPath can be an S3 URL.
  * TunnelKeyPath: Path to a keyfile for workstation-to-hub ssh connections. TunnelKeyPath can be an S3 URL.
  * TunnelNameAddr: `user@host` login for workstation-to-hub ssh connections, like `support@hub.example.com`
  * DeviceNameAddr: `user@host` login for workstation-to-device ssh connections, like `user@localhost`
  * SelfUpdatePath: Path to check for updated binaries. If present, the substring `$platform` is replaced with the runtime value of `runtime.GOOS+"-"+runtime.GOARCH`, for example `linux-amd64`. Similarly, the substring `$argv0` is replaced with the basename of the path returned by [os.Executable()](https://pkg.go.dev/os#Executable). SelfUpdatePath can be an S3 URL.
  * PortBase: Integer value added to device port offset to calculate actual port number for device connections.
  * CommonForwards: Common `-L` and `-R` ssh forwarding specifications.

#### Device database

`devices.json` should exist at build time, and it shares the same syntax as the "DevicesPath" value in the config file.

The format must be a JSON array of objects with these attributes,

  * serial: string, usually a alphanumeric serial number identifying the device
  * id: string representing an integer offset, which is added to the PortBase value in the config file. The id is used to launch connections.
  * allocation: string representing location or grouping of the device
  * notes: string with any special notes about the device
  * hidden: true/false indicator of whether the device should be listed by default. Use `unlock-hidden` to show hidden devices. This is only intended a a "speed bump" for accessing more important devices. Further layers of security should be implemented using keys or password.

Additional attributes may be present, but will be ignored.


### TCP port forwarding

A string containing space-separated `-L` and `-R` forward specifications can be set in
the config file key "CommonForwards". The first successful connection will use these
forwardings for the duration of that connection. When that connection
exits, they will be available for use by the next connection
made. Other connections can be made in the meantime, but the common
forwardings will not be set.

The intent of this feature is to allow transparent access to device-local
services which may appear on any device in the fleet. To keep things simple,
forwardings are only set for one device at a time, rather than trying to
remap different port ranges to accommodate multiple devices.


### AWS environment variable forwarding

`rdevcon` checks for AWS credential environment variables and sets
them in the ssh session. These include: `AWS_SECRET_ACCESS_KEY`,
`AWS_ACCESS_KEY_ID`, and `AWS_SESSION_TOKEN`.

This extends access to `aws s3` and other commands from the
developer's local system to the remote device.

### Git credential forwarding

If you use ssh credentials for access to Github (or any git hosting
service), and they are present in your ssh-agent keys, they will be
forwarded to the device by means of the ssh `-A` option.

Additionally, `rdevcon` will attempt to run `git config --global -l` and parse
out the `user.email` and `user.name` values, passing them over the connection
as git environment variables.

Like AWS forwarding above, the intent of this is to extend the developer's
working environment from their workstation to the device.


### Sshfs mounts

`rdevcon` can mount a device as a network drive using [sshfs](https://github.com/libfuse/sshfs), using the use the `<port>~` syntax. This is useful for developing on the device using your IDE.

In this example, the root filesystem `/` of device 123 should be mounted at `$HOME/sshfs/LAB-00000123/`.

```
> 123~
[sshfs -o StrictHostKeychecking=no -o UpdateHostKeys=no -o port=22123 user@localhost:/ /home/user/sshfs/LAB-00000123]
```

There are some prerequisites for this to work,

* A pubkey installed on the device via `ssh-copy-id`, since it is not interactive.
* An active connection to the device.
* The `sshfs` package must be installed.
  * On linux distributions `sshfs` should be a standard package.
  * On MacOS it must be installed separately.
    * Install `sshfs` from https://osxfuse.github.io/
    * Boot your system into recovery mode and use the startup utility script to enable kernel extensions
    * Reboot and navigate to the Privacy and Security settings page. From here, under extensions, you'll see something about "allow Benjamin Fleischer". Select allow and reboot.
    * `sshfs` should now be successfully installed.
  * Windows is not supported.
    * It might be possible to set it up manually, see [this superuser post](https://superuser.com/questions/1423371/sshfs-remote-directory-mounting-syntax)
    * Sftp clients and IDEs with built-in sftp support might solve use cases.



## Hub server setup

The hub server needs to run an SSH server, with 2 special accounts. The convention used by the author is,

  * `device` for device-to-hub ssh connections
  * `support` for workstation-to-hub ssh connections

Both accounts have shell `/bin/true` so that they can't run arbitrary
commands, but port forwards still work.


## Building

`rdevcon` is written in Go and requires a reasonably recent Go toolchain.

* Create a `config.json` file, following the [config.json](#configjson) section above.
* Create a `devices.json` file, following the [Device database](#device-database) section above, or simply create an empty `devices.json` if your `config.json` specifies a path to an external file.
* Build,
    * `go get`
    * `go build`
* To cross-build for other architectures, specify `GOOS` and `GOARCH`. The following have been tested, and there is some conditional support in the code for different OS and ARCH values,
    * `env GOOS=linux GOARCH=amd64 go build -o bin/linux-amd64/rdevcon`
    * `env GOOS=windows GOARCH=amd64 go build -o bin/windows-amd64/rdevcon.exe`
    * `env GOOS=darwin GOARCH=amd64 go build -o bin/darwin-amd64/rdevcon`
    * `env GOOS=darwin GOARCH=arm64 go build -o bin/darwin-arm64/rdevcon`

For use within an organization, it might make sense to create a parent repo that includes,
a `config.json` and `devices.json` (or references one in S3), then either clone `rdevcon`
or reference it as a sub-module, copy those files in, and build for the platforms you
are interested in.


## Notes

### SSH host key options

`rdevcon` connects to different devices at `user@localhost` via
different forwarded ports. It doesn't make sense to cache the host
keys, because they'll be for different for every device, resulting in
stern warnings from ssh ("ITS POSSIBLE THAT SOMEONE IS DOING SOMETHING
NASTY"). To avoid this, the ssh connection invocations add these
options,

  * `-o StrictHostKeychecking=no`
  * `-o UpdateHostKeys=no`

There is a small tradeoff of security for convenience here.

### S3 resources

Certain config options can be S3 URIs of the form
`s3://bucket/key`. You must have the necessary permissions to access
these URIs.


### Securing the deployment

  * To participate in an `rdevcon` deployment, devices only need to create and maintain
    outbound ssh connections to the server. Everything else can be firewalled off as needed.
    Workstations will tunnel back to the device through the outbound ssh connections.
  * The server can be configured to whitelist devices and workstations.


## Licensing

`rdevcon`, including all source and data files, is licensed under the MIT License, see the
LICENSE file.
