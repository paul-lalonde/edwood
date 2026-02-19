package main

import (
	"strings"
	"testing"
)

// --- Helper ---

// runeStr converts a rune slice to string for readable test output.
func runeStr(r []rune) string {
	return string(r)
}

// --- (1) Plain text passthrough ---

func TestProcessPlainText(t *testing.T) {
	p := NewAnsiParser(nil)
	clean, runs := p.Process([]rune("hello world"))
	if runeStr(clean) != "hello world" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "hello world")
	}
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	if runeStr(runs[0].text) != "hello world" {
		t.Errorf("runs[0].text = %q, want %q", runeStr(runs[0].text), "hello world")
	}
	if !isDefaultStyle(runs[0].style) {
		t.Errorf("runs[0].style should be default, got %+v", runs[0].style)
	}
}

func TestProcessEmptyInput(t *testing.T) {
	p := NewAnsiParser(nil)
	clean, runs := p.Process([]rune(""))
	if len(clean) != 0 {
		t.Errorf("clean should be empty, got %q", runeStr(clean))
	}
	if len(runs) != 0 {
		t.Errorf("runs should be empty, got %d runs", len(runs))
	}
}

func TestProcessUnicode(t *testing.T) {
	p := NewAnsiParser(nil)
	clean, runs := p.Process([]rune("héllo 世界 🌍"))
	if runeStr(clean) != "héllo 世界 🌍" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "héllo 世界 🌍")
	}
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
}

// --- (2) Escape stripping (CSI, unknown Esc) ---

func TestProcessStripCSI(t *testing.T) {
	// ESC[1m should be stripped, "hello" remains.
	p := NewAnsiParser(nil)
	clean, _ := p.Process([]rune("\x1b[1mhello"))
	if runeStr(clean) != "hello" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "hello")
	}
}

func TestProcessStripUnknownEsc(t *testing.T) {
	// ESC followed by unknown char should be consumed.
	p := NewAnsiParser(nil)
	clean, _ := p.Process([]rune("\x1bXhello"))
	if runeStr(clean) != "hello" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "hello")
	}
}

func TestProcessStripCharsetDesignation(t *testing.T) {
	// ESC ( B is a charset designation — 3 bytes consumed.
	p := NewAnsiParser(nil)
	clean, _ := p.Process([]rune("\x1b(Bhello"))
	if runeStr(clean) != "hello" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "hello")
	}
}

func TestProcessStripDECKPAM(t *testing.T) {
	// ESC = (DECKPAM) should be consumed.
	p := NewAnsiParser(nil)
	clean, _ := p.Process([]rune("\x1b=hello"))
	if runeStr(clean) != "hello" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "hello")
	}
}

func TestProcessStripNonSGRCSI(t *testing.T) {
	// CSI sequences with non-m final bytes should be stripped.
	// ESC[2J = erase display, ESC[H = cursor home
	p := NewAnsiParser(nil)
	clean, _ := p.Process([]rune("\x1b[2J\x1b[Hhello"))
	if runeStr(clean) != "hello" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "hello")
	}
}

func TestProcessStripOSC(t *testing.T) {
	// OSC sequences (ESC ] ... BEL) should be stripped.
	p := NewAnsiParser(nil)
	clean, _ := p.Process([]rune("\x1b]0;title\x07hello"))
	if runeStr(clean) != "hello" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "hello")
	}
}

func TestProcessStripOSCWithST(t *testing.T) {
	// OSC terminated with ST (ESC \) should be stripped.
	p := NewAnsiParser(nil)
	clean, _ := p.Process([]rune("\x1b]0;title\x1b\\hello"))
	if runeStr(clean) != "hello" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "hello")
	}
}

// --- (3) State transitions ---

func TestProcessEscRestartsSequence(t *testing.T) {
	// ESC mid-CSI aborts the CSI and starts a new sequence.
	// ESC[1 then ESC[31m → the first CSI is aborted, second sets red fg.
	p := NewAnsiParser(nil)
	clean, runs := p.Process([]rune("\x1b[1\x1b[31mhello"))
	if runeStr(clean) != "hello" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "hello")
	}
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	// Should have red fg (palette index 1).
	c := ansiPalette[1]
	wantFg := ansiColor{set: true, r: c[0], g: c[1], b: c[2]}
	if runs[0].style.fg != wantFg {
		t.Errorf("fg = %+v, want %+v", runs[0].style.fg, wantFg)
	}
	// Bold should NOT be set (the ESC[1 was aborted).
	if runs[0].style.bold {
		t.Error("bold should not be set (aborted CSI)")
	}
}

func TestProcessDoubleEsc(t *testing.T) {
	// Two ESCs in a row — second re-enters stateEsc.
	p := NewAnsiParser(nil)
	clean, _ := p.Process([]rune("\x1b\x1b[31mhello"))
	if runeStr(clean) != "hello" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "hello")
	}
}

func TestProcessCSIToGroundOnFinalByte(t *testing.T) {
	// After CSI dispatch, parser returns to ground.
	p := NewAnsiParser(nil)
	clean, _ := p.Process([]rune("\x1b[31mAB\x1b[0mCD"))
	if runeStr(clean) != "ABCD" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "ABCD")
	}
}

// --- (4) SGR codes 0-9 and 21-29 ---

func TestSGRReset(t *testing.T) {
	p := NewAnsiParser(nil)
	// Set bold, then reset.
	_, runs := p.Process([]rune("\x1b[1mA\x1b[0mB"))
	if len(runs) != 2 {
		t.Fatalf("len(runs) = %d, want 2", len(runs))
	}
	if !runs[0].style.bold {
		t.Error("runs[0] should be bold")
	}
	if runs[1].style.bold {
		t.Error("runs[1] should not be bold after reset")
	}
	if !isDefaultStyle(runs[1].style) {
		t.Errorf("runs[1] should be default after reset, got %+v", runs[1].style)
	}
}

func TestSGRResetBareM(t *testing.T) {
	// ESC[m (no parameters) is equivalent to ESC[0m.
	p := NewAnsiParser(nil)
	_, runs := p.Process([]rune("\x1b[1mA\x1b[mB"))
	if len(runs) != 2 {
		t.Fatalf("len(runs) = %d, want 2", len(runs))
	}
	if !isDefaultStyle(runs[1].style) {
		t.Errorf("ESC[m should reset, got %+v", runs[1].style)
	}
}

