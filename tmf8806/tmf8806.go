// Package tmf8806 reads the ams OSRAM TMF8806 time-of-flight distance sensor
// (for example the Adafruit TMF8806 breakout) over I2C.
//
// The protocol follows the register map in Adafruit's CircuitPython driver
// and ams OSRAM's own driver. The sensor's address is fixed at 0x41.
//
// Only distance ranging is supported: not the 10 m mode, which needs a RAM
// firmware patch, nor histograms, calibration or the GPIO functions.
package tmf8806

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Addr is the TMF8806's I2C address. It can't be changed.
const Addr = 0x41

// Bus is an I2C bus the sensor can be reached through, such as an
// *mcp2221.Device or an *ft232h.Device.
type Bus interface {
	I2CWrite(addr byte, data []byte) error
	I2CRead(addr byte, buf []byte) error
}

// Registers and values, from the TMF8806 datasheet.
const (
	regAppID      = 0x00
	regAppRequest = 0x02
	regCmdData    = 0x06 // start of the 11-byte command block; the command is its last byte
	regCommand    = 0x10
	regState      = 0x1C
	regContents   = 0x1E
	regResult     = 0x20
	regEnable     = 0xE0
	regIntStatus  = 0xE1
	regIntEnable  = 0xE2
	regID         = 0xE3

	appMeasurement = 0xC0
	chipID         = 0x09

	enablePower    = 0x01
	enableCPUReady = 0x40

	cmdMeasure = 0x02
	cmdSerial  = 0x47
	cmdStop    = 0xFF
	cmdIdle    = 0x00

	contentsResult = 0x55
	contentsSerial = 0x47
	stateError     = 0x02
	intResult      = 0x01
	intDiagnostic  = 0x02

	algoDistance = 0x02
	algoClockDiv = 0x04
	algoLong     = 0x08

	commandSize = 11
	frameSize   = 34
)

// Mode is the sensor's ranging mode.
type Mode int

const (
	ShortRange Mode = iota // proximity only, up to about 200 mm
	Range2_5m              // distance up to about 2.5 m (the default)
	Range5m                // distance up to about 5 m
)

var maxDistance = [...]int{200, 2650, 5300}

// Result is one measurement.
type Result struct {
	Number        uint8  // rolling result counter
	Distance      int    // mm; 0 if nothing reliable was found
	Reliability   int    // 0 (invalid) to 63 (best)
	Status        int    // 0 normal, 2 delayed by a GPIO input
	Clock         uint32 // the sensor's clock when the result was taken
	ReferenceHits uint32 // raw hit counts
	ObjectHits    uint32
	Temperature   int    // die temperature in °C
	Crosstalk     uint16 // measured optical crosstalk
}

// Sensor is a TMF8806. It is safe for concurrent use.
type Sensor struct {
	mu  sync.Mutex
	bus Bus

	// Mode selects the ranging mode used when measuring starts. The zero
	// value is ShortRange; New sets Range2_5m.
	Mode Mode

	// Iterations is the integration time per measurement, in thousands of
	// iterations from 10 to 4000. More is more accurate and slower.
	Iterations int

	// Period is the interval between results while measuring continuously,
	// from 1 to 255 ms.
	Period time.Duration

	// NoiseThreshold is the object detection threshold, 0 to 63. Zero uses
	// the sensor's default.
	NoiseThreshold int

	measuring bool
}

