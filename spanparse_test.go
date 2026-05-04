package main

import (
	"image/color"
	"strings"
	"testing"
)

// =========================================================================
// parseColor tests
// =========================================================================

func TestParseColor(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    color.Color
		wantErr bool
	}{
		{
			name:  "dash returns nil (default)",
			input: "-",
			want:  nil,
		},
		{
			name:  "valid hex #ff0000",
			input: "#ff0000",
			want:  color.RGBA{R: 0xff, G: 0x00, B: 0x00, A: 0xff},
		},
		{
			name:  "valid hex #00ff00",
			input: "#00ff00",
			want:  color.RGBA{R: 0x00, G: 0xff, B: 0x00, A: 0xff},
		},
		{
			name:  "valid hex #0000ff",
			input: "#0000ff",
			want:  color.RGBA{R: 0x00, G: 0x00, B: 0xff, A: 0xff},
		},
		{
			name:  "valid hex #aabbcc",
			input: "#aabbcc",
			want:  color.RGBA{R: 0xaa, G: 0xbb, B: 0xcc, A: 0xff},
		},
		{
			name:  "uppercase hex #AABBCC",
			input: "#AABBCC",
			want:  color.RGBA{R: 0xaa, G: 0xbb, B: 0xcc, A: 0xff},
		},
		{
			name:  "mixed case hex #AaBbCc",
			input: "#AaBbCc",
			want:  color.RGBA{R: 0xaa, G: 0xbb, B: 0xcc, A: 0xff},
		},
		{
			name:    "missing hash prefix",
			input:   "ff0000",
			wantErr: true,
		},
		{
			name:    "short hex",
			input:   "#fff",
			wantErr: true,
		},
		{
			name:    "too long hex",
			input:   "#ff00001",
			wantErr: true,
		},
		{
			name:    "invalid hex chars",
			input:   "#gghhii",
			wantErr: true,
		},
		{
			name:    "named color",
			input:   "red",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "just hash",
			input:   "#",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseColor(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("parseColor(%q) returned nil error; want error", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseColor(%q) returned error: %v", tc.input, err)
			}
			if !colorEqual(got, tc.want) {
				t.Errorf("parseColor(%q) = %v; want %v", tc.input, got, tc.want)
			}
		})
	}
}

// =========================================================================
// parseSpanDefs tests
// =========================================================================

