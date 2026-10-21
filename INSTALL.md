# Installing mcp2221

How to install the library, the `claudematrix` program, the Claude Code plugins
and the Linux permissions they need. For what they do, see the [README](README.md).

## Contents

- [Library](#library)
- [Linux permissions](#linux-permissions)
- [claudematrix and the led-matrix plugin](#claudematrix-and-the-led-matrix-plugin)
  - [1. The program](#1-the-program)
  - [2. The Claude Code plugin](#2-the-claude-code-plugin)
  - [3. Settings](#3-settings)
- [progress-pie plugin](#progress-pie-plugin)

## Library

Needs Go (see `go.mod` for the version).

```sh
go get github.com/ApolloX939/mcp2221
```

## Linux permissions

By default only root can open the HID device. To use it as a normal user, create
`/etc/udev/rules.d/99-mcp2221.rules`:

```
SUBSYSTEM=="hidraw", ATTRS{idVendor}=="04d8", ATTRS{idProduct}=="00dd", MODE="0660", GROUP="plugdev", TAG+="uaccess"
```

Then run `sudo udevadm control --reload-rules`, replug the adapter, and make sure you
are in the `plugdev` group.

The rule must match `SUBSYSTEM=="hidraw"`. A rule matching `SUBSYSTEM=="usb"`
(common for libusb-based tools such as Blinka) doesn't cover the HID device this
library uses.

For the FT232H, create `/etc/udev/rules.d/99-ft232h.rules`:

```
SUBSYSTEM=="usb", ENV{DEVTYPE}=="usb_device", ATTRS{idVendor}=="0403", ATTRS{idProduct}=="6014", MODE="0660", GROUP="plugdev", TAG+="uaccess"
```

This one matches `SUBSYSTEM=="usb"`, because the FT232H is driven through its
raw USB device file.

Windows and macOS need no setup for the MCP2221.

## claudematrix and the led-matrix plugin

The program draws on an HT16K33 LED matrix; the plugin runs it while Claude works.

### 1. The program

```sh
go install github.com/ApolloX939/mcp2221/cmd/claudematrix@latest
```

Or, from a checkout of this repo:

```sh
go install ./cmd/claudematrix
```

Either way it goes to `~/go/bin/claudematrix` (or `$GOBIN`). On Linux, also
add the udev rule from [Linux permissions](#linux-permissions).

### 2. The Claude Code plugin

For every session. This repo is also a plugin marketplace. From a checkout:

```sh
claude plugin marketplace add /path/to/mcp2221
claude plugin install led-matrix@mcp2221 --scope user
```

Installed from a local folder like this, Claude Code reads the plugin straight
from the checkout. After you edit it, run `/reload-plugins` in a session.

Without a checkout, add the marketplace from git instead:
`claude plugin marketplace add ApolloX939/mcp2221`.

Then start a new session, or run `/reload-plugins` in an open one.

### 3. Settings

Optional. In Claude Code, run `/plugin configure led-matrix@mcp2221`:

| Setting | Default | Description |
|---------|---------|-------------|
| `binary` | `/home/apollo/go/bin/claudematrix` | Path to `claudematrix`. Change it if yours is elsewhere. |
| `brightness` | `0.1` | Matrix brightness, 0.0 to 1.0 |
| `addr` | `0x70` | I2C address of the matrix, 0x70 to 0x77 (see [I2C addresses](README.md#i2c-addresses)) |
| `limitFile` | `/home/apollo/.claude/usage-limit` | Where the usage limit's reset time is written. `claudematrix` reads `~/.claude/usage-limit` unless given `-limit`. |

## progress-pie plugin

Install `claudematrix` and add the marketplace as for `led-matrix` above, then:

```sh
claude plugin install progress-pie@mcp2221 --scope user
```

Settings, in `/plugin configure progress-pie@mcp2221`:

| Setting | Default | Description |
|---------|---------|-------------|
| `enabled` | `true` | Off: no progress tool, no status line entry, nothing on the matrix |
| `binary` | `/home/apollo/go/bin/claudematrix` | Path to `claudematrix`. Empty: status line only. |
| `brightness` | `0.1` | Matrix brightness, 0.0 to 1.0 |
| `addr` | `0x70` | I2C address of the matrix, 0x70 to 0x77 (see [I2C addresses](README.md#i2c-addresses)) |
| `progressFile` | `/home/apollo/.claude/progress-pie` | Where progress is written. `claudematrix` reads `~/.claude/progress-pie` unless given `-progress`. |