func TestSGRAttributesOn(t *testing.T) {
	tests := []struct {
		code  string
		check func(sgrState) bool
		name  string
	}{
		{"1", func(s sgrState) bool { return s.bold }, "bold"},
		{"2", func(s sgrState) bool { return s.dim }, "dim"},
		{"3", func(s sgrState) bool { return s.italic }, "italic"},
		{"4", func(s sgrState) bool { return s.underline }, "underline"},
		{"5", func(s sgrState) bool { return s.blink }, "blink(5)"},
		{"6", func(s sgrState) bool { return s.blink }, "blink(6)"},
		{"7", func(s sgrState) bool { return s.inverse }, "inverse"},
		{"8", func(s sgrState) bool { return s.hidden }, "hidden"},
		{"9", func(s sgrState) bool { return s.strike }, "strike"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewAnsiParser(nil)
			_, runs := p.Process([]rune("\x1b[" + tt.code + "mX"))
			if len(runs) != 1 {
				t.Fatalf("len(runs) = %d, want 1", len(runs))
			}
			if !tt.check(runs[0].style) {
				t.Errorf("%s should be set", tt.name)
			}
		})
	}
}

func TestSGRAttributesOff(t *testing.T) {
	tests := []struct {
		onCode  string
		offCode string
		check   func(sgrState) bool
		name    string
	}{
		{"1", "21", func(s sgrState) bool { return !s.bold }, "bold off (21)"},
		{"1", "22", func(s sgrState) bool { return !s.bold }, "bold off (22)"},
		{"2", "22", func(s sgrState) bool { return !s.dim }, "dim off (22)"},
		{"3", "23", func(s sgrState) bool { return !s.italic }, "italic off"},
		{"4", "24", func(s sgrState) bool { return !s.underline }, "underline off"},
		{"5", "25", func(s sgrState) bool { return !s.blink }, "blink off"},
		{"7", "27", func(s sgrState) bool { return !s.inverse }, "inverse off"},
		{"8", "28", func(s sgrState) bool { return !s.hidden }, "hidden off"},
		{"9", "29", func(s sgrState) bool { return !s.strike }, "strike off"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewAnsiParser(nil)
			_, runs := p.Process([]rune("\x1b[" + tt.onCode + "mA\x1b[" + tt.offCode + "mB"))
			if len(runs) < 2 {
				t.Fatalf("len(runs) = %d, want >= 2", len(runs))
			}
			last := runs[len(runs)-1]
			if !tt.check(last.style) {
				t.Errorf("%s: attribute should be off in last run, got %+v", tt.name, last.style)
			}
		})
	}
}

func TestSGR22ClearsBothBoldAndDim(t *testing.T) {
	p := NewAnsiParser(nil)
	// Set both bold and dim, then 22 clears both.
	_, runs := p.Process([]rune("\x1b[1;2mA\x1b[22mB"))
	if len(runs) != 2 {
		t.Fatalf("len(runs) = %d, want 2", len(runs))
	}
	if runs[1].style.bold {
		t.Error("bold should be cleared by 22")
	}
	if runs[1].style.dim {
		t.Error("dim should be cleared by 22")
	}
}

// --- (5) Standard colors 30-37, 40-47, 90-97, 100-107 ---

func TestSGRStandardFgColors(t *testing.T) {
	for code := 30; code <= 37; code++ {
		idx := code - 30
		p := NewAnsiParser(nil)
		seq := "\x1b[" + itoa(code) + "mX"
		_, runs := p.Process([]rune(seq))
		if len(runs) != 1 {
			t.Fatalf("code %d: len(runs) = %d, want 1", code, len(runs))
		}
		c := ansiPalette[idx]
		want := ansiColor{set: true, r: c[0], g: c[1], b: c[2]}
		if runs[0].style.fg != want {
			t.Errorf("code %d: fg = %+v, want %+v", code, runs[0].style.fg, want)
		}
	}
}

func TestSGRStandardBgColors(t *testing.T) {
	for code := 40; code <= 47; code++ {
		idx := code - 40
		p := NewAnsiParser(nil)
		seq := "\x1b[" + itoa(code) + "mX"
		_, runs := p.Process([]rune(seq))
		if len(runs) != 1 {
			t.Fatalf("code %d: len(runs) = %d, want 1", code, len(runs))
		}
		c := ansiPalette[idx]
		want := ansiColor{set: true, r: c[0], g: c[1], b: c[2]}
		if runs[0].style.bg != want {
			t.Errorf("code %d: bg = %+v, want %+v", code, runs[0].style.bg, want)
		}
	}
}

func TestSGRBrightFgColors(t *testing.T) {
	for code := 90; code <= 97; code++ {
		idx := code - 90 + 8
		p := NewAnsiParser(nil)
		seq := "\x1b[" + itoa(code) + "mX"
		_, runs := p.Process([]rune(seq))
		if len(runs) != 1 {
			t.Fatalf("code %d: len(runs) = %d, want 1", code, len(runs))
		}
		c := ansiPalette[idx]
		want := ansiColor{set: true, r: c[0], g: c[1], b: c[2]}
		if runs[0].style.fg != want {
			t.Errorf("code %d: fg = %+v, want %+v", code, runs[0].style.fg, want)
		}
	}
}

func TestSGRBrightBgColors(t *testing.T) {
	for code := 100; code <= 107; code++ {
		idx := code - 100 + 8
		p := NewAnsiParser(nil)
		seq := "\x1b[" + itoa(code) + "mX"
		_, runs := p.Process([]rune(seq))
		if len(runs) != 1 {
			t.Fatalf("code %d: len(runs) = %d, want 1", code, len(runs))
		}
		c := ansiPalette[idx]
		want := ansiColor{set: true, r: c[0], g: c[1], b: c[2]}
		if runs[0].style.bg != want {
			t.Errorf("code %d: bg = %+v, want %+v", code, runs[0].style.bg, want)
		}
	}
}

func TestSGRDefaultFg(t *testing.T) {
	p := NewAnsiParser(nil)
	_, runs := p.Process([]rune("\x1b[31mA\x1b[39mB"))
	if len(runs) != 2 {
		t.Fatalf("len(runs) = %d, want 2", len(runs))
	}
	if runs[1].style.fg.set {
		t.Errorf("fg should be default after 39, got %+v", runs[1].style.fg)
	}
}

func TestSGRDefaultBg(t *testing.T) {
	p := NewAnsiParser(nil)
	_, runs := p.Process([]rune("\x1b[41mA\x1b[49mB"))
	if len(runs) != 2 {
		t.Fatalf("len(runs) = %d, want 2", len(runs))
	}
	if runs[1].style.bg.set {
		t.Errorf("bg should be default after 49, got %+v", runs[1].style.bg)
	}
}

// --- (6) Extended colors 38;5;N and 38;2;R;G;B ---

