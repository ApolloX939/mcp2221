package ht16k33

import "testing"

type nopBus struct{}

func (nopBus) I2CWrite(byte, []byte) error { return nil }

func TestSetRotation(t *testing.T) {
	// The top-left pixel lands in each corner in turn, going clockwise.
	corners := map[int][2]int{0: {0, 0}, 90: {7, 0}, 180: {7, 7}, 270: {0, 7}}
	for deg, want := range corners {
		m, _ := NewMatrix8x8x2(nopBus{}, 0x70, 0)
		if err := m.SetRotation(deg); err != nil {
			t.Fatal(err)
		}
		m.Set(0, 0, Green)
		ref, _ := NewMatrix8x8x2(nopBus{}, 0x70, 0)
		ref.Set(want[0], want[1], Green)
		if m.buf != ref.buf {
			t.Errorf("rotation %d: pixel not at %v", deg, want)
		}
	}
	m, _ := NewMatrix8x8x2(nopBus{}, 0x70, 0)
	if err := m.SetRotation(45); err == nil {
		t.Error("SetRotation(45) should fail")
	}
}
