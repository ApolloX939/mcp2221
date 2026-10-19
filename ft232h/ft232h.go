// Package ft232h drives the FTDI FT232H as a USB-to-I2C bridge (for example
// the Adafruit FT232H breakout), from pure Go with no cgo.
//
// It uses the chip's MPSSE engine with the same I2C wiring as Adafruit
// Blinka: D0 is SCL, and D1 and D2 together are SDA. On the Adafruit board,
// set the I2C switch on (or wire D1 to D2 on older boards without one).
//
// Only Linux is supported for now; Open returns an error elsewhere.
package ft232h

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// USB IDs of the FT232H.
const (
	VendorID  = 0x0403
	ProductID = 0x6014
)

var (
	// ErrNoDevice means no FT232H is attached.
	ErrNoDevice = errors.New("ft232h: no FT232H found")
	// ErrInUse means another program has the FT232H open.
	ErrInUse = errors.New("ft232h: FT232H is in use by another program")
	// ErrNACK means no device acknowledged the I2C address.
	ErrNACK = errors.New("ft232h: no I2C device acknowledged the address")
	// ErrTimeout means the chip did not answer in time.
	ErrTimeout = errors.New("ft232h: timed out waiting for the FT232H")
)

// transport moves bytes to and from the chip.
type transport interface {
	control(request uint8, value, index uint16) error
	write(b []byte) error
	read(buf []byte, timeout time.Duration) error
	discard()
	close() error
}

// FTDI vendor requests.
const (
	reqReset      = 0x00
	reqSetLatency = 0x09
	reqSetBitmode = 0x0B

	resetSIO    = 0
	purgeRX     = 1
	purgeTX     = 2
	portA       = 1 // wIndex for the FT232H's only port
	modeReset   = 0x00
	modeMPSSE   = 0x02
	readLimit   = 250 * time.Millisecond
	maxPerBurst = 256 // data bytes per USB round trip; keeps replies inside the chip's buffer
)

// MPSSE commands.
const (
	mpsseWriteBytesNeg = 0x11 // clock bytes out on falling edge, MSB first
	mpsseWriteBitsNeg  = 0x13 // clock bits out on falling edge, MSB first
	mpsseReadBytesPos  = 0x20 // clock bytes in on rising edge, MSB first
	mpsseReadBitsPos   = 0x22 // clock bits in on rising edge, MSB first
	mpsseSetLow        = 0x80 // set D0-D7 value and direction
	mpsseSetHigh       = 0x82 // set C0-C7 value and direction
	mpsseLoopbackOff   = 0x85
	mpsseSetDivisor    = 0x86
	mpsseSendImmediate = 0x87
	mpsseDiv5Off       = 0x8A
	mpsse3PhaseOn      = 0x8C
	mpsseAdaptiveOff   = 0x97
	mpsseDriveZero     = 0x9E // open-drain outputs, FT232H only
	mpsseBogus         = 0xAA // invalid command, used to sync
)

// Pins on the low byte: D0 = SCL, D1 = SDA out, D2 = SDA in.
const (
	pinSCL = 0x01
	pinSDA = 0x02
	pinDir = pinSCL | pinSDA
)

// Device is an open FT232H. It is safe for concurrent use.
type Device struct {
	mu   sync.Mutex
	usb  transport
	hold int // SET_BITS repeats that stretch start/stop timing
}

// Open opens the FT232H attached to this computer, switches it to I2C and
// sets the bus to 100 kHz. It fails if there is not exactly one FT232H.
//
// While open, the chip's serial port (/dev/ttyUSB*) goes away; Close brings
// it back. The device is held exclusively: opening it while another
// program has it fails with ErrInUse.
//
// The USB device file must be readable and writable by the current user;
// see the README for a udev rule.
func Open() (*Device, error) {
	u, err := openUSB()
	if err != nil {
		return nil, err
	}
	d := &Device{usb: u}
	if err := d.init(); err != nil {
		u.close()
		return nil, err
	}
	return d, nil
}

