package main

import "fmt"

// ansiColor represents an RGB color that may or may not be explicitly set.
// When set is false, the color is "default" — the span should use "-".
type ansiColor struct {
	set     bool
	r, g, b uint8
}

// sgrState tracks the cumulative SGR attribute state. Each SGR command
// modifies the current state; it does not replace it (except reset).
type sgrState struct {
	fg        ansiColor
	bg        ansiColor
	bold      bool
	dim       bool
	italic    bool
	underline bool
	blink     bool
	inverse   bool
	hidden    bool
	strike    bool
}

func (s *sgrState) reset() {
	*s = sgrState{}
}

// styledRun is a contiguous run of text sharing the same style.
type styledRun struct {
	text  []rune
	style sgrState
}

// ansiPalette maps 256-color indices to RGB values.
var ansiPalette [256][3]uint8

// standard16 holds the traditional CGA/VGA palette for indices 0-15.
var standard16 = [16][3]uint8{
	{0x00, 0x00, 0x00}, // 0  Black
	{0xaa, 0x00, 0x00}, // 1  Red
	{0x00, 0xaa, 0x00}, // 2  Green
	{0xaa, 0x55, 0x00}, // 3  Yellow (dark)
	{0x00, 0x00, 0xaa}, // 4  Blue
	{0xaa, 0x00, 0xaa}, // 5  Magenta
	{0x00, 0xaa, 0xaa}, // 6  Cyan
	{0xaa, 0xaa, 0xaa}, // 7  White (light gray)
	{0x55, 0x55, 0x55}, // 8  Bright black (dark gray)
	{0xff, 0x55, 0x55}, // 9  Bright red
	{0x55, 0xff, 0x55}, // 10 Bright green
	{0xff, 0xff, 0x55}, // 11 Bright yellow
	{0x55, 0x55, 0xff}, // 12 Bright blue
	{0xff, 0x55, 0xff}, // 13 Bright magenta
	{0x55, 0xff, 0xff}, // 14 Bright cyan
	{0xff, 0xff, 0xff}, // 15 Bright white
}

func init() {
	copy(ansiPalette[:16], standard16[:])

	// 216-color cube (indices 16-231).
	levels := [6]uint8{0x00, 0x5f, 0x87, 0xaf, 0xd7, 0xff}
	for i := 0; i < 216; i++ {
		r := levels[i/36]
		g := levels[(i%36)/6]
		b := levels[i%6]
		ansiPalette[16+i] = [3]uint8{r, g, b}
	}

	// 24 grayscale (indices 232-255).
	for i := 0; i < 24; i++ {
		v := uint8(i*10 + 8)
		ansiPalette[232+i] = [3]uint8{v, v, v}
	}
}

// colorToHex converts an ansiColor to a "#rrggbb" hex string for the spans
// protocol, or "-" if the color is default (unset).
func colorToHex(c ansiColor) string {
	if !c.set {
		return "-"
	}
	return fmt.Sprintf("#%02x%02x%02x", c.r, c.g, c.b)
}

// applyDim approximates SGR dim (faint) by halving each RGB component.
// If the color is default (unset), returns mid-gray #808080.
func applyDim(c ansiColor) ansiColor {
	if !c.set {
		return ansiColor{set: true, r: 0x80, g: 0x80, b: 0x80}
	}
	return ansiColor{set: true, r: c.r / 2, g: c.g / 2, b: c.b / 2}
}

// resolveColors applies inverse and dim transformations to produce the final
// foreground and background colors for a span.
func resolveColors(s sgrState) (fg, bg ansiColor) {
	fg, bg = s.fg, s.bg
	if s.inverse {
		fg, bg = bg, fg
		if !fg.set {
			fg = ansiColor{set: true, r: 0xff, g: 0xff, b: 0xf0}
		}
		if !bg.set {
			bg = ansiColor{set: true, r: 0x00, g: 0x00, b: 0x00}
		}
	}
	if s.dim {
		fg = applyDim(fg)
	}
	return fg, bg
}

// isDefaultStyle returns true if the style has no attributes set.
func isDefaultStyle(s sgrState) bool {
	return !s.fg.set && !s.bg.set &&
		!s.bold && !s.dim && !s.italic && !s.underline &&
		!s.inverse && !s.hidden && !s.strike && !s.blink
}