func TestParseSpanDefs_SingleSpan(t *testing.T) {
	// Test 1: "0 10 #ff0000" with bufLen=10 produces one run with Fg=red.
	runs, regionStart, err := parseSpanDefs("0 10 #ff0000", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if regionStart != 0 {
		t.Errorf("regionStart = %d; want 0", regionStart)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs; want 1", len(runs))
	}
	if runs[0].Len != 10 {
		t.Errorf("run[0].Len = %d; want 10", runs[0].Len)
	}
	wantFg := color.RGBA{R: 0xff, A: 0xff}
	if !colorEqual(runs[0].Style.Fg, wantFg) {
		t.Errorf("run[0].Style.Fg = %v; want %v", runs[0].Style.Fg, wantFg)
	}
	if runs[0].Style.Bg != nil {
		t.Errorf("run[0].Style.Bg = %v; want nil (default)", runs[0].Style.Bg)
	}
	if runs[0].Style.Bold || runs[0].Style.Italic || runs[0].Style.Hidden {
		t.Errorf("run[0] has unexpected flags: Bold=%v Italic=%v Hidden=%v",
			runs[0].Style.Bold, runs[0].Style.Italic, runs[0].Style.Hidden)
	}
}

func TestParseSpanDefs_MultiSpanContiguous(t *testing.T) {
	// Test 2: "0 4 #0000ff\n4 6 -" with bufLen=10 produces two runs.
	runs, regionStart, err := parseSpanDefs("0 4 #0000ff\n4 6 -", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if regionStart != 0 {
		t.Errorf("regionStart = %d; want 0", regionStart)
	}
	if len(runs) != 2 {
		t.Fatalf("got %d runs; want 2", len(runs))
	}

	// First run: 4 runes, blue foreground.
	if runs[0].Len != 4 {
		t.Errorf("run[0].Len = %d; want 4", runs[0].Len)
	}
	wantFg := color.RGBA{B: 0xff, A: 0xff}
	if !colorEqual(runs[0].Style.Fg, wantFg) {
		t.Errorf("run[0].Style.Fg = %v; want %v", runs[0].Style.Fg, wantFg)
	}

	// Second run: 6 runes, default foreground.
	if runs[1].Len != 6 {
		t.Errorf("run[1].Len = %d; want 6", runs[1].Len)
	}
	if runs[1].Style.Fg != nil {
		t.Errorf("run[1].Style.Fg = %v; want nil", runs[1].Style.Fg)
	}
}

func TestParseSpanDefs_OptionalBgColor(t *testing.T) {
	// Test 3: "0 5 #ff0000 #00ff00" parses both fg and bg.
	runs, _, err := parseSpanDefs("0 5 #ff0000 #00ff00", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs; want 1", len(runs))
	}
	wantFg := color.RGBA{R: 0xff, A: 0xff}
	wantBg := color.RGBA{G: 0xff, A: 0xff}
	if !colorEqual(runs[0].Style.Fg, wantFg) {
		t.Errorf("run[0].Style.Fg = %v; want %v", runs[0].Style.Fg, wantFg)
	}
	if !colorEqual(runs[0].Style.Bg, wantBg) {
		t.Errorf("run[0].Style.Bg = %v; want %v", runs[0].Style.Bg, wantBg)
	}
}

func TestParseSpanDefs_DashBgColor(t *testing.T) {
	// Dash as bg-color means default background.
	runs, _, err := parseSpanDefs("0 5 #ff0000 -", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs; want 1", len(runs))
	}
	if runs[0].Style.Bg != nil {
		t.Errorf("run[0].Style.Bg = %v; want nil (default)", runs[0].Style.Bg)
	}
}

func TestParseSpanDefs_FlagsNoBg(t *testing.T) {
	// Test 5: "0 5 #ff0000 bold italic" produces run with Bold=true, Italic=true.
	// Field 3 is "bold" which is not "#..." or "-", so it's treated as a flag.
	runs, _, err := parseSpanDefs("0 5 #ff0000 bold italic", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs; want 1", len(runs))
	}
	if !runs[0].Style.Bold {
		t.Error("run[0].Style.Bold = false; want true")
	}
	if !runs[0].Style.Italic {
		t.Error("run[0].Style.Italic = false; want true")
	}
	if runs[0].Style.Bg != nil {
		t.Errorf("run[0].Style.Bg = %v; want nil (no bg specified)", runs[0].Style.Bg)
	}
}

func TestParseSpanDefs_BgColorAndFlags(t *testing.T) {
	// Test 6: "0 5 #ff0000 #000000 bold" parses bg and bold.
	runs, _, err := parseSpanDefs("0 5 #ff0000 #000000 bold", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs; want 1", len(runs))
	}
	wantBg := color.RGBA{R: 0, G: 0, B: 0, A: 0xff}
	if !colorEqual(runs[0].Style.Bg, wantBg) {
		t.Errorf("run[0].Style.Bg = %v; want %v", runs[0].Style.Bg, wantBg)
	}
	if !runs[0].Style.Bold {
		t.Error("run[0].Style.Bold = false; want true")
	}
	if runs[0].Style.Italic {
		t.Error("run[0].Style.Italic = true; want false")
	}
}

func TestParseSpanDefs_HiddenFlag(t *testing.T) {
	runs, _, err := parseSpanDefs("0 5 #ff0000 hidden", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs; want 1", len(runs))
	}
	if !runs[0].Style.Hidden {
		t.Error("run[0].Style.Hidden = false; want true")
	}
}

func TestParseSpanDefs_AllFlags(t *testing.T) {
	runs, _, err := parseSpanDefs("0 5 #ff0000 #000000 bold italic hidden", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs; want 1", len(runs))
	}
	if !runs[0].Style.Bold {
		t.Error("run[0].Style.Bold = false; want true")
	}
	if !runs[0].Style.Italic {
		t.Error("run[0].Style.Italic = false; want true")
	}
	if !runs[0].Style.Hidden {
		t.Error("run[0].Style.Hidden = false; want true")
	}
}

func TestParseSpanDefs_RegionStartNonZero(t *testing.T) {
	// A partial update: only spans starting at offset 5.
	runs, regionStart, err := parseSpanDefs("5 5 #ff0000", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if regionStart != 5 {
		t.Errorf("regionStart = %d; want 5", regionStart)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs; want 1", len(runs))
	}
	if runs[0].Len != 5 {
		t.Errorf("run[0].Len = %d; want 5", runs[0].Len)
	}
}

func TestParseSpanDefs_ZeroLengthSpan(t *testing.T) {
	// Test 10: "5 0 #ff0000\n5 5 -" is valid.
	runs, regionStart, err := parseSpanDefs("5 0 #ff0000\n5 5 -", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if regionStart != 5 {
		t.Errorf("regionStart = %d; want 5", regionStart)
	}
	if len(runs) != 2 {
		t.Fatalf("got %d runs; want 2", len(runs))
	}
	if runs[0].Len != 0 {
		t.Errorf("run[0].Len = %d; want 0", runs[0].Len)
	}
	if runs[1].Len != 5 {
		t.Errorf("run[1].Len = %d; want 5", runs[1].Len)
	}
}

func TestParseSpanDefs_MultipleContiguousSpans(t *testing.T) {
	// Three contiguous spans covering the full buffer.
	input := "0 3 #ff0000\n3 4 #00ff00\n7 3 #0000ff"
	runs, regionStart, err := parseSpanDefs(input, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if regionStart != 0 {
		t.Errorf("regionStart = %d; want 0", regionStart)
	}
	if len(runs) != 3 {
		t.Fatalf("got %d runs; want 3", len(runs))
	}
	if runs[0].Len != 3 || runs[1].Len != 4 || runs[2].Len != 3 {
		t.Errorf("run lengths = [%d, %d, %d]; want [3, 4, 3]",
			runs[0].Len, runs[1].Len, runs[2].Len)
	}
}

func TestParseSpanDefs_DefaultFg(t *testing.T) {
	// Dash as fg-color means default foreground.
	runs, _, err := parseSpanDefs("0 10 -", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs; want 1", len(runs))
	}
	if runs[0].Style.Fg != nil {
		t.Errorf("run[0].Style.Fg = %v; want nil (default)", runs[0].Style.Fg)
	}
}

// =========================================================================
// Validation error tests
// =========================================================================

func TestParseSpanDefs_ErrTooFewFields(t *testing.T) {
	// Less than 3 fields is an error.
	_, _, err := parseSpanDefs("0 10", 10)
	if err == nil {
		t.Fatal("expected error for too few fields; got nil")
	}
	if !strings.Contains(err.Error(), "bad span format") {
		t.Errorf("error = %q; want to contain %q", err.Error(), "bad span format")
	}
}

func TestParseSpanDefs_ErrBadOffset(t *testing.T) {
	_, _, err := parseSpanDefs("abc 10 #ff0000", 10)
	if err == nil {
		t.Fatal("expected error for bad offset; got nil")
	}
	if !strings.Contains(err.Error(), "bad span offset") {
		t.Errorf("error = %q; want to contain %q", err.Error(), "bad span offset")
	}
}

func TestParseSpanDefs_ErrBadLength(t *testing.T) {
	_, _, err := parseSpanDefs("0 abc #ff0000", 10)
	if err == nil {
		t.Fatal("expected error for bad length; got nil")
	}
	if !strings.Contains(err.Error(), "bad span length") {
		t.Errorf("error = %q; want to contain %q", err.Error(), "bad span length")
	}
}

func TestParseSpanDefs_ErrBadColor(t *testing.T) {
	_, _, err := parseSpanDefs("0 5 red", 10)
	if err == nil {
		t.Fatal("expected error for bad color; got nil")
	}
	if !strings.Contains(err.Error(), "bad color value") {
		t.Errorf("error = %q; want to contain %q", err.Error(), "bad color value")
	}
}

func TestParseSpanDefs_ErrUnknownFlag(t *testing.T) {
	_, _, err := parseSpanDefs("0 5 #ff0000 underline", 10)
	if err == nil {
		t.Fatal("expected error for unknown flag; got nil")
	}
	if !strings.Contains(err.Error(), "unknown span flag") {
		t.Errorf("error = %q; want to contain %q", err.Error(), "unknown span flag")
	}
}

func TestParseSpanDefs_ErrNonContiguousGap(t *testing.T) {
	// Gap between spans: first ends at 3, next starts at 5.
	_, _, err := parseSpanDefs("0 3 #ff0000\n5 5 #00ff00", 10)
	if err == nil {
		t.Fatal("expected error for non-contiguous spans (gap); got nil")
	}
	if !strings.Contains(err.Error(), "contiguous") {
		t.Errorf("error = %q; want to contain %q", err.Error(), "contiguous")
	}
}

func TestParseSpanDefs_ErrOverlappingSpans(t *testing.T) {
	// Overlap: first span [0,5), second starts at 3 (overlaps).
	_, _, err := parseSpanDefs("0 5 #ff0000\n3 5 #00ff00", 10)
	if err == nil {
		t.Fatal("expected error for overlapping spans; got nil")
	}
	if !strings.Contains(err.Error(), "contiguous") {
		t.Errorf("error = %q; want to contain %q", err.Error(), "contiguous")
	}
}

func TestParseSpanDefs_RegionExceedsBufferClamped(t *testing.T) {
	// Span region extends past buffer: offset 0, length 20, but bufLen=10.
	// Should clamp rather than error.
	runs, start, err := parseSpanDefs("0 20 #ff0000", 10)
	if err != nil {
		t.Fatalf("expected clamping, got error: %v", err)
	}
	if start != 0 {
		t.Errorf("regionStart = %d; want 0", start)
	}
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d; want 1", len(runs))
	}
	if runs[0].Len != 10 {
		t.Errorf("runs[0].Len = %d; want 10 (clamped from 20)", runs[0].Len)
	}
}

func TestParseSpanDefs_OffsetBeyondBufferDiscarded(t *testing.T) {
	// Offset beyond buffer length — spans are silently discarded.
	runs, start, err := parseSpanDefs("15 5 #ff0000", 10)
	if err != nil {
		t.Fatalf("expected empty result, got error: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("len(runs) = %d; want 0 (discarded)", len(runs))
	}
	_ = start
}

func TestParseSpanDefs_MultiRunClamp(t *testing.T) {
	// Two runs totaling 20 runes, but buffer is only 12.
	// First run (10) fits, second run (10) should be clamped to 2.
	runs, start, err := parseSpanDefs("0 10 #ff0000\n10 10 #00ff00", 12)
	if err != nil {
		t.Fatalf("expected clamping, got error: %v", err)
	}
	if start != 0 {
		t.Errorf("regionStart = %d; want 0", start)
	}
	if len(runs) != 2 {
		t.Fatalf("len(runs) = %d; want 2", len(runs))
	}
	if runs[0].Len != 10 {
		t.Errorf("runs[0].Len = %d; want 10", runs[0].Len)
	}
	if runs[1].Len != 2 {
		t.Errorf("runs[1].Len = %d; want 2 (clamped from 10)", runs[1].Len)
	}
}

func TestParseSpanDefs_ErrNegativeOffset(t *testing.T) {
	_, _, err := parseSpanDefs("-1 5 #ff0000", 10)
	if err == nil {
		t.Fatal("expected error for negative offset; got nil")
	}
	if !strings.Contains(err.Error(), "negative") {
		t.Errorf("error = %q; want to contain %q", err.Error(), "negative")
	}
}

func TestParseSpanDefs_ErrNegativeLength(t *testing.T) {
	_, _, err := parseSpanDefs("0 -5 #ff0000", 10)
	if err == nil {
		t.Fatal("expected error for negative length; got nil")
	}
	if !strings.Contains(err.Error(), "negative") {
		t.Errorf("error = %q; want to contain %q", err.Error(), "negative")
	}
}

func TestParseSpanDefs_ErrBadBgColor(t *testing.T) {
	// A bg-color field that starts with # but is invalid.
	_, _, err := parseSpanDefs("0 5 #ff0000 #xyz", 10)
	if err == nil {
		t.Fatal("expected error for bad bg color; got nil")
	}
	if !strings.Contains(err.Error(), "bad color value") {
		t.Errorf("error = %q; want to contain %q", err.Error(), "bad color value")
	}
}

func TestParseSpanDefs_DuplicateFlagsIgnored(t *testing.T) {
	// Duplicate flags should be idempotent, not an error.
	runs, _, err := parseSpanDefs("0 5 #ff0000 bold bold", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs; want 1", len(runs))
	}
	if !runs[0].Style.Bold {
		t.Error("run[0].Style.Bold = false; want true")
	}
}

func TestParseSpanDefs_TrailingNewline(t *testing.T) {
	// The write handler trims trailing newlines before calling parseSpanDefs,
	// but test that the parser handles single-span input cleanly.
	runs, _, err := parseSpanDefs("0 10 #ff0000", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs; want 1", len(runs))
	}
}

func TestParseSpanDefs_ExactBufferEnd(t *testing.T) {
	// Span region exactly fills the buffer.
	runs, regionStart, err := parseSpanDefs("0 5 #ff0000\n5 5 #00ff00", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if regionStart != 0 {
		t.Errorf("regionStart = %d; want 0", regionStart)
	}
	if len(runs) != 2 {
		t.Fatalf("got %d runs; want 2", len(runs))
	}
	totalLen := 0
	for _, r := range runs {
		totalLen += r.Len
	}
	if totalLen != 10 {
		t.Errorf("total span length = %d; want 10", totalLen)
	}
}

func TestParseSpanDefs_PartialRegionInMiddle(t *testing.T) {
	// Partial update: spans only cover [3, 7) of a 10-rune buffer.
	runs, regionStart, err := parseSpanDefs("3 2 #ff0000\n5 2 #00ff00", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if regionStart != 3 {
		t.Errorf("regionStart = %d; want 3", regionStart)
	}
	if len(runs) != 2 {
		t.Fatalf("got %d runs; want 2", len(runs))
	}
}

func TestParseSpanDefs_SpanAtBufferEnd(t *testing.T) {
	// Span at the very end of the buffer.
	runs, regionStart, err := parseSpanDefs("8 2 #ff0000", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if regionStart != 8 {
		t.Errorf("regionStart = %d; want 8", regionStart)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs; want 1", len(runs))
	}
}

func TestParseSpanDefs_SpanAtBufferStart(t *testing.T) {
	// Span at the start, not covering whole buffer.
	runs, regionStart, err := parseSpanDefs("0 3 #ff0000", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if regionStart != 0 {
		t.Errorf("regionStart = %d; want 0", regionStart)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs; want 1", len(runs))
	}
	if runs[0].Len != 3 {
		t.Errorf("run[0].Len = %d; want 3", runs[0].Len)
	}
}

// =========================================================================
// parseSpanMessage tests (prefixed format)
// =========================================================================

func TestParseSpanMessage_Clear(t *testing.T) {
	_, _, _, isClear, err := parseSpanMessage("c", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isClear {
		t.Error("expected isClear=true")
	}
}

func TestParseSpanMessage_SingleSpan(t *testing.T) {
	runs, regionStart, _, isClear, err := parseSpanMessage("s 0 10 #ff0000", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isClear {
		t.Error("expected isClear=false")
	}
	if regionStart != 0 {
		t.Errorf("regionStart = %d; want 0", regionStart)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs; want 1", len(runs))
	}
	if runs[0].Len != 10 {
		t.Errorf("run[0].Len = %d; want 10", runs[0].Len)
	}
	wantFg := color.RGBA{R: 0xff, A: 0xff}
	if !colorEqual(runs[0].Style.Fg, wantFg) {
		t.Errorf("run[0].Style.Fg = %v; want %v", runs[0].Style.Fg, wantFg)
	}
	if runs[0].Style.IsBox {
		t.Error("span run should not be a box")
	}
}

func TestParseSpanMessage_MultiSpanContiguous(t *testing.T) {
	input := "s 0 4 #0000ff\ns 4 6 -"
	runs, regionStart, _, _, err := parseSpanMessage(input, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if regionStart != 0 {
		t.Errorf("regionStart = %d; want 0", regionStart)
	}
	if len(runs) != 2 {
		t.Fatalf("got %d runs; want 2", len(runs))
	}
	if runs[0].Len != 4 {
		t.Errorf("run[0].Len = %d; want 4", runs[0].Len)
	}
	if runs[1].Len != 6 {
		t.Errorf("run[1].Len = %d; want 6", runs[1].Len)
	}
}

func TestParseSpanMessage_SpanWithFlags(t *testing.T) {
	runs, _, _, _, err := parseSpanMessage("s 0 5 #ff0000 bold italic", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs; want 1", len(runs))
	}
	if !runs[0].Style.Bold {
		t.Error("expected Bold=true")
	}
	if !runs[0].Style.Italic {
		t.Error("expected Italic=true")
	}
}

func TestParseSpanMessage_BoxBasic(t *testing.T) {
	runs, regionStart, _, _, err := parseSpanMessage("b 0 5 200 150", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if regionStart != 0 {
		t.Errorf("regionStart = %d; want 0", regionStart)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs; want 1", len(runs))
	}
	if !runs[0].Style.IsBox {
		t.Error("expected IsBox=true")
	}
	if runs[0].Style.BoxWidth != 200 {
		t.Errorf("BoxWidth = %d; want 200", runs[0].Style.BoxWidth)
	}
	if runs[0].Style.BoxHeight != 150 {
		t.Errorf("BoxHeight = %d; want 150", runs[0].Style.BoxHeight)
	}
	if runs[0].Len != 5 {
		t.Errorf("run Len = %d; want 5", runs[0].Len)
	}
}

func TestParseSpanMessage_BoxWithPayload(t *testing.T) {
	runs, _, _, _, err := parseSpanMessage("b 10 5 200 150 image:/path with spaces/img.png", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs; want 1", len(runs))
	}
	if runs[0].Style.BoxPayload != "image:/path with spaces/img.png" {
		t.Errorf("BoxPayload = %q; want %q", runs[0].Style.BoxPayload, "image:/path with spaces/img.png")
	}
}

func TestParseSpanMessage_BoxWithColorsAndFlags(t *testing.T) {
	runs, _, _, _, err := parseSpanMessage("b 0 8 200 150 #ff0000 #00ff00 bold image:/tmp/test.png", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs; want 1", len(runs))
	}
	r := runs[0]
	if !r.Style.IsBox {
		t.Error("expected IsBox=true")
	}
	wantFg := color.RGBA{R: 0xff, A: 0xff}
	if !colorEqual(r.Style.Fg, wantFg) {
		t.Errorf("Fg = %v; want %v", r.Style.Fg, wantFg)
	}
	wantBg := color.RGBA{G: 0xff, A: 0xff}
	if !colorEqual(r.Style.Bg, wantBg) {
		t.Errorf("Bg = %v; want %v", r.Style.Bg, wantBg)
	}
	if !r.Style.Bold {
		t.Error("expected Bold=true")
	}
	if r.Style.BoxPayload != "image:/tmp/test.png" {
		t.Errorf("BoxPayload = %q; want %q", r.Style.BoxPayload, "image:/tmp/test.png")
	}
}

func TestParseSpanMessage_BoxWithFgOnly(t *testing.T) {
	runs, _, _, _, err := parseSpanMessage("b 0 5 100 50 #ff0000 bold image:foo.png", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs; want 1", len(runs))
	}
	r := runs[0]
	wantFg := color.RGBA{R: 0xff, A: 0xff}
	if !colorEqual(r.Style.Fg, wantFg) {
		t.Errorf("Fg = %v; want %v", r.Style.Fg, wantFg)
	}
	if r.Style.Bg != nil {
		t.Errorf("Bg = %v; want nil", r.Style.Bg)
	}
	if !r.Style.Bold {
		t.Error("expected Bold=true")
	}
	if r.Style.BoxPayload != "image:foo.png" {
		t.Errorf("BoxPayload = %q; want %q", r.Style.BoxPayload, "image:foo.png")
	}
}

func TestParseSpanMessage_MixedSpansAndBoxes(t *testing.T) {
	input := "s 0 10 #0000cc\ns 10 5 - bold\nb 15 8 200 150 image:/tmp/diagram.png\ns 23 12 #008000 italic"
	runs, regionStart, _, _, err := parseSpanMessage(input, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if regionStart != 0 {
		t.Errorf("regionStart = %d; want 0", regionStart)
	}
	if len(runs) != 4 {
		t.Fatalf("got %d runs; want 4", len(runs))
	}

	// Verify span types.
	if runs[0].Style.IsBox {
		t.Error("run[0] should not be a box")
	}
	if runs[1].Style.IsBox {
		t.Error("run[1] should not be a box")
	}
	if !runs[2].Style.IsBox {
		t.Error("run[2] should be a box")
	}
	if runs[3].Style.IsBox {
		t.Error("run[3] should not be a box")
	}

	// Verify contiguity.
	expectedLens := []int{10, 5, 8, 12}
	for i, want := range expectedLens {
		if runs[i].Len != want {
			t.Errorf("run[%d].Len = %d; want %d", i, runs[i].Len, want)
		}
	}
}

func TestParseSpanMessage_NonContiguous(t *testing.T) {
	input := "s 0 5 #ff0000\ns 7 3 #00ff00"
	_, _, _, _, err := parseSpanMessage(input, 10)
	if err == nil {
		t.Fatal("expected error for non-contiguous spans")
	}
	if !strings.Contains(err.Error(), "contiguous") {
		t.Errorf("error = %q; want to contain %q", err.Error(), "contiguous")
	}
}

func TestParseSpanMessage_UnknownPrefix(t *testing.T) {
	_, _, _, _, err := parseSpanMessage("x 0 5 #ff0000", 10)
	if err == nil {
		t.Fatal("expected error for unknown prefix")
	}
	if !strings.Contains(err.Error(), "unknown span command") {
		t.Errorf("error = %q; want to contain %q", err.Error(), "unknown span command")
	}
}

func TestParseSpanMessage_BoxMissingFields(t *testing.T) {
	_, _, _, _, err := parseSpanMessage("b 0 5 200", 10)
	if err == nil {
		t.Fatal("expected error for box with missing height")
	}
	if !strings.Contains(err.Error(), "bad box format") {
		t.Errorf("error = %q; want to contain %q", err.Error(), "bad box format")
	}
}

func TestParseSpanMessage_ClearMustBeAlone(t *testing.T) {
	_, _, _, _, err := parseSpanMessage("s 0 5 #ff0000\nc", 10)
	if err == nil {
		t.Fatal("expected error for clear after span")
	}
	if !strings.Contains(err.Error(), "clear must be the only command") {
		t.Errorf("error = %q; want to contain %q", err.Error(), "clear must be the only command")
	}
}

func TestParseSpanMessage_BoxClampToBuffer(t *testing.T) {
	runs, _, _, _, err := parseSpanMessage("b 0 20 200 150 image:test.png", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs; want 1", len(runs))
	}
	if runs[0].Len != 10 {
		t.Errorf("run Len = %d; want 10 (clamped from 20)", runs[0].Len)
	}
}

// =========================================================================
// isPrefixedFormat tests
// =========================================================================

func TestIsPrefixedFormat(t *testing.T) {
	tests := []struct {
		name string
		data string
		want bool
	}{
		{"clear command", "c", true},
		{"span prefix", "s 0 10 #ff0000", true},
		{"box prefix", "b 0 5 200 150", true},
		{"legacy numeric", "0 10 #ff0000", false},
		{"legacy clear", "clear", false},
		{"empty", "", false},
		{"unknown prefix", "x 0 5", false},
		{"multiline span", "s 0 5 #ff0000\ns 5 5 -", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isPrefixedFormat(tc.data)
			if got != tc.want {
				t.Errorf("isPrefixedFormat(%q) = %v; want %v", tc.data, got, tc.want)
			}
		})
	}
}

// --- Scale flag tests (Phase 3 round 1) ----------------------------------

// TestParseSpanScaleBasic: `scale=2.0` on an s-line round-trips
// to StyleAttrs.Scale = 2.0.
func TestParseSpanScaleBasic(t *testing.T) {
	data := "s 0 5 - scale=2.0"
	runs, _, _, isClear, err := parseSpanMessage(data, 100)
	if err != nil {
		t.Fatalf("parseSpanMessage: %v", err)
	}
	if isClear {
		t.Fatal("isClear should be false")
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}
	if runs[0].Style.Scale != 2.0 {
		t.Errorf("Scale = %v, want 2.0", runs[0].Style.Scale)
	}
}

// TestParseSpanScaleAbsentMeansZero: omitting the scale flag
// leaves Scale at its zero value (the unset sentinel).
func TestParseSpanScaleAbsentMeansZero(t *testing.T) {
	data := "s 0 5 - bold"
	runs, _, _, _, err := parseSpanMessage(data, 100)
	if err != nil {
		t.Fatalf("parseSpanMessage: %v", err)
	}
	if runs[0].Style.Scale != 0 {
		t.Errorf("Scale = %v, want 0 (unset)", runs[0].Style.Scale)
	}
}

// TestParseSpanScaleWithOtherFlags: scale coexists with bold,
// italic in any order.
func TestParseSpanScaleWithOtherFlags(t *testing.T) {
	cases := []string{
		"s 0 5 - bold scale=1.5",
		"s 0 5 - scale=1.5 bold",
		"s 0 5 - italic scale=1.25 bold",
	}
	for _, data := range cases {
		t.Run(data, func(t *testing.T) {
			runs, _, _, _, err := parseSpanMessage(data, 100)
			if err != nil {
				t.Fatalf("parseSpanMessage: %v", err)
			}
			if runs[0].Style.Scale == 0 {
				t.Errorf("Scale not parsed; line was %q", data)
			}
			if !runs[0].Style.Bold {
				t.Errorf("Bold not parsed; line was %q", data)
			}
		})
	}
}

// TestParseSpanScaleFractional: non-integer scales parse.
func TestParseSpanScaleFractional(t *testing.T) {
	cases := map[string]float64{
		"s 0 5 - scale=1.25":  1.25,
		"s 0 5 - scale=1.05":  1.05,
		"s 0 5 - scale=0.875": 0.875,
		"s 0 5 - scale=2":     2.0,
		"s 0 5 - scale=0.5":   0.5,
	}
	for data, want := range cases {
		runs, _, _, _, err := parseSpanMessage(data, 100)
		if err != nil {
			t.Errorf("parseSpanMessage(%q): %v", data, err)
			continue
		}
		if runs[0].Style.Scale != want {
			t.Errorf("%q: Scale = %v, want %v", data, runs[0].Style.Scale, want)
		}
	}
}

// TestParseSpanScaleErrors: invalid scale values are errors.
func TestParseSpanScaleErrors(t *testing.T) {
	cases := []struct {
		data string
		why  string
	}{
		{"s 0 5 - scale=0", "zero rejected"},
		{"s 0 5 - scale=-1.0", "negative rejected"},
		{"s 0 5 - scale=NaN", "NaN rejected"},
		{"s 0 5 - scale=Inf", "Inf rejected"},
		{"s 0 5 - scale=11", "above cap rejected"},
		{"s 0 5 - scale=", "empty rejected"},
		{"s 0 5 - scale=abc", "non-numeric rejected"},
	}
	for _, tc := range cases {
		t.Run(tc.why, func(t *testing.T) {
			_, _, _, _, err := parseSpanMessage(tc.data, 100)
			if err == nil {
				t.Errorf("%q: expected error (%s), got nil", tc.data, tc.why)
			}
		})
	}
}

// TestParseBoxScale: scale flag also applies to b-lines.
func TestParseBoxScale(t *testing.T) {
	data := "b 0 1 100 50 - - scale=1.5 image:foo.png"
	runs, _, _, _, err := parseSpanMessage(data, 100)
	if err != nil {
		t.Fatalf("parseSpanMessage: %v", err)
	}
	if !runs[0].Style.IsBox {
		t.Error("IsBox should be true")
	}
	if runs[0].Style.Scale != 1.5 {
		t.Errorf("Scale = %v, want 1.5", runs[0].Style.Scale)
	}
	if runs[0].Style.BoxPayload != "image:foo.png" {
		t.Errorf("BoxPayload = %q, want image:foo.png", runs[0].Style.BoxPayload)
	}
}

// --- Family flag tests (Phase 3 round 2) ---------------------------------

// TestParseSpanFamilyCode: `family=code` parses to Family="code".
func TestParseSpanFamilyCode(t *testing.T) {
	data := "s 0 5 - family=code"
	runs, _, _, _, err := parseSpanMessage(data, 100)
	if err != nil {
		t.Fatalf("parseSpanMessage: %v", err)
	}
	if runs[0].Style.Family != "code" {
		t.Errorf("Family = %q, want %q", runs[0].Style.Family, "code")
	}
}

// TestParseSpanFamilyAbsentMeansEmpty: omitting family leaves
// Family at its zero value ("").
func TestParseSpanFamilyAbsentMeansEmpty(t *testing.T) {
	data := "s 0 5 - bold"
	runs, _, _, _, err := parseSpanMessage(data, 100)
	if err != nil {
		t.Fatalf("parseSpanMessage: %v", err)
	}
	if runs[0].Style.Family != "" {
		t.Errorf("Family = %q, want \"\" (unset)", runs[0].Style.Family)
	}
}

// TestParseSpanFamilyWithOtherFlags: family coexists with bold,
// italic, scale in any order.
func TestParseSpanFamilyWithOtherFlags(t *testing.T) {
	cases := []string{
		"s 0 5 - bold family=code",
		"s 0 5 - family=code bold",
		"s 0 5 - italic family=code scale=1.5",
		"s 0 5 - scale=2 family=code bold",
	}
	for _, data := range cases {
		t.Run(data, func(t *testing.T) {
			runs, _, _, _, err := parseSpanMessage(data, 100)
			if err != nil {
				t.Fatalf("parseSpanMessage: %v", err)
			}
			if runs[0].Style.Family != "code" {
				t.Errorf("Family not parsed; line was %q", data)
			}
		})
	}
}

// TestParseSpanFamilyErrors: empty value and unknown family
// names are errors.
func TestParseSpanFamilyErrors(t *testing.T) {
	cases := []struct {
		data string
		why  string
	}{
		{"s 0 5 - family=", "empty rejected"},
		{"s 0 5 - family=serif", "unknown family rejected"},
		{"s 0 5 - family=GoMono", "Plan 9 path rejected"},
		{"s 0 5 - family=Code", "case-sensitive rejected (only lowercase 'code')"},
		{"s 0 5 - family=fancy", "arbitrary name rejected"},
	}
	for _, tc := range cases {
		t.Run(tc.why, func(t *testing.T) {
			_, _, _, _, err := parseSpanMessage(tc.data, 100)
			if err == nil {
				t.Errorf("%q: expected error (%s), got nil", tc.data, tc.why)
			}
		})
	}
}

// TestParseBoxFamily: family flag also applies to b-lines.
func TestParseBoxFamily(t *testing.T) {
	data := "b 0 1 100 50 - - family=code"
	runs, _, _, _, err := parseSpanMessage(data, 100)
	if err != nil {
		t.Fatalf("parseSpanMessage: %v", err)
	}
	if runs[0].Style.Family != "code" {
		t.Errorf("Family = %q, want code", runs[0].Style.Family)
	}
}

// --- HRule flag tests (Phase 3 round 3) ----------------------------------

// TestParseSpanHRule: `hrule` flag parses to HRule=true.
func TestParseSpanHRule(t *testing.T) {
	data := "s 0 3 - hrule"
	runs, _, _, _, err := parseSpanMessage(data, 100)
	if err != nil {
		t.Fatalf("parseSpanMessage: %v", err)
	}
	if !runs[0].Style.HRule {
		t.Error("HRule should be true")
	}
}

// TestParseSpanHRuleAbsentMeansFalse: omitting the flag leaves
// HRule at its zero value (false).
func TestParseSpanHRuleAbsentMeansFalse(t *testing.T) {
	data := "s 0 5 - bold"
	runs, _, _, _, err := parseSpanMessage(data, 100)
	if err != nil {
		t.Fatalf("parseSpanMessage: %v", err)
	}
	if runs[0].Style.HRule {
		t.Error("HRule should be false when flag absent")
	}
}

// TestParseSpanHRuleWithOtherFlags: hrule coexists with bold,
// italic, scale, family in any order.
func TestParseSpanHRuleWithOtherFlags(t *testing.T) {
	cases := []string{
		"s 0 3 - hrule bold",
		"s 0 3 - bold hrule",
		"s 0 3 - scale=1.0 hrule",
		"s 0 3 - family=code hrule",
		"s 0 3 - hrule family=code",
	}
	for _, data := range cases {
		t.Run(data, func(t *testing.T) {
			runs, _, _, _, err := parseSpanMessage(data, 100)
			if err != nil {
				t.Fatalf("parseSpanMessage: %v", err)
			}
			if !runs[0].Style.HRule {
				t.Errorf("HRule not parsed; line was %q", data)
			}
		})
	}
}

// TestParseBoxHRule: hrule flag also applies to b-lines.
func TestParseBoxHRule(t *testing.T) {
	data := "b 0 1 100 1 - - hrule"
	runs, _, _, _, err := parseSpanMessage(data, 100)
	if err != nil {
		t.Fatalf("parseSpanMessage: %v", err)
	}
	if !runs[0].Style.HRule {
		t.Error("HRule should be true")
	}
}

// --- placement=NAME flag tests (Phase 3 round 4) -----------------------

// TestParseBoxPlacementBelow: `placement=below` covers the
// source markdown runes [offset, offset+length); the
// renderer renders the source text normally and paints the
// image below the line. W=H=0 is the canonical
// "renderer-probes" sentinel for image dimensions.
func TestParseBoxPlacementBelow(t *testing.T) {
	data := "b 5 11 0 0 - - placement=below image:./pic.png"
	runs, _, _, _, err := parseSpanMessage(data, 100)
	if err != nil {
		t.Fatalf("parseSpanMessage: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].Style.BoxPlacement != "below" {
		t.Errorf("BoxPlacement = %q, want %q", runs[0].Style.BoxPlacement, "below")
	}
	if !runs[0].Style.IsBox {
		t.Error("IsBox should be true on a b-line")
	}
	if runs[0].Style.BoxWidth != 0 || runs[0].Style.BoxHeight != 0 {
		t.Errorf("expected W=H=0 (probe sentinel), got W=%d H=%d",
			runs[0].Style.BoxWidth, runs[0].Style.BoxHeight)
	}
	if runs[0].Style.BoxPayload != "image:./pic.png" {
		t.Errorf("BoxPayload = %q, want %q", runs[0].Style.BoxPayload, "image:./pic.png")
	}
	if runs[0].Len != 11 {
		t.Errorf("Len = %d, want 11 (source rune count)", runs[0].Len)
	}
}

// TestParseBoxPlacementReplaceExplicit: `placement=replace`
// is allowed and parses to BoxPlacement="replace". The
// renderer treats it the same as the absent flag (existing
// replacing semantic) — but the storage preserves the
// explicit value for round-trip fidelity.
func TestParseBoxPlacementReplaceExplicit(t *testing.T) {
	data := "b 0 1 100 50 - - placement=replace image:./pic.png"
	runs, _, _, _, err := parseSpanMessage(data, 100)
	if err != nil {
		t.Fatalf("parseSpanMessage: %v", err)
	}
	if runs[0].Style.BoxPlacement != "replace" {
		t.Errorf("BoxPlacement = %q, want %q", runs[0].Style.BoxPlacement, "replace")
	}
}

// TestParseBoxPlacementAbsentMeansEmpty: a b-line without
// the `placement=` flag has BoxPlacement="" (the unset
// sentinel — existing replacing semantics apply, matching
// pre-round-4 producers).
func TestParseBoxPlacementAbsentMeansEmpty(t *testing.T) {
	data := "b 0 1 100 50 - - image:./pic.png"
	runs, _, _, _, err := parseSpanMessage(data, 100)
	if err != nil {
		t.Fatalf("parseSpanMessage: %v", err)
	}
	if runs[0].Style.BoxPlacement != "" {
		t.Errorf("BoxPlacement = %q, want empty", runs[0].Style.BoxPlacement)
	}
}

// TestParseBoxPlacementUnknownRejected: unknown values are
// rejected (mirrors family=NAME; future placements extend
// the spec, not the parser via silent acceptance).
func TestParseBoxPlacementUnknownRejected(t *testing.T) {
	cases := []string{
		"b 0 0 0 0 - - placement=center image:./p.png",
		"b 0 0 0 0 - - placement=above image:./p.png",
		"b 0 0 0 0 - - placement= image:./p.png",
	}
	for _, data := range cases {
		t.Run(data, func(t *testing.T) {
			_, _, _, _, err := parseSpanMessage(data, 100)
			if err == nil {
				t.Errorf("expected error for %q; got nil", data)
			}
		})
	}
}

// TestParseBoxPlacementWithOtherFlags: placement coexists
// with bold/italic/scale/family in any order. The flags
// apply to the box's covered runes (which render as text
// for placement=below).
func TestParseBoxPlacementWithOtherFlags(t *testing.T) {
	cases := []string{
		"b 0 11 0 0 - - placement=below bold image:./p.png",
		"b 0 11 0 0 - - bold placement=below image:./p.png",
		"b 0 11 0 0 - - scale=1.5 placement=below image:./p.png",
		"b 0 11 0 0 - - placement=below family=code image:./p.png",
	}
	for _, data := range cases {
		t.Run(data, func(t *testing.T) {
			runs, _, _, _, err := parseSpanMessage(data, 100)
			if err != nil {
				t.Fatalf("parseSpanMessage: %v", err)
			}
			if runs[0].Style.BoxPlacement != "below" {
				t.Errorf("BoxPlacement not parsed; line was %q", data)
			}
		})
	}
}

// TestParseBoxPlacementBelowContiguity: a placement=below b
// at offset N (length=L) followed by a span at offset N+L
// satisfies the contiguity rule.
func TestParseBoxPlacementBelowContiguity(t *testing.T) {
	data := "s 0 5 -\n" +
		"b 5 11 0 0 - - placement=below image:./p.png\n" +
		"s 16 4 -\n"
	runs, _, _, _, err := parseSpanMessage(data, 100)
	if err != nil {
		t.Fatalf("parseSpanMessage: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(runs))
	}
	if runs[1].Style.BoxPlacement != "below" {
		t.Error("middle run should be a placement=below box")
	}
}

// --- Region directive tests (Phase 3 round 5) --------------------------

// TestParseSpanMessageBeginEndRegion: a balanced begin/end
// region pair around s directives produces a single region
// in the result.
func TestParseSpanMessageBeginEndRegion(t *testing.T) {
	data := "s 0 5 -\n" +
		"begin region code\n" +
		"s 5 10 - family=code\n" +
		"end region\n" +
		"s 15 5 -"
	runs, _, regions, _, err := parseSpanMessage(data, 100)
	if err != nil {
		t.Fatalf("parseSpanMessage: %v", err)
	}
	if len(runs) != 3 {
		t.Errorf("got %d runs, want 3", len(runs))
	}
	if len(regions) != 1 {
		t.Fatalf("got %d regions, want 1", len(regions))
	}
	r := regions[0]
	if r.Kind != "code" {
		t.Errorf("region.Kind = %q, want %q", r.Kind, "code")
	}
	if r.Start != 5 || r.End != 15 {
		t.Errorf("region range = [%d, %d), want [5, 15)", r.Start, r.End)
	}
	if len(r.Params) != 0 {
		t.Errorf("region.Params = %v, want empty", r.Params)
	}
}

// TestParseSpanMessageRegionWithLangParam: the optional
// language hint lands in Params.
func TestParseSpanMessageRegionWithLangParam(t *testing.T) {
	data := "begin region code lang=go\n" +
		"s 0 5 - family=code\n" +
		"end region"
	_, _, regions, _, err := parseSpanMessage(data, 100)
	if err != nil {
		t.Fatalf("parseSpanMessage: %v", err)
	}
	if len(regions) != 1 {
		t.Fatalf("got %d regions, want 1", len(regions))
	}
	if got := regions[0].Params["lang"]; got != "go" {
		t.Errorf("Params[lang] = %q, want %q", got, "go")
	}
}

// TestParseSpanMessageRegionDoesNotAdvanceCursor: the
// directives slot between s/b directives without breaking
// contiguity.
func TestParseSpanMessageRegionDoesNotAdvanceCursor(t *testing.T) {
	data := "s 0 5 -\n" +
		"begin region code\n" +
		"s 5 5 -\n" +
		"end region\n" +
		"s 10 5 -"
	_, _, _, _, err := parseSpanMessage(data, 100)
	if err != nil {
		t.Errorf("expected no error from contiguous spans around region; got %v", err)
	}
}

// TestParseSpanMessageRegionUnmatchedBegin: a begin without
// matching end is a protocol error.
func TestParseSpanMessageRegionUnmatchedBegin(t *testing.T) {
	data := "begin region code\n" +
		"s 0 5 - family=code"
	_, _, _, _, err := parseSpanMessage(data, 100)
	if err == nil {
		t.Error("expected error for unmatched begin; got nil")
	}
}

// TestParseSpanMessageRegionUnmatchedEnd: an end without
// matching begin is a protocol error.
func TestParseSpanMessageRegionUnmatchedEnd(t *testing.T) {
	data := "s 0 5 -\n" +
		"end region"
	_, _, _, _, err := parseSpanMessage(data, 100)
	if err == nil {
		t.Error("expected error for unmatched end; got nil")
	}
}

// TestParseSpanMessageRegionUnknownKind: only v1-recognized
// kinds parse; anything else is a protocol error.
func TestParseSpanMessageRegionUnknownKind(t *testing.T) {
	cases := []string{
		"begin region UNKNOWN_KIND\nend region",
		"begin region\nend region",             // missing kind
		"begin region blockquote\nend region",  // future, not v1
	}
	for _, data := range cases {
		t.Run(data, func(t *testing.T) {
			_, _, _, _, err := parseSpanMessage(data, 100)
			if err == nil {
				t.Errorf("expected error for %q; got nil", data)
			}
		})
	}
}

// TestParseSpanMessageRegionMalformedParam: empty / missing-=
// params are silently ignored (forward-compat).
func TestParseSpanMessageRegionMalformedParam(t *testing.T) {
	cases := []string{
		"begin region code lang=\nend region",
		"begin region code unknownkey=val\nend region",
		"begin region code malformedtoken\nend region",
	}
	for _, data := range cases {
		t.Run(data, func(t *testing.T) {
			_, _, regions, _, err := parseSpanMessage(data, 100)
			if err != nil {
				t.Errorf("expected no error for %q; got %v", data, err)
			}
			if len(regions) != 1 {
				t.Errorf("got %d regions, want 1", len(regions))
			}
		})
	}
}

// TestParseSpanMessageRegionNested: parser handles nested
// begin/end. Round 5 doesn't emit nesting from md2spans;
// the parser accepts it for forward-compat. Returned flat —
// Region.Parent/Children set later by the store.
func TestParseSpanMessageRegionNested(t *testing.T) {
	data := "begin region code\n" +
		"begin region code\n" +
		"s 0 5 - family=code\n" +
		"end region\n" +
		"end region"
	_, _, regions, _, err := parseSpanMessage(data, 100)
	if err != nil {
		t.Fatalf("parseSpanMessage: %v", err)
	}
	if len(regions) != 2 {
		t.Errorf("got %d regions, want 2 (outer + inner)", len(regions))
	}
}

// TestParseSpanMessageBeginRegionBeforeAnyDirective: a
// `begin region` at the very top of a write (before any
// s/b directive sets the contiguity cursor) anchors the
// region at offset 0, not at the parser's internal
// expectedOffset=-1 sentinel. Pin the degenerate-case
// behavior so a future cursor refactor doesn't break it.
func TestParseSpanMessageBeginRegionBeforeAnyDirective(t *testing.T) {
	data := "begin region code\n" +
		"s 0 5 - family=code\n" +
		"end region"
	_, _, regions, _, err := parseSpanMessage(data, 100)
	if err != nil {
		t.Fatalf("parseSpanMessage: %v", err)
	}
	if len(regions) != 1 {
		t.Fatalf("got %d regions, want 1", len(regions))
	}
	if regions[0].Start != 0 || regions[0].End != 5 {
		t.Errorf("region range = [%d, %d), want [0, 5)", regions[0].Start, regions[0].End)
	}
}

// TestParseSpanMessageRegionEmpty: a begin immediately
// followed by end with no spans inside produces a region
// with Start == End.
func TestParseSpanMessageRegionEmpty(t *testing.T) {
	data := "s 0 5 -\n" +
		"begin region code\n" +
		"end region\n" +
		"s 5 5 -"
	_, _, regions, _, err := parseSpanMessage(data, 100)
	if err != nil {
		t.Fatalf("parseSpanMessage: %v", err)
	}
	if len(regions) != 1 {
		t.Fatalf("got %d regions, want 1", len(regions))
	}
	if regions[0].Start != 5 || regions[0].End != 5 {
		t.Errorf("empty region: [%d, %d), want [5, 5)", regions[0].Start, regions[0].End)
	}
}

// TestParseBoxPayloadWithParams: a placement=below b with
// `image:URL width=N` payload preserves the multi-token
// payload string verbatim. Param interpretation happens in
// the consumer (boxStyleToRichStyle), not the parser.
func TestParseBoxPayloadWithParams(t *testing.T) {
	data := "b 5 11 0 0 - - placement=below image:./p.png width=200"
	runs, _, _, _, err := parseSpanMessage(data, 100)
	if err != nil {
		t.Fatalf("parseSpanMessage: %v", err)
	}
	want := "image:./p.png width=200"
	if runs[0].Style.BoxPayload != want {
		t.Errorf("BoxPayload = %q, want %q", runs[0].Style.BoxPayload, want)
	}
}
