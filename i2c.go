package mcp2221

import (
	"errors"
	"fmt"
	"time"
)

// I2CWrite writes data to the device at 7-bit address addr.
func (d *Device) I2CWrite(addr byte, data []byte) error {
	return d.I2CTx(addr, data, nil)
}

// I2CRead reads len(buf) bytes from the device at 7-bit address addr.
func (d *Device) I2CRead(addr byte, buf []byte) error {
	return d.I2CTx(addr, nil, buf)
}

// I2CTx writes w and then reads into r. Either may be empty, but not both,
// and each may be up to 65535 bytes.
//
// The write and read are separate transfers (stop, then start), not a
// repeated start: the MCP2221's repeated-start read returned corrupted data
// in testing. Devices that keep their register pointer between transfers,
// which is nearly all of them, work as expected.
func (d *Device) I2CTx(addr byte, w, r []byte) error {
	if addr > 0x7F {
		return fmt.Errorf("mcp2221: I2C address 0x%02x is not 7-bit", addr)
	}
	if len(w) > 0xFFFF || len(r) > 0xFFFF {
		return errors.New("mcp2221: I2C transfer longer than 65535 bytes")
	}
	if len(w) == 0 && len(r) == 0 {
		return errors.New("mcp2221: empty I2C transfer")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	switch {
	case len(r) == 0:
		return d.write(cmdWrite, addr, w)
	case len(w) == 0:
		return d.read(cmdRead, addr, r)
	default:
		if err := d.write(cmdWrite, addr, w); err != nil {
			return err
		}
		return d.read(cmdRead, addr, r)
	}
}

// Scan returns the 7-bit addresses (0x08-0x77) of devices that answer a
// one-byte read.
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

func (d *Device) write(cmd, addr byte, data []byte) error {
	if state, err := d.i2cState(); err != nil {
		return err
	} else if state != stateIdle {
		if err := d.cancel(); err != nil {
			return err
		}
	}

	length := len(data)
	deadline := time.Now().Add(xferLimit)
	for start := 0; start < length; {
		if time.Now().After(deadline) {
			return ErrTimeout
		}
		chunk := min(length-start, maxChunk)
		report := append([]byte{cmd, byte(length), byte(length >> 8), addr << 1}, data[start:start+chunk]...)
		resp, err := d.xfer(report...)
		if err != nil {
			return err
		}
		if resp[1] != 0 {
			if fatalStates[resp[2]] {
				return d.fail(addr, resp[2])
			}
			continue // busy, try again
		}
		for {
			state, err := d.i2cState()
			if err != nil {
				return err
			}
			if state != statePartialData {
				break
			}
			time.Sleep(time.Millisecond)
		}
		start += chunk
	}

	// The chip accepts data before the bus transfer finishes, so poll the
	// status to learn whether the device actually acknowledged it.
	for {
		if time.Now().After(deadline) {
			return ErrTimeout
		}
		resp, err := d.status()
		if err != nil {
			return err
		}
		if resp[20]&statusNACKMask != 0 {
			return d.fail(addr, stateAddrNACK)
		}
		state := resp[8]
		if state == stateIdle {
			return nil
		}
		if fatalStates[state] {
			return d.fail(addr, state)
		}
		time.Sleep(time.Millisecond)
	}
}

func (d *Device) read(cmd, addr byte, buf []byte) error {
	if state, err := d.i2cState(); err != nil {
		return err
	} else if state != stateIdle {
		if err := d.cancel(); err != nil {
			return err
		}
	}

	length := len(buf)
	resp, err := d.xfer(cmd, byte(length), byte(length>>8), addr<<1|1)
	if err != nil {
		return err
	}
	if resp[1] != 0 {
		_ = d.cancel()
		return fmt.Errorf("mcp2221: I2C read from 0x%02x rejected", addr)
	}

	deadline := time.Now().Add(xferLimit)
	for start := 0; start < length; {
		if time.Now().After(deadline) {
			_ = d.cancel()
			return ErrTimeout
		}
		resp, err := d.xfer(cmdGetData)
		if err != nil {
			return err
		}
		switch {
		case resp[2] == stateAddrNACK: // reported alongside the busy flag
			return d.fail(addr, stateAddrNACK)
		case resp[1] == statePartialData, resp[3] == stateReadError:
			time.Sleep(time.Millisecond) // data not ready yet
			continue
		case resp[1] != 0:
			_ = d.cancel()
			return fmt.Errorf("mcp2221: I2C read from 0x%02x failed (0x%02x)", addr, resp[1])
		case resp[2] == stateReadComplete, resp[2] == stateReadPartial,
			resp[2] == stateIdle && resp[3] == 0:
		default:
			time.Sleep(time.Millisecond)
			continue
		}
		chunk := min(length-start, maxChunk)
		copy(buf[start:start+chunk], resp[4:4+chunk])
		start += chunk
	}
	return nil
}

// fail clears the error on the chip so the next transfer starts clean, and
// returns an error describing the failed state.
func (d *Device) fail(addr, state byte) error {
	_ = d.cancel()
	if state == stateAddrNACK {
		return fmt.Errorf("%w (0x%02x)", ErrNACK, addr)
	}
	return fmt.Errorf("mcp2221: I2C transfer with 0x%02x failed (state 0x%02x)", addr, state)
}
