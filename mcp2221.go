// Package mcp2221 drives the Microchip MCP2221 / MCP2221A USB-to-I2C bridge
// from pure Go (no cgo), on Linux, Windows and macOS.
//
// It talks to the chip with raw 64-byte HID reports, following the same
// protocol steps as Adafruit Blinka's MCP2221 support.
package mcp2221

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"rafaelmartins.com/p/usbhid"
)

// USB IDs of the MCP2221 and MCP2221A.
const (
	VendorID  = 0x04D8
	ProductID = 0x00DD
)

var (
	// ErrNACK means no device acknowledged the I2C address.
	ErrNACK = errors.New("mcp2221: no I2C device acknowledged the address")
	// ErrTimeout means an I2C transfer did not finish in time.
	ErrTimeout = errors.New("mcp2221: I2C transfer timed out")
)

const (
	reportLen  = 64
	maxChunk   = 60 // I2C data bytes per HID report
	xferLimit  = 250 * time.Millisecond
	cmdStatus  = 0x10 // status / set parameters
	cmdGetData = 0x40 // fetch data from a previous I2C read

	cmdWrite = 0x90
	cmdRead  = 0x91

	stateIdle         = 0x00
	stateAddrNACK     = 0x25
	statePartialData  = 0x41
	stateReadPartial  = 0x54
	stateReadComplete = 0x55
	stateReadError    = 0x7F

	statusNACKMask = 0x40 // status report byte 20: address not acknowledged
)

// I2C states that mean the transfer failed and retrying won't help.
var fatalStates = map[byte]bool{
	0x12: true, // start timeout
	0x23: true, // address timeout
	0x25: true, // address NACK
	0x44: true, // data timeout
	0x62: true, // stop timeout
}

// Device is an open MCP2221. It is safe for concurrent use.
type Device struct {
	mu  sync.Mutex
	hid *usbhid.Device
}

// Open opens the MCP2221 attached to this computer, clears any stuck I2C
// transfer and sets the bus to 100 kHz. It fails if there is not exactly
// one MCP2221 connected.
//
// The device is held exclusively until Close, because the chip answers
// commands in order and two programs sharing it would get each other's
// replies. Opening it while another program has it open fails with an
// error wrapping usbhid.ErrDeviceLocked.
//
// On Linux the HID device must be readable and writable by the current user;
// see the README for a udev rule.
func Open() (*Device, error) {
	h, err := usbhid.Get(func(d *usbhid.Device) bool {
		return d.VendorId() == VendorID && d.ProductId() == ProductID
	}, true, true)
	if err != nil {
		return nil, fmt.Errorf("mcp2221: %w", err)
	}
	d := &Device{hid: h}
	if err := d.SetI2CSpeed(100_000); err != nil {
		h.Close()
		return nil, err
	}
	return d, nil
}

// Close releases the device.
func (d *Device) Close() error {
	return d.hid.Close()
}

// SetI2CSpeed sets the I2C clock in Hz, from 47 kHz to 400 kHz.
func (d *Device) SetI2CSpeed(hz int) error {
	if hz < 47_000 || hz > 400_000 {
		return fmt.Errorf("mcp2221: I2C speed %d Hz out of range (47000-400000)", hz)
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	if state, err := d.i2cState(); err != nil {
		return err
	} else if state != stateIdle {
		if err := d.cancel(); err != nil {
			return err
		}
	}
	resp, err := d.xfer(cmdStatus, 0x00, 0x00, 0x20, byte(12_000_000/hz-3))
	if err != nil {
		return err
	}
	if resp[3] != 0x20 {
		return errors.New("mcp2221: I2C speed not accepted (bus busy)")
	}
	return nil
}

// xfer sends one command report and returns the 64-byte reply.
// The caller must hold d.mu.
func (d *Device) xfer(report ...byte) ([]byte, error) {
	buf := make([]byte, reportLen)
	copy(buf, report)
	if err := d.hid.SetOutputReport(0, buf); err != nil {
		return nil, fmt.Errorf("mcp2221: %w", err)
	}
	_, resp, err := d.hid.GetInputReport()
	if err != nil {
		return nil, fmt.Errorf("mcp2221: %w", err)
	}
	if len(resp) < reportLen {
		return nil, fmt.Errorf("mcp2221: short reply (%d bytes)", len(resp))
	}
	if resp[0] != report[0] {
		return nil, fmt.Errorf("mcp2221: reply 0x%02x does not match command 0x%02x", resp[0], report[0])
	}
	return resp, nil
}

func (d *Device) status() ([]byte, error) {
	resp, err := d.xfer(cmdStatus)
	if err != nil {
		return nil, err
	}
	if resp[1] != 0 {
		return nil, errors.New("mcp2221: couldn't read I2C status")
	}
	return resp, nil
}

func (d *Device) i2cState() (byte, error) {
	resp, err := d.status()
	if err != nil {
		return 0, err
	}
	return resp[8], nil
}

// cancel aborts the current I2C transfer and frees the bus.
func (d *Device) cancel() error {
	resp, err := d.xfer(cmdStatus, 0x00, 0x10)
	if err != nil {
		return err
	}
	if resp[1] != 0 {
		return errors.New("mcp2221: couldn't cancel I2C transfer")
	}
	if resp[2] == 0x10 {
		time.Sleep(time.Millisecond) // bus release takes a few hundred µs
	}
	return nil
}
