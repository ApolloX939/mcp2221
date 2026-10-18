package main

import "testing"

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
