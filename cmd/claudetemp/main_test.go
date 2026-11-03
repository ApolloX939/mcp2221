package main

import "testing"

func TestFormat(t *testing.T) {
	for _, c := range []struct {
		celsius float64
		unit    string
		want    string
	}{
		{21.5, "C", "21.5"},
		{21.5, "F", "70.7"},
		{0, "F", "32.0"},
		{-40, "F", "-40.0"},
		{21.54, "C", "21.5"},
	} {
		if got := format(c.celsius, c.unit); got != c.want {
			t.Errorf("format(%v, %s) = %s, want %s", c.celsius, c.unit, got, c.want)
		}
	}
}
