package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ApolloX939/mcp2221/ht16k33"
)

// The led-matrix Claude Code plugin writes the time the usage limit resets to
// a file, as unix seconds ("0" when no limit is hit). Until then, claudematrix
// draws a countdown to the reset in place of the pie and the spinner.

// defaultLimitFile is ~/.claude/usage-limit, where the plugin writes.
func defaultLimitFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "usage-limit")
}

// readLimit reports how long until the limit in path resets, if it hasn't yet.
func readLimit(path string, now time.Time) (time.Duration, bool) {
	if path == "" {
		return 0, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	return parseLimit(string(data), now)
}

func parseLimit(text string, now time.Time) (time.Duration, bool) {
	var stamp int64
	if _, err := fmt.Sscan(strings.TrimSpace(text), &stamp); err != nil || stamp <= 0 {
		return 0, false
	}
	left := time.Unix(stamp, 0).Sub(now)
	if left <= 0 {
		return 0, false
	}
	return left, true
}

// 3x5 digits.
var digits = [10][5]string{
	{"###", "#.#", "#.#", "#.#", "###"},
	{".#.", "##.", ".#.", ".#.", "###"},
	{"###", "..#", "###", "#..", "###"},
	{"###", "..#", ".##", "..#", "###"},
	{"#.#", "#.#", "###", "..#", "..#"},
	{"###", "#..", "###", "..#", "###"},
	{"###", "#..", "###", "#.#", "###"},
	{"###", "..#", ".#.", ".#.", ".#."},
	{"###", "#.#", "###", "#.#", "###"},
	{"###", "#.#", "###", "..#", "###"},
}

// clockFace is what the countdown shows: a number of hour dots along the top,
// and a two-digit number in a colour that says its unit.
type clockFace struct {
	dots   int // hours, as dots on the top row (0 to 4)
	number int // 0 to 99
	unit   byte
}

// Units of the big number.
const (
	unitMinutes = ht16k33.Red
	unitHours   = ht16k33.Orange
	unitDays    = ht16k33.Green
)

// faceFor picks the face for the time left: under 5 hours, hours as dots and
// minutes as the number (3 dots and 42 is 3:42); under 100 hours, the hours;
// beyond that, the days.
func faceFor(left time.Duration) clockFace {
	minutes := int(left / time.Minute)
	hours := minutes / 60
	switch {
	case hours < 5:
		return clockFace{dots: hours, number: minutes % 60, unit: unitMinutes}
	case hours < 100:
		return clockFace{number: hours, unit: unitHours}
	default:
		return clockFace{number: min(hours/24, 99), unit: unitDays}
	}
}

// drawClock draws the time left. A dot right of the number blinks once a
// second, so a frozen display is easy to tell from a slow countdown.
func drawClock(grid *ht16k33.Matrix8x8x2, left time.Duration, now time.Time) {
	grid.Fill(ht16k33.Off)
	face := faceFor(left)
	for i := range face.dots {
		grid.Set(2*i, 0, ht16k33.Orange)
	}
	drawDigit(grid, face.number/10, 0, 2, face.unit)
	drawDigit(grid, face.number%10, 4, 2, face.unit)
	if now.Nanosecond() < int(500*time.Millisecond) {
		grid.Set(7, 6, face.unit)
	}
}

func drawDigit(grid *ht16k33.Matrix8x8x2, d, x0, y0 int, color byte) {
	for y, row := range digits[d] {
		for x, c := range row {
			if c == '#' {
				grid.Set(x0+x, y0+y, color)
			}
		}
	}
}
