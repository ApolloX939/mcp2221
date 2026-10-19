// Package shtc3 reads the Sensirion SHTC3 temperature and humidity sensor
// (for example the Adafruit SHTC3 breakout) over I2C.
//
// The sensor's address is fixed at 0x70, the same as the default address of
// an HT16K33 LED backpack, so the two can't share a bus unless the backpack's
// address is changed.
//
// The sensor sleeps between reads, and a bus scan doesn't find it while it
// is asleep. New checks that it is there.
package shtc3

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// Addr is the SHTC3's I2C address. It can't be changed.
const Addr = 0x70

// Bus is an I2C bus the sensor can be reached through, such as an
// *mcp2221.Device or an *ft232h.Device.
type Bus interface {
	I2CWrite(addr byte, data []byte) error
	I2CRead(addr byte, buf []byte) error
}

// ErrCRC means a reading failed its checksum.
var ErrCRC = errors.New("shtc3: checksum mismatch")

// Commands, from the SHTC3 datasheet.
var (
	cmdWake        = []byte{0x35, 0x17}
	cmdSleep       = []byte{0xB0, 0x98}
	cmdReset       = []byte{0x80, 0x5D}
	cmdID          = []byte{0xEF, 0xC8}
	cmdMeasure     = []byte{0x78, 0x66} // normal mode, temperature first, no clock stretching
	cmdMeasureLow  = []byte{0x60, 0x9C} // low-power mode
	measureTime    = 13 * time.Millisecond
	measureTimeLow = time.Millisecond
	wakeTime       = 240 * time.Microsecond
)

// Reading is one measurement.
type Reading struct {
	Celsius  float64 // temperature in °C
	Humidity float64 // relative humidity in %
}

// Fahrenheit returns the temperature in °F.
func (r Reading) Fahrenheit() float64 { return r.Celsius*9/5 + 32 }

// Sensor is an SHTC3. It is safe for concurrent use.
type Sensor struct {
	mu  sync.Mutex
	bus Bus

	// LowPower makes Read use the sensor's faster, less precise mode
	// (about 1 ms instead of 12 ms).
	LowPower bool
}

// New checks that an SHTC3 answers on bus, resets it and puts it to sleep.
func New(bus Bus) (*Sensor, error) {
	s := &Sensor{bus: bus}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.wake(); err != nil {
		return nil, err
	}
	if err := bus.I2CWrite(Addr, cmdReset); err != nil {
		return nil, fmt.Errorf("shtc3: reset: %w", err)
	}
	time.Sleep(time.Millisecond)
	id, err := s.id()
	if err != nil {
		return nil, err
	}
	if id&0x083F != 0x0807 {
		return nil, fmt.Errorf("shtc3: device at 0x70 is not an SHTC3 (ID 0x%04x)", id)
	}
	return s, s.sleep()
}

// Read takes one measurement. The sensor is woken for it and put back to
// sleep afterwards.
func (s *Sensor) Read() (Reading, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.wake(); err != nil {
		return Reading{}, err
	}
	cmd, wait := cmdMeasure, measureTime
	if s.LowPower {
		cmd, wait = cmdMeasureLow, measureTimeLow
	}
	if err := s.bus.I2CWrite(Addr, cmd); err != nil {
		return Reading{}, fmt.Errorf("shtc3: measure: %w", err)
	}
	time.Sleep(wait)

	var buf [6]byte
	if err := s.bus.I2CRead(Addr, buf[:]); err != nil {
		return Reading{}, fmt.Errorf("shtc3: read: %w", err)
	}
	t, err := word(buf[0:3])
	if err != nil {
		return Reading{}, err
	}
	h, err := word(buf[3:6])
	if err != nil {
		return Reading{}, err
	}
	r := Reading{
		Celsius:  -45 + 175*float64(t)/65536,
		Humidity: 100 * float64(h) / 65536,
	}
	return r, s.sleep()
}

func (s *Sensor) wake() error {
	if err := s.bus.I2CWrite(Addr, cmdWake); err != nil {
		return fmt.Errorf("shtc3: wake: %w", err)
	}
	time.Sleep(wakeTime)
	return nil
}

func (s *Sensor) sleep() error {
	if err := s.bus.I2CWrite(Addr, cmdSleep); err != nil {
		return fmt.Errorf("shtc3: sleep: %w", err)
	}
	return nil
}

func (s *Sensor) id() (uint16, error) {
	if err := s.bus.I2CWrite(Addr, cmdID); err != nil {
		return 0, fmt.Errorf("shtc3: read ID: %w", err)
	}
	var buf [3]byte
	if err := s.bus.I2CRead(Addr, buf[:]); err != nil {
		return 0, fmt.Errorf("shtc3: read ID: %w", err)
	}
	return word(buf[:])
}

// word checks the CRC of a 2-byte word followed by its CRC byte.
func word(b []byte) (uint16, error) {
	if crc8(b[:2]) != b[2] {
		return 0, ErrCRC
	}
	return uint16(b[0])<<8 | uint16(b[1]), nil
}

// crc8 is Sensirion's CRC: polynomial 0x31, initial value 0xFF.
func crc8(data []byte) byte {
	crc := byte(0xFF)
	for _, b := range data {
		crc ^= b
		for i := 0; i < 8; i++ {
			if crc&0x80 != 0 {
				crc = crc<<1 ^ 0x31
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