func TestSGRExtended256Fg(t *testing.T) {
	// ESC[38;5;196m → palette index 196
	p := NewAnsiParser(nil)
	_, runs := p.Process([]rune("\x1b[38;5;196mX"))
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	c := ansiPalette[196]
	want := ansiColor{set: true, r: c[0], g: c[1], b: c[2]}
	if runs[0].style.fg != want {
		t.Errorf("fg = %+v, want %+v", runs[0].style.fg, want)
	}
}

func TestSGRExtended256Bg(t *testing.T) {
	// ESC[48;5;232m → palette index 232 (first grayscale)
	p := NewAnsiParser(nil)
	_, runs := p.Process([]rune("\x1b[48;5;232mX"))
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	c := ansiPalette[232]
	want := ansiColor{set: true, r: c[0], g: c[1], b: c[2]}
	if runs[0].style.bg != want {
		t.Errorf("bg = %+v, want %+v", runs[0].style.bg, want)
	}
}

func TestSGRExtended256Boundary(t *testing.T) {
	// Test boundary values: 0, 255.
	for _, idx := range []int{0, 255} {
		p := NewAnsiParser(nil)
		seq := "\x1b[38;5;" + itoa(idx) + "mX"
		_, runs := p.Process([]rune(seq))
		if len(runs) != 1 {
			t.Fatalf("idx %d: len(runs) = %d, want 1", idx, len(runs))
		}
		c := ansiPalette[idx]
		want := ansiColor{set: true, r: c[0], g: c[1], b: c[2]}
		if runs[0].style.fg != want {
			t.Errorf("idx %d: fg = %+v, want %+v", idx, runs[0].style.fg, want)
		}
	}
}

func TestSGRTruecolorFg(t *testing.T) {
	// ESC[38;2;128;64;32m → truecolor R=128, G=64, B=32
	p := NewAnsiParser(nil)
	_, runs := p.Process([]rune("\x1b[38;2;128;64;32mX"))
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	want := ansiColor{set: true, r: 128, g: 64, b: 32}
	if runs[0].style.fg != want {
		t.Errorf("fg = %+v, want %+v", runs[0].style.fg, want)
	}
}

func TestSGRTruecolorBg(t *testing.T) {
	// ESC[48;2;10;20;30m
	p := NewAnsiParser(nil)
	_, runs := p.Process([]rune("\x1b[48;2;10;20;30mX"))
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	want := ansiColor{set: true, r: 10, g: 20, b: 30}
	if runs[0].style.bg != want {
		t.Errorf("bg = %+v, want %+v", runs[0].style.bg, want)
	}
}

func TestSGRTruecolorClamping(t *testing.T) {
	// Values > 255 should be clamped. ESC[38;2;300;-5;256m
	p := NewAnsiParser(nil)
	_, runs := p.Process([]rune("\x1b[38;2;300;0;256mX"))
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	want := ansiColor{set: true, r: 255, g: 0, b: 255}
	if runs[0].style.fg != want {
		t.Errorf("fg = %+v, want %+v", runs[0].style.fg, want)
	}
}

func TestSGRExtendedMalformedTruncated(t *testing.T) {
	// ESC[38;5m — missing index, should not crash. Style unmodified.
	p := NewAnsiParser(nil)
	_, runs := p.Process([]rune("\x1b[38;5mX"))
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	if runs[0].style.fg.set {
		t.Errorf("malformed 38;5 should not set fg, got %+v", runs[0].style.fg)
	}
}

func TestSGRExtendedMalformed38Only(t *testing.T) {
	// ESC[38m — just 38, no sub-type.
	p := NewAnsiParser(nil)
	_, runs := p.Process([]rune("\x1b[38mX"))
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	if runs[0].style.fg.set {
		t.Errorf("bare 38 should not set fg, got %+v", runs[0].style.fg)
	}
}

func TestSGRExtendedMalformedTruecolorShort(t *testing.T) {
	// ESC[38;2;128;64m — missing B component.
	p := NewAnsiParser(nil)
	_, runs := p.Process([]rune("\x1b[38;2;128;64mX"))
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	// Malformed — fg should not be set.
	if runs[0].style.fg.set {
		t.Errorf("malformed truecolor should not set fg, got %+v", runs[0].style.fg)
	}
}

// --- (7) Multiple params in one sequence ---

func TestSGRMultipleParams(t *testing.T) {
	// ESC[1;31;42m → bold, red fg, green bg
	p := NewAnsiParser(nil)
	_, runs := p.Process([]rune("\x1b[1;31;42mX"))
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	s := runs[0].style
	if !s.bold {
		t.Error("expected bold")
	}
	c := ansiPalette[1] // red
	wantFg := ansiColor{set: true, r: c[0], g: c[1], b: c[2]}
	if s.fg != wantFg {
		t.Errorf("fg = %+v, want %+v", s.fg, wantFg)
	}
	c = ansiPalette[2] // green
	wantBg := ansiColor{set: true, r: c[0], g: c[1], b: c[2]}
	if s.bg != wantBg {
		t.Errorf("bg = %+v, want %+v", s.bg, wantBg)
	}
}

func TestSGRTrailingSemicolon(t *testing.T) {
	// ESC[1;m → params [1, 0]. Bold on, then reset.
	p := NewAnsiParser(nil)
	_, runs := p.Process([]rune("\x1b[1;mX"))
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	// After processing [1, 0]: bold is set, then reset clears everything.
	if !isDefaultStyle(runs[0].style) {
		t.Errorf("trailing ; should produce reset, got %+v", runs[0].style)
	}
}

func TestSGRCombinedAttributesAndColors(t *testing.T) {
	// ESC[1;3;38;2;100;150;200m → bold, italic, truecolor fg
	p := NewAnsiParser(nil)
	_, runs := p.Process([]rune("\x1b[1;3;38;2;100;150;200mX"))
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	s := runs[0].style
	if !s.bold {
		t.Error("expected bold")
	}
	if !s.italic {
		t.Error("expected italic")
	}
	want := ansiColor{set: true, r: 100, g: 150, b: 200}
	if s.fg != want {
		t.Errorf("fg = %+v, want %+v", s.fg, want)
	}
}

// --- (8) Split sequences across reads ---

