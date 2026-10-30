// Package is31fl3741 drives the Adafruit 13x9 PWM RGB LED matrix driver
// (IS31FL3741, product 5201) over I2C.
//
// The board's address is 0x30 by default and can be set to 0x30-0x33 with
// its ADDR pin, so it doesn't clash with an HT16K33 backpack at
// 0x70-0x77.
package is31fl3741

import (
	"fmt"
	"math"
)

// Size of the matrix in pixels. (0, 0) is the top left.
const (
	Width  = 13
	Height = 9
)

// DefaultAddr is the board's I2C address with the ADDR pin unconnected.
const DefaultAddr = 0x30

// Bus is an I2C bus the matrix can be reached through, such as an
// *mcp2221.Device or an *ft232h.Device.
type Bus interface {
	I2CWrite(addr byte, data []byte) error
	I2CRead(addr byte, buf []byte) error
}

// Registers, from the IS31FL3741 datasheet. The chip's registers sit in
// pages that are selected through a write-protected command register.
const (
	regCommand     = 0xFD
	regCommandLock = 0xFE
	unlockKey      = 0xC5

	pagePWM0    = 0 // PWM duty of channels 0-179
	pagePWM1    = 1 // PWM duty of channels 180-350
	pageScale0  = 2 // scaling of channels 0-179
	pageScale1  = 3 // scaling of channels 180-350
	pageFunc    = 4
	pwm0Size    = 180
	channels    = Width * Height * 3
	regConfig   = 0x00 // function page: bit 0 is normal operation
	regGlobal   = 0x01 // function page: global current
	regReset    = 0x3F // function page: write resetKey
	regID       = 0xFC // function page: the chip's address, shifted left
	resetKey    = 0xAE
	configOn    = 0x01
	configShut  = 0x00
	maxCurrent  = 0xFF
	scalingFull = 0xFF
)

// Matrix is an Adafruit 13x9 RGB LED matrix. Pixel changes are buffered
// until Show is called.
type Matrix struct {
	bus  Bus
	addr byte
	buf  [channels]byte
}

// New resets the chip at addr, checks that it is an IS31FL3741, sets the
// brightness from 0.0 to 1.0 and blanks the display.
func New(bus Bus, addr byte, brightness float64) (*Matrix, error) {
	m := &Matrix{bus: bus, addr: addr}
	if err := m.selectPage(pageFunc); err != nil {
		return nil, fmt.Errorf("is31fl3741: %w", err)
	}
	id, err := m.read(regID)
	if err != nil {
		return nil, fmt.Errorf("is31fl3741: read ID: %w", err)
	}
	if id != addr<<1 {
		return nil, fmt.Errorf("is31fl3741: device at 0x%02x is not an IS31FL3741 (ID 0x%02x)", addr, id)
	}
	if err := m.writeReg(regReset, resetKey); err != nil {
		return nil, fmt.Errorf("is31fl3741: reset: %w", err)
	}

	// Scaling defaults to off after a reset; brightness is set globally
	// instead, so every channel is fully scaled.
	full := make([]byte, pwm0Size)
	for i := range full {
		full[i] = scalingFull
	}
	if err := m.writePage(pageScale0, full); err != nil {
		return nil, err
	}
	if err := m.writePage(pageScale1, full[:channels-pwm0Size]); err != nil {
		return nil, err
	}
	if err := m.SetBrightness(brightness); err != nil {
		return nil, err
	}
	if err := m.selectPage(pageFunc); err != nil {
		return nil, err
	}
	if err := m.writeReg(regConfig, configOn); err != nil {
		return nil, err
	}
	return m, m.Show()
}

// SetBrightness sets the overall brightness from 0.0 to 1.0. It takes effect
// at once, without Show.
func (m *Matrix) SetBrightness(brightness float64) error {
	level := byte(math.Round(maxCurrent * math.Max(0, math.Min(1, brightness))))
	if err := m.selectPage(pageFunc); err != nil {
		return err
	}
	return m.writeReg(regGlobal, level)
}

// rowMap gives the chip's row for each row of the display.
var rowMap = [Height]int{8, 5, 4, 3, 2, 1, 0, 7, 6}

// Set colors the pixel at (x, y). Out-of-range pixels are ignored.
//
// The board is wired in an odd order, from the datasheet's register layout:
// columns 0-9 take 10 pixels per chip row and columns 10-12 are kept after
// them. The LEDs are blue, green, red, except that odd columns and the last
// column are red, green, blue shifted by one.
func (m *Matrix) Set(x, y int, r, g, b byte) {
	if x < 0 || x >= Width || y < 0 || y >= Height {
		return
	}
	row := rowMap[y]
	pixel := x + row*10
	if x >= 10 {
		pixel = x + 80 + row*3
	}
	i := pixel * 3
	if x&1 == 1 || x == Width-1 {
		m.buf[i+1], m.buf[i], m.buf[i+2] = r, g, b
	} else {
		m.buf[i+2], m.buf[i+1], m.buf[i] = r, g, b
	}
}

// Fill colors every pixel.
func (m *Matrix) Fill(r, g, b byte) {
	for y := range Height {
		for x := range Width {
			m.Set(x, y, r, g, b)
		}
	}
}

// Show sends the buffered pixels to the display.
func (m *Matrix) Show() error {
	if err := m.writePage(pagePWM0, m.buf[:pwm0Size]); err != nil {
		return err
	}
	return m.writePage(pagePWM1, m.buf[pwm0Size:])
}

// Shutdown turns the display off with software shutdown. The pixels are
// kept; Wake brings them back.
func (m *Matrix) Shutdown() error { return m.setConfig(configShut) }

// Wake leaves software shutdown.
func (m *Matrix) Wake() error { return m.setConfig(configOn) }

func (m *Matrix) setConfig(v byte) error {
	if err := m.selectPage(pageFunc); err != nil {
		return err
	}
	return m.writeReg(regConfig, v)
}

// selectPage unlocks the command register and selects a register page.
func (m *Matrix) selectPage(page byte) error {
	if err := m.bus.I2CWrite(m.addr, []byte{regCommandLock, unlockKey}); err != nil {
		return err
	}
	return m.bus.I2CWrite(m.addr, []byte{regCommand, page})
}

func (m *Matrix) writeReg(reg, v byte) error {
	if err := m.bus.I2CWrite(m.addr, []byte{reg, v}); err != nil {
		return fmt.Errorf("is31fl3741: write register 0x%02x: %w", reg, err)
	}
	return nil
}

// writePage writes data from the start of a page.
func (m *Matrix) writePage(page byte, data []byte) error {
	if err := m.selectPage(page); err != nil {
		return fmt.Errorf("is31fl3741: select page %d: %w", page, err)
	}
	if err := m.bus.I2CWrite(m.addr, append([]byte{0x00}, data...)); err != nil {
		return fmt.Errorf("is31fl3741: write page %d: %w", page, err)
	}
	return nil
}

func (m *Matrix) read(reg byte) (byte, error) {
	if err := m.bus.I2CWrite(m.addr, []byte{reg}); err != nil {
		return 0, err
	}
	var b [1]byte
	err := m.bus.I2CRead(m.addr, b[:])
	return b[0], err
}
