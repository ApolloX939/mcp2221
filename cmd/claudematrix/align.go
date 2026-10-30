package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/ApolloX939/mcp2221/ht16k33"
)

// Alignment mode shows a sample countdown (3:42) with a green pixel in each
// corner of the matrix, so a matrix mounted off-centre can be lined up in
// software. The arrow keys move the countdown, r turns the display a quarter
// turn, and q prints the flags that keep it there.

// alignLeft is the time the sample countdown shows.
const alignLeft = 3*time.Hour + 42*time.Minute

// alignKey maps a key to how far it moves the countdown.
func alignKey(b byte) (dx, dy int, ok bool) {
	switch b {
	case 'a', 'h':
		return -1, 0, true
	case 'd', 'l':
		return 1, 0, true
	case 'w', 'k':
		return 0, -1, true
	case 's', 'j':
		return 0, 1, true
	}
	return 0, 0, false
}

// alignKeys reads keys from the terminal, one at a time, and sends each on
// the returned channel. Arrow keys arrive as ESC [ A to D and are turned into
// h j k l. The channel closes when the terminal does.
func alignKeys(ctx context.Context) (<-chan byte, func(), error) {
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return nil, nil, fmt.Errorf("alignment mode needs a terminal: %w", err)
	}
	stty := func(args ...string) error {
		cmd := exec.Command("stty", args...)
		cmd.Stdin = tty
		return cmd.Run()
	}
	if err := stty("-icanon", "-echo"); err != nil {
		tty.Close()
		return nil, nil, fmt.Errorf("alignment mode needs stty: %w", err)
	}
	restore := func() {
		_ = stty("icanon", "echo")
		tty.Close()
	}

	keys := make(chan byte)
	go func() {
		defer close(keys)
		buf := make([]byte, 8)
		for {
			n, err := tty.Read(buf)
			if err != nil {
				return
			}
			for _, b := range translateKeys(buf[:n]) {
				select {
				case keys <- b:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return keys, restore, nil
}

// translateKeys turns arrow-key escape sequences into h j k l and passes
// other bytes through.
func translateKeys(in []byte) []byte {
	var out []byte
	for i := 0; i < len(in); i++ {
		if in[i] == 0x1b && i+2 < len(in) && in[i+1] == '[' {
			switch in[i+2] {
			case 'A':
				out = append(out, 'k')
			case 'B':
				out = append(out, 'j')
			case 'C':
				out = append(out, 'l')
			case 'D':
				out = append(out, 'h')
			}
			i += 2
			continue
		}
		out = append(out, in[i])
	}
	return out
}

// runAlign runs alignment mode, starting from off.
func runAlign(ctx context.Context, addr byte, brightness float64, rotate int, mirror bool, off offset) error {
	keys, restore, err := alignKeys(ctx)
	if err != nil {
		return err
	}
	defer restore()

	bus, err := openBus()
	if err != nil {
		return err
	}
	defer bus.Close()
	grid, err := openDisplay(bus, addr, brightness)
	if err != nil {
		return err
	}
	defer func() {
		grid.Fill(ht16k33.Off)
		_ = grid.Show()
	}()

	grid.SetMirror(mirror)
	if err := grid.SetRotation(rotate); err != nil {
		return err
	}

	fmt.Print("Arrow keys, WASD or hjkl move the countdown, r turns the display, m mirrors it; q or Enter finishes.\r\n")
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		now := time.Now()
		drawClock(grid, alignLeft, now, off)
		for _, c := range [][2]int{{0, 0}, {7, 0}, {0, 7}, {7, 7}} {
			grid.Set(c[0], c[1], ht16k33.Green)
		}
		if err := grid.Show(); err != nil {
			return err
		}
		fmt.Printf("\r-rotate %d -mirror=%t -clock-x %d -clock-y %d   ", rotate, mirror, off.x, off.y)
		select {
		case <-ctx.Done():
			fmt.Print("\r\n")
			return nil
		case b, ok := <-keys:
			if !ok || b == 'q' || b == '\r' || b == '\n' {
				fmt.Printf("\r\nSet rotate to %d, mirror to %t, clockX to %d and clockY to %d in /plugin configure led-matrix@mcp2221.\r\n", rotate, mirror, off.x, off.y)
				return nil
			}
			if b == 'r' {
				rotate = (rotate + 90) % 360
				_ = grid.SetRotation(rotate)
			} else if b == 'm' {
				mirror = !mirror
				grid.SetMirror(mirror)
			} else if dx, dy, ok := alignKey(b); ok {
				off.x = clampShift(off.x + dx)
				off.y = clampShift(off.y + dy)
			}
		case <-ticker.C:
		}
	}
}

// clampShift keeps a shift within the matrix, past which nothing would show.
func clampShift(v int) int { return max(-7, min(7, v)) }
