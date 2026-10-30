package tmf8806

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

const regPrevious = 0x11 // the last command the sensor ran

// fakeBus models enough of the TMF8806's register file for the driver.
type fakeBus struct {
	regs   [256]byte
	ptr    byte
	writes [][]byte
}

func newFake() *fakeBus {
	f := &fakeBus{}
	f.regs[regID] = chipID
	return f
}

func (f *fakeBus) I2CWrite(addr byte, data []byte) error {
	f.writes = append(f.writes, append([]byte(nil), data...))
	f.ptr = data[0]
	for i, v := range data[1:] {
		f.regs[int(f.ptr)+i] = v
	}
	if len(data) == 1 {
		return nil
	}
	end := int(f.ptr) + len(data) - 1
	switch {
	case f.ptr == regEnable:
		if f.regs[regEnable]&enablePower != 0 {
			f.regs[regEnable] |= enableCPUReady
		}
	case f.ptr == regAppRequest:
		f.regs[regAppID] = f.regs[regAppRequest]
	case f.ptr == regIntStatus:
		// write-one-to-clear
		f.regs[regIntStatus] = 0
	case int(f.ptr) <= regCommand && regCommand < end:
		// A command was written: the sensor runs it at once.
		f.regs[regPrevious] = f.regs[regCommand]
		f.regs[regCommand] = cmdIdle
	}
	return nil
}

func (f *fakeBus) I2CRead(addr byte, buf []byte) error {
	copy(buf, f.regs[f.ptr:])
	return nil
}

func (f *fakeBus) lastWriteTo(reg byte) []byte {
	for i := len(f.writes) - 1; i >= 0; i-- {
		if f.writes[i][0] == reg && len(f.writes[i]) > 1 {
			return f.writes[i][1:]
		}
	}
	return nil
}

func TestNew(t *testing.T) {
	bus := newFake()
	if _, err := New(bus); err != nil {
		t.Fatal(err)
	}
	if bus.regs[regAppID] != appMeasurement {
		t.Errorf("application = 0x%02x, want measurement", bus.regs[regAppID])
	}
	if got := bus.regs[regIntEnable]; got != intResult {
		t.Errorf("interrupt enable = 0x%02x", got)
	}
}

func TestWrongChip(t *testing.T) {
	bus := newFake()
	bus.regs[regID] = 0x12
	if _, err := New(bus); err == nil {
		t.Fatal("accepted a wrong ID")
	}
}

func TestStartCommand(t *testing.T) {
	bus := newFake()
	s, _ := New(bus)
	s.Mode = Range5m
	s.Iterations = 0x0123
	if err := s.Start(true); err != nil {
		t.Fatal(err)
	}
	want := []byte{0, 0, 0, 0x0E, 0, 0, 6, 33, 0x23, 0x01, cmdMeasure}
	if got := bus.lastWriteTo(regCmdData); !bytes.Equal(got, want) {
		t.Errorf("command block = % x, want % x", got, want)
	}

	s.Mode = ShortRange
	if err := s.Start(false); err != nil {
		t.Fatal(err)
	}
	if got := bus.lastWriteTo(regCmdData); got[3] != 0 || got[7] != 0 {
		t.Errorf("short-range single shot block = % x", got)
	}
}

func TestStartRejectsBadSettings(t *testing.T) {
	s, _ := New(newFake())
	s.Iterations = 5
	if s.Start(true) == nil {
		t.Error("accepted Iterations 5")
	}
	s.Iterations = 400
	s.Period = 0
	if s.Start(true) == nil {
		t.Error("accepted a zero Period")
	}
	s.Mode = 9
	if s.Start(false) == nil {
		t.Error("accepted an invalid Mode")
	}
}

func frame(number, quality byte, dist uint16, temp int8) []byte {
	f := make([]byte, frameSize)
	f[2] = contentsResult
	f[4] = number
	f[5] = quality
	binary.LittleEndian.PutUint16(f[6:], dist)
	binary.LittleEndian.PutUint32(f[8:], 1234)
	f[23] = byte(temp)
	binary.LittleEndian.PutUint32(f[24:], 100)
	binary.LittleEndian.PutUint32(f[28:], 200)
	binary.LittleEndian.PutUint16(f[32:], 7)
	return f
}

func TestRead(t *testing.T) {
	bus := newFake()
	s, _ := New(bus)
	if _, err := s.Read(time.Millisecond); err == nil {
		t.Fatal("Read worked before Start")
	}
	s.Start(true)

	if _, err := s.Read(5 * time.Millisecond); err == nil {
		t.Fatal("Read returned with no result ready")
	}

	copy(bus.regs[regState:], frame(9, 2<<6|40, 1500, -3))
	bus.regs[regIntStatus] = intResult
	r, err := s.Read(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	want := Result{Number: 9, Distance: 1500, Reliability: 40, Status: 2, Clock: 1234,
		ReferenceHits: 100, ObjectHits: 200, Temperature: -3, Crosstalk: 7}
	if r != want {
		t.Errorf("got %+v\nwant %+v", r, want)
	}
	if ok, _ := s.Ready(); ok {
		t.Error("result still marked ready after reading it")
	}
}

func TestOutOfRangeIsInvalid(t *testing.T) {
	bus := newFake()
	s, _ := New(bus)
	s.Mode = ShortRange
	s.Start(true)
	copy(bus.regs[regState:], frame(1, 50, 1000, 20))
	bus.regs[regIntStatus] = intResult
	r, err := s.Read(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if r.Distance != 0 || r.Reliability != 0 {
		t.Errorf("a 1000 mm result in short range was kept: %+v", r)
	}
}

func TestSerial(t *testing.T) {
	bus := newFake()
	s, _ := New(bus)
	bus.regs[regContents] = contentsSerial
	binary.LittleEndian.PutUint32(bus.regs[regResult+8:], 0xDEADBEEF)
	got, err := s.Serial()
	if err != nil || got != 0xDEADBEEF {
		t.Fatalf("Serial = 0x%x, %v", got, err)
	}
}

func TestSleepWake(t *testing.T) {
	bus := newFake()
	s, _ := New(bus)
	if err := s.Sleep(); err != nil {
		t.Fatal(err)
	}
	if bus.regs[regEnable]&enablePower != 0 {
		t.Error("CPU still powered after Sleep")
	}
	bus.regs[regAppID] = 0 // the application is lost while asleep
	if err := s.Wake(); err != nil {
		t.Fatal(err)
	}
	if bus.regs[regAppID] != appMeasurement {
		t.Error("measurement application not restarted by Wake")
	}
}
