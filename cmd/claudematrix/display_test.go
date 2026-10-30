package main

import "testing"

func TestIsRGBAddr(t *testing.T) {
	for addr, want := range map[byte]bool{0x2f: false, 0x30: true, 0x33: true, 0x34: false, 0x70: false, 0x77: false} {
		if got := isRGBAddr(addr); got != want {
			t.Errorf("isRGBAddr(0x%02x) = %t, want %t", addr, got, want)
		}
	}
}
