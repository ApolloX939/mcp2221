package ft232h

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// usbfs talks to the FT232H through Linux's raw USB device files
// (/dev/bus/usb/BBB/DDD), with no cgo and no libusb.
type usbfs struct {
	f    *os.File
	pkt  []byte // receive buffer
	left []byte // bytes received but not yet returned
}

// usbfs ioctl structures, laid out like the kernel's.
type ctrlTransfer struct {
	requestType uint8
	request     uint8
	value       uint16
	index       uint16
	length      uint16
	timeout     uint32 // ms
	data        unsafe.Pointer
}

type bulkTransfer struct {
	ep      uint32
	len     uint32
	timeout uint32 // ms
	data    unsafe.Pointer
}

type disconnectClaim struct {
	iface  uint32
	flags  uint32
	driver [256]byte
}

type usbIoctl struct {
	ifno int32
	code int32
	data unsafe.Pointer
}

// Linux _IOC encoding.
func ioc(dir, nr, size uintptr) uintptr { return dir<<30 | size<<16 | 'U'<<8 | nr }

const (
	iocNone  = 0
	iocWrite = 1
	iocRead  = 2
)

var (
	usbdevfsControl          = ioc(iocRead|iocWrite, 0, unsafe.Sizeof(ctrlTransfer{}))
	usbdevfsBulk             = ioc(iocRead|iocWrite, 2, unsafe.Sizeof(bulkTransfer{}))
	usbdevfsReleaseInterface = ioc(iocRead, 16, 4)
	usbdevfsIoctl            = ioc(iocRead|iocWrite, 18, unsafe.Sizeof(usbIoctl{}))
	usbdevfsConnect          = ioc(iocNone, 23, 0)
	usbdevfsDisconnectClaim  = ioc(iocRead, 27, unsafe.Sizeof(disconnectClaim{}))
)

const disconnectExceptDriver = 0x02

const (
	iface  = 0    // the FT232H has one interface
	epOut  = 0x02 // bulk OUT
	epIn   = 0x81 // bulk IN
	inSize = 512  // high-speed bulk packet size
)

func ioctl(fd uintptr, req uintptr, arg unsafe.Pointer) error {
	for {
		_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg))
		switch errno {
		case 0:
			return nil
		case syscall.EINTR:
			continue
		default:
			return errno
		}
	}
}

// openUSB finds the one FT232H on the system, detaches the kernel's serial
// driver (ftdi_sio) from it and claims it.
func openUSB() (transport, error) {
	paths, err := findDevices()
	if err != nil {
		return nil, err
	}
	switch len(paths) {
	case 0:
		return nil, ErrNoDevice
	case 1:
	default:
		return nil, fmt.Errorf("ft232h: %d FT232H adapters found, expected one", len(paths))
	}

	f, err := os.OpenFile(paths[0], os.O_RDWR, 0)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return nil, fmt.Errorf("ft232h: %w (see the README for a udev rule)", err)
		}
		return nil, fmt.Errorf("ft232h: %w", err)
	}
	u := &usbfs{f: f, pkt: make([]byte, inSize)}
	fd := f.Fd()

	// Detach ftdi_sio (the /dev/ttyUSB driver) and claim the interface in
	// one step. Another program holding it through usbfs is left alone, and
	// the claim fails with EBUSY.
	dc := disconnectClaim{iface: iface, flags: disconnectExceptDriver}
	copy(dc.driver[:], "usbfs")
	if err := ioctl(fd, usbdevfsDisconnectClaim, unsafe.Pointer(&dc)); err != nil {
		f.Close()
		if err == syscall.EBUSY {
			return nil, ErrInUse
		}
		return nil, fmt.Errorf("ft232h: claim interface: %w", err)
	}
	return u, nil
}

// findDevices returns the usbfs paths of all attached FT232Hs.
func findDevices() ([]string, error) {
	dirs, err := filepath.Glob("/sys/bus/usb/devices/*")
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, dir := range dirs {
		if readHex(dir, "idVendor") != VendorID || readHex(dir, "idProduct") != ProductID {
			continue
		}
		bus, err1 := readInt(dir, "busnum")
		dev, err2 := readInt(dir, "devnum")
		if err1 != nil || err2 != nil {
			continue
		}
		paths = append(paths, fmt.Sprintf("/dev/bus/usb/%03d/%03d", bus, dev))
	}
	return paths, nil
}

func readHex(dir, name string) int {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return -1
	}
	v, err := strconv.ParseUint(strings.TrimSpace(string(b)), 16, 16)
	if err != nil {
		return -1
	}
	return int(v)
}

func readInt(dir, name string) (int, error) {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(b)))
}

func (u *usbfs) control(request uint8, value, index uint16) error {
	c := ctrlTransfer{requestType: 0x40, request: request, value: value, index: index, timeout: 1000}
	if err := ioctl(u.f.Fd(), usbdevfsControl, unsafe.Pointer(&c)); err != nil {
		return fmt.Errorf("ft232h: control request 0x%02x: %w", request, err)
	}
	return nil
}

func (u *usbfs) write(b []byte) error {
	for len(b) > 0 {
		t := bulkTransfer{ep: epOut, len: uint32(len(b)), timeout: 1000, data: unsafe.Pointer(&b[0])}
		n, _, errno := syscall.Syscall(syscall.SYS_IOCTL, u.f.Fd(), usbdevfsBulk, uintptr(unsafe.Pointer(&t)))
		if errno == syscall.EINTR {
			continue
		}
		if errno != 0 {
			return fmt.Errorf("ft232h: write: %w", errno)
		}
		b = b[n:]
	}
	return nil
}

// read fills buf with data from the chip, dropping the two modem-status
// bytes at the start of every USB packet. Extra bytes are kept for the
// next read.
func (u *usbfs) read(buf []byte, timeout time.Duration) error {
	pkt := u.pkt
	deadline := time.Now().Add(timeout)
	got := copy(buf, u.left)
	u.left = u.left[got:]
	for got < len(buf) {
		left := time.Until(deadline)
		if left <= 0 {
			return ErrTimeout
		}
		t := bulkTransfer{ep: epIn, len: inSize, timeout: uint32(left.Milliseconds()) + 1, data: unsafe.Pointer(&pkt[0])}
		n, _, errno := syscall.Syscall(syscall.SYS_IOCTL, u.f.Fd(), usbdevfsBulk, uintptr(unsafe.Pointer(&t)))
		switch errno {
		case 0:
		case syscall.EINTR:
			continue
		case syscall.ETIMEDOUT:
			return ErrTimeout
		default:
			return fmt.Errorf("ft232h: read: %w", errno)
		}
		for p := 0; p < int(n); p += inSize {
			end := min(p+inSize, int(n))
			if end-p > 2 {
				data := pkt[p+2 : end]
				c := copy(buf[got:], data)
				got += c
				u.left = append(u.left, data[c:]...)
			}
		}
	}
	return nil
}

// discard drops anything received but not yet read.
func (u *usbfs) discard() {
	u.left = nil
	_ = u.control(0x00, 1, 1) // purge RX
}

// close releases the chip and gives it back to the serial driver.
func (u *usbfs) close() error {
	fd := u.f.Fd()
	n := uint32(iface)
	_ = ioctl(fd, usbdevfsReleaseInterface, unsafe.Pointer(&n))
	c := usbIoctl{ifno: iface, code: int32(usbdevfsConnect)}
	_ = ioctl(fd, usbdevfsIoctl, unsafe.Pointer(&c))
	return u.f.Close()
}
