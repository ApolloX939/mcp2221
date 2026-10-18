package main

import (
	"strings"
	"testing"
	"time"
)

func TestSpinnerFramesAre8x8(t *testing.T) {
	for i, frame := range spinner {
		for y, row := range frame {
			if len(row) != 8 {
				t.Errorf("frame %d row %d is %d wide, want 8", i, y, len(row))
			}
			for _, c := range row {
				if c != '.' && c != '#' {
					t.Errorf("frame %d row %d has %q, want only . and #", i, y, c)
				}
			}
		}
	}
}

func TestSpinnerPingPongs(t *testing.T) {
	want := []int{0, 1, 2, 3, 4, 5, 4, 3, 2, 1, 0, 1}
	for step, w := range want {
		if got := spinnerFrame(step); got != w {
			t.Errorf("step %d: frame %d, want %d", step, got, w)
		}
	}
}

func TestParseProgress(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	cases := []struct {
		text string
		want progress
		ok   bool
	}{
		{"2 5 1000000\n", progress{2, 5}, true},
		{"0 3 999990", progress{0, 3}, true},
		{"3 3 1000000", progress{3, 3}, true},
		{"0 0 1000000", progress{}, false}, // cleared
		{"4 3 1000000", progress{}, false},
		{"1 3 900000", progress{}, false}, // stale
		{"", progress{}, false},
		{"junk", progress{}, false},
	}
	for _, c := range cases {
		got, ok := parseProgress(c.text, now)
		if got != c.want || ok != c.ok {
			t.Errorf("parseProgress(%q) = %v, %v; want %v, %v", c.text, got, ok, c.want, c.ok)
		}
	}
}

// pieString draws a pie as text: G done, O current, R rim, . off.
func pieString(p progress) string {
	var b strings.Builder
	for y := range 8 {
		for x := range 8 {
			b.WriteByte(".GORe"[piePixel(p, x, y)])
		}
		b.WriteByte('\n')
	}
	return strings.ReplaceAll(b.String(), "e", ".")
}

// Half done, third quarter in progress (O), last quarter to go (R rim).
func TestPieHalf(t *testing.T) {
	want := "" +
		"..RRGG..\n" +
		".R..GGG.\n" +
		"R...GGGG\n" +
		"R...GGGG\n" +
		"OOOOGGGG\n" +
		"OOOOGGGG\n" +
		".OOOGGG.\n" +
		"..OOGG..\n"
	if got := pieString(progress{2, 4}); got != want {
		t.Errorf("2 of 4:\n%swant\n%s", got, want)
	}
}

func TestPieEnds(t *testing.T) {
	full := pieString(progress{3, 3})
	if strings.ContainsAny(full, "ORe") {
		t.Errorf("3 of 3 should be all green:\n%s", full)
	}
	start := pieString(progress{0, 4})
	if strings.Contains(start, "G") || !strings.Contains(start, "O") {
		t.Errorf("0 of 4 should have no green and a current part:\n%s", start)
	}
}
