// claudematrix plays a "working" animation on the LED matrix until it is
// stopped, then blanks the display. While a multi-part job is in progress
// (see -progress) it draws a progress pie instead, and while the usage limit
// is hit (see -limit) a countdown to when it resets. With -countdown it shows
// only the countdown, and exits once the limit resets.
//
// The led-matrix Claude Code mod (led-matrix/ in this repo) starts it when Claude starts working and
// stops it (SIGTERM) when Claude is done. It also stops if the program that
// started it exits, or if -heartbeat is set and its standard output is
// closed, so the matrix is never left lit.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	mcp2221 "github.com/ApolloX939/mcp2221"
	"github.com/ApolloX939/mcp2221/ht16k33"
)

func main() {
	brightness := flag.Float64("brightness", 0.1, "display brightness, 0.0 to 1.0")
	addr := flag.Uint("addr", 0x70, "I2C address of the matrix")
	frame := flag.Duration("frame", 120*time.Millisecond, "time per animation step")
	heartbeat := flag.Duration("heartbeat", 0, "print a line this often, and stop when output is closed (0 = off)")
	progressFile := flag.String("progress", defaultProgressFile(), "file the progress-pie plugin writes; \"\" = no pie")
	limitFile := flag.String("limit", defaultLimitFile(), "file the led-matrix plugin writes the usage limit's reset time to; \"\" = no countdown")
	countdown := flag.Bool("countdown", false, "show only the countdown to the usage limit's reset, and exit when there is none")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go watchParent(ctx, cancel)
	if *heartbeat > 0 {
		// A closed pipe should stop the animation, not kill the process
		// before it can blank the display.
		signal.Ignore(syscall.SIGPIPE)
		go beat(ctx, cancel, *heartbeat)
	}

	// Keep trying while the adapter is unplugged or another program
	// (such as statmatrix) has it open.
	if *countdown {
		*progressFile = ""
	}
	show := shows{progress: *progressFile, limit: *limitFile, countdownOnly: *countdown}
	lastErr := ""
	for ctx.Err() == nil {
		if *countdown {
			if _, ok := readLimit(*limitFile, time.Now()); !ok {
				return
			}
		}
		err := animate(ctx, byte(*addr), *brightness, *frame, show)
		if err != nil && err.Error() != lastErr {
			fmt.Fprintln(os.Stderr, "claudematrix:", err)
			lastErr = err.Error()
		}
		select {
		case <-ctx.Done():
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// shows says what animate draws besides the spinner.
type shows struct {
	progress      string // progress file for the pie; "" = none
	limit         string // usage-limit file for the countdown; "" = none
	countdownOnly bool   // only the countdown; stop when there is none
}

// animate runs the animation until ctx ends (or, countdown only, until the
// limit resets), then blanks the display. The countdown comes before the
// pie, and the pie before the spinner.
func animate(ctx context.Context, addr byte, brightness float64, frame time.Duration, show shows) error {
	bus, err := mcp2221.Open()
	if err != nil {
		return err
	}
	defer bus.Close()

	grid, err := ht16k33.NewMatrix8x8x2(bus, addr, brightness)
	if err != nil {
		return err
	}
	defer func() {
		grid.Fill(ht16k33.Off)
		_ = grid.Show()
	}()

	ticker := time.NewTicker(frame)
	defer ticker.Stop()
	for step := 0; ; step++ {
		now := time.Now()
		if left, ok := readLimit(show.limit, now); ok {
			drawClock(grid, left, now)
		} else if show.countdownOnly {
			return nil
		} else if p, ok := readProgress(show.progress, now); ok {
			drawPie(grid, p, step)
		} else {
			drawSpinner(grid, step)
		}
		if err := grid.Show(); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// Claude Code's spinner, drawn for the 8x8 matrix: an orange dot that grows
// into a star (· ✢ ✳ ✶ ✻ ✽) and shrinks back again.
var spinner = [][8]string{
	{
		"........",
		"........",
		"........",
		"...##...",
		"...##...",
		"........",
		"........",
		"........",
	},
	{
		"........",
		"........",
		"...##...",
		"..####..",
		"..####..",
		"...##...",
		"........",
		"........",
	},
	{
		"........",
		"...##...",
		"...##...",
		".######.",
		".######.",
		"...##...",
		"...##...",
		"........",
	},
	{
		"........",
		".#.##.#.",
		"..####..",
		".######.",
		".######.",
		"..####..",
		".#.##.#.",
		"........",
	},
	{
		"...##...",
		".#.##.#.",
		"..####..",
		"########",
		"########",
		"..####..",
		".#.##.#.",
		"...##...",
	},
	{
		"#..##..#",
		".#.##.#.",
		"..####..",
		"########",
		"########",
		"..####..",
		".#.##.#.",
		"#..##..#",
	},
}

// spinnerFrame returns which frame to show at a step: forward through the
// frames, then back, without repeating the ends (0 1 2 3 4 5 4 3 2 1 0 ...).
func spinnerFrame(step int) int {
	n := len(spinner)
	cycle := 2*n - 2
	i := step % cycle
	if i >= n {
		i = cycle - i
	}
	return i
}

func drawSpinner(grid *ht16k33.Matrix8x8x2, step int) {
	frame := spinner[spinnerFrame(step)]
	for y, row := range frame {
		for x, c := range row {
			color := ht16k33.Off
			if c == '#' {
				color = ht16k33.Orange
			}
			grid.Set(x, y, color)
		}
	}
}

// watchParent cancels ctx when the process that started this one exits.
func watchParent(ctx context.Context, cancel context.CancelFunc) {
	parent := os.Getppid()
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if os.Getppid() != parent {
				cancel()
				return
			}
		}
	}
}

// beat prints a line every interval, so the program reading our output
// wakes up regularly, and cancels ctx once nobody is reading.
func beat(ctx context.Context, cancel context.CancelFunc, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := fmt.Println("working"); err != nil {
				cancel()
				return
			}
		}
	}
}
