package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ApolloX939/mcp2221/ht16k33"
	"github.com/ApolloX939/mcp2221/is31fl3741"
)

// display is what the animations draw on: an 8x8 canvas of ht16k33 colours
// (Off, Red, Green, Orange), whichever matrix is behind it.
type display interface {
	Set(x, y int, color byte)
	Fill(color byte)
	Show() error
	SetRotation(degrees int) error
	SetMirror(on bool)
}

// The IS31FL3741 answers at 0x30-0x33, clear of the HT16K33's 0x70-0x77, so
// the address says which matrix the user wants as the working light.
const (
	rgbAddrFirst = 0x30
	rgbAddrLast  = 0x33
)

// isRGBAddr reports whether addr is in the IS31FL3741's range.
func isRGBAddr(addr byte) bool { return addr >= rgbAddrFirst && addr <= rgbAddrLast }

// openDisplay sets up the matrix at addr: the 13x9 RGB matrix for 0x30-0x33,
// the bicolor 8x8 for any other address.
func openDisplay(b bus, addr byte, brightness float64) (display, error) {
	if isRGBAddr(addr) {
		m, err := is31fl3741.New(b, addr, brightness)
		if err != nil {
			return nil, err
		}
		return &rgbDisplay{m: m}, nil
	}
	return ht16k33.NewMatrix8x8x2(b, addr, brightness)
}

// rgbDisplay draws the 8x8 canvas in the middle of the 13x9 matrix, in the
// colours the bicolor matrix has.
type rgbDisplay struct {
	m    *is31fl3741.Matrix
	rot  int // quarter turns clockwise
	flip bool
}

// Where the 8x8 canvas sits on the 13x9 matrix.
const (
	rgbOriginX = (is31fl3741.Width - 8) / 2
	rgbOriginY = (is31fl3741.Height - 8) / 2
)

func (d *rgbDisplay) SetRotation(degrees int) error {
	if degrees < 0 || degrees > 270 || degrees%90 != 0 {
		return fmt.Errorf("rotation must be 0, 90, 180 or 270 degrees, not %d", degrees)
	}
	d.rot = degrees / 90
	return nil
}

func (d *rgbDisplay) SetMirror(on bool) { d.flip = on }

// rgbColors gives the red, green and blue of each ht16k33 colour, indexed by it.
var rgbColors = [4][3]byte{
	ht16k33.Off:    {0, 0, 0},
	ht16k33.Red:    {255, 0, 0},
	ht16k33.Green:  {0, 255, 0},
	ht16k33.Orange: {255, 20, 0},
}

// Set mirrors and turns like ht16k33.Matrix8x8x2.Set does.
func (d *rgbDisplay) Set(x, y int, color byte) {
	if x < 0 || x > 7 || y < 0 || y > 7 {
		return
	}
	if d.flip {
		x = 7 - x
	}
	for range d.rot {
		x, y = 7-y, x
	}
	c := rgbColors[color&3]
	d.m.Set(rgbOriginX+x, rgbOriginY+y, c[0], c[1], c[2])
}

func (d *rgbDisplay) Fill(color byte) {
	for y := range 8 {
		for x := range 8 {
			d.Set(x, y, color)
		}
	}
}

func (d *rgbDisplay) Show() error { return d.m.Show() }

// runColors fills the display with each colour in turn until ctx ends, to
// check that red, green and orange come out as they should.
func runColors(ctx context.Context, addr byte, brightness float64, rotate int, mirror bool) error {
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
	names := map[byte]string{ht16k33.Red: "red", ht16k33.Green: "green", ht16k33.Orange: "orange"}
	for {
		for _, c := range []byte{ht16k33.Red, ht16k33.Green, ht16k33.Orange} {
			grid.Fill(c)
			if err := grid.Show(); err != nil {
				return err
			}
			fmt.Println(names[c])
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(2 * time.Second):
			}
		}
	}
}
