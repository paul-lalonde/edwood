package main

import (
	"strings"
	"testing"
)

// helper: make a styledRun with the given text and style.
func mkRun(text string, style sgrState) styledRun {
	return styledRun{text: []rune(text), style: style}
}

// helper: make an sgrState with a set foreground color.
func fgStyle(r, g, b uint8) sgrState {
	return sgrState{fg: ansiColor{set: true, r: r, g: g, b: b}}
}

// helper: make an sgrState with set fg and bg.
func fgBgStyle(fr, fg, fb, br, bg, bb uint8) sgrState {
	return sgrState{
		fg: ansiColor{set: true, r: fr, g: fg, b: fb},
		bg: ansiColor{set: true, r: br, g: bg, b: bb},
	}
}

// --- (1) Single colored run ---

func TestBuildSpanWriteSingleColoredRun(t *testing.T) {
	runs := []styledRun{
		mkRun("hello", fgStyle(0xaa, 0, 0)),
	}
	got := buildSpanWrite(100, runs)
	want := "s 100 5 #aa0000\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- (2) Multiple contiguous runs ---

func TestBuildSpanWriteMultipleContiguousRuns(t *testing.T) {
	runs := []styledRun{
		mkRun("red", fgStyle(0xaa, 0, 0)),
		mkRun("text", sgrState{}), // default
		mkRun("green", func() sgrState {
			s := fgStyle(0, 0xaa, 0)
			s.bold = true
			return s
		}()),
	}
	got := buildSpanWrite(50, runs)
	// default run still emits since block has non-default runs
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3:\n%s", len(lines), got)
	}
	if lines[0] != "s 50 3 #aa0000" {
		t.Errorf("line 0: got %q, want %q", lines[0], "s 50 3 #aa0000")
	}
	if lines[1] != "s 53 4 -" {
		t.Errorf("line 1: got %q, want %q", lines[1], "s 53 4 -")
	}
	if lines[2] != "s 57 5 #00aa00 - bold" {
		t.Errorf("line 2: got %q, want %q", lines[2], "s 57 5 #00aa00 - bold")
	}
}

// --- (3) Default-style-only returns empty string ---

func TestBuildSpanWriteAllDefault(t *testing.T) {
	runs := []styledRun{
		mkRun("hello", sgrState{}),
		mkRun(" world", sgrState{}),
	}
	got := buildSpanWrite(0, runs)
	if got != "" {
		t.Errorf("all-default should return empty string, got %q", got)
	}
}

func TestBuildSpanWriteEmptyRuns(t *testing.T) {
	got := buildSpanWrite(0, nil)
	if got != "" {
		t.Errorf("nil runs should return empty string, got %q", got)
	}
}

func TestBuildSpanWriteEmptySlice(t *testing.T) {
	got := buildSpanWrite(0, []styledRun{})
	if got != "" {
		t.Errorf("empty runs should return empty string, got %q", got)
	}
}

// --- (4) Mixed default and styled runs ---

func TestBuildSpanWriteMixedDefaultAndStyled(t *testing.T) {
	runs := []styledRun{
		mkRun("plain", sgrState{}),
		mkRun("red", fgStyle(0xaa, 0, 0)),
		mkRun("plain2", sgrState{}),
	}
	got := buildSpanWrite(10, runs)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3:\n%s", len(lines), got)
	}
	// default run at start still emits
	if lines[0] != "s 10 5 -" {
		t.Errorf("line 0: got %q, want %q", lines[0], "s 10 5 -")
	}
	if lines[1] != "s 15 3 #aa0000" {
		t.Errorf("line 1: got %q, want %q", lines[1], "s 15 3 #aa0000")
	}
	if lines[2] != "s 18 6 -" {
		t.Errorf("line 2: got %q, want %q", lines[2], "s 18 6 -")
	}
}

// --- (5) Bold/italic/hidden flags ---