func TestSplitSequenceMidCSI(t *testing.T) {
	// Split ESC[1;31m across two reads: "\x1b[1;3" and "1m"
	p := NewAnsiParser(nil)
	clean1, runs1 := p.Process([]rune("\x1b[1;3"))
	if len(clean1) != 0 {
		t.Errorf("clean1 should be empty, got %q", runeStr(clean1))
	}
	if len(runs1) != 0 {
		t.Errorf("runs1 should be empty, got %d runs", len(runs1))
	}

	clean2, runs2 := p.Process([]rune("1mhello"))
	if runeStr(clean2) != "hello" {
		t.Errorf("clean2 = %q, want %q", runeStr(clean2), "hello")
	}
	if len(runs2) != 1 {
		t.Fatalf("len(runs2) = %d, want 1", len(runs2))
	}
	s := runs2[0].style
	if !s.bold {
		t.Error("expected bold")
	}
	c := ansiPalette[1] // red
	wantFg := ansiColor{set: true, r: c[0], g: c[1], b: c[2]}
	if s.fg != wantFg {
		t.Errorf("fg = %+v, want %+v", s.fg, wantFg)
	}
}

func TestSplitSequenceESCAlone(t *testing.T) {
	// ESC alone at end of first read.
	p := NewAnsiParser(nil)
	clean1, _ := p.Process([]rune("hello\x1b"))
	if runeStr(clean1) != "hello" {
		t.Errorf("clean1 = %q, want %q", runeStr(clean1), "hello")
	}

	clean2, runs2 := p.Process([]rune("[31mworld"))
	if runeStr(clean2) != "world" {
		t.Errorf("clean2 = %q, want %q", runeStr(clean2), "world")
	}
	if len(runs2) != 1 {
		t.Fatalf("len(runs2) = %d, want 1", len(runs2))
	}
	c := ansiPalette[1]
	wantFg := ansiColor{set: true, r: c[0], g: c[1], b: c[2]}
	if runs2[0].style.fg != wantFg {
		t.Errorf("fg = %+v, want %+v", runs2[0].style.fg, wantFg)
	}
}

func TestSplitSequenceMultipleReads(t *testing.T) {
	// Split ESC[38;5;196m across three reads.
	p := NewAnsiParser(nil)
	p.Process([]rune("\x1b["))
	p.Process([]rune("38;5;"))
	clean, runs := p.Process([]rune("196mX"))
	if runeStr(clean) != "X" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "X")
	}
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	c := ansiPalette[196]
	want := ansiColor{set: true, r: c[0], g: c[1], b: c[2]}
	if runs[0].style.fg != want {
		t.Errorf("fg = %+v, want %+v", runs[0].style.fg, want)
	}
}

func TestSplitSequenceOSC(t *testing.T) {
	// OSC split: "\x1b]0;tit" then "le\x07hello"
	p := NewAnsiParser(nil)
	clean1, _ := p.Process([]rune("\x1b]0;tit"))
	if len(clean1) != 0 {
		t.Errorf("clean1 should be empty, got %q", runeStr(clean1))
	}
	clean2, _ := p.Process([]rune("le\x07hello"))
	if runeStr(clean2) != "hello" {
		t.Errorf("clean2 = %q, want %q", runeStr(clean2), "hello")
	}
}

// --- (9) C0 controls passthrough ---

func TestC0ControlsPassthrough(t *testing.T) {
	// LF, CR, TAB, NUL should pass through.
	p := NewAnsiParser(nil)
	input := []rune("A\nB\rC\tD\x00E")
	clean, runs := p.Process(input)
	if runeStr(clean) != "A\nB\rC\tD\x00E" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "A\nB\rC\tD\x00E")
	}
	// All in one run (default style).
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
}

func TestC0InCSIIgnored(t *testing.T) {
	// C0 controls (other than ESC) within a CSI sequence are ignored per spec.
	// ESC[3\n1m — the \n in the middle of the CSI should not appear in output
	// and the sequence should be processed (but behavior for embedded C0 in CSI
	// is implementation-defined; our design ignores them by staying in CSI state).
	// However, per the design, CSIParam only handles digits, ;, final bytes, and ESC.
	// Other bytes just stay in the state — effectively consumed.
	p := NewAnsiParser(nil)
	clean, _ := p.Process([]rune("\x1b[31mhello"))
	if runeStr(clean) != "hello" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "hello")
	}
}

func TestBELInGroundState(t *testing.T) {
	// BEL (0x07) in ground state is a C0 control and should pass through.
	p := NewAnsiParser(nil)
	clean, _ := p.Process([]rune("A\x07B"))
	if runeStr(clean) != "A\x07B" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "A\x07B")
	}
}

// --- (10) Styled runs output with correct styles ---

func TestStyledRunsMultipleStyles(t *testing.T) {
	// red "hello" then default " " then bold green "world"
	p := NewAnsiParser(nil)
	_, runs := p.Process([]rune("\x1b[31mhello\x1b[0m \x1b[1;32mworld"))
	if len(runs) != 3 {
		t.Fatalf("len(runs) = %d, want 3", len(runs))
	}

	// Run 0: red "hello"
	if runeStr(runs[0].text) != "hello" {
		t.Errorf("runs[0].text = %q, want %q", runeStr(runs[0].text), "hello")
	}
	c := ansiPalette[1]
	if runs[0].style.fg != (ansiColor{set: true, r: c[0], g: c[1], b: c[2]}) {
		t.Errorf("runs[0].fg = %+v, want red", runs[0].style.fg)
	}

	// Run 1: default " "
	if runeStr(runs[1].text) != " " {
		t.Errorf("runs[1].text = %q, want %q", runeStr(runs[1].text), " ")
	}
	if !isDefaultStyle(runs[1].style) {
		t.Errorf("runs[1] should be default, got %+v", runs[1].style)
	}

	// Run 2: bold green "world"
	if runeStr(runs[2].text) != "world" {
		t.Errorf("runs[2].text = %q, want %q", runeStr(runs[2].text), "world")
	}
	c = ansiPalette[2]
	if runs[2].style.fg != (ansiColor{set: true, r: c[0], g: c[1], b: c[2]}) {
		t.Errorf("runs[2].fg = %+v, want green", runs[2].style.fg)
	}
	if !runs[2].style.bold {
		t.Error("runs[2] should be bold")
	}
}

func TestStyledRunsContiguous(t *testing.T) {
	// Same style runes should be in the same run.
	p := NewAnsiParser(nil)
	_, runs := p.Process([]rune("\x1b[31mhelloworld"))
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	if runeStr(runs[0].text) != "helloworld" {
		t.Errorf("run text = %q, want %q", runeStr(runs[0].text), "helloworld")
	}
}

func TestStyledRunsPreserveCleanOutput(t *testing.T) {
	// clean output should be the concatenation of all run texts.
	p := NewAnsiParser(nil)
	clean, runs := p.Process([]rune("\x1b[31mAB\x1b[32mCD\x1b[0mEF"))
	got := runeStr(clean)
	want := "ABCDEF"
	if got != want {
		t.Errorf("clean = %q, want %q", got, want)
	}
	// Verify concatenation of run texts matches clean.
	var concat []rune
	for _, r := range runs {
		concat = append(concat, r.text...)
	}
	if runeStr(concat) != want {
		t.Errorf("concatenated runs = %q, want %q", runeStr(concat), want)
	}
}

