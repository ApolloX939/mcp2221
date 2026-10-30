// Package nunchuk reads a Nintendo Wii Nunchuk controller over I2C, through
// an adapter such as the Adafruit Wii Nunchuck Breakout Adapter.
//
// The nunchuk's address is fixed at 0x52. The protocol follows
// http://wiibrew.org/wiki/Wiimote/Extension_Controllers.
package nunchuk

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// Addr is the nunchuk's I2C address. It can't be changed.
const Addr = 0x52

// Bus is an I2C bus the nunchuk can be reached through, such as an
// *mcp2221.Device or an *ft232h.Device.
type Bus interface {
	I2CWrite(addr byte, data []byte) error
	I2CRead(addr byte, buf []byte) error
}

// ErrNoData means the nunchuk answered with nothing but 0xFF, as it does
// when it is unplugged or not yet awake.
var ErrNoData = errors.New("nunchuk: no data (is the controller plugged in?)")

const (
	initDelay = 100 * time.Millisecond
	readDelay = 2 * time.Millisecond // the controller needs at least 200 µs
)

// State is one reading of the controller.
type State struct {
	JoyX, JoyY uint8  // joystick position, about 0 to 255 with the center near 128
	AccelX     uint16 // acceleration, 10 bits: 0 to 1023 with 1 g near 512 on a vertical axis
	AccelY     uint16
	AccelZ     uint16
	C, Z       bool // button pressed
}

// Stick returns the joystick position as x and y from -1 to 1, with y
// positive upward.
func (s State) Stick() (x, y float64) {
	norm := func(v uint8) float64 {
		return max(-1, min(1, (float64(v)-128)/100))
	}
	return norm(s.JoyX), norm(s.JoyY)
}

// Nunchuk is a controller on an I2C bus. It is safe for concurrent use.
type Nunchuk struct {
	mu  sync.Mutex
	bus Bus
}

// New wakes the nunchuk on bus, turns off its data encryption and checks
// that it answers.
func New(bus Bus) (*Nunchuk, error) {
	n := &Nunchuk{bus: bus}
	time.Sleep(initDelay)
	if err := bus.I2CWrite(Addr, []byte{0xF0, 0x55}); err != nil {
		return nil, fmt.Errorf("nunchuk: init: %w", err)
	}
	time.Sleep(initDelay)
	if err := bus.I2CWrite(Addr, []byte{0xFB, 0x00}); err != nil {
		return nil, fmt.Errorf("nunchuk: init: %w", err)
	}
	time.Sleep(initDelay)
	if _, err := n.Read(); err != nil {
		return nil, err
	}
	return n, nil
}

// Read takes one reading. It takes a few milliseconds.
func (n *Nunchuk) Read() (State, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if err := n.bus.I2CWrite(Addr, []byte{0x00}); err != nil {
		return State{}, fmt.Errorf("nunchuk: read: %w", err)
	}
	time.Sleep(readDelay)
	var b [6]byte
	if err := n.bus.I2CRead(Addr, b[:]); err != nil {
		return State{}, fmt.Errorf("nunchuk: read: %w", err)
	}
	if b == [6]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF} {
		return State{}, ErrNoData
	}
	return State{
		JoyX:   b[0],
		JoyY:   b[1],
		AccelX: uint16(b[2])<<2 | uint16(b[5]>>6&3),
		AccelY: uint16(b[3])<<2 | uint16(b[5]>>4&3),
		AccelZ: uint16(b[4])<<2 | uint16(b[5]>>2&3),
		Z:      b[5]&0x01 == 0, // the buttons read 0 while pressed
		C:      b[5]&0x02 == 0,
	}, nil
}
