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

func TestParseSpanDefs_ErrRegionExceedsBuffer(t *testing.T) {
	// Span region extends past buffer: offset 0, length 20, but bufLen=10.
	_, _, err := parseSpanDefs("0 20 #ff0000", 10)
	if err == nil {
		t.Fatal("expected error for region exceeding buffer; got nil")
	}
	if !strings.Contains(err.Error(), "exceeds buffer") {
		t.Errorf("error = %q; want to contain %q", err.Error(), "exceeds buffer")
	}
}

func TestParseSpanDefs_ErrOffsetBeyondBuffer(t *testing.T) {
	// Offset beyond buffer length.
	_, _, err := parseSpanDefs("15 5 #ff0000", 10)
	if err == nil {
		t.Fatal("expected error for offset beyond buffer; got nil")
	}
	// Should get either "offset beyond buffer" or "exceeds buffer"
	errStr := err.Error()
	if !strings.Contains(errStr, "beyond buffer") && !strings.Contains(errStr, "exceeds buffer") {
		t.Errorf("error = %q; want to contain 'beyond buffer' or 'exceeds buffer'", errStr)
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
