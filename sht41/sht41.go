// Package sht41 reads the Sensirion SHT41 temperature and humidity sensor
// (for example the Adafruit SHT41 breakout) over I2C.
//
// The sensor's address is fixed at 0x44. Unlike the SHTC3 it doesn't sleep
// between reads, so a bus scan finds it.
package sht41

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// Addr is the SHT41's I2C address. It can't be changed.
const Addr = 0x44

// Bus is an I2C bus the sensor can be reached through, such as an
// *mcp2221.Device or an *ft232h.Device.
type Bus interface {
	I2CWrite(addr byte, data []byte) error
	I2CRead(addr byte, buf []byte) error
}

// ErrCRC means a reading failed its checksum.
var ErrCRC = errors.New("sht41: checksum mismatch")

// Precision is how carefully the sensor measures. Higher precision is
// slower and uses more power.
type Precision int

const (
	High   Precision = iota // about 8 ms
	Medium                  // about 5 ms
	Low                     // about 2 ms
)

// Heater is a heater setting: power and pulse length. The heater can burn
// off condensation; it warms the sensor, so readings taken during and just
// after a pulse are too warm.
type Heater int

const (
	Heater200mW1s   Heater = iota // 200 mW for 1 s
	Heater200mW100m               // 200 mW for 0.1 s
	Heater110mW1s                 // 110 mW for 1 s
	Heater110mW100m               // 110 mW for 0.1 s
	Heater20mW1s                  // 20 mW for 1 s
	Heater20mW100m                // 20 mW for 0.1 s
)

// Commands, from the SHT4x datasheet.
const (
	cmdSerial = 0x89
	cmdReset  = 0x94
)

var measure = [...]struct {
	cmd  byte
	wait time.Duration
}{
	High:   {0xFD, 9 * time.Millisecond},
	Medium: {0xF6, 5 * time.Millisecond},
	Low:    {0xE0, 2 * time.Millisecond},
}

var heat = [...]struct {
	cmd  byte
	wait time.Duration
}{
	Heater200mW1s:   {0x39, 1100 * time.Millisecond},
	Heater200mW100m: {0x32, 110 * time.Millisecond},
	Heater110mW1s:   {0x2F, 1100 * time.Millisecond},
	Heater110mW100m: {0x24, 110 * time.Millisecond},
	Heater20mW1s:    {0x1E, 1100 * time.Millisecond},
	Heater20mW100m:  {0x15, 110 * time.Millisecond},
}

// Reading is one measurement.
type Reading struct {
	Celsius  float64 // temperature in °C
	Humidity float64 // relative humidity in %, limited to 0-100
}

// Fahrenheit returns the temperature in °F.
func (r Reading) Fahrenheit() float64 { return r.Celsius*9/5 + 32 }

// Sensor is an SHT41. It is safe for concurrent use.
type Sensor struct {
	mu  sync.Mutex
	bus Bus

	// Precision selects the measurement mode used by Read. The zero value
	// is High.
	Precision Precision
}

// New resets the SHT41 on bus and checks that it answers with a valid serial
// number.
func New(bus Bus) (*Sensor, error) {
	s := &Sensor{bus: bus}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := bus.I2CWrite(Addr, []byte{cmdReset}); err != nil {
		return nil, fmt.Errorf("sht41: reset: %w", err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := s.serial(); err != nil {
		return nil, err
	}
	return s, nil
}

// Serial returns the sensor's 32-bit serial number.
func (s *Sensor) Serial() (uint32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.serial()
}

// Read takes one measurement at the sensor's Precision.
func (s *Sensor) Read() (Reading, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Precision < High || s.Precision > Low {
		return Reading{}, fmt.Errorf("sht41: invalid precision %d", s.Precision)
	}
	m := measure[s.Precision]
	return s.measure(m.cmd, m.wait)
}

// Heat runs the heater for one pulse and returns a measurement taken at the
// end of it. It blocks for the length of the pulse. Let the sensor cool
// before trusting later readings.
func (s *Sensor) Heat(h Heater) (Reading, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if h < Heater200mW1s || h > Heater20mW100m {
		return Reading{}, fmt.Errorf("sht41: invalid heater setting %d", h)
	}
	return s.measure(heat[h].cmd, heat[h].wait)
}

func (s *Sensor) measure(cmd byte, wait time.Duration) (Reading, error) {
	if err := s.bus.I2CWrite(Addr, []byte{cmd}); err != nil {
		return Reading{}, fmt.Errorf("sht41: measure: %w", err)
	}
	time.Sleep(wait)

	var buf [6]byte
	if err := s.bus.I2CRead(Addr, buf[:]); err != nil {
		return Reading{}, fmt.Errorf("sht41: read: %w", err)
	}
	t, err := word(buf[0:3])
	if err != nil {
		return Reading{}, err
	}
	h, err := word(buf[3:6])
	if err != nil {
		return Reading{}, err
	}
	rh := -6 + 125*float64(h)/65535
	return Reading{
		Celsius:  -45 + 175*float64(t)/65535,
		Humidity: min(max(rh, 0), 100),
	}, nil
}

func (s *Sensor) serial() (uint32, error) {
	if err := s.bus.I2CWrite(Addr, []byte{cmdSerial}); err != nil {
		return 0, fmt.Errorf("sht41: read serial: %w", err)
	}
	time.Sleep(time.Millisecond)
	var buf [6]byte
	if err := s.bus.I2CRead(Addr, buf[:]); err != nil {
		return 0, fmt.Errorf("sht41: read serial: %w", err)
	}
	hi, err := word(buf[0:3])
	if err != nil {
		return 0, err
	}
	lo, err := word(buf[3:6])
	if err != nil {
		return 0, err
	}
	return uint32(hi)<<16 | uint32(lo), nil
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