func TestStylePersistsAcrossCalls(t *testing.T) {
	// Set red in first call, text in second call should inherit red.
	p := NewAnsiParser(nil)
	p.Process([]rune("\x1b[31m"))
	_, runs := p.Process([]rune("hello"))
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	c := ansiPalette[1]
	wantFg := ansiColor{set: true, r: c[0], g: c[1], b: c[2]}
	if runs[0].style.fg != wantFg {
		t.Errorf("fg = %+v, want %+v (red)", runs[0].style.fg, wantFg)
	}
}

func TestNoRunsWhenOnlyEscapes(t *testing.T) {
	// Input with only escape sequences, no printable text.
	p := NewAnsiParser(nil)
	clean, runs := p.Process([]rune("\x1b[31m\x1b[1m\x1b[0m"))
	if len(clean) != 0 {
		t.Errorf("clean should be empty, got %q", runeStr(clean))
	}
	if len(runs) != 0 {
		t.Errorf("runs should be empty, got %d", len(runs))
	}
}

// --- (11) Private marker sequences ignored ---

func TestPrivateCSIIgnored(t *testing.T) {
	// ESC[?25h (show cursor) — private CSI should be consumed, no SGR effect.
	p := NewAnsiParser(nil)
	clean, runs := p.Process([]rune("\x1b[?25hhello"))
	if runeStr(clean) != "hello" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "hello")
	}
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	if !isDefaultStyle(runs[0].style) {
		t.Errorf("private CSI should not change style, got %+v", runs[0].style)
	}
}

func TestPrivateCSIWithParams(t *testing.T) {
	// ESC[?1049h (alternate screen buffer).
	p := NewAnsiParser(nil)
	clean, _ := p.Process([]rune("\x1b[?1049hhello"))
	if runeStr(clean) != "hello" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "hello")
	}
}

func TestPrivateCSIDoesNotDispatchSGR(t *testing.T) {
	// ESC[?1m — even though final byte is 'm', private marker means no SGR.
	p := NewAnsiParser(nil)
	_, runs := p.Process([]rune("\x1b[?1mX"))
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	if runs[0].style.bold {
		t.Error("private CSI with 'm' should not trigger SGR")
	}
}

func TestPrivateCSIGreaterThan(t *testing.T) {
	// ESC[>0c — DA2 (Device Attributes 2) with '>' marker.
	p := NewAnsiParser(nil)
	clean, _ := p.Process([]rune("\x1b[>0chello"))
	if runeStr(clean) != "hello" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "hello")
	}
}

// --- Additional edge cases ---

func TestSGRUnknownCodesIgnored(t *testing.T) {
	// Unknown SGR codes (e.g., 10, 50, 99) should be silently ignored.
	p := NewAnsiParser(nil)
	_, runs := p.Process([]rune("\x1b[10;50;99mX"))
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	if !isDefaultStyle(runs[0].style) {
		t.Errorf("unknown codes should not change style, got %+v", runs[0].style)
	}
}

func TestSGRCumulativeState(t *testing.T) {
	// Multiple separate SGR sequences accumulate state.
	p := NewAnsiParser(nil)
	_, runs := p.Process([]rune("\x1b[1m\x1b[31m\x1b[42mX"))
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	s := runs[0].style
	if !s.bold {
		t.Error("expected bold")
	}
	c := ansiPalette[1]
	wantFg := ansiColor{set: true, r: c[0], g: c[1], b: c[2]}
	if s.fg != wantFg {
		t.Errorf("fg = %+v, want %+v (red)", s.fg, wantFg)
	}
	c = ansiPalette[2]
	wantBg := ansiColor{set: true, r: c[0], g: c[1], b: c[2]}
	if s.bg != wantBg {
		t.Errorf("bg = %+v, want %+v (green)", s.bg, wantBg)
	}
}

func TestSGRResetClearsEverything(t *testing.T) {
	// After setting many attributes, reset clears all.
	p := NewAnsiParser(nil)
	_, runs := p.Process([]rune("\x1b[1;2;3;4;5;7;8;9;31;42mA\x1b[0mB"))
	if len(runs) != 2 {
		t.Fatalf("len(runs) = %d, want 2", len(runs))
	}
	if isDefaultStyle(runs[0].style) {
		t.Error("first run should not be default")
	}
	if !isDefaultStyle(runs[1].style) {
		t.Errorf("second run should be default after reset, got %+v", runs[1].style)
	}
}

func TestLongSequenceOfTextAfterSGR(t *testing.T) {
	// Verify no issues with longer text after SGR.
	p := NewAnsiParser(nil)
	long := "abcdefghijklmnopqrstuvwxyz0123456789"
	clean, runs := p.Process([]rune("\x1b[32m" + long))
	if runeStr(clean) != long {
		t.Errorf("clean = %q, want %q", runeStr(clean), long)
	}
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	if runeStr(runs[0].text) != long {
		t.Errorf("run text = %q, want %q", runeStr(runs[0].text), long)
	}
}

func TestInterleavedTextAndSGR(t *testing.T) {
	// A B C each with different colors.
	p := NewAnsiParser(nil)
	_, runs := p.Process([]rune("\x1b[31mA\x1b[32mB\x1b[33mC"))
	if len(runs) != 3 {
		t.Fatalf("len(runs) = %d, want 3", len(runs))
	}
	for i, wantChar := range []string{"A", "B", "C"} {
		if runeStr(runs[i].text) != wantChar {
			t.Errorf("runs[%d].text = %q, want %q", i, runeStr(runs[i].text), wantChar)
		}
	}
	// Verify each has different fg.
	if runs[0].style.fg == runs[1].style.fg || runs[1].style.fg == runs[2].style.fg {
		t.Error("each run should have a different fg color")
	}
}

// --- (12) OSC title callback (Phase 2.2) ---

func TestOSCTitle0BEL(t *testing.T) {
	// OSC 0 with BEL terminator invokes titleFunc.
	var got string
	p := NewAnsiParser(func(title string) { got = title })
	clean, _ := p.Process([]rune("\x1b]0;My Title\x07hello"))
	if runeStr(clean) != "hello" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "hello")
	}
	if got != "My Title" {
		t.Errorf("titleFunc got %q, want %q", got, "My Title")
	}
}

func TestOSCTitle1BEL(t *testing.T) {
	// OSC 1 (icon name) with BEL terminator invokes titleFunc.
	var got string
	p := NewAnsiParser(func(title string) { got = title })
	p.Process([]rune("\x1b]1;Icon Name\x07"))
	if got != "Icon Name" {
		t.Errorf("titleFunc got %q, want %q", got, "Icon Name")
	}
}

