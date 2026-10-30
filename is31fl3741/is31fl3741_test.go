package is31fl3741

import (
	"bytes"
	"testing"
)

// fakeBus models the chip's paged registers.
type fakeBus struct {
	id    byte
	page  byte
	ptr   byte
	pages [5][256]byte
}

func (f *fakeBus) I2CWrite(addr byte, data []byte) error {
	switch data[0] {
	case regCommandLock:
		return nil
	case regCommand:
		f.page = data[1]
		return nil
	}
	f.ptr = data[0]
	copy(f.pages[f.page][f.ptr:], data[1:])
	return nil
}

func (f *fakeBus) I2CRead(addr byte, buf []byte) error {
	if f.page == pageFunc && f.ptr == regID {
		buf[0] = f.id
	}
	return nil
}

func TestNew(t *testing.T) {
	bus := &fakeBus{id: 0x60}
	if _, err := New(bus, 0x30, 0.5); err != nil {
		t.Fatal(err)
	}
	if got := bus.pages[pageFunc][regConfig]; got != configOn {
		t.Errorf("config = 0x%02x, want normal operation", got)
	}
	if got := bus.pages[pageFunc][regGlobal]; got != 128 {
		t.Errorf("global current = %d, want 128", got)
	}
	if got := bus.pages[pageScale1][channels-pwm0Size-1]; got != 0xFF {
		t.Errorf("last scaling register = 0x%02x, want 0xFF", got)
	}
}

func TestWrongChip(t *testing.T) {
	if _, err := New(&fakeBus{id: 0x00}, 0x30, 1); err == nil {
		t.Fatal("accepted a wrong ID")
	}
}

func TestSet(t *testing.T) {
	m, err := New(&fakeBus{id: 0x60}, 0x30, 1)
	if err != nil {
		t.Fatal(err)
	}
	// Channel numbers worked out from Adafruit's Adafruit_IS31FL3741_QT
	// drawPixel: the pixel is set to red 1, green 2, blue 3.
	tests := []struct {
		x, y          int
		rch, gch, bch int
	}{
		{0, 6, 2, 1, 0},        // chip row 0, even column: blue, green, red
		{1, 6, 4, 3, 5},        // odd column: red, green, blue shifted by one
		{0, 5, 32, 31, 30},     // chip row 1
		{0, 0, 242, 241, 240},  // chip row 8
		{10, 6, 272, 271, 270}, // columns 10-12 come after the first 10
		{12, 0, 349, 348, 350}, // last column is shifted like odd ones
	}
	for _, tt := range tests {
		m.buf = [channels]byte{}
		m.Set(tt.x, tt.y, 1, 2, 3)
		if m.buf[tt.rch] != 1 || m.buf[tt.gch] != 2 || m.buf[tt.bch] != 3 {
			t.Errorf("(%d,%d): channels %d,%d,%d hold %d,%d,%d, want 1,2,3", tt.x, tt.y,
				tt.rch, tt.gch, tt.bch, m.buf[tt.rch], m.buf[tt.gch], m.buf[tt.bch])
		}
	}
}

func TestEveryPixelIsOwnChannels(t *testing.T) {
	m, _ := New(&fakeBus{id: 0x60}, 0x30, 1)
	seen := map[int]bool{}
	for y := range Height {
		for x := range Width {
			m.buf = [channels]byte{}
			m.Set(x, y, 1, 1, 1)
			for i, v := range m.buf {
				if v == 0 {
					continue
				}
				if seen[i] {
					t.Fatalf("channel %d used twice, again by (%d,%d)", i, x, y)
				}
				seen[i] = true
			}
		}
	}
	if len(seen) != channels {
		t.Errorf("%d channels used, want %d", len(seen), channels)
	}
}

func TestShow(t *testing.T) {
	bus := &fakeBus{id: 0x60}
	m, err := New(bus, 0x30, 1)
	if err != nil {
		t.Fatal(err)
	}
	m.Set(13, 0, 9, 9, 9) // out of range, ignored
	m.Set(0, -1, 9, 9, 9)
	m.Set(0, 6, 1, 2, 3)  // channels 0-2, on the first PWM page
	m.Set(12, 0, 4, 5, 6) // channels 348-350, on the second
	if err := m.Show(); err != nil {
		t.Fatal(err)
	}
	if got := bus.pages[pagePWM0][:3]; !bytes.Equal(got, []byte{3, 2, 1}) {
		t.Errorf("first channels = % x", got)
	}
	if got := bus.pages[pagePWM1][168:171]; !bytes.Equal(got, []byte{5, 4, 6}) {
		t.Errorf("last channels = % x", got)
	}
	for _, p := range []byte{pagePWM0, pagePWM1} {
		n := 0
		for _, v := range bus.pages[p] {
			if v != 0 {
				n++
			}
		}
		if n != 3 {
			t.Errorf("page %d has %d lit channels, want 3 (out-of-range pixels must be ignored)", p, n)
		}
	}
}