func TestBuildSpanWriteBoldFlag(t *testing.T) {
	s := fgStyle(0xff, 0, 0)
	s.bold = true
	runs := []styledRun{mkRun("bold", s)}
	got := buildSpanWrite(0, runs)
	want := "s 0 4 #ff0000 - bold\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildSpanWriteItalicFlag(t *testing.T) {
	s := fgStyle(0, 0xff, 0)
	s.italic = true
	runs := []styledRun{mkRun("ital", s)}
	got := buildSpanWrite(0, runs)
	want := "s 0 4 #00ff00 - italic\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildSpanWriteHiddenFlag(t *testing.T) {
	s := fgStyle(0, 0, 0xff)
	s.hidden = true
	runs := []styledRun{mkRun("hide", s)}
	got := buildSpanWrite(0, runs)
	want := "s 0 4 #0000ff - hidden\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildSpanWriteAllFlags(t *testing.T) {
	s := fgStyle(0xaa, 0xbb, 0xcc)
	s.bold = true
	s.italic = true
	s.hidden = true
	runs := []styledRun{mkRun("all", s)}
	got := buildSpanWrite(0, runs)
	want := "s 0 3 #aabbcc - bold italic hidden\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- (6) Foreground only vs fg+bg ---

func TestBuildSpanWriteFgOnly(t *testing.T) {
	runs := []styledRun{
		mkRun("fg", fgStyle(0xaa, 0, 0)),
	}
	got := buildSpanWrite(0, runs)
	// No bg, no flags → just fg
	want := "s 0 2 #aa0000\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildSpanWriteFgAndBg(t *testing.T) {
	runs := []styledRun{
		mkRun("both", fgBgStyle(0xaa, 0, 0, 0, 0, 0xaa)),
	}
	got := buildSpanWrite(0, runs)
	want := "s 0 4 #aa0000 #0000aa\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildSpanWriteBgOnly(t *testing.T) {
	// Default fg but explicit bg
	s := sgrState{bg: ansiColor{set: true, r: 0, g: 0xaa, b: 0}}
	runs := []styledRun{mkRun("bg", s)}
	got := buildSpanWrite(0, runs)
	want := "s 0 2 - #00aa00\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- (7) Inverse color swap ---

func TestBuildSpanWriteInverse(t *testing.T) {
	// Default fg/bg with inverse → resolveColors produces #fffff0 fg, #000000 bg
	s := sgrState{inverse: true}
	runs := []styledRun{mkRun("inv-text00", s)}
	got := buildSpanWrite(0, runs)
	want := "s 0 10 #fffff0 #000000\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildSpanWriteInverseExplicitColors(t *testing.T) {
	// Explicit fg=red, bg=blue with inverse → fg=blue, bg=red
	s := fgBgStyle(0xaa, 0, 0, 0, 0, 0xaa)
	s.inverse = true
	runs := []styledRun{mkRun("swap", s)}
	got := buildSpanWrite(0, runs)
	// After inverse: fg=#0000aa, bg=#aa0000
	want := "s 0 4 #0000aa #aa0000\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- (8) Dim brightness reduction ---

func TestBuildSpanWriteDim(t *testing.T) {
	s := fgStyle(0xaa, 0, 0)
	s.dim = true
	runs := []styledRun{mkRun("dim", s)}
	got := buildSpanWrite(0, runs)
	// Dim halves fg: 0xaa/2 = 0x55
	want := "s 0 3 #550000\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildSpanWriteDimDefaultFg(t *testing.T) {
	s := sgrState{dim: true}
	runs := []styledRun{mkRun("dimdef", s)}
	got := buildSpanWrite(0, runs)
	// Dim with default fg → mid-gray #808080
	want := "s 0 6 #808080\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- (9) Offset arithmetic correctness ---

func TestBuildSpanWriteOffsetArithmetic(t *testing.T) {
	runs := []styledRun{
		mkRun("ab", fgStyle(0xff, 0, 0)),    // offset=1000, len=2
		mkRun("cde", fgStyle(0, 0xff, 0)),   // offset=1002, len=3
		mkRun("f", fgStyle(0, 0, 0xff)),      // offset=1005, len=1
	}
	got := buildSpanWrite(1000, runs)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3:\n%s", len(lines), got)
	}
	if lines[0] != "s 1000 2 #ff0000" {
		t.Errorf("line 0: got %q, want %q", lines[0], "s 1000 2 #ff0000")
	}
	if lines[1] != "s 1002 3 #00ff00" {
		t.Errorf("line 1: got %q, want %q", lines[1], "s 1002 3 #00ff00")
	}
	if lines[2] != "s 1005 1 #0000ff" {
		t.Errorf("line 2: got %q, want %q", lines[2], "s 1005 1 #0000ff")
	}
}

func TestBuildSpanWriteZeroBaseOffset(t *testing.T) {
	runs := []styledRun{
		mkRun("abc", fgStyle(0xff, 0, 0)),
	}
	got := buildSpanWrite(0, runs)
	want := "s 0 3 #ff0000\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- (10) All-default except one run still emits full block ---

func TestBuildSpanWriteOneStyledAmongDefaults(t *testing.T) {
	runs := []styledRun{
		mkRun("aaa", sgrState{}),
		mkRun("bbb", sgrState{}),
		mkRun("ccc", fgStyle(0xff, 0, 0)),
		mkRun("ddd", sgrState{}),
	}
	got := buildSpanWrite(0, runs)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4:\n%s", len(lines), got)
	}
	// All runs emit, including the default ones (contiguity requirement)
	if lines[0] != "s 0 3 -" {
		t.Errorf("line 0: got %q, want %q", lines[0], "s 0 3 -")
	}
	if lines[1] != "s 3 3 -" {
		t.Errorf("line 1: got %q, want %q", lines[1], "s 3 3 -")
	}
	if lines[2] != "s 6 3 #ff0000" {
		t.Errorf("line 2: got %q, want %q", lines[2], "s 6 3 #ff0000")
	}
	if lines[3] != "s 9 3 -" {
		t.Errorf("line 3: got %q, want %q", lines[3], "s 9 3 -")
	}
}

// --- Edge cases ---

func TestBuildSpanWriteZeroLengthRunSkipped(t *testing.T) {
	// Zero-length runs should be skipped (can occur at style boundaries)
	runs := []styledRun{
		mkRun("abc", fgStyle(0xff, 0, 0)),
		{text: []rune{}, style: fgStyle(0, 0xff, 0)}, // zero-length
		mkRun("def", fgStyle(0, 0, 0xff)),
	}
	got := buildSpanWrite(0, runs)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2:\n%s", len(lines), got)
	}
	if lines[0] != "s 0 3 #ff0000" {
		t.Errorf("line 0: got %q, want %q", lines[0], "s 0 3 #ff0000")
	}
	if lines[1] != "s 3 3 #0000ff" {
		t.Errorf("line 1: got %q, want %q", lines[1], "s 3 3 #0000ff")
	}
}

func TestBuildSpanWriteBgNoFlagsNoDash(t *testing.T) {
	// bg set, no flags → "s offset length fg bg" (no trailing dash)
	s := fgBgStyle(0xff, 0, 0, 0, 0xff, 0)
	runs := []styledRun{mkRun("x", s)}
	got := buildSpanWrite(0, runs)
	want := "s 0 1 #ff0000 #00ff00\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildSpanWriteFlagsWithBg(t *testing.T) {
	// bg set + flags → "s offset length fg bg flags..."
	s := fgBgStyle(0, 0, 0xaa, 0xaa, 0x55, 0)
	s.bold = true
	s.italic = true
	runs := []styledRun{mkRun("flagbg", s)}
	got := buildSpanWrite(200, runs)
	want := "s 200 6 #0000aa #aa5500 bold italic\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildSpanWriteUnicodeRuneLength(t *testing.T) {
	// len(run.text) counts runes, not bytes
	runs := []styledRun{
		mkRun("世界🌍", fgStyle(0xff, 0, 0)), // 3 runes
	}
	got := buildSpanWrite(0, runs)
	want := "s 0 3 #ff0000\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- buildFlags tests ---

func TestBuildFlagsNone(t *testing.T) {
	flags := buildFlags(sgrState{})
	if len(flags) != 0 {
		t.Errorf("got flags %v, want empty", flags)
	}
}

func TestBuildFlagsBold(t *testing.T) {
	s := sgrState{bold: true}
	flags := buildFlags(s)
	if len(flags) != 1 || flags[0] != "bold" {
		t.Errorf("got %v, want [bold]", flags)
	}
}

func TestBuildFlagsItalic(t *testing.T) {
	s := sgrState{italic: true}
	flags := buildFlags(s)
	if len(flags) != 1 || flags[0] != "italic" {
		t.Errorf("got %v, want [italic]", flags)
	}
}

func TestBuildFlagsHidden(t *testing.T) {
	s := sgrState{hidden: true}
	flags := buildFlags(s)
	if len(flags) != 1 || flags[0] != "hidden" {
		t.Errorf("got %v, want [hidden]", flags)
	}
}

func TestBuildFlagsAllThree(t *testing.T) {
	s := sgrState{bold: true, italic: true, hidden: true}
	flags := buildFlags(s)
	if len(flags) != 3 || flags[0] != "bold" || flags[1] != "italic" || flags[2] != "hidden" {
		t.Errorf("got %v, want [bold italic hidden]", flags)
	}
}

func TestBuildFlagsBoldAndDimIndependent(t *testing.T) {
	// bold is emitted regardless of dim (dim handled by color reduction)
	s := sgrState{bold: true, dim: true}
	flags := buildFlags(s)
	if len(flags) != 1 || flags[0] != "bold" {
		t.Errorf("got %v, want [bold]", flags)
	}
}

func TestBuildFlagsUnsupportedNotEmitted(t *testing.T) {
	// underline, blink, strike, inverse, dim → not emitted as flags
	s := sgrState{
		underline: true,
		blink:     true,
		strike:    true,
		inverse:   true,
		dim:       true,
	}
	flags := buildFlags(s)
	if len(flags) != 0 {
		t.Errorf("got %v, want empty (unsupported attrs not emitted)", flags)
	}
}

// --- Design doc examples ---

func TestBuildSpanWriteDesignExampleSingleRed(t *testing.T) {
	// From design doc: "Single red foreground run"
	runs := []styledRun{
		mkRun("hello", fgStyle(0xaa, 0, 0)), // 5 runes
	}
	got := buildSpanWrite(100, runs)
	want := "s 100 5 #aa0000\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildSpanWriteDesignExampleMultiple(t *testing.T) {
	// From design doc: "Multiple contiguous runs"
	s3 := fgStyle(0, 0xaa, 0)
	s3.bold = true
	runs := []styledRun{
		mkRun("red", fgStyle(0xaa, 0, 0)),
		mkRun("text", sgrState{}),
		mkRun("green", s3),
	}
	got := buildSpanWrite(50, runs)
	want := "s 50 3 #aa0000\ns 53 4 -\ns 57 5 #00aa00 - bold\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildSpanWriteDesignExampleInverse(t *testing.T) {
	// From design doc: "Inverse video"
	s := sgrState{inverse: true}
	runs := []styledRun{mkRun("inv-text00", s)} // 10 runes
	got := buildSpanWrite(0, runs)
	want := "s 0 10 #fffff0 #000000\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildSpanWriteDesignExampleAllDefault(t *testing.T) {
	// From design doc: "All default (optimization)"
	runs := []styledRun{
		mkRun("hello", sgrState{}),
		mkRun("world", sgrState{}),
	}
	got := buildSpanWrite(0, runs)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestBuildSpanWriteDesignExampleBoldItalicBg(t *testing.T) {
	// From design doc: "Bold + italic with background"
	s := fgBgStyle(0, 0, 0xaa, 0xaa, 0x55, 0)
	s.bold = true
	s.italic = true
	runs := []styledRun{mkRun("boldital!", s)} // 8 runes... wait, "boldital!" is 9
	// The example says 8 runes. Use exactly 8.
	runs = []styledRun{mkRun("boldital", s)} // 8 runes
	got := buildSpanWrite(200, runs)
	want := "s 200 8 #0000aa #aa5500 bold italic\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
