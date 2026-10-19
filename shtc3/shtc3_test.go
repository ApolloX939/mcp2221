package shtc3

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

func TestCRCExample(t *testing.T) {
	// From the SHTC3 datasheet: CRC of 0xBEEF is 0x92.
	if got := crc8([]byte{0xBE, 0xEF}); got != 0x92 {
		t.Fatalf("crc8 = 0x%02x, want 0x92", got)
	}
}

func TestRead(t *testing.T) {
	bus := &fakeBus{replies: [][]byte{withCRC(0x08, 0x87)}}
	s, err := New(bus)
	if err != nil {
		t.Fatal(err)
	}
	// raw 0x6666 -> 25 °C; raw 0x8000 -> 50 %RH.
	bus.replies = [][]byte{append(withCRC(0x66, 0x66), withCRC(0x80, 0x00)...)}
	r, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(r.Celsius-25) > 0.01 || r.Humidity != 50 {
		t.Fatalf("got %+v", r)
	}
	if math.Abs(r.Fahrenheit()-77) > 0.02 {
		t.Fatalf("Fahrenheit = %v", r.Fahrenheit())
	}
	last := bus.writes[len(bus.writes)-1]
	if !bytes.Equal(last, cmdSleep) {
		t.Errorf("sensor not put to sleep, last write % x", last)
	}
}

func TestNotSHTC3(t *testing.T) {
	bus := &fakeBus{replies: [][]byte{withCRC(0x12, 0x34)}}
	if _, err := New(bus); err == nil {
		t.Fatal("accepted a wrong ID")
	}
}

func TestBadCRC(t *testing.T) {
	bus := &fakeBus{replies: [][]byte{withCRC(0x08, 0x87)}}
	s, _ := New(bus)
	bad := append(withCRC(0x66, 0x66), 0x80, 0x00, 0x00)
	bus.replies = [][]byte{bad}
	if _, err := s.Read(); !errors.Is(err, ErrCRC) {
		t.Fatalf("got %v, want ErrCRC", err)
	}
}
