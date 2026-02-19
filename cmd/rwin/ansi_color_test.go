package main

import (
	"fmt"
	"testing"
)

// --- Palette tests ---

func TestPaletteStandard16(t *testing.T) {
	// Standard 16 colors from the design doc table.
	want := [16][3]uint8{
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
	for i, w := range want {
		got := ansiPalette[i]
		if got != w {
			t.Errorf("ansiPalette[%d] = %v, want %v", i, got, w)
		}
	}
}

func TestPalette216Cube(t *testing.T) {
	levels := [6]uint8{0x00, 0x5f, 0x87, 0xaf, 0xd7, 0xff}

	// Spot-check corners and a few interior points of the 6x6x6 cube.
	tests := []struct {
		index int
		r, g, b uint8
	}{
		{16, 0x00, 0x00, 0x00},   // (0,0,0)
		{231, 0xff, 0xff, 0xff},  // (5,5,5)
		{16 + 5, 0x00, 0x00, 0xff},   // (0,0,5)
		{16 + 5*36, 0xff, 0x00, 0x00}, // (5,0,0)
		{16 + 5*6, 0x00, 0xff, 0x00},  // (0,5,0)
		{16 + 3*36 + 2*6 + 4, levels[3], levels[2], levels[4]}, // (3,2,4)
		{16 + 1*36 + 1*6 + 1, levels[1], levels[1], levels[1]}, // (1,1,1)
	}
	for _, tt := range tests {
		got := ansiPalette[tt.index]
		want := [3]uint8{tt.r, tt.g, tt.b}
		if got != want {
			t.Errorf("ansiPalette[%d] = %v, want %v", tt.index, got, want)
		}
	}
}

func TestPalette24Grayscale(t *testing.T) {
	// Spot-check first, last, and a middle grayscale entry.
	tests := []struct {
		index int
		v     uint8
	}{
		{232, 8},       // first: (0*10)+8 = 8
		{255, 238},     // last: (23*10)+8 = 238
		{232 + 12, 128}, // middle: (12*10)+8 = 128
	}
	for _, tt := range tests {
		got := ansiPalette[tt.index]
		want := [3]uint8{tt.v, tt.v, tt.v}
		if got != want {
			t.Errorf("ansiPalette[%d] = %v, want %v", tt.index, got, want)
		}
	}

	// Verify all 24 grayscale entries follow the formula.
	for i := 0; i < 24; i++ {
		v := uint8(i*10 + 8)
		got := ansiPalette[232+i]
		want := [3]uint8{v, v, v}
		if got != want {
			t.Errorf("ansiPalette[%d] = %v, want %v", 232+i, got, want)
		}
	}
}

// --- colorToHex tests ---

func TestColorToHex(t *testing.T) {
	tests := []struct {
		name string
		c    ansiColor
		want string
	}{
		{"default/unset", ansiColor{}, "-"},
		{"black", ansiColor{set: true, r: 0, g: 0, b: 0}, "#000000"},
		{"white", ansiColor{set: true, r: 0xff, g: 0xff, b: 0xff}, "#ffffff"},
		{"red", ansiColor{set: true, r: 0xaa, g: 0, b: 0}, "#aa0000"},
		{"arbitrary", ansiColor{set: true, r: 0x12, g: 0x34, b: 0x56}, "#123456"},
		{"bright yellow", ansiColor{set: true, r: 0xff, g: 0xff, b: 0x55}, "#ffff55"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := colorToHex(tt.c)
			if got != tt.want {
				t.Errorf("colorToHex(%+v) = %q, want %q", tt.c, got, tt.want)
			}
		})
	}
}

// --- applyDim tests ---

func TestApplyDim(t *testing.T) {
	tests := []struct {
		name string
		c    ansiColor
		want ansiColor
	}{
		{
			"default returns mid-gray",
			ansiColor{},
			ansiColor{set: true, r: 0x80, g: 0x80, b: 0x80},
		},
		{
			"white halved",
			ansiColor{set: true, r: 0xff, g: 0xff, b: 0xff},
			ansiColor{set: true, r: 0x7f, g: 0x7f, b: 0x7f},
		},
		{
			"red halved",
			ansiColor{set: true, r: 0xaa, g: 0x00, b: 0x00},
			ansiColor{set: true, r: 0x55, g: 0x00, b: 0x00},
		},
		{
			"arbitrary color halved",
			ansiColor{set: true, r: 100, g: 200, b: 50},
			ansiColor{set: true, r: 50, g: 100, b: 25},
		},
		{
			"black stays black",
			ansiColor{set: true, r: 0, g: 0, b: 0},
			ansiColor{set: true, r: 0, g: 0, b: 0},
		},
		{
			"odd values truncate",
			ansiColor{set: true, r: 1, g: 3, b: 5},
			ansiColor{set: true, r: 0, g: 1, b: 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyDim(tt.c)
			if got != tt.want {
				t.Errorf("applyDim(%+v) = %+v, want %+v", tt.c, got, tt.want)
			}
		})
	}
}

