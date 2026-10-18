package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ApolloX939/mcp2221/ht16k33"
)

// The progress-pie Claude Code plugin writes a job's progress to a file as
// "done total unix-seconds". While a job is unfinished, claudematrix draws a
// pie in place of the spinner.

// progressStale is how old a progress file may be before it is ignored, so a
// job that was never finished doesn't leave a pie up forever.
const progressStale = 6 * time.Hour

type progress struct {
	done, total int
}

// defaultProgressFile is ~/.claude/progress-pie, where the plugin writes.
func defaultProgressFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "progress-pie")
}

// readProgress reports the unfinished job in path, if there is one.
func readProgress(path string, now time.Time) (progress, bool) {
	if path == "" {
		return progress{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return progress{}, false
	}
	return parseProgress(string(data), now)
}

func parseProgress(text string, now time.Time) (progress, bool) {
	var p progress
	var stamp int64
	if _, err := fmt.Sscan(strings.TrimSpace(text), &p.done, &p.total, &stamp); err != nil {
		return progress{}, false
	}
	if p.total <= 0 || p.done < 0 || p.done > p.total {
		return progress{}, false
	}
	if now.Sub(time.Unix(stamp, 0)) > progressStale {
		return progress{}, false
	}
	return p, true
}

// Pixel kinds of the pie.
const (
	pieOutside = iota // not part of the circle
	pieDone           // a finished part: green
	pieCurrent        // the part being worked on: blinking orange
	pieRim            // the edge of what is left: red
	pieEmpty          // inside what is left: off
)

// piePixel says what (x, y) is in a pie of p, filled clockwise from 12 o'clock.
func piePixel(p progress, x, y int) int {
	dx, dy := float64(x)-3.5, float64(y)-3.5
	if !inCircle(dx, dy) {
		return pieOutside
	}
	// Fraction of a turn clockwise from straight up, 0 to 1.
	turn := math.Atan2(dx, -dy) / (2 * math.Pi)
	if turn < 0 {
		turn++
	}
	switch {
	case turn < float64(p.done)/float64(p.total):
		return pieDone
	case p.done < p.total && turn < float64(p.done+1)/float64(p.total):
		return pieCurrent
	}
	// The rim is any circle pixel with a neighbour outside the circle.
	for _, d := range [][2]float64{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		if !inCircle(dx+d[0], dy+d[1]) {
			return pieRim
		}
	}
	return pieEmpty
}

// inCircle reports whether a pixel centre at offset (dx, dy) from the middle
// of the matrix is inside the pie's circle.
func inCircle(dx, dy float64) bool {
	return dx*dx+dy*dy <= 16
}

// drawPie draws p; step makes the current part blink.
func drawPie(grid *ht16k33.Matrix8x8x2, p progress, step int) {
	blinkOn := step%8 < 5
	for y := range 8 {
		for x := range 8 {
			color := ht16k33.Off
			switch piePixel(p, x, y) {
			case pieDone:
				color = ht16k33.Green
			case pieCurrent:
				if blinkOn {
					color = ht16k33.Orange
				}
			case pieRim:
				color = ht16k33.Red
			}
			grid.Set(x, y, color)
		}
	}
}
