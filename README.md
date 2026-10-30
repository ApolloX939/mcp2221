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
| [`shtc3/`](shtc3) | Driver for the Adafruit SHTC3 temperature and humidity sensor |
| [`sht41/`](sht41) | Driver for the Adafruit Sensirion SHT41 temperature and humidity sensor |
| [`is31fl3741/`](is31fl3741) | Driver for the Adafruit 13x9 PWM RGB LED matrix (IS31FL3741) |
| [`tmf8806/`](tmf8806) | Driver for the Adafruit TMF8806 time-of-flight distance sensor |
| [`sh1106/`](sh1106) | Driver for the Adafruit monochrome 1.3" 128x64 OLED (SH1106) |
| [`nunchuk/`](nunchuk) | Driver for a Wii Nunchuk through the Adafruit Nunchuck Breakout Adapter |
| [`ht16k33/`](ht16k33) | Driver for the Adafruit bicolor 8x8 LED matrix (HT16K33 backpack) |
| [`cmd/claudematrix/`](cmd/claudematrix) | A "Claude is working" animation for that matrix |
| [`led-matrix/`](led-matrix) | Claude Code plugin that runs `claudematrix` while Claude works |
| [`progress-pie/`](progress-pie) | Claude Code plugin that shows a multi-part job's progress as a pie |

## Contents