// --- resolveColors tests ---

func TestResolveColorsNoInverseNoDim(t *testing.T) {
	s := sgrState{
		fg: ansiColor{set: true, r: 0xaa, g: 0, b: 0},
		bg: ansiColor{set: true, r: 0, g: 0, b: 0xaa},
	}
	fg, bg := resolveColors(s)
	if fg != s.fg {
		t.Errorf("fg = %+v, want %+v", fg, s.fg)
	}
	if bg != s.bg {
		t.Errorf("bg = %+v, want %+v", bg, s.bg)
	}
}

func TestResolveColorsInverse(t *testing.T) {
	s := sgrState{
		fg:      ansiColor{set: true, r: 0xaa, g: 0, b: 0},
		bg:      ansiColor{set: true, r: 0, g: 0, b: 0xaa},
		inverse: true,
	}
	fg, bg := resolveColors(s)
	// fg and bg should be swapped.
	if fg != s.bg {
		t.Errorf("inverse fg = %+v, want %+v (original bg)", fg, s.bg)
	}
	if bg != s.fg {
		t.Errorf("inverse bg = %+v, want %+v (original fg)", bg, s.fg)
	}
}

func TestResolveColorsInverseDefaultFg(t *testing.T) {
	// Default fg, explicit bg. Inverse should swap, and the new fg (was default bg)
	// gets #fffff0 only if the *bg* was default. Here bg is explicit, so new fg = old bg.
	s := sgrState{
		bg:      ansiColor{set: true, r: 0, g: 0xaa, b: 0},
		inverse: true,
	}
	fg, bg := resolveColors(s)
	// New fg = old bg (explicit green).
	if fg != s.bg {
		t.Errorf("fg = %+v, want %+v (original bg)", fg, s.bg)
	}
	// New bg = old fg (was default/unset) → substituted with #000000.
	wantBg := ansiColor{set: true, r: 0, g: 0, b: 0}
	if bg != wantBg {
		t.Errorf("bg = %+v, want %+v", bg, wantBg)
	}
}

func TestResolveColorsInverseDefaultBg(t *testing.T) {
	// Explicit fg, default bg. Inverse swaps → new fg was default bg → #fffff0.
	s := sgrState{
		fg:      ansiColor{set: true, r: 0xaa, g: 0, b: 0},
		inverse: true,
	}
	fg, bg := resolveColors(s)
	// New fg = old bg (was default) → #fffff0.
	wantFg := ansiColor{set: true, r: 0xff, g: 0xff, b: 0xf0}
	if fg != wantFg {
		t.Errorf("fg = %+v, want %+v", fg, wantFg)
	}
	// New bg = old fg.
	if bg != s.fg {
		t.Errorf("bg = %+v, want %+v (original fg)", bg, s.fg)
	}
}

func TestResolveColorsInverseBothDefault(t *testing.T) {
	s := sgrState{inverse: true}
	fg, bg := resolveColors(s)
	wantFg := ansiColor{set: true, r: 0xff, g: 0xff, b: 0xf0}
	wantBg := ansiColor{set: true, r: 0, g: 0, b: 0}
	if fg != wantFg {
		t.Errorf("fg = %+v, want %+v", fg, wantFg)
	}
	if bg != wantBg {
		t.Errorf("bg = %+v, want %+v", bg, wantBg)
	}
}

func TestResolveColorsDimOnly(t *testing.T) {
	s := sgrState{
		fg:  ansiColor{set: true, r: 0xaa, g: 0x00, b: 0x00},
		dim: true,
	}
	fg, bg := resolveColors(s)
	wantFg := ansiColor{set: true, r: 0x55, g: 0, b: 0}
	if fg != wantFg {
		t.Errorf("fg = %+v, want %+v", fg, wantFg)
	}
	// bg should be unchanged (default).
	if bg != s.bg {
		t.Errorf("bg = %+v, want %+v", bg, s.bg)
	}
}

