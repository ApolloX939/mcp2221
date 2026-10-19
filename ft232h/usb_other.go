//go:build !linux

package ft232h

import "errors"

func openUSB() (transport, error) {
	return nil, errors.New("ft232h: only supported on Linux")
}