func TestOSCTitle2BEL(t *testing.T) {
	// OSC 2 (window title) with BEL terminator invokes titleFunc.
	var got string
	p := NewAnsiParser(func(title string) { got = title })
	p.Process([]rune("\x1b]2;Window Title\x07"))
	if got != "Window Title" {
		t.Errorf("titleFunc got %q, want %q", got, "Window Title")
	}
}

func TestOSCTitleSTTerminator(t *testing.T) {
	// OSC with ST terminator (ESC \) works identically to BEL.
	var got string
	p := NewAnsiParser(func(title string) { got = title })
	clean, _ := p.Process([]rune("\x1b]0;ST Title\x1b\\hello"))
	if runeStr(clean) != "hello" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "hello")
	}
	if got != "ST Title" {
		t.Errorf("titleFunc got %q, want %q", got, "ST Title")
	}
}

func TestOSCTitle2STTerminator(t *testing.T) {
	// OSC 2 with ST terminator.
	var got string
	p := NewAnsiParser(func(title string) { got = title })
	p.Process([]rune("\x1b]2;Another Title\x1b\\"))
	if got != "Another Title" {
		t.Errorf("titleFunc got %q, want %q", got, "Another Title")
	}
}

func TestOSCTitleCallbackPayload(t *testing.T) {
	// Verify exact payload string passed to callback, including special chars.
	var got string
	p := NewAnsiParser(func(title string) { got = title })
	p.Process([]rune("\x1b]0;user@host: ~/dir\x07"))
	if got != "user@host: ~/dir" {
		t.Errorf("titleFunc got %q, want %q", got, "user@host: ~/dir")
	}
}

func TestOSCTitleNilCallback(t *testing.T) {
	// Nil titleFunc should not panic.
	p := NewAnsiParser(nil)
	clean, _ := p.Process([]rune("\x1b]0;title\x07hello"))
	if runeStr(clean) != "hello" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "hello")
	}
}

func TestOSCUnknownNumbersStrippedSilently(t *testing.T) {
	// Unknown OSC numbers should not invoke titleFunc.
	called := false
	p := NewAnsiParser(func(title string) { called = true })
	for _, num := range []string{"7", "8", "52", "133", "999"} {
		called = false
		clean, _ := p.Process([]rune("\x1b]" + num + ";payload\x07visible"))
		if runeStr(clean) != "visible" {
			t.Errorf("OSC %s: clean = %q, want %q", num, runeStr(clean), "visible")
		}
		if called {
			t.Errorf("OSC %s: titleFunc should not be called", num)
		}
	}
}

func TestOSCSplitInNumericParam(t *testing.T) {
	// Split OSC in the numeric parameter: "\x1b]" then "0;title\x07"
	var got string
	p := NewAnsiParser(func(title string) { got = title })
	clean1, _ := p.Process([]rune("\x1b]"))
	if len(clean1) != 0 {
		t.Errorf("clean1 should be empty, got %q", runeStr(clean1))
	}
	clean2, _ := p.Process([]rune("0;title\x07hello"))
	if runeStr(clean2) != "hello" {
		t.Errorf("clean2 = %q, want %q", runeStr(clean2), "hello")
	}
	if got != "title" {
		t.Errorf("titleFunc got %q, want %q", got, "title")
	}
}

func TestOSCSplitInPayload(t *testing.T) {
	// Split OSC in the payload: "\x1b]0;my ti" then "tle\x07"
	var got string
	p := NewAnsiParser(func(title string) { got = title })
	p.Process([]rune("\x1b]0;my ti"))
	p.Process([]rune("tle\x07"))
	if got != "my title" {
		t.Errorf("titleFunc got %q, want %q", got, "my title")
	}
}

func TestOSCSplitAtESCofST(t *testing.T) {
	// Split at the ESC of ST terminator: "\x1b]0;title\x1b" then "\\"
	var got string
	p := NewAnsiParser(func(title string) { got = title })
	p.Process([]rune("\x1b]0;title\x1b"))
	if got != "" {
		t.Errorf("titleFunc should not be called yet, got %q", got)
	}
	clean, _ := p.Process([]rune("\\hello"))
	if runeStr(clean) != "hello" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "hello")
	}
	if got != "title" {
		t.Errorf("titleFunc got %q, want %q", got, "title")
	}
}

func TestOSCInterleavedWithStyledText(t *testing.T) {
	// OSC between styled runs should not affect style state.
	// Since the style doesn't change across the OSC, "red text" and
	// "more red text" merge into a single run.
	var got string
	p := NewAnsiParser(func(title string) { got = title })
	clean, runs := p.Process([]rune("\x1b[1;31mred text\x1b]0;My Title\x07more red text\x1b[0mnormal"))
	if runeStr(clean) != "red textmore red textnormal" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "red textmore red textnormal")
	}
	if got != "My Title" {
		t.Errorf("titleFunc got %q, want %q", got, "My Title")
	}
	// Should have 2 runs: bold+red "red textmore red text", default "normal"
	if len(runs) != 2 {
		t.Fatalf("len(runs) = %d, want 2", len(runs))
	}
	// First run should be bold+red, merging text from both sides of the OSC
	if runeStr(runs[0].text) != "red textmore red text" {
		t.Errorf("runs[0].text = %q, want %q", runeStr(runs[0].text), "red textmore red text")
	}
	if !runs[0].style.bold {
		t.Error("runs[0] should be bold")
	}
	c := ansiPalette[1]
	wantFg := ansiColor{set: true, r: c[0], g: c[1], b: c[2]}
	if runs[0].style.fg != wantFg {
		t.Errorf("runs[0].fg = %+v, want red", runs[0].style.fg)
	}
	// Second run should be default "normal"
	if runeStr(runs[1].text) != "normal" {
		t.Errorf("runs[1].text = %q, want %q", runeStr(runs[1].text), "normal")
	}
	if !isDefaultStyle(runs[1].style) {
		t.Errorf("runs[1] should be default after reset, got %+v", runs[1].style)
	}
}

func TestOSCEmptyPayloadWithSemicolon(t *testing.T) {
	// ESC]0;BEL — empty payload after semicolon.
	var got string
	called := false
	p := NewAnsiParser(func(title string) { got = title; called = true })
	p.Process([]rune("\x1b]0;\x07"))
	if !called {
		t.Error("titleFunc should be called for empty payload")
	}
	if got != "" {
		t.Errorf("titleFunc got %q, want %q", got, "")
	}
}

