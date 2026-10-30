package sh1106

import (
	"strings"
	"testing"
)

// art returns the pixels in the w x h area at (x, y) as rows of '#' and '.'.
func art(d *Display, x, y, w, h int) string {
	var sb strings.Builder
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			if d.Get(xx, yy) {
				sb.WriteByte('#')
			} else {
				sb.WriteByte('.')
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

const glyphA = ".###.\n" +
	"#...#\n" +
	"#...#\n" +
	"#...#\n" +
	"#####\n" +
	"#...#\n" +
	"#...#\n"

func newDisplay(t *testing.T) *Display {
	t.Helper()
	d, err := New(&fakeBus{}, DefaultAddr)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDrawTextGlyph(t *testing.T) {
	d := newDisplay(t)
	end := d.DrawText(2, 3, "A", 1, true)
	if end != 2+CharWidth {
		t.Errorf("returned x = %d, want %d", end, 2+CharWidth)
	}
	if got := art(d, 2, 3, 5, 7); got != glyphA {
		t.Errorf("A drawn as:\n%s", got)
	}
	// Nothing outside the 5x7 glyph, including the pixel of spacing.
	n := 0
	for y := range Height {
		for x := range Width {
			if d.Get(x, y) {
				n++
			}
		}
	}
	if n != strings.Count(glyphA, "#") {
		t.Errorf("%d pixels on, want %d", n, strings.Count(glyphA, "#"))
	}
}

func TestDrawTextAdvanceAndOff(t *testing.T) {
	d := newDisplay(t)
	end := d.DrawText(0, 0, "AA", 1, true)
	if end != 2*CharWidth {
		t.Errorf("returned x = %d, want %d", end, 2*CharWidth)
	}
	if got := art(d, CharWidth, 0, 5, 7); got != glyphA {
		t.Errorf("second A drawn as:\n%s", got)
	}
	d.Fill(true)
	d.DrawText(0, 0, "A", 1, false)
	inverse := strings.NewReplacer("#", ".", ".", "#").Replace(glyphA)
	if got := art(d, 0, 0, 5, 7); got != inverse {
		t.Errorf("unlit A drawn as:\n%s", got)
	}
}

func TestDrawTextScale(t *testing.T) {
	d := newDisplay(t)
	end := d.DrawText(0, 0, "A", 2, true)
	if end != 2*CharWidth {
		t.Errorf("returned x = %d, want %d", end, 2*CharWidth)
	}
	var want strings.Builder
	for _, row := range strings.Split(strings.TrimSuffix(glyphA, "\n"), "\n") {
		var line strings.Builder
		for _, c := range row {
			line.WriteString(strings.Repeat(string(c), 2))
		}
		want.WriteString(line.String() + "\n" + line.String() + "\n")
	}
	if got := art(d, 0, 0, 10, 14); got != want.String() {
		t.Errorf("scale 2 A drawn as:\n%s\nwant:\n%s", got, want.String())
	}

	d.Fill(false)
	d.DrawText(0, 0, "A", 0, true) // below 1 counts as 1
	if got := art(d, 0, 0, 5, 7); got != glyphA {
		t.Errorf("scale 0 A drawn as:\n%s", got)
	}
}

func TestDrawTextNewlineAndUnknown(t *testing.T) {
	d := newDisplay(t)
	d.DrawText(4, 1, "A\nA", 1, true)
	if got := art(d, 4, 1, 5, 7); got != glyphA {
		t.Errorf("first line:\n%s", got)
	}
	if got := art(d, 4, 1+CharHeight, 5, 7); got != glyphA {
		t.Errorf("second line:\n%s", got)
	}

	d.Fill(false)
	d.DrawText(0, 0, "?", 1, true)
	q := art(d, 0, 0, 5, 7)
	for _, r := range []string{"é", "\x01", "日"} {
		d.Fill(false)
		d.DrawText(0, 0, r, 1, true)
		if got := art(d, 0, 0, 5, 7); got != q {
			t.Errorf("%q not drawn as '?':\n%s", r, got)
		}
	}
}

func TestDrawTextClips(t *testing.T) {
	d := newDisplay(t)
	d.DrawText(-3, -3, "A", 1, true)            // only the bottom right of the A is on screen
	d.DrawText(Width-2, Height-2, "A", 1, true) // only the top left of the A is
	d.DrawText(Width, 0, "ABC", 2, true)        // entirely off to the right
	if got, want := art(d, 0, 0, 2, 4), ".#\n##\n.#\n.#\n"; got != want {
		t.Errorf("top left:\n%s\nwant:\n%s", got, want)
	}
	if got, want := art(d, Width-2, Height-2, 2, 2), ".#\n#.\n"; got != want {
		t.Errorf("bottom right:\n%s\nwant:\n%s", got, want)
	}
	n := 0
	for y := range Height {
		for x := range Width {
			if d.Get(x, y) {
				n++
			}
		}
	}
	if n != 5+2 {
		t.Errorf("%d pixels on, want 7", n)
	}
}

func TestTextWidth(t *testing.T) {
	tests := []struct {
		s     string
		scale int
		want  int
	}{
		{"", 1, 0},
		{"A", 1, 5},
		{"AB", 1, 11},
		{"AB", 2, 22},
		{"ABC\nA", 1, 17},
		{"A", 0, 5},
	}
	for _, tt := range tests {
		if got := TextWidth(tt.s, tt.scale); got != tt.want {
			t.Errorf("TextWidth(%q, %d) = %d, want %d", tt.s, tt.scale, got, tt.want)
		}
	}
}

func TestFontGlyphsFitSevenRows(t *testing.T) {
	for i, g := range font5x7 {
		for _, col := range g {
			if col&0x80 != 0 {
				t.Errorf("glyph %q uses the eighth row", rune(' '+i))
			}
		}
	}
}
