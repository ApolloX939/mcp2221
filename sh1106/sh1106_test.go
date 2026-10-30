package sh1106

import (
	"bytes"
	"testing"
)

type fakeBus struct{ writes [][]byte }

func (f *fakeBus) I2CWrite(addr byte, data []byte) error {
	f.writes = append(f.writes, append([]byte(nil), data...))
	return nil
}

func TestNew(t *testing.T) {
	bus := &fakeBus{}
	if _, err := New(bus, DefaultAddr); err != nil {
		t.Fatal(err)
	}
	first := bus.writes[0]
	if first[0] != ctrlCommand || first[1] != cmdDisplayOff {
		t.Errorf("init starts % x, want a command write beginning with display off", first[:2])
	}
	last := bus.writes[len(bus.writes)-1]
	if !bytes.Equal(last, []byte{ctrlCommand, cmdDisplayOn}) {
		t.Errorf("last write % x, want display on", last)
	}
}

func TestSetGetFill(t *testing.T) {
	d, _ := New(&fakeBus{}, DefaultAddr)
	d.Set(0, 0, true)
	d.Set(127, 63, true)
	d.Set(5, 9, true) // second page, bit 1
	d.Set(-1, 0, true)
	d.Set(128, 0, true)
	d.Set(0, 64, true)
	if d.buf[0] != 0x01 || d.buf[7*Width+127] != 0x80 || d.buf[Width+5] != 0x02 {
		t.Errorf("wrong bits: %02x %02x %02x", d.buf[0], d.buf[7*Width+127], d.buf[Width+5])
	}
	n := 0
	for _, b := range d.buf {
		for ; b != 0; b &= b - 1 {
			n++
		}
	}
	if n != 3 {
		t.Errorf("%d pixels on, want 3 (out-of-range pixels must be ignored)", n)
	}
	if !d.Get(5, 9) || d.Get(5, 8) || d.Get(-1, -1) {
		t.Error("Get disagrees with Set")
	}
	d.Set(5, 9, false)
	if d.Get(5, 9) {
		t.Error("pixel still on after Set false")
	}
	d.Fill(true)
	if !d.Get(64, 32) {
		t.Error("Fill(true) left a pixel off")
	}
	d.Fill(false)
	if d.Get(64, 32) {
		t.Error("Fill(false) left a pixel on")
	}
}

func TestShow(t *testing.T) {
	bus := &fakeBus{}
	d, _ := New(bus, DefaultAddr)
	d.Set(3, 17, true) // page 2, column 3
	bus.writes = nil
	if err := d.Show(); err != nil {
		t.Fatal(err)
	}
	if len(bus.writes) != 16 {
		t.Fatalf("%d writes, want 16 (a command and a data write per page)", len(bus.writes))
	}
	for p := 0; p < 8; p++ {
		cmd, data := bus.writes[2*p], bus.writes[2*p+1]
		// The page, then the starting column 0 as high and low nibbles.
		if !bytes.Equal(cmd, []byte{ctrlCommand, 0xB0 + byte(p), 0x10, 0x00}) {
			t.Errorf("page %d address = % x", p, cmd)
		}
		if len(data) != 1+Width || data[0] != ctrlData {
			t.Fatalf("page %d data is %d bytes starting 0x%02x", p, len(data), data[0])
		}
		want := byte(0)
		if p == 2 {
			want = 0x02
		}
		if data[1+3] != want {
			t.Errorf("page %d column 3 = 0x%02x, want 0x%02x", p, data[1+3], want)
		}
	}
}

func TestSettings(t *testing.T) {
	bus := &fakeBus{}
	d, _ := New(bus, DefaultAddr)
	bus.writes = nil
	d.SetContrast(0x20)
	d.SetInvert(true)
	d.SetInvert(false)
	d.SetPower(false)
	d.SetPower(true)
	want := [][]byte{
		{ctrlCommand, cmdSetContrast, 0x20},
		{ctrlCommand, cmdInverse},
		{ctrlCommand, cmdNormal},
		{ctrlCommand, cmdDisplayOff},
		{ctrlCommand, cmdDisplayOn},
	}
	if len(bus.writes) != len(want) {
		t.Fatalf("%d writes, want %d", len(bus.writes), len(want))
	}
	for i := range want {
		if !bytes.Equal(bus.writes[i], want[i]) {
			t.Errorf("write %d = % x, want % x", i, bus.writes[i], want[i])
		}
	}
}

func TestSetRotation(t *testing.T) {
	bus := &fakeBus{}
	d, _ := New(bus, DefaultAddr)
	if err := d.SetRotation(90); err == nil {
		t.Error("accepted a 90 degree rotation")
	}

	bus.writes = nil
	if err := d.SetRotation(180); err != nil {
		t.Fatal(err)
	}
	// The flip command, then the whole buffer again from column 4.
	if !bytes.Equal(bus.writes[0], []byte{ctrlCommand, cmdSegRemap, cmdComScanInc}) {
		t.Errorf("first write = % x", bus.writes[0])
	}
	if len(bus.writes) != 1+16 {
		t.Fatalf("%d writes, want the flip and a full redraw", len(bus.writes))
	}
	if want := []byte{ctrlCommand, 0xB0, 0x10, 0x04}; !bytes.Equal(bus.writes[1], want) {
		t.Errorf("page address after rotating = % x, want % x", bus.writes[1], want)
	}

	bus.writes = nil
	if err := d.SetRotation(0); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bus.writes[0], []byte{ctrlCommand, cmdSegRemap | 1, cmdComScanDec}) {
		t.Errorf("first write = % x", bus.writes[0])
	}
	if want := []byte{ctrlCommand, 0xB0, 0x10, 0x00}; !bytes.Equal(bus.writes[1], want) {
		t.Errorf("page address after rotating back = % x, want % x", bus.writes[1], want)
	}
}
