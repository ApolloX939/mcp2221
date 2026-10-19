# mcp2221

A Go library for the Microchip **MCP2221 / MCP2221A** USB-to-I2C bridge
(for example the [Adafruit MCP2221A breakout](https://www.adafruit.com/product/4471)).

- Pure Go with no cgo, so no C compiler is needed. Works on Linux, Windows and macOS.
- Talks to the chip with raw HID reports, using the same protocol steps as Adafruit Blinka.
- Covers I2C write, read, write-then-read and bus scan. Safe for concurrent use.

GPIO, ADC/DAC and UART are not implemented.

The [`ft232h`](ft232h) package drives the FTDI **FT232H** (for example the
[Adafruit FT232H breakout](https://www.adafruit.com/product/2264)) with the same
I2C methods. It is pure Go too, but Linux only. See [FT232H](#ft232h).

Also in this repo:

| Path | What it is |
|------|------------|
| [`ft232h/`](ft232h) | I2C through an FTDI FT232H (Linux only) |
| [`ht16k33/`](ht16k33) | Driver for the Adafruit bicolor 8x8 LED matrix (HT16K33 backpack) |
| [`cmd/claudematrix/`](cmd/claudematrix) | A "Claude is working" animation for that matrix |
| [`led-matrix/`](led-matrix) | Claude Code plugin that runs `claudematrix` while Claude works |
| [`progress-pie/`](progress-pie) | Claude Code plugin that shows a multi-part job's progress as a pie |

## Install

```sh
go get github.com/ApolloX939/mcp2221
```

## Usage

```go
import mcp2221 "github.com/ApolloX939/mcp2221"

dev, err := mcp2221.Open() // also sets the bus to 100 kHz
if err != nil {
	log.Fatal(err)
}
defer dev.Close()

addrs, _ := dev.Scan()                          // e.g. [0x70]
err = dev.I2CWrite(0x70, []byte{0x21})          // write
err = dev.I2CRead(0x70, buf)                    // read len(buf) bytes
err = dev.I2CTx(0x70, []byte{0x00}, buf)        // write register, then read
err = dev.SetI2CSpeed(400_000)                  // 47 kHz to 400 kHz

if errors.Is(err, mcp2221.ErrNACK) { /* nothing at that address */ }
```

`Open` expects exactly one MCP2221 to be connected. The adapter is held
exclusively until `Close`; opening it while another program has it fails with
an error wrapping `usbhid.ErrDeviceLocked`.

### LED matrix (ht16k33)

```go
import "github.com/ApolloX939/mcp2221/ht16k33"

grid, err := ht16k33.NewMatrix8x8x2(dev, 0x70, 0.1) // any I2CWrite-er, brightness 0-1
grid.Set(x, y, ht16k33.Green)                        // Off, Red, Green, Orange
grid.Fill(ht16k33.Off)
err = grid.Show()                                    // send the buffered pixels
```

### Notes

- `I2CTx` sends the write and the read as two transfers (stop, then start), not
  with a repeated start. The MCP2221's repeated-start read returned corrupted
  data in testing. Nearly all devices keep their register pointer between
  transfers, so this works as expected.
- `Scan` probes 0x08–0x77 with a one-byte read.
- Transfers are up to 65535 bytes each and time out after 250 ms.

### FT232H

```go
import "github.com/ApolloX939/mcp2221/ft232h"

dev, err := ft232h.Open() // also sets the bus to 100 kHz
defer dev.Close()
// Scan, I2CWrite, I2CRead, I2CTx and SetI2CSpeed (1 kHz to 1 MHz) work as above.
if errors.Is(err, ft232h.ErrNACK) { /* nothing at that address */ }
```

- **Wiring:** D0 is SCL, and D1 and D2 together are SDA. On the Adafruit board,
  turn the I2C switch on (it ties D1 to D2). Older boards without the switch
  need a wire from D1 to D2.
- The FT232H isn't a HID device, so the package uses Linux's raw USB device
  files (usbfs). On Windows and macOS, `Open` returns an error.
- While the FT232H is open, its serial port (`/dev/ttyUSB*`) disappears.
  `Close` brings it back.
- `I2CTx` uses a repeated start between the write and the read. The HT16K33
  matrix returns scrambled data for a multi-byte read after a repeated start
  (single bytes are fine), so read it with `I2CWrite` then `I2CRead`.
- Also needs a udev rule; see [Linux permissions](#linux-permissions).

## claudematrix: a "Claude is working" light

`claudematrix` plays Claude Code's spinner on the LED matrix until it is stopped,
then blanks the display: an orange dot that grows into a star (`·` `✢` `✳` `✶`
`✻` `✽`) and shrinks back again. The `led-matrix` Claude Code plugin runs it
while Claude is working on a prompt and stops it when Claude finishes or is
interrupted.

What the matrix shows, one frame every 120ms (`●` is a lit orange LED). It plays
0 to 5 and back down to 0, then repeats:

```
frame 0     frame 1     frame 2     frame 3     frame 4     frame 5
········    ········    ········    ········    ···●●···    ●··●●··●
········    ········    ···●●···    ·●·●●·●·    ·●·●●·●·    ·●·●●·●·
········    ···●●···    ···●●···    ··●●●●··    ··●●●●··    ··●●●●··
···●●···    ··●●●●··    ·●●●●●●·    ·●●●●●●·    ●●●●●●●●    ●●●●●●●●
···●●···    ··●●●●··    ·●●●●●●·    ·●●●●●●·    ●●●●●●●●    ●●●●●●●●
········    ···●●···    ···●●···    ··●●●●··    ··●●●●··    ··●●●●··
········    ········    ···●●···    ·●·●●·●·    ·●·●●·●·    ·●·●●·●·
········    ········    ········    ········    ···●●···    ●··●●··●
```

See it running on the matrix: [claude-ing.mp4](claude-ing.mp4) (silent video).

It uses an MCP2221 if one is plugged in, and otherwise an FT232H.

### Install

**1. The program.**

```sh
go install github.com/ApolloX939/mcp2221/cmd/claudematrix@latest
```

Or, from a checkout of this repo:

```sh
go install ./cmd/claudematrix
```

Either way it goes to `~/go/bin/claudematrix` (or `$GOBIN`). On Linux, also
add the udev rule from [Linux permissions](#linux-permissions).

**2. The Claude Code plugin**, for every session. This repo is also a plugin
marketplace. From a checkout:

```sh
claude plugin marketplace add /path/to/mcp2221
claude plugin install led-matrix@mcp2221 --scope user
```

Installed from a local folder like this, Claude Code reads the plugin straight
from the checkout. After you edit it, run `/reload-plugins` in a session.

Without a checkout, add the marketplace from git instead:
`claude plugin marketplace add ApolloX939/mcp2221`.

Then start a new session, or run `/reload-plugins` in an open one.

**3. Settings** (optional). In Claude Code, run `/plugin configure led-matrix@mcp2221`:

| Setting | Default | Description |
|---------|---------|-------------|
| `binary` | `/home/apollo/go/bin/claudematrix` | Path to `claudematrix`. Change it if yours is elsewhere. |
| `brightness` | `0.1` | Matrix brightness, 0.0 to 1.0 |
| `limitFile` | `/home/apollo/.claude/usage-limit` | Where the usage limit's reset time is written. `claudematrix` reads `~/.claude/usage-limit` unless given `-limit`. |

### How it behaves

- **A turn starts:** the plugin starts `claudematrix`. Subagents don't start
  it again; they run inside Claude's turn.
- **The turn ends, or you press Esc:** the plugin stops it and the display
  blanks within about half a second.
- **The adapter is unplugged, or another program has it open** (such as
  another Claude session that is working): `claudematrix` keeps
  retrying quietly. Its errors go to Claude Code's debug log (`claude --debug`).
- **Claude Code exits or crashes:** `claudematrix` notices within a second,
  blanks the display and exits.
- **A usage limit is hit** (a window such as the five-hour session limit at
  100%): between turns the matrix counts down to when the limit resets, then
  blanks. See [Usage-limit countdown](#usage-limit-countdown).

### Usage-limit countdown

Claude Code reports how much of each usage window is used and when it resets.
When one reaches 100%, the plugin writes the reset time to
`~/.claude/usage-limit` (unix seconds; `0` when nothing is limited), and
between turns runs `claudematrix -countdown`, which shows the time left and
exits at the reset. Starting a new prompt hands the matrix back to the working
light.

The big number's colour gives its unit, and a dot at its lower right blinks
once a second:

- **Under 5 hours:** an orange dot along the top for each whole hour, and the
  minutes in red. 3 hours 42 minutes (`O` orange, `R` red):

```
O·O·O···
········
R·R·RRR·
R·R···R·
RRR·RRR·
··R·R···
··R·RRRR
········
```

- **5 to 99 hours:** the hours in orange (a single digit is centred).
- **100 hours or more** (the weekly limit): the days in green (a single digit
  is centred).

The progress pie's display shows the countdown too, ahead of the pie, so the
two plugins never fight over what the matrix shows.

### Running it by hand

```sh
claudematrix              # until Ctrl+C
```

| Flag | Default | Description |
|------|---------|-------------|
| `-brightness` | `0.1` | Display brightness, 0.0 to 1.0 |
| `-addr` | `0x70` | I2C address of the matrix |
| `-frame` | `120ms` | Time per animation step |
| `-heartbeat` | off | Print a line this often, and stop if output is closed |
| `-progress` | `~/.claude/progress-pie` | Progress file to draw a pie from (see [progress-pie](#progress-pie-a-progress-pie-for-multi-part-jobs)); `""` for no pie |
| `-limit` | `~/.claude/usage-limit` | Usage-limit file to count down from (see [Usage-limit countdown](#usage-limit-countdown)); `""` for no countdown |
| `-countdown` | off | Show only the countdown, and exit when no limit is hit |

It blanks the display and exits on SIGTERM or Ctrl+C, when the program that
started it exits, or, with `-heartbeat`, when nothing reads its output any
more. While Claude works, the `led-matrix` plugin runs it with `-progress ""
-limit ""`, so the working light always wins over a pie or a countdown, and with `-heartbeat 500ms` because
Claude Code only stops a child after the child's next line of output.

## progress-pie: a progress pie for multi-part jobs

When Claude works on a request with several parts, the `progress-pie` plugin
shows how far along it is as a pie, filled clockwise from 12 o'clock: empty at
the start, half filled at 50%, full when done.

- **Status line:** `◔ 1/4 running tests` (`○ ◔ ◑ ◕ ●`).
- **LED matrix:** between turns, while background work runs, `claudematrix`
  draws the pie (while Claude works, the matrix keeps the working light). Green
  is finished, the blinking orange wedge is the part being worked on, and the
  red rim outlines what is left. At 2 of 4 (`G` green, `O` orange, `R` red):

```
··RRGG··
·R··GGG·
R···GGGG
R···GGGG
OOOOGGGG
OOOOGGGG
·OOOGGG·
··OOGG··
```

Progress comes from a `progress` tool the plugin gives Claude (Claude is told
to report each part as it finishes), or from Claude's task list when it keeps
one. The plugin writes it to `~/.claude/progress-pie` as `done total
unix-seconds`, and `claudematrix` checks that file every frame.

### How it behaves

- **While Claude works:** the working light wins: the `led-matrix` plugin keeps
  the spinner on the matrix, and the pie is in the status line only.
- **Between turns, with the job unfinished** (Claude is waiting on background
  work): this plugin runs `claudematrix` itself to show the pie. When the
  background work wakes Claude, the matrix is handed back to `led-matrix`.
- **The job finishes, or you press Esc:** the pie is cleared when the turn ends.
- A progress file older than 6 hours is ignored, so an abandoned job doesn't
  leave a pie up forever.

### Install

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
| `progressFile` | `/home/apollo/.claude/progress-pie` | Where progress is written. `claudematrix` reads `~/.claude/progress-pie` unless given `-progress`. |

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

## Tested

Tested on Linux with an MCP2221A and an HT16K33 LED matrix backpack. Library:
- Bus scan.
- Write, then read back.
- No-device errors on writes and reads.
- Recovery after errors.
- 100 kHz and 400 kHz.

FT232H, on Linux with an Adafruit FT232H breakout and the same matrix:
- Bus scan at 100 kHz and 400 kHz.
- Write, then read back (one byte, and many bytes without a repeated start).
- No-device errors.
- A second program opening the adapter gets `ErrInUse`.
- The serial port comes back after `Close`.
- claudematrix runs and stops cleanly.

claudematrix and the plugin:
- Animation start and stop in a live session and a fresh headless session.
- Blanking on SIGTERM, on parent exit, and on closed output.
- Waiting while another program holds the adapter.
- `claude plugin test led-matrix` passes.

## License

MIT. See [LICENSE](LICENSE).
