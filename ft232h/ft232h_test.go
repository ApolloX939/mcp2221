package ft232h

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

// fakeChip plays back canned replies and records what was written.
type fakeChip struct {
	written []byte
	replies []byte
}

func (f *fakeChip) control(uint8, uint16, uint16) error { return nil }
func (f *fakeChip) write(b []byte) error                { f.written = append(f.written, b...); return nil }
func (f *fakeChip) discard()                            {}
func (f *fakeChip) close() error                        { return nil }
func (f *fakeChip) read(buf []byte, _ time.Duration) error {
	if len(f.replies) < len(buf) {
		return ErrTimeout
	}
	copy(buf, f.replies)
	f.replies = f.replies[len(buf):]
	return nil
}

func newFake(replies ...byte) (*Device, *fakeChip) {
	f := &fakeChip{replies: replies}
	return &Device{usb: f, hold: 4}, f
}

func TestWriteSendsAddressAndData(t *testing.T) {
	d, f := newFake(0, 0, 0) // ACK address and both bytes
	if err := d.I2CWrite(0x70, []byte{0x21, 0x81}); err != nil {
		t.Fatal(err)
	}
	for _, b := range []byte{0x70 << 1, 0x21, 0x81} {
		if !bytes.Contains(f.written, []byte{mpsseWriteBytesNeg, 0, 0, b}) {
			t.Errorf("byte 0x%02x not sent", b)
		}
	}
	// Bus ends idle.
	if !bytes.HasSuffix(f.written, []byte{mpsseSetLow, pinSCL | pinSDA, pinDir}) {
		t.Error("no stop at the end")
	}
}

func TestAddressNACK(t *testing.T) {
	d, _ := newFake(1, 1)
	err := d.I2CWrite(0x42, []byte{0x00})
	if !errors.Is(err, ErrNACK) {
		t.Fatalf("got %v, want ErrNACK", err)
	}
}

func TestDataNACK(t *testing.T) {
	d, _ := newFake(0, 1)
	err := d.I2CWrite(0x42, []byte{0x00})
	if err == nil || errors.Is(err, ErrNACK) {
		t.Fatalf("got %v, want a data NACK error", err)
	}
}

func TestTxReadsWithRepeatedStart(t *testing.T) {
	// ACK address, ACK register, ACK read address, then two data bytes.
	d, f := newFake(0, 0, 0, 0xAB, 0xCD)
	buf := make([]byte, 2)
	if err := d.I2CTx(0x50, []byte{0x10}, buf); err != nil {
		t.Fatal(err)
	}
	if buf[0] != 0xAB || buf[1] != 0xCD {
		t.Fatalf("read % x", buf)
	}
	if !bytes.Contains(f.written, []byte{mpsseWriteBytesNeg, 0, 0, 0x50<<1 | 1}) {
		t.Error("read address not sent")
	}
	// First byte ACKed, last NACKed.
	if !bytes.Contains(f.written, []byte{mpsseWriteBitsNeg, 0, 0x00}) ||
		!bytes.Contains(f.written, []byte{mpsseWriteBitsNeg, 0, 0xFF}) {
		t.Error("missing ACK/NACK after read bytes")
	}
}

func TestScan(t *testing.T) {
	// 0x08..0x77: only 0x70 answers.
	var replies []byte
	for a := 0x08; a <= 0x77; a++ {
		if a == 0x70 {
			replies = append(replies, 0, 0x55)
		} else {
			replies = append(replies, 1)
		}
	}
	d, _ := newFake(replies...)
	found, err := d.Scan()
	if err != nil || len(found) != 1 || found[0] != 0x70 {
		t.Fatalf("Scan = %x, %v", found, err)
	}
}

func TestRejectsBadArgs(t *testing.T) {
	d, _ := newFake()
	if err := d.I2CWrite(0x80, []byte{1}); err == nil {
		t.Error("8-bit address accepted")
	}
	if err := d.I2CTx(0x10, nil, nil); err == nil {
		t.Error("empty transfer accepted")
	}
	if err := d.SetI2CSpeed(5_000_000); err == nil {
		t.Error("5 MHz accepted")
	}
}
