// Package ht16k33 drives an Adafruit bicolor 8x8 LED matrix (HT16K33 backpack).
package ht16k33

import (
	"fmt"
	"math"
)

// Colors for the bicolor 8x8 matrix.
const (
	Off    byte = 0
	Red    byte = 1
	Green  byte = 2
	Orange byte = 3
)

const (
	htOscillatorOn = 0x21
	htDisplayOn    = 0x81 // display on, blink off
	htBrightness   = 0xE0
)

// I2CWriter is an I2C bus the matrix can be written through,
// such as an *mcp2221.Device.
type I2CWriter interface {
	I2CWrite(addr byte, data []byte) error
}

// Matrix8x8x2 drives an Adafruit bicolor 8x8 LED matrix (HT16K33 backpack).
// Pixel changes are buffered until Show is called.
type Matrix8x8x2 struct {
	bus  I2CWriter
	addr byte
	buf  [16]byte
	rot  int  // quarter turns clockwise
	flip bool // mirror left to right, before the rotation
}

// NewMatrix8x8x2 sets up the display at addr with brightness from 0.0 to 1.0.
func NewMatrix8x8x2(bus I2CWriter, addr byte, brightness float64) (*Matrix8x8x2, error) {
	m := &Matrix8x8x2{bus: bus, addr: addr}
	level := byte(math.Round(15 * math.Max(0, math.Min(1, brightness))))
	for _, cmd := range []byte{htOscillatorOn, htDisplayOn, htBrightness | level} {
		if err := bus.I2CWrite(addr, []byte{cmd}); err != nil {
			return nil, err
		}
	}
	return m, m.Show()
}

// SetRotation turns what is drawn clockwise by degrees, for a matrix mounted
// sideways or upside down. degrees must be 0, 90, 180 or 270. It applies to
// pixels set afterwards.
func (m *Matrix8x8x2) SetRotation(degrees int) error {
	if degrees < 0 || degrees > 270 || degrees%90 != 0 {
		return fmt.Errorf("ht16k33: rotation must be 0, 90, 180 or 270 degrees, not %d", degrees)
	}
	m.rot = degrees / 90
	return nil
}

// SetMirror mirrors what is drawn left to right, for a matrix that shows
// everything backwards. It is applied before the rotation, so left and right
// are as seen once the rotation has the display upright. It applies to pixels
// set afterwards.
func (m *Matrix8x8x2) SetMirror(on bool) { m.flip = on }

// Set colors the pixel at (x, y), mirrored and turned as set. Out-of-range pixels
// are ignored.
func (m *Matrix8x8x2) Set(x, y int, color byte) {
	if x < 0 || x > 7 || y < 0 || y > 7 {
		return
	}
	if m.flip {
		x = 7 - x
	}
	for range m.rot {
		x, y = 7-y, x
	}
	mask := byte(1) << y
	setBit(&m.buf[2*x], mask, color&Green != 0)
	setBit(&m.buf[2*x+1], mask, color&Red != 0)
}

func setBit(b *byte, mask byte, on bool) {
	if on {
		*b |= mask
	} else {
		*b &^= mask
	}
}

func (m *Matrix8x8x2) Fill(color byte) {
	for x := range 8 {
		for y := range 8 {
			m.Set(x, y, color)
		}
	}
}

// Show sends the buffered pixels to the display.
func (m *Matrix8x8x2) Show() error {
	return m.bus.I2CWrite(m.addr, append([]byte{0x00}, m.buf[:]...))
}