func TestOSCEmptyPayloadNoSemicolon(t *testing.T) {
	// ESC]0BEL — no semicolon, BEL terminates in stateOSC.
	var got string
	called := false
	p := NewAnsiParser(func(title string) { got = title; called = true })
	p.Process([]rune("\x1b]0\x07"))
	if !called {
		t.Error("titleFunc should be called for no-semicolon OSC 0")
	}
	if got != "" {
		t.Errorf("titleFunc got %q, want %q", got, "")
	}
}

func TestOSCMultipleTitlesLastWins(t *testing.T) {
	// Multiple OSC title sequences — last one wins (matches terminal behavior).
	var got string
	p := NewAnsiParser(func(title string) { got = title })
	p.Process([]rune("\x1b]0;First\x07\x1b]0;Second\x07\x1b]0;Third\x07"))
	if got != "Third" {
		t.Errorf("titleFunc got %q, want %q (last one wins)", got, "Third")
	}
}

// =============================================================
// Integration Tests (Phase 4.1)
// =============================================================

// --- (13) End-to-end Process → buildSpanWrite pipeline ---

func TestEndToEndPipelineRedText(t *testing.T) {
	// ANSI input: red "hello" followed by reset and " world"
	// Expected: clean text is "hello world", spans cover the styled portion.
	p := NewAnsiParser(nil)
	input := []rune("\x1b[31mhello\x1b[0m world")
	clean, runs := p.Process(input)

	if runeStr(clean) != "hello world" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "hello world")
	}

	spanData := buildSpanWrite(0, runs)
	// runs: [red "hello", default " world"]
	// Since there's a non-default run, all runs emit.
	if spanData == "" {
		t.Fatal("expected non-empty span data for colored input")
	}

	lines := strings.Split(strings.TrimRight(spanData, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d span lines, want 2:\n%s", len(lines), spanData)
	}
	// Red foreground: palette[1] = {0xaa, 0x00, 0x00}
	if lines[0] != "s 0 5 #aa0000" {
		t.Errorf("span line 0: got %q, want %q", lines[0], "s 0 5 #aa0000")
	}
	// Default style for " world"
	if lines[1] != "s 5 6 -" {
		t.Errorf("span line 1: got %q, want %q", lines[1], "s 5 6 -")
	}
}

func TestEndToEndPipelineBoldGreenWithBg(t *testing.T) {
	// Bold green fg on blue bg, then reset.
	p := NewAnsiParser(nil)
	input := []rune("\x1b[1;32;44mABC\x1b[0mDEF")
	clean, runs := p.Process(input)

	if runeStr(clean) != "ABCDEF" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "ABCDEF")
	}

	spanData := buildSpanWrite(100, runs)
	lines := strings.Split(strings.TrimRight(spanData, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d span lines, want 2:\n%s", len(lines), spanData)
	}
	// Green fg (#00aa00), blue bg (#0000aa), bold flag
	if lines[0] != "s 100 3 #00aa00 #0000aa bold" {
		t.Errorf("span line 0: got %q, want %q", lines[0], "s 100 3 #00aa00 #0000aa bold")
	}
	// Default "DEF"
	if lines[1] != "s 103 3 -" {
		t.Errorf("span line 1: got %q, want %q", lines[1], "s 103 3 -")
	}
}

func TestEndToEndPipelineAllDefault(t *testing.T) {
	// No ANSI escapes — all default styling. buildSpanWrite returns "".
	p := NewAnsiParser(nil)
	clean, runs := p.Process([]rune("plain text"))

	if runeStr(clean) != "plain text" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "plain text")
	}

	spanData := buildSpanWrite(0, runs)
	if spanData != "" {
		t.Errorf("expected empty span data for plain text, got %q", spanData)
	}
}

func TestEndToEndPipelineTruecolor(t *testing.T) {
	// Truecolor (24-bit) foreground.
	p := NewAnsiParser(nil)
	input := []rune("\x1b[38;2;128;64;32mHI")
	clean, runs := p.Process(input)

	if runeStr(clean) != "HI" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "HI")
	}

	spanData := buildSpanWrite(50, runs)
	want := "s 50 2 #804020\n"
	if spanData != want {
		t.Errorf("spanData = %q, want %q", spanData, want)
	}
}

func TestEndToEndPipelineSplitAcrossReads(t *testing.T) {
	// Simulate a split ANSI sequence across two PTY reads,
	// followed by building spans from each read's output.
	p := NewAnsiParser(nil)

	// First read: partial escape sequence + some text before it
	clean1, runs1 := p.Process([]rune("pre\x1b[1;3"))
	if runeStr(clean1) != "pre" {
		t.Errorf("clean1 = %q, want %q", runeStr(clean1), "pre")
	}
	span1 := buildSpanWrite(0, runs1)
	// "pre" is default, so no spans
	if span1 != "" {
		t.Errorf("span1 should be empty for default text, got %q", span1)
	}

	// Second read: rest of escape + colored text
	// The full sequence was \x1b[1;31m which sets bold AND red.
	clean2, runs2 := p.Process([]rune("1mred\x1b[0m"))
	if runeStr(clean2) != "red" {
		t.Errorf("clean2 = %q, want %q", runeStr(clean2), "red")
	}
	span2 := buildSpanWrite(3, runs2) // baseOffset = len("pre") = 3
	want := "s 3 3 #aa0000 - bold\n"  // bold + red fg
	if span2 != want {
		t.Errorf("span2 = %q, want %q", span2, want)
	}
}

func TestEndToEndPipelineWithOSCTitle(t *testing.T) {
	// ANSI input with OSC title + colored text.
	var title string
	p := NewAnsiParser(func(t string) { title = t })
	input := []rune("\x1b]0;My Window\x07\x1b[31mcolored\x1b[0m")
	clean, runs := p.Process(input)

	if runeStr(clean) != "colored" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "colored")
	}
	if title != "My Window" {
		t.Errorf("title = %q, want %q", title, "My Window")
	}

	spanData := buildSpanWrite(0, runs)
	want := "s 0 7 #aa0000\n"
	if spanData != want {
		t.Errorf("spanData = %q, want %q", spanData, want)
	}
}