// Close puts the chip back in serial mode and releases it.
func (d *Device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_ = d.usb.control(reqSetBitmode, modeReset<<8, portA)
	return d.usb.close()
}

func (d *Device) init() error {
	u := d.usb
	for _, c := range [][2]uint16{
		{reqReset, resetSIO},
		{reqReset, purgeRX},
		{reqReset, purgeTX},
		{reqSetLatency, 1},
		{reqSetBitmode, modeReset << 8},
		{reqSetBitmode, modeMPSSE << 8},
	} {
		if err := u.control(uint8(c[0]), c[1], portA); err != nil {
			return err
		}
	}
	time.Sleep(20 * time.Millisecond) // let MPSSE start

	// An invalid command makes the chip reply 0xFA and echo it, which
	// confirms MPSSE is up and the read pipe is in step.
	if err := u.write([]byte{mpsseBogus, mpsseSendImmediate}); err != nil {
		return err
	}
	if err := d.expectEcho(mpsseBogus); err != nil {
		return err
	}

	if err := u.write([]byte{
		mpsseDiv5Off, mpsseAdaptiveOff, mpsse3PhaseOn, mpsseLoopbackOff,
		mpsseDriveZero, pinDir, 0x00,
		mpsseSetLow, pinSCL | pinSDA, pinDir, // bus idle
		mpsseSetHigh, 0x00, 0x00,
	}); err != nil {
		return err
	}
	return d.setSpeed(100_000)
}

// expectEcho reads replies until it sees 0xFA followed by cmd.
func (d *Device) expectEcho(cmd byte) error {
	b := make([]byte, 1)
	prev := byte(0)
	for i := 0; i < 64; i++ {
		if err := d.usb.read(b, readLimit); err != nil {
			return fmt.Errorf("ft232h: MPSSE did not start: %w", err)
		}
		if prev == 0xFA && b[0] == cmd {
			return nil
		}
		prev = b[0]
	}
	return errors.New("ft232h: MPSSE did not start (no echo)")
}

