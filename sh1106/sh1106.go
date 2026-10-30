// Package sh1106 drives the Adafruit monochrome 1.3" 128x64 OLED display
// (product 938, an SH1106 controller) over I2C.
//
// The display's address is 0x3D by default and 0x3C with its address jumper
// changed. The controller has no ID register, so New can only check that the
// display answers.
package sh1106

import "fmt"

// Size of the display in pixels. (0, 0) is the top left.
const (
	Width  = 128
	Height = 64
)

// DefaultAddr is the display's I2C address as shipped.
const DefaultAddr = 0x3D

// I2CWriter is an I2C bus the display can be written through, such as an
// *mcp2221.Device or an *ft232h.Device.
type I2CWriter interface {
	I2CWrite(addr byte, data []byte) error
}

// Control bytes that say whether what follows is commands or pixel data.
const (
	ctrlCommand = 0x00
	ctrlData    = 0x40
)

// Commands, from the SH1106 datasheet.
const (
	cmdSetContrast   = 0x81
	cmdAllOnResume   = 0xA4
	cmdNormal        = 0xA6
	cmdInverse       = 0xA7
	cmdSegRemap      = 0xA0 // |1 flips left to right
	cmdMultiplex     = 0xA8
	cmdDCDC          = 0xAD
	cmdDisplayOff    = 0xAE
	cmdDisplayOn     = 0xAF
	cmdPageAddr      = 0xB0 // + page
	cmdComScanInc    = 0xC0
	cmdComScanDec    = 0xC8
	cmdDisplayOffset = 0xD3
	cmdClockDiv      = 0xD5
	cmdPrecharge     = 0xD9
	cmdComPins       = 0xDA
	cmdVCOM          = 0xDB
	cmdStartLine     = 0x40
	cmdMemoryMode    = 0x20
	cmdPumpVoltage   = 0x30 // | 0-3 for 6.4, 7.4, 8.0 or 9.0 V
	cmdColumnHigh    = 0x10 // | the column's high nibble
	cmdColumnLow     = 0x00 // | the column's low nibble

	// The controller has 132 columns of memory and this panel shows 128 of
	// them: the first 128 in the default orientation, the last 128 when
	// rotated, because the rotation reads the memory backwards. (Adafruit's
	// library starts at column 2, which cut off the right edge on a 938.)
	columnOffset        = 0
	columnOffsetRotated = 4
	pages               = Height / 8
)

// Display is an SH1106 OLED. Pixel changes are buffered until Show is called.
type Display struct {
	bus  I2CWriter
	addr byte
	col  byte                // first memory column shown, which depends on the rotation
	buf  [pages * Width]byte // one byte is a column of 8 pixels, least significant bit on top
}

// New sets up the display at addr and turns it on, blank.
func New(bus I2CWriter, addr byte) (*Display, error) {
	d := &Display{bus: bus, addr: addr, col: columnOffset}
	init := []byte{
		cmdDisplayOff,
		cmdClockDiv, 0x80,
		cmdMultiplex, Height - 1,
		cmdDisplayOffset, 0x00,
		cmdStartLine,
		cmdDCDC, 0x8B, // internal DC-DC on
		cmdSegRemap | 1, // the default orientation, as set by SetRotation(0)
		cmdComScanDec,
		cmdComPins, 0x12,
		cmdSetContrast, 0xFF,
		cmdPrecharge, 0x22, // Adafruit's 0x1F left this panel dark
		cmdVCOM, 0x35,
		cmdPumpVoltage | 2, // 8.0 V
		cmdNormal,
		cmdMemoryMode, 0x10,
		cmdAllOnResume,
	}
	if err := d.command(init...); err != nil {
		return nil, fmt.Errorf("sh1106: %w", err)
	}
	if err := d.Show(); err != nil {
		return nil, err
	}
	if err := d.command(cmdDisplayOn); err != nil {
		return nil, fmt.Errorf("sh1106: %w", err)
	}
	return d, nil
}

// Set turns the pixel at (x, y) on or off. Out-of-range pixels are ignored.
func (d *Display) Set(x, y int, on bool) {
	if x < 0 || x >= Width || y < 0 || y >= Height {
		return
	}
	i := y/8*Width + x
	mask := byte(1) << (y % 8)
	if on {
		d.buf[i] |= mask
	} else {
		d.buf[i] &^= mask
	}
}

// Get reports whether the pixel at (x, y) is on in the buffer.
func (d *Display) Get(x, y int) bool {
	if x < 0 || x >= Width || y < 0 || y >= Height {
		return false
	}
	return d.buf[y/8*Width+x]&(1<<(y%8)) != 0
}

// Fill turns every pixel on or off.
func (d *Display) Fill(on bool) {
	v := byte(0)
	if on {
		v = 0xFF
	}
	for i := range d.buf {
		d.buf[i] = v
	}
}

// Show sends the buffered pixels to the display.
func (d *Display) Show() error {
	for p := 0; p < pages; p++ {
		err := d.command(cmdPageAddr+byte(p),
			cmdColumnHigh|d.col>>4, cmdColumnLow|d.col&0x0F)
		if err != nil {
			return fmt.Errorf("sh1106: %w", err)
		}
		pkt := append([]byte{ctrlData}, d.buf[p*Width:(p+1)*Width]...)
		if err := d.bus.I2CWrite(d.addr, pkt); err != nil {
			return fmt.Errorf("sh1106: %w", err)
		}
	}
	return nil
}

// SetRotation turns the display by degrees, for a board mounted upside down.
// degrees must be 0 or 180. It sends the buffer again, because the controller
// only flips what is written after the change.
func (d *Display) SetRotation(degrees int) error {
	var err error
	switch degrees {
	case 0:
		d.col = columnOffset
		err = d.command(cmdSegRemap|1, cmdComScanDec)
	case 180:
		d.col = columnOffsetRotated
		err = d.command(cmdSegRemap, cmdComScanInc)
	default:
		return fmt.Errorf("sh1106: rotation must be 0 or 180 degrees, not %d", degrees)
	}
	if err != nil {
		return fmt.Errorf("sh1106: %w", err)
	}
	return d.Show()
}

// SetContrast sets the brightness from 0 to 255. It takes effect at once.
func (d *Display) SetContrast(level byte) error {
	return d.command(cmdSetContrast, level)
}

// SetInvert swaps lit and unlit pixels. It takes effect at once, without
// Show.
func (d *Display) SetInvert(on bool) error {
	if on {
		return d.command(cmdInverse)
	}
	return d.command(cmdNormal)
}

// SetPower turns the panel on or off. The buffer and what is shown are kept.
func (d *Display) SetPower(on bool) error {
	if on {
		return d.command(cmdDisplayOn)
	}
	return d.command(cmdDisplayOff)
}

func (d *Display) command(cmds ...byte) error {
	return d.bus.I2CWrite(d.addr, append([]byte{ctrlCommand}, cmds...))
}
