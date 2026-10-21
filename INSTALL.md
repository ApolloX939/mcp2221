# Installing mcp2221

How to install the `claudematrix` program, the Claude Code plugins, the Go
library and the Linux permissions they need. For what they do, see the [README](README.md).

## Contents

- [Quick start: the LED matrix in Claude Code](#quick-start-the-led-matrix-in-claude-code)
- [Linux permissions](#linux-permissions)
- [The led-matrix plugin in detail](#the-led-matrix-plugin-in-detail)
  - [Settings](#settings)
  - [Troubleshooting](#troubleshooting)
- [progress-pie plugin](#progress-pie-plugin)
- [Go library](#go-library)

## Quick start: the LED matrix in Claude Code

Needs [Go](https://go.dev/dl/), the Claude Code CLI, and a matrix on an MCP2221
or FT232H adapter. Three commands:

```sh
go install github.com/ApolloX939/mcp2221/cmd/claudematrix@latest
claude plugin marketplace add ApolloX939/mcp2221
claude plugin install led-matrix@mcp2221 --scope user
```

Then start a new Claude Code session. The matrix animates while Claude works.

- **On Linux**, also add the udev rule in [Linux permissions](#linux-permissions),
  or only root can open the adapter.
- `claudematrix` is looked for in Go's install folder (`~/go/bin`) and then on
  `PATH`, so there is nothing to configure, and `PATH` needs no change.
- Matrix at another I2C address, other brightness: see [Settings](#settings).
- Something wrong: see [Troubleshooting](#troubleshooting).

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

## The led-matrix plugin in detail

`claudematrix` is the program that draws on the matrix, and the plugin runs it
while Claude works. The quick start installs both. This repo is also a plugin
marketplace, so the same steps work from a local checkout:

```sh
go install ./cmd/claudematrix
claude plugin marketplace add /path/to/mcp2221
claude plugin install led-matrix@mcp2221 --scope user
```

Installed from a local folder, Claude Code reads the plugin straight from the
checkout. After you edit it, run `/reload-plugins` in a session.

### Settings

Optional. In Claude Code, run `/plugin configure led-matrix@mcp2221`:

| Setting | Default | Description |
|---------|---------|-------------|
| `binary` | `claudematrix` | Name or path of the program. A bare name is looked for in `$GOBIN`, `$GOPATH/bin` and `~/go/bin`, then on `PATH`. |
| `brightness` | `0.1` | Matrix brightness, 0.0 to 1.0 |
| `addr` | `0x70` | I2C address of the matrix, 0x70 to 0x77 (see [I2C addresses](README.md#i2c-addresses)) |
| `limitFile` | empty | Where the usage limit's reset time is written. Empty: `~/.claude/usage-limit`. |

### Troubleshooting

- **The matrix stays dark:** run `claudematrix` in a terminal. A permission
  error means the udev rule is missing; "locked by another application" means
  another program has the adapter, such as another Claude session that is
  working. Plugin errors go to Claude Code's debug log (`claude --debug`).
- **`claudematrix: command not found`:** `go install` puts it in `~/go/bin`
  (or `$GOBIN`), which may not be on your `PATH`. The plugin finds it there
  anyway. To run it by hand, add `export PATH="$PATH:$HOME/go/bin"` to your
  shell's rc file.
- **`Failed to add marketplace "mcp2221": its source doesn't match`:** this
  machine already has a marketplace named `mcp2221` from another source. Use
  it as it is, or remove it first with `claude plugin marketplace remove
  mcp2221`.

## progress-pie plugin

Install `claudematrix` and add the marketplace as in the [quick start](#quick-start-the-led-matrix-in-claude-code), then:

```sh
claude plugin install progress-pie@mcp2221 --scope user
```

Settings, in `/plugin configure progress-pie@mcp2221`:

| Setting | Default | Description |
|---------|---------|-------------|
| `enabled` | `true` | Off: no progress tool, no status line entry, nothing on the matrix |
| `binary` | `claudematrix` | Name or path of the program, found as for `led-matrix`. Empty: status line only. |
| `brightness` | `0.1` | Matrix brightness, 0.0 to 1.0 |
| `addr` | `0x70` | I2C address of the matrix, 0x70 to 0x77 (see [I2C addresses](README.md#i2c-addresses)) |
| `progressFile` | empty | Where progress is written. Empty: `~/.claude/progress-pie`. |

## Go library

Needs Go (see `go.mod` for the version).

```sh
go get github.com/ApolloX939/mcp2221
```