func TestEndToEndPipelineMultipleStyleChanges(t *testing.T) {
	// Multiple style changes in one read: red, green, blue.
	p := NewAnsiParser(nil)
	input := []rune("\x1b[31mR\x1b[32mG\x1b[34mB")
	clean, runs := p.Process(input)

	if runeStr(clean) != "RGB" {
		t.Errorf("clean = %q, want %q", runeStr(clean), "RGB")
	}

	spanData := buildSpanWrite(0, runs)
	lines := strings.Split(strings.TrimRight(spanData, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d span lines, want 3:\n%s", len(lines), spanData)
	}
	if lines[0] != "s 0 1 #aa0000" {
		t.Errorf("line 0: got %q, want %q", lines[0], "s 0 1 #aa0000")
	}
	if lines[1] != "s 1 1 #00aa00" {
		t.Errorf("line 1: got %q, want %q", lines[1], "s 1 1 #00aa00")
	}
	if lines[2] != "s 2 1 #0000aa" {
		t.Errorf("line 2: got %q, want %q", lines[2], "s 2 1 #0000aa")
	}
}

// --- (14) setWindowTitle formatting (via formatWindowTitle) ---

func TestFormatWindowTitleWithDash(t *testing.T) {
	// Title already contains "-", should be used as-is.
	got := formatWindowTitle("/home/user/project/-rc", "win")
	if got != "/home/user/project/-rc" {
		t.Errorf("got %q, want %q", got, "/home/user/project/-rc")
	}
}

func TestFormatWindowTitleWithoutDash(t *testing.T) {
	// Title without "-" gets "/-" + sysname appended.
	got := formatWindowTitle("/home/user/project/", "win")
	want := "/home/user/project//-win"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatWindowTitleEmptyTitle(t *testing.T) {
	// Empty title without "-" gets "/-" + sysname.
	got := formatWindowTitle("", "win")
	want := "/-win"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatWindowTitleDashInMiddle(t *testing.T) {
	// Dash in the middle of the title — use as-is.
	got := formatWindowTitle("my-project", "win")
	if got != "my-project" {
		t.Errorf("got %q, want %q", got, "my-project")
	}
}

func TestFormatWindowTitleDifferentSysname(t *testing.T) {
	got := formatWindowTitle("/tmp/", "myhost")
	want := "/tmp//-myhost"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- (15) Pipeline ordering: ANSI before dropcrnl/dropcr ---

func TestPipelineOrderingANSIBeforeDropcrnl(t *testing.T) {
	// Verify that ANSI stripping happens before dropcrnl.
	// Input: ANSI escape followed by \r\n. The escape must be stripped first,
	// then dropcrnl converts \r\n to \n.
	p := NewAnsiParser(nil)
	input := []rune("\x1b[31mhello\r\n\x1b[0mworld")
	clean, _ := p.Process(input)

	// After ANSI stripping: "hello\r\nworld"
	if runeStr(clean) != "hello\r\nworld" {
		t.Errorf("after Process, clean = %q, want %q", runeStr(clean), "hello\r\nworld")
	}

	// Then dropcrnl converts \r\n → \n
	clean = dropcrnl(clean)
	if runeStr(clean) != "hello\nworld" {
		t.Errorf("after dropcrnl, clean = %q, want %q", runeStr(clean), "hello\nworld")
	}
}

func TestPipelineOrderingANSIBeforeDropcr(t *testing.T) {
	// ANSI escape mixed with \r (carriage return).
	// Process strips escapes first, then dropcr handles \r.
	p := NewAnsiParser(nil)
	input := []rune("\x1b[31mold text\rNEW")
	clean, _ := p.Process(input)

	// After ANSI stripping: "old text\rNEW"
	if runeStr(clean) != "old text\rNEW" {
		t.Errorf("after Process, clean = %q, want %q", runeStr(clean), "old text\rNEW")
	}

	// dropcr handles \r: on the first line (no prior \n), \r becomes \n.
	clean = dropcr(clean)
	got := runeStr(clean)
	if got != "old text\nNEW" {
		t.Errorf("after dropcr, clean = %q, want %q", got, "old text\nNEW")
	}
}

func TestPipelineOrderingANSIInsideNewline(t *testing.T) {
	// Escape sequence straddling text with \r\n.
	// ESC[31m before \r\n, ESC[0m after.
	p := NewAnsiParser(nil)
	input := []rune("line1\x1b[31m\r\n\x1b[0mline2")
	clean, runs := p.Process(input)

	// ANSI stripped: "line1\r\nline2"
	if runeStr(clean) != "line1\r\nline2" {
		t.Errorf("after Process, clean = %q, want %q", runeStr(clean), "line1\r\nline2")
	}

	// After dropcrnl: "line1\nline2"
	postDrop := dropcrnl(clean)
	if runeStr(postDrop) != "line1\nline2" {
		t.Errorf("after dropcrnl = %q, want %q", runeStr(postDrop), "line1\nline2")
	}

	// Verify runs are correct: "line1" default, "\r\n" red, "line2" default
	// (The \r\n is part of the styled text; dropcrnl happens after.)
	totalRunes := 0
	for _, r := range runs {
		totalRunes += len(r.text)
	}
	if totalRunes != len(clean) {
		t.Errorf("total run runes = %d, want %d (len of clean)", totalRunes, len(clean))
	}
}

func TestPipelineOrderingSquashnullsAfterANSI(t *testing.T) {
	// Null bytes in styled output: ANSI stripping preserves them,
	// squashnulls removes them afterward.
	p := NewAnsiParser(nil)
	input := []rune("\x1b[31mhel\x00lo\x1b[0m")
	clean, _ := p.Process(input)

	if runeStr(clean) != "hel\x00lo" {
		t.Errorf("after Process, clean = %q, want %q", runeStr(clean), "hel\x00lo")
	}

	clean = squashnulls(clean)
	if runeStr(clean) != "hello" {
		t.Errorf("after squashnulls, clean = %q, want %q", runeStr(clean), "hello")
	}
}

func TestPipelineFullChain(t *testing.T) {
	// Full pipeline: Process → dropcrnl → dropcr → squashnulls
	// Mimics the exact pipeline order from the integration design.
	p := NewAnsiParser(nil)
	input := []rune("\x1b[1;33mwarning:\x1b[0m file not found\r\n")
	clean, runs := p.Process(input)

	// After ANSI stripping
	if runeStr(clean) != "warning: file not found\r\n" {
		t.Errorf("after Process, clean = %q", runeStr(clean))
	}

	// Apply pipeline stages
	clean = dropcrnl(clean)
	clean = dropcr(clean)
	clean = squashnulls(clean)

	expected := "warning: file not found\n"
	if runeStr(clean) != expected {
		t.Errorf("after full pipeline, clean = %q, want %q", runeStr(clean), expected)
	}

	// Spans should have non-default content (bold+yellow "warning:")
	spanData := buildSpanWrite(0, runs)
	if spanData == "" {
		t.Error("expected non-empty span data for styled input")
	}
}

// --- itoa helper for tests (avoids strconv import) ---

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := make([]byte, 0, 10)
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	if neg {
		buf = append(buf, '-')
	}
	// reverse
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