func TestResolveColorsDimDefault(t *testing.T) {
	// Dim with default fg should produce mid-gray.
	s := sgrState{dim: true}
	fg, _ := resolveColors(s)
	wantFg := ansiColor{set: true, r: 0x80, g: 0x80, b: 0x80}
	if fg != wantFg {
		t.Errorf("fg = %+v, want %+v", fg, wantFg)
	}
}

func TestResolveColorsInverseAndDim(t *testing.T) {
	// Inverse then dim: swap first, then dim the fg.
	s := sgrState{
		fg:      ansiColor{set: true, r: 0xaa, g: 0, b: 0},
		bg:      ansiColor{set: true, r: 0, g: 0, b: 0xaa},
		inverse: true,
		dim:     true,
	}
	fg, bg := resolveColors(s)
	// After inverse: fg=old bg (0,0,0xaa), bg=old fg (0xaa,0,0).
	// Then dim fg: (0, 0, 0x55).
	wantFg := ansiColor{set: true, r: 0, g: 0, b: 0x55}
	wantBg := ansiColor{set: true, r: 0xaa, g: 0, b: 0}
	if fg != wantFg {
		t.Errorf("fg = %+v, want %+v", fg, wantFg)
	}
	if bg != wantBg {
		t.Errorf("bg = %+v, want %+v", bg, wantBg)
	}
}

// --- isDefaultStyle tests ---

func TestIsDefaultStyle(t *testing.T) {
	tests := []struct {
		name string
		s    sgrState
		want bool
	}{
		{"zero value", sgrState{}, true},
		{"fg set", sgrState{fg: ansiColor{set: true, r: 0xff}}, false},
		{"bg set", sgrState{bg: ansiColor{set: true, b: 0xff}}, false},
		{"bold", sgrState{bold: true}, false},
		{"dim", sgrState{dim: true}, false},
		{"italic", sgrState{italic: true}, false},
		{"underline", sgrState{underline: true}, false},
		{"blink", sgrState{blink: true}, false},
		{"inverse", sgrState{inverse: true}, false},
		{"hidden", sgrState{hidden: true}, false},
		{"strike", sgrState{strike: true}, false},
		{"multiple attrs", sgrState{bold: true, fg: ansiColor{set: true, r: 0xff}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDefaultStyle(tt.s)
			if got != tt.want {
				t.Errorf("isDefaultStyle(%+v) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

// --- sgrState.reset() test ---

func TestSgrStateReset(t *testing.T) {
	s := sgrState{
		fg:        ansiColor{set: true, r: 0xff},
		bg:        ansiColor{set: true, b: 0xff},
		bold:      true,
		dim:       true,
		italic:    true,
		underline: true,
		blink:     true,
		inverse:   true,
		hidden:    true,
		strike:    true,
	}
	s.reset()
	if s != (sgrState{}) {
		t.Errorf("after reset: %+v, want zero value", s)
	}
}

// --- Palette-to-ansiColor round-trip test ---

func TestPaletteColorToHex(t *testing.T) {
	// Verify a few palette entries produce the expected hex strings
	// when converted to ansiColor then to hex.
	tests := []struct {
		index int
		want  string
	}{
		{0, "#000000"},
		{1, "#aa0000"},
		{15, "#ffffff"},
		{232, "#080808"},
		{255, "#eeeeee"},
	}
	for _, tt := range tests {
		rgb := ansiPalette[tt.index]
		c := ansiColor{set: true, r: rgb[0], g: rgb[1], b: rgb[2]}
		got := colorToHex(c)
		if got != tt.want {
			t.Errorf("colorToHex(palette[%d]) = %q, want %q", tt.index, got, tt.want)
		}
	}
}

// --- colorToHex formatting test ---

func TestColorToHexFormatting(t *testing.T) {
	// Ensure lowercase hex and zero-padded.
	c := ansiColor{set: true, r: 0x0a, g: 0x0b, b: 0x0c}
	got := colorToHex(c)
	want := "#0a0b0c"
	if got != want {
		t.Errorf("colorToHex(%+v) = %q, want %q", c, got, want)
	}
}

// --- Truecolor (24-bit) hex formatting ---

func TestColorToHexTruecolor(t *testing.T) {
	// 24-bit truecolor values should format correctly.
	tests := []struct {
		r, g, b uint8
		want    string
	}{
		{0, 0, 0, "#000000"},
		{255, 255, 255, "#ffffff"},
		{128, 64, 32, "#804020"},
		{1, 2, 3, "#010203"},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("rgb(%d,%d,%d)", tt.r, tt.g, tt.b)
		t.Run(name, func(t *testing.T) {
			c := ansiColor{set: true, r: tt.r, g: tt.g, b: tt.b}
			got := colorToHex(c)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
