package main

import (
	"testing"
	"time"
)

func TestParseLimit(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	cases := []struct {
		text string
		want time.Duration
		ok   bool
	}{
		{"1003600\n", time.Hour, true},
		{"1000000", 0, false}, // resets now
		{"999000", 0, false},  // already reset
		{"0", 0, false},       // no limit hit
		{"", 0, false},
		{"junk", 0, false},
	}
	for _, c := range cases {
		got, ok := parseLimit(c.text, now)
		if got != c.want || ok != c.ok {
			t.Errorf("parseLimit(%q) = %v, %v; want %v, %v", c.text, got, ok, c.want, c.ok)
		}
	}
}

func TestFaceFor(t *testing.T) {
	cases := []struct {
		left time.Duration
		want clockFace
	}{
		{3*time.Hour + 42*time.Minute + 30*time.Second, clockFace{3, 42, unitMinutes}},
		{4*time.Hour + 59*time.Minute, clockFace{4, 59, unitMinutes}},
		{45 * time.Second, clockFace{0, 0, unitMinutes}},
		{5 * time.Hour, clockFace{0, 5, unitHours}},
		{99*time.Hour + 59*time.Minute, clockFace{0, 99, unitHours}},
		{100 * time.Hour, clockFace{0, 4, unitDays}},
	}
	for _, c := range cases {
		if got := faceFor(c.left); got != c.want {
			t.Errorf("faceFor(%v) = %+v; want %+v", c.left, got, c.want)
		}
	}
}
