package sht41

import (
	"bytes"
	"errors"
	"math"
	"testing"
)

// fakeBus records writes and answers reads from a queue.
type fakeBus struct {
	writes  [][]byte
	replies [][]byte
}

func (f *fakeBus) I2CWrite(addr byte, data []byte) error {
	f.writes = append(f.writes, append([]byte(nil), data...))
	return nil
}

func (f *fakeBus) I2CRead(addr byte, buf []byte) error {
	copy(buf, f.replies[0])
	f.replies = f.replies[1:]
	return nil
}

func withCRC(hi, lo byte) []byte { return []byte{hi, lo, crc8([]byte{hi, lo})} }

func pair(a, b []byte) []byte { return append(append([]byte(nil), a...), b...) }

func newSensor(t *testing.T) (*Sensor, *fakeBus) {
	t.Helper()
	bus := &fakeBus{replies: [][]byte{pair(withCRC(0x12, 0x34), withCRC(0x56, 0x78))}}
	s, err := New(bus)
	if err != nil {
		t.Fatal(err)
	}
	return s, bus
}

func TestCRCExample(t *testing.T) {
	// From the SHT4x datasheet: CRC of 0xBEEF is 0x92.
	if got := crc8([]byte{0xBE, 0xEF}); got != 0x92 {
		t.Fatalf("crc8 = 0x%02x, want 0x92", got)
	}
}

func TestSerial(t *testing.T) {
	s, bus := newSensor(t)
	bus.replies = [][]byte{pair(withCRC(0x12, 0x34), withCRC(0x56, 0x78))}
	got, err := s.Serial()
	if err != nil || got != 0x12345678 {
		t.Fatalf("Serial = 0x%x, %v", got, err)
	}
}

func TestRead(t *testing.T) {
	s, bus := newSensor(t)
	// raw 0x6666 -> 25 °C (approximately); raw 0x7FFF -> 56.5 %RH.
	bus.replies = [][]byte{pair(withCRC(0x66, 0x66), withCRC(0x7F, 0xFF))}
	r, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(r.Celsius-25) > 0.01 || math.Abs(r.Humidity-56.5) > 0.01 {
		t.Fatalf("got %+v", r)
	}
	if math.Abs(r.Fahrenheit()-77) > 0.02 {
		t.Fatalf("Fahrenheit = %v", r.Fahrenheit())
	}
	if last := bus.writes[len(bus.writes)-1]; !bytes.Equal(last, []byte{0xFD}) {
		t.Errorf("last write % x, want high-precision command", last)
	}
}

func TestPrecisionAndClamp(t *testing.T) {
	s, bus := newSensor(t)
	s.Precision = Low
	bus.replies = [][]byte{pair(withCRC(0x66, 0x66), withCRC(0x00, 0x00))}
	r, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if r.Humidity != 0 {
		t.Errorf("humidity = %v, want clamped to 0", r.Humidity)
	}
	if last := bus.writes[len(bus.writes)-1]; !bytes.Equal(last, []byte{0xE0}) {
		t.Errorf("last write % x, want low-precision command", last)
	}
	s.Precision = 9
	if _, err := s.Read(); err == nil {
		t.Error("accepted an invalid precision")
	}
}

func TestHeat(t *testing.T) {
	s, bus := newSensor(t)
	bus.replies = [][]byte{pair(withCRC(0x66, 0x66), withCRC(0x7F, 0xFF))}
	if _, err := s.Heat(Heater20mW100m); err != nil {
		t.Fatal(err)
	}
	if last := bus.writes[len(bus.writes)-1]; !bytes.Equal(last, []byte{0x15}) {
		t.Errorf("last write % x, want 0x15", last)
	}
	if _, err := s.Heat(Heater(-1)); err == nil {
		t.Error("accepted an invalid heater setting")
	}
}

func TestBadCRC(t *testing.T) {
	s, bus := newSensor(t)
	bus.replies = [][]byte{append(withCRC(0x66, 0x66), 0x7F, 0xFF, 0x00)}
	if _, err := s.Read(); !errors.Is(err, ErrCRC) {
		t.Fatalf("got %v, want ErrCRC", err)
	}
}

func TestNotSHT41(t *testing.T) {
	bus := &fakeBus{replies: [][]byte{{1, 2, 3, 4, 5, 6}}}
	if _, err := New(bus); !errors.Is(err, ErrCRC) {
		t.Fatalf("got %v, want ErrCRC", err)
	}
}