// SetI2CSpeed sets the I2C clock in Hz, from 1 kHz to 1 MHz.
func (d *Device) SetI2CSpeed(hz int) error {
	if hz < 1_000 || hz > 1_000_000 {
		return fmt.Errorf("ft232h: I2C speed %d Hz out of range (1000-1000000)", hz)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.setSpeed(hz)
}

func (d *Device) setSpeed(hz int) error {
	// 60 MHz base clock; three-phase clocking stretches each bit by 3/2.
	div := (20_000_000+hz-1)/hz - 1
	d.hold = max(4, 10*100_000/hz)
	return d.usb.write([]byte{mpsseSetDivisor, byte(div), byte(div >> 8)})
}

// I2CWrite writes data to the device at 7-bit address addr.
func (d *Device) I2CWrite(addr byte, data []byte) error {
	return d.I2CTx(addr, data, nil)
}

// I2CRead reads len(buf) bytes from the device at 7-bit address addr.
func (d *Device) I2CRead(addr byte, buf []byte) error {
	return d.I2CTx(addr, nil, buf)
}

// I2CTx writes w and then reads into r, with a repeated start between them.
// Either may be empty, but not both.
func (d *Device) I2CTx(addr byte, w, r []byte) error {
	if addr > 0x7F {
		return fmt.Errorf("ft232h: I2C address 0x%02x is not 7-bit", addr)
	}
	if len(w) == 0 && len(r) == 0 {
		return errors.New("ft232h: empty I2C transfer")
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	err := d.tx(addr, w, r)
	if err != nil && !errors.Is(err, ErrNACK) {
		// Late replies would be mistaken for the next transfer's.
		d.usb.discard()
	}
	// Always leave the bus idle, whatever happened.
	if serr := d.usb.write(d.stop(nil)); err == nil {
		err = serr
	}
	return err
}

func (d *Device) tx(addr byte, w, r []byte) error {
	if len(w) > 0 {
		cmd := d.start(nil)
		if err := d.sendBytes(cmd, append([]byte{addr << 1}, w...), addr); err != nil {
			return err
		}
	}
	if len(r) > 0 {
		var cmd []byte
		if len(w) > 0 {
			cmd = d.restart(cmd)
		} else {
			cmd = d.start(cmd)
		}
		if err := d.sendBytes(cmd, []byte{addr<<1 | 1}, addr); err != nil {
			return err
		}
		return d.recvBytes(r)
	}
	return nil
}

// sendBytes sends prefix then clocks out each byte, checking for an ACK
// after each. A NACK on the first byte (the address) returns ErrNACK.
func (d *Device) sendBytes(prefix, data []byte, addr byte) error {
	cmd := prefix
	for sent := 0; sent < len(data); {
		n := min(len(data)-sent, maxPerBurst)
		for _, b := range data[sent : sent+n] {
			cmd = append(cmd,
				mpsseWriteBytesNeg, 0, 0, b,
				mpsseSetLow, pinSDA, pinDir, // release SDA for the ACK
				mpsseReadBitsPos, 0,
			)
		}
		cmd = append(cmd, mpsseSendImmediate)
		if err := d.usb.write(cmd); err != nil {
			return err
		}
		acks := make([]byte, n)
		if err := d.usb.read(acks, readLimit); err != nil {
			return err
		}
		for i, a := range acks {
			if a&1 != 0 {
				if sent+i == 0 {
					return fmt.Errorf("%w (0x%02x)", ErrNACK, addr)
				}
				return fmt.Errorf("ft232h: I2C device 0x%02x did not acknowledge byte %d", addr, sent+i-1)
			}
		}
		sent += n
		cmd = cmd[:0]
	}
	return nil
}

// recvBytes reads len(buf) bytes, ACKing each but the last.
func (d *Device) recvBytes(buf []byte) error {
	var cmd []byte
	for got := 0; got < len(buf); {
		n := min(len(buf)-got, maxPerBurst)
		cmd = cmd[:0]
		for i := 0; i < n; i++ {
			ack := byte(0x00)
			if got+i == len(buf)-1 {
				ack = 0xFF // NACK the last byte
			}
			cmd = append(cmd,
				mpsseSetLow, pinSDA, pinDir, // release SDA so the device can drive it
				mpsseReadBytesPos, 0, 0,
				mpsseWriteBitsNeg, 0, ack,
			)
		}
		cmd = append(cmd, mpsseSendImmediate)
		if err := d.usb.write(cmd); err != nil {
			return err
		}
		if err := d.usb.read(buf[got:got+n], readLimit); err != nil {
			return err
		}
		got += n
	}
	return nil
}

func (d *Device) pins(cmd []byte, value byte, repeat int) []byte {
	for i := 0; i < repeat; i++ {
		cmd = append(cmd, mpsseSetLow, value, pinDir)
	}
	return cmd
}

// start: SDA falls while SCL is high, then SCL goes low.
func (d *Device) start(cmd []byte) []byte {
	cmd = d.pins(cmd, pinSCL|pinSDA, d.hold)
	cmd = d.pins(cmd, pinSCL, d.hold)
	return d.pins(cmd, 0, d.hold)
}

// restart releases SDA and raises SCL, then sends a start.
func (d *Device) restart(cmd []byte) []byte {
	cmd = d.pins(cmd, pinSDA, d.hold)
	return d.start(cmd)
}

// stop: SDA rises while SCL is high.
func (d *Device) stop(cmd []byte) []byte {
	cmd = d.pins(cmd, 0, d.hold)
	cmd = d.pins(cmd, pinSCL, d.hold)
	return d.pins(cmd, pinSCL|pinSDA, d.hold)
}

// Scan returns the 7-bit addresses (0x08-0x77) of devices that acknowledge
// their address.
func (d *Device) Scan() ([]byte, error) {
	var found []byte
	buf := make([]byte, 1)
	for addr := byte(0x08); addr <= 0x77; addr++ {
		err := d.I2CRead(addr, buf)
		switch {
		case err == nil:
			found = append(found, addr)
		case errors.Is(err, ErrNACK):
		default:
			return found, err
		}
	}
	return found, nil
}