// New checks that a TMF8806 answers on bus and starts its measurement
// application.
func New(bus Bus) (*Sensor, error) {
	s := &Sensor{
		bus:            bus,
		Mode:           Range2_5m,
		Iterations:     400,
		Period:         33 * time.Millisecond,
		NoiseThreshold: 6,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id, err := s.readReg(regID, 1)
	if err != nil {
		return nil, fmt.Errorf("tmf8806: read ID: %w", err)
	}
	if id[0]&0x3F != chipID {
		return nil, fmt.Errorf("tmf8806: device at 0x41 is not a TMF8806 (ID 0x%02x)", id[0])
	}
	if err := s.startApp(); err != nil {
		return nil, err
	}
	return s, nil
}

// Start begins measuring with the current settings. With continuous set the
// sensor measures every Period; otherwise it takes one measurement.
func (s *Sensor) Start(continuous bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Mode < ShortRange || s.Mode > Range5m {
		return fmt.Errorf("tmf8806: invalid mode %d", s.Mode)
	}
	if s.Iterations < 10 || s.Iterations > 4000 {
		return errors.New("tmf8806: Iterations must be from 10 to 4000")
	}
	if s.NoiseThreshold < 0 || s.NoiseThreshold > 0x3F {
		return errors.New("tmf8806: NoiseThreshold must be from 0 to 63")
	}
	period := s.Period / time.Millisecond
	if continuous && (period < 1 || period > 255) {
		return errors.New("tmf8806: Period must be from 1 to 255 ms")
	}

	var cmd [commandSize]byte
	switch s.Mode {
	case Range2_5m:
		cmd[3] = algoDistance
	case Range5m:
		cmd[3] = algoDistance | algoClockDiv | algoLong
	}
	cmd[6] = byte(s.NoiseThreshold)
	if continuous {
		cmd[7] = byte(period)
	}
	binary.LittleEndian.PutUint16(cmd[8:], uint16(s.Iterations))
	cmd[10] = cmdMeasure

	if err := s.writeReg(regCmdData, cmd[:]); err != nil {
		return fmt.Errorf("tmf8806: start: %w", err)
	}
	if err := s.waitCommand(cmdMeasure, 50*time.Millisecond); err != nil {
		return err
	}
	s.measuring = true
	return nil
}

// Stop ends measuring.
func (s *Sensor) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writeReg(regCommand, []byte{cmdStop}); err != nil {
		return fmt.Errorf("tmf8806: stop: %w", err)
	}
	// A measurement in flight has to finish first.
	wait := 50*time.Millisecond + time.Duration(s.Iterations)*time.Second/18000
	if err := s.waitCommand(cmdStop, wait); err != nil {
		return err
	}
	s.measuring = false
	return nil
}

// Ready reports whether a result is waiting to be read.
func (s *Sensor) Ready() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.readReg(regIntStatus, 1)
	if err != nil {
		return false, fmt.Errorf("tmf8806: %w", err)
	}
	return st[0]&intResult != 0, nil
}

// Read waits up to timeout for the next result and returns it. Start must
// have been called.
func (s *Sensor) Read(timeout time.Duration) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.measuring {
		return Result{}, errors.New("tmf8806: not measuring; call Start first")
	}
	deadline := time.Now().Add(timeout)
	for {
		st, err := s.readReg(regIntStatus, 1)
		if err != nil {
			return Result{}, fmt.Errorf("tmf8806: %w", err)
		}
		if st[0]&intResult != 0 {
			break
		}
		if time.Now().After(deadline) {
			return Result{}, errors.New("tmf8806: timed out waiting for a result")
		}
		time.Sleep(time.Millisecond)
	}

	f, err := s.readReg(regState, frameSize)
	if err != nil {
		return Result{}, fmt.Errorf("tmf8806: read result: %w", err)
	}
	if f[2] != contentsResult {
		// Not a result (such as diagnostics); drop it and let the caller retry.
		return Result{}, s.writeReg(regIntStatus, []byte{intResult})
	}
	r := Result{
		Number:        f[4],
		Reliability:   int(f[5] & 0x3F),
		Status:        int(f[5] >> 6),
		Distance:      int(binary.LittleEndian.Uint16(f[6:])),
		Clock:         binary.LittleEndian.Uint32(f[8:]),
		Temperature:   int(int8(f[23])),
		ReferenceHits: binary.LittleEndian.Uint32(f[24:]),
		ObjectHits:    binary.LittleEndian.Uint32(f[28:]),
		Crosstalk:     binary.LittleEndian.Uint16(f[32:]),
	}
	if r.Distance > maxDistance[s.Mode] {
		r.Distance, r.Reliability = 0, 0
	}
	if err := s.writeReg(regIntStatus, []byte{intResult}); err != nil {
		return Result{}, fmt.Errorf("tmf8806: clear interrupt: %w", err)
	}
	return r, nil
}

