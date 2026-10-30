package sh1106

// Size of a character cell at scale 1: the glyph is 5x7 pixels, with a pixel
// of space to its right and below.
const (
	CharWidth  = 6
	CharHeight = 8
)

// DrawText draws s with its top left corner at (x, y), each font pixel
// scale pixels square, and returns the x just past the text. A newline moves
// to the next line, back at x. Characters the font doesn't have, which is
// anything outside printable ASCII, are drawn as '?'. Whatever falls off the
// display is clipped. A scale below 1 is treated as 1.
//
// With on false it draws the text in unlit pixels, over whatever is there.
func (d *Display) DrawText(x, y int, s string, scale int, on bool) int {
	scale = max(scale, 1)
	cx, cy := x, y
	for _, r := range s {
		if r == '\n' {
			cx, cy = x, cy+CharHeight*scale
			continue
		}
		if r < ' ' || r > '~' {
			r = '?'
		}
		glyph := &font5x7[r-' ']
		for col, bits := range glyph {
			for row := range 7 {
				if bits&(1<<row) != 0 {
					d.fillRect(cx+col*scale, cy+row*scale, scale, scale, on)
				}
			}
		}
		cx += CharWidth * scale
	}
	return cx
}

// TextWidth returns the width in pixels of the widest line of s at the given
// scale, as DrawText would draw it, without the space after the last
// character.
func TextWidth(s string, scale int) int {
	scale = max(scale, 1)
	widest, n := 0, 0
	for _, r := range s {
		if r == '\n' {
			n = 0
			continue
		}
		n++
		widest = max(widest, n)
	}
	if widest == 0 {
		return 0
	}
	return (widest*CharWidth - 1) * scale
}

func (d *Display) fillRect(x, y, w, h int, on bool) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			d.Set(xx, yy, on)
		}
	}
}