- [Install](#install)
- [Usage](#usage)
  - [LED matrix (ht16k33)](#led-matrix-ht16k33)
  - [Temperature and humidity (shtc3)](#temperature-and-humidity-shtc3)
  - [Temperature and humidity (sht41)](#temperature-and-humidity-sht41)
  - [RGB LED matrix (is31fl3741)](#rgb-led-matrix-is31fl3741)
  - [Distance (tmf8806)](#distance-tmf8806)
  - [OLED display (sh1106)](#oled-display-sh1106)
  - [Wii Nunchuk (nunchuk)](#wii-nunchuk-nunchuk)
  - [I2C addresses](#i2c-addresses)
  - [Notes](#notes)
  - [FT232H](#ft232h)
- [claudematrix: a "Claude is working" light](#claudematrix-a-claude-is-working-light)
  - [Install](#install-1)
  - [How it behaves](#how-it-behaves)
  - [Usage-limit countdown](#usage-limit-countdown)
  - [Running it by hand](#running-it-by-hand)
- [progress-pie: a progress pie for multi-part jobs](#progress-pie-a-progress-pie-for-multi-part-jobs)
  - [How it behaves](#how-it-behaves-1)
  - [Install](#install-2)
- [Tested](#tested)
- [License](#license)

## Install

See [INSTALL.md](INSTALL.md) for the library, `claudematrix`, the Claude Code
plugins and the Linux permissions, [UPDATING.md](UPDATING.md) to move to a newer
release, and [UNINSTALL.md](UNINSTALL.md) to remove them. The library:

```sh
go get github.com/ApolloX939/mcp2221
```

The LED matrix in Claude Code takes three commands: see the
[quick start](INSTALL.md#quick-start-the-led-matrix-in-claude-code).

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

### Temperature and humidity (shtc3)

```go
import "github.com/ApolloX939/mcp2221/shtc3"

sensor, err := shtc3.New(dev)  // any adapter; checks the sensor's ID
r, err := sensor.Read()        // about 13 ms; the sensor sleeps between reads
fmt.Printf("%.1f °C (%.1f °F), %.1f %%RH\n", r.Celsius, r.Fahrenheit(), r.Humidity)
sensor.LowPower = true         // about 1 ms per read, less precise
```

The sensor sleeps between reads, and `Scan` doesn't find it while it is
asleep. `shtc3.New` is the way to check that it is there.

The SHTC3's address is fixed at 0x70, the same as the LED matrix's default,
so the two clash on one bus. See [I2C addresses](#i2c-addresses).

### Temperature and humidity (sht41)

```go
import "github.com/ApolloX939/mcp2221/sht41"

sensor, err := sht41.New(dev)  // any adapter; resets and checks the serial number
r, err := sensor.Read()        // about 9 ms
fmt.Printf("%.1f °C (%.1f °F), %.1f %%RH\n", r.Celsius, r.Fahrenheit(), r.Humidity)
sensor.Precision = sht41.Low   // about 2 ms per read, less precise
r, err = sensor.Heat(sht41.Heater20mW1s) // heater pulse to clear condensation
```

The SHT41's address is fixed at 0x44. It doesn't sleep between reads, so
`Scan` finds it. The heater warms the sensor, so let it cool before trusting
later readings.

### RGB LED matrix (is31fl3741)

```go
import "github.com/ApolloX939/mcp2221/is31fl3741"

m, err := is31fl3741.New(dev, is31fl3741.DefaultAddr, 0.1) // checks the chip; brightness 0.0-1.0
m.Set(0, 0, 255, 0, 0)  // pixel (x, y) to r, g, b; (0, 0) is the top left
m.Fill(0, 0, 16)        // every pixel
err = m.Show()          // send the buffered pixels
m.SetBrightness(0.5)    // takes effect at once
m.Shutdown()            // display off, pixels kept; m.Wake() brings them back
```

The board is 13 pixels wide and 9 tall (`is31fl3741.Width`, `Height`). It is
very bright: even 0.1 is a lot at close range, so start low. `Set` replaces a
pixel's whole color. The board's wiring is irregular, and the driver follows
the mapping in Adafruit's own library.

The address is 0x30 by default, and 0x30-0x33 with the board's ADDR pin.

### Distance (tmf8806)

```go
import "github.com/ApolloX939/mcp2221/tmf8806"

sensor, err := tmf8806.New(dev)       // checks the chip and starts it
mm, ok, err := sensor.Distance()      // one measurement; ok is false if nothing reliable was seen

sensor.Mode = tmf8806.Range5m         // ShortRange (~200 mm), Range2_5m (default) or Range5m
err = sensor.Start(true)              // measure continuously, every sensor.Period
r, err := sensor.Read(time.Second)    // r.Distance in mm, r.Reliability 0-63, r.Temperature in °C
err = sensor.Stop()
```

The address is fixed at 0x41. Not supported: the 10 m mode (it needs a RAM
firmware patch), histograms, calibration and the GPIO functions.

### OLED display (sh1106)

```go
import "github.com/ApolloX939/mcp2221/sh1106"

d, err := sh1106.New(dev, sh1106.DefaultAddr) // turns the display on, blank
d.Set(10, 5, true)      // pixel (x, y) on or off; (0, 0) is the top left, 128x64
d.Fill(false)           // every pixel
d.DrawText(0, 0, "Hello!", 2, true) // text at (x, y), scale 2 is 10x14 pixels per letter
w := sh1106.TextWidth("Hello!", 2)  // width in pixels, to center or right-align it
err = d.Show()          // send the buffered pixels
d.SetContrast(0x40)     // brightness, 0-255
d.SetRotation(180)      // for a board mounted upside down
d.SetInvert(true)       // swap lit and unlit pixels
```

The address is 0x3D as shipped, and 0x3C with the board's address jumper
changed. The controller has no ID register, so `New` can only check that the
display answers.

`DrawText` uses a built-in 5x7 font for printable ASCII; other characters
come out as `?`. Each letter takes 6x8 pixels at scale 1 (`CharWidth`,
`CharHeight`), so a line holds 21 of them, and `\n` starts a new line. Text
that runs off the display is clipped. Pass `false` to draw unlit pixels,
which cuts text out of a filled area. The letters `g`, `j`, `p`, `q` and `y`
have no descenders in this font.

### Wii Nunchuk (nunchuk)

```go
import "github.com/ApolloX939/mcp2221/nunchuk"

n, err := nunchuk.New(dev)   // wakes the controller and checks it answers
s, err := n.Read()           // about 3 ms
fmt.Println(s.JoyX, s.JoyY)  // joystick, 0-255 with the center near 128
fmt.Println(s.C, s.Z)        // buttons, true while pressed
fmt.Println(s.AccelX, s.AccelY, s.AccelZ) // 10-bit accelerometer, 0-1023
x, y := s.Stick()            // joystick as -1 to 1, up is positive
```

`New` returns `nunchuk.ErrNoData` if the controller is unplugged: it answers
with all 0xFF. The address is fixed at 0x52. Joysticks differ, so the raw
range and center vary a little from one controller to the next.

### I2C addresses

Every device on an I2C bus needs its own 7-bit address. Two devices at the same
address answer together and corrupt each other's data, with no error to warn you.

| Device | Default | Can it change? |
|--------|---------|----------------|
| [HT16K33 LED matrix backpack](https://www.adafruit.com/product/902) | 0x70 | Yes, 0x70-0x77, with solder jumpers |
| [SHTC3 sensor](https://www.adafruit.com/product/4636) | 0x70 | No, always 0x70 |
| [SHT41 sensor](https://www.adafruit.com/product/5776) | 0x44 | No, always 0x44 |
| [1.3" 128x64 OLED](https://www.adafruit.com/product/938) | 0x3D | Yes, 0x3C, with the address jumper |
| [Wii Nunchuk adapter](https://www.adafruit.com/product/4836) | 0x52 | No, always 0x52 |
| [TMF8806 distance sensor](https://www.adafruit.com/product/6523) | 0x41 | No, always 0x41 |
| [IS31FL3741 13x9 RGB matrix](https://www.adafruit.com/product/5201) | 0x30 | Yes, 0x30-0x33, with the ADDR pin |

The MCP2221 and FT232H are the bus itself, not devices on it, so they have no
address.

**Using the matrix and the SHTC3 together:** leave the sensor at 0x70 and move
the matrix. The backpack has three solder jumpers on the back, A0, A1 and A2.
Bridging a jumper with a blob of solder adds 1, 2 or 4 to the address:

| Bridged | Address |
|---------|---------|
| none | 0x70 |
| A0 | 0x71 |
| A1 | 0x72 |
| A0 + A1 | 0x73 |
| A2 | 0x74 |
| A0 + A2 | 0x75 |
| A1 + A2 | 0x76 |
| A0 + A1 + A2 | 0x77 |

Adafruit's guide has photos: [Changing I2C address](https://learn.adafruit.com/adafruit-led-backpack/changing-i2c-address).
To undo it, remove the solder with a solder sucker or wick.

After moving the matrix, use its new address everywhere:

- In Go: `ht16k33.NewMatrix8x8x2(dev, 0x71, 0.1)`.
- By hand: `claudematrix -addr 0x71`.
- In the `led-matrix` and `progress-pie` plugins: set their `addr` option, with
  `/plugin configure led-matrix@mcp2221` in Claude Code, or from a shell:

  ```sh
  echo '{"addr":"0x71"}' | claude plugin configure led-matrix@mcp2221 --values-stdin
  echo '{"addr":"0x71"}' | claude plugin configure progress-pie@mcp2221 --values-stdin
  ```

To check what is on the bus, run `Scan`. It lists the matrix, but not an SHTC3,
which ignores the probe while it sleeps between readings. `shtc3.New` checks
for the sensor instead.

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
- Also needs a udev rule; see [Linux permissions](INSTALL.md#linux-permissions).

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

See [INSTALL.md](INSTALL.md#quick-start-the-led-matrix-in-claude-code) for the program,
the plugin and its settings.

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

See [INSTALL.md](INSTALL.md#progress-pie-plugin) for the plugin and its settings.

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

SHTC3, on the FT232H:
- ID check, then normal and low-power readings with valid checksums.

claudematrix and the plugin:
- Animation start and stop in a live session and a fresh headless session.
- Blanking on SIGTERM, on parent exit, and on closed output.
- Waiting while another program holds the adapter.
- `claude plugin test led-matrix` passes.

## License

MIT. See [LICENSE](LICENSE).