// Distance takes one measurement and returns the distance in millimeters.
// ok is false if the sensor found nothing reliable. Don't use it while
// measuring continuously.
func (s *Sensor) Distance() (mm int, ok bool, err error) {
	if err := s.Start(false); err != nil {
		return 0, false, err
	}
	r, err := s.Read(time.Second + time.Duration(s.Iterations)*time.Second/1000)
	if err != nil {
		return 0, false, err
	}
	s.mu.Lock()
	s.measuring = false
	s.mu.Unlock()
	return r.Distance, r.Reliability > 0, nil
}

// Serial returns the sensor's 32-bit serial number. Don't use it while
// measuring.
func (s *Sensor) Serial() (uint32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writeReg(regCommand, []byte{cmdSerial}); err != nil {
		return 0, fmt.Errorf("tmf8806: serial: %w", err)
	}
	if err := s.waitCommand(cmdSerial, 50*time.Millisecond); err != nil {
		return 0, err
	}
	if err := s.waitReg(regContents, contentsSerial, 100*time.Millisecond, "the serial number"); err != nil {
		return 0, err
	}
	b, err := s.readReg(regResult+8, 4)
	if err != nil {
		return 0, fmt.Errorf("tmf8806: serial: %w", err)
	}
	return binary.LittleEndian.Uint32(b), nil
}

// Sleep puts the sensor in its low-power state, which draws a few
// microamps. Wake restarts it.
func (s *Sensor) Sleep() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.measuring = false
	return s.writeReg(regEnable, []byte{0})
}

// Wake brings the sensor out of Sleep. Call Start to measure again.
func (s *Sensor) Wake() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startApp()
}

// startApp powers the CPU and starts the measurement application.
func (s *Sensor) startApp() error {
	if err := s.writeReg(regEnable, []byte{enablePower}); err != nil {
		return fmt.Errorf("tmf8806: power on: %w", err)
	}
	const ready = enableCPUReady | enablePower
	if err := s.waitMasked(regEnable, ready, ready, 100*time.Millisecond, "the CPU"); err != nil {
		return err
	}
	app, err := s.readReg(regAppID, 1)
	if err != nil {
		return fmt.Errorf("tmf8806: %w", err)
	}
	if app[0] != appMeasurement {
		if err := s.writeReg(regAppRequest, []byte{appMeasurement}); err != nil {
			return fmt.Errorf("tmf8806: start application: %w", err)
		}
		if err := s.waitReg(regAppID, appMeasurement, 200*time.Millisecond, "the measurement application"); err != nil {
			return err
		}
	}
	if err := s.writeReg(regIntStatus, []byte{intResult | intDiagnostic}); err != nil {
		return err
	}
	return s.writeReg(regIntEnable, []byte{intResult})
}

// waitCommand waits for the sensor to finish a command.
func (s *Sensor) waitCommand(cmd byte, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		b, err := s.readReg(regCommand, 2)
		if err != nil {
			return fmt.Errorf("tmf8806: %w", err)
		}
		if b[0] == cmdIdle && b[1] == cmd {
			return nil
		}
		st, err := s.readReg(regState, 1)
		if err != nil {
			return fmt.Errorf("tmf8806: %w", err)
		}
		if st[0] == stateError {
			return fmt.Errorf("tmf8806: sensor rejected command 0x%02x", cmd)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("tmf8806: timed out waiting for command 0x%02x", cmd)
		}
		time.Sleep(time.Millisecond)
	}
}

func (s *Sensor) waitReg(reg, want byte, timeout time.Duration, what string) error {
	return s.waitMasked(reg, 0xFF, want, timeout, what)
}

func (s *Sensor) waitMasked(reg, mask, want byte, timeout time.Duration, what string) error {
	deadline := time.Now().Add(timeout)
	for {
		b, err := s.readReg(reg, 1)
		if err != nil {
			return fmt.Errorf("tmf8806: %w", err)
		}
		if b[0]&mask == want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("tmf8806: timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func (s *Sensor) writeReg(reg byte, data []byte) error {
	return s.bus.I2CWrite(Addr, append([]byte{reg}, data...))
}

func (s *Sensor) readReg(reg byte, n int) ([]byte, error) {
	if err := s.bus.I2CWrite(Addr, []byte{reg}); err != nil {
		return nil, err
	}
	buf := make([]byte, n)
	return buf, s.bus.I2CRead(Addr, buf)
}
