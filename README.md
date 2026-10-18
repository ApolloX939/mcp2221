# mcp2221

A Go library for the Microchip **MCP2221 / MCP2221A** USB-to-I2C bridge
(for example the [Adafruit MCP2221A breakout](https://www.adafruit.com/product/4471)).

- Pure Go with no cgo, so no C compiler is needed. Works on Linux, Windows and macOS.
- Talks to the chip with raw HID reports, using the same protocol steps as Adafruit Blinka.
- Covers I2C write, read, write-then-read and bus scan. Safe for concurrent use.

GPIO, ADC/DAC and UART are not implemented.

Also in this repo:

| Path | What it is |
|------|------------|
| [`ht16k33/`](ht16k33) | Driver for the Adafruit bicolor 8x8 LED matrix (HT16K33 backpack) |
| [`cmd/claudematrix/`](cmd/claudematrix) | A "Claude is working" animation for that matrix |
| [`led-matrix/`](led-matrix) | Claude Code plugin that runs `claudematrix` while Claude works |

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

It blanks the display and exits on SIGTERM or Ctrl+C, when the program that
started it exits, or, with `-heartbeat`, when nothing reads its output any
more. The plugin uses `-heartbeat 500ms` because Claude Code only stops a
child after the child's next line of output.

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

Windows and macOS need no setup.

## Tested

Tested on Linux with an MCP2221A and an HT16K33 LED matrix backpack. Library:
- Bus scan.
- Write, then read back.
- No-device errors on writes and reads.
- Recovery after errors.
- 100 kHz and 400 kHz.

claudematrix and the plugin:
- Animation start and stop in a live session and a fresh headless session.
- Blanking on SIGTERM, on parent exit, and on closed output.
- Waiting while another program holds the adapter.
- `claude plugin test led-matrix` passes.

## License

MIT. See [LICENSE](LICENSE).
