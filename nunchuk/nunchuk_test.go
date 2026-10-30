package nunchuk

import (
	"bytes"
	"math"
	"testing"
)

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
	if len(f.replies) > 1 {
		f.replies = f.replies[1:]
	}
	return nil
}

var idle = []byte{128, 128, 128, 128, 128, 0x03} // stick centered, no buttons pressed

func TestNew(t *testing.T) {
	bus := &fakeBus{replies: [][]byte{idle}}
	if _, err := New(bus); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bus.writes[0], []byte{0xF0, 0x55}) || !bytes.Equal(bus.writes[1], []byte{0xFB, 0x00}) {
		t.Errorf("init writes = % x, % x", bus.writes[0], bus.writes[1])
	}
	if !bytes.Equal(bus.writes[2], []byte{0x00}) {
		t.Errorf("first read request = % x", bus.writes[2])
	}
}

func TestNotPluggedIn(t *testing.T) {
	bus := &fakeBus{replies: [][]byte{{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}}}
	if _, err := New(bus); err != ErrNoData {
		t.Fatalf("got %v, want ErrNoData", err)
	}
}

func TestRead(t *testing.T) {
	bus := &fakeBus{replies: [][]byte{idle}}
	n, _ := New(bus)

	// Stick right and up, Z pressed (bit 0 clear), C not, and accelerations
	// with distinct low bits: x 0b11, y 0b10, z 0b01.
	bus.replies = [][]byte{{200, 30, 0x80, 0x81, 0x82, 0b11_10_01_1_0}}
	s, err := n.Read()
	if err != nil {
		t.Fatal(err)
	}
	want := State{JoyX: 200, JoyY: 30, AccelX: 0x80<<2 | 3, AccelY: 0x81<<2 | 2, AccelZ: 0x82<<2 | 1,
		Z: true, C: false}
	if s != want {
		t.Errorf("got %+v\nwant %+v", s, want)
	}

	bus.replies = [][]byte{{128, 128, 0, 0, 0, 0b00_00_00_0_1}}
	s, _ = n.Read()
	if !s.C || s.Z {
		t.Errorf("C=%v Z=%v, want C pressed only", s.C, s.Z)
	}
	bus.replies = [][]byte{idle}
	s, _ = n.Read()
	if s.C || s.Z {
		t.Errorf("buttons reported pressed while idle: %+v", s)
	}
}

func TestStick(t *testing.T) {
	x, y := State{JoyX: 128, JoyY: 128}.Stick()
	if x != 0 || y != 0 {
		t.Errorf("centered stick = %v, %v", x, y)
	}
	x, y = State{JoyX: 255, JoyY: 0}.Stick()
	if x != 1 || y != -1 {
		t.Errorf("full right and down = %v, %v, want clamped to 1 and -1", x, y)
	}
	x, _ = State{JoyX: 178}.Stick()
	if math.Abs(x-0.5) > 1e-9 {
		t.Errorf("halfway = %v", x)
	}
}
