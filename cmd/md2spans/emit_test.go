package main

import (
	"strings"
	"testing"
)

// TestFormatSpansEmptyBody: zero rune count emits empty string.
func TestFormatSpansEmptyBody(t *testing.T) {
	if got := FormatSpans(nil, 0); got != "" {
		t.Errorf("FormatSpans(nil, 0) = %q, want empty", got)
	}
}

// TestFormatSpansAllDefault: no styled spans, body of 10 runes
// emits a single default-styled span covering [0, 10).
func TestFormatSpansAllDefault(t *testing.T) {
	got := FormatSpans(nil, 10)
	want := "s 0 10 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansOneItalicMidBody: one italic span at [3, 8) in
// a body of 10 runes produces a contiguous sequence:
//
//	default [0, 3), italic [3, 8), default [8, 10).
func TestFormatSpansOneItalicMidBody(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 3, Length: 5, Italic: true}}, 10)
	want := "s 0 3 -\ns 3 5 - italic\ns 8 2 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansOneBoldAtStart: span at offset 0 needs no
// leading default.
func TestFormatSpansOneBoldAtStart(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 0, Length: 4, Bold: true}}, 10)
	want := "s 0 4 - bold\ns 4 6 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansOneAtEnd: span ending at totalRunes needs no
// trailing default.
func TestFormatSpansOneAtEnd(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 6, Length: 4, Italic: true}}, 10)
	want := "s 0 6 -\ns 6 4 - italic\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansAdjacentStyled: two styled spans touching with
// no gap between produce no intermediate default.
func TestFormatSpansAdjacentStyled(t *testing.T) {
	got := FormatSpans([]Span{
		{Offset: 0, Length: 3, Bold: true},
		{Offset: 3, Length: 4, Italic: true},
	}, 10)
	want := "s 0 3 - bold\ns 3 4 - italic\ns 7 3 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansFg: a colored span emits the hex.
func TestFormatSpansFg(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 0, Length: 5, Fg: "#0000cc"}}, 10)
	want := "s 0 5 #0000cc\ns 5 5 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansFgPlusFlags: color + flags coexist.
func TestFormatSpansFgPlusFlags(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 0, Length: 4, Fg: "#0000cc", Italic: true}}, 5)
	want := "s 0 4 #0000cc italic\ns 4 1 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansMultiple: three styled spans separated by gaps,
// in a body of 22 runes — matches the layout from the link-
// adjacent-to-emphasis parser test ("*pre* [mid](u) *post*",
// minus the bracket runes that aren't covered by spans).
func TestFormatSpansMultiple(t *testing.T) {
	styled := []Span{
		{Offset: 1, Length: 3, Italic: true},
		{Offset: 7, Length: 3, Fg: "#0000cc"},
		{Offset: 16, Length: 4, Italic: true},
	}
	got := FormatSpans(styled, 22)
	want := "s 0 1 -\n" +
		"s 1 3 - italic\n" +
		"s 4 3 -\n" +
		"s 7 3 #0000cc\n" +
		"s 10 6 -\n" +
		"s 16 4 - italic\n" +
		"s 20 2 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansContiguous: pin the contiguity invariant — every
// `s` line's offset must equal the previous line's offset+length.
// This is what spanparse.go enforces and what we must produce.
func TestFormatSpansContiguous(t *testing.T) {
	styled := []Span{
		{Offset: 5, Length: 3, Italic: true},
		{Offset: 12, Length: 4, Bold: true},
		{Offset: 20, Length: 5, Fg: "#0000cc"},
	}
	got := FormatSpans(styled, 30)
	// Walk lines, parse offset+length, verify contiguity.
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	cursor := 0
	for i, line := range lines {
		var off, length int
		var fg string
		// "s OFFSET LENGTH FG ..." — only need the first three numeric/text fields.
		_, err := stringScan(line, "s %d %d %s", &off, &length, &fg)
		if err != nil {
			t.Fatalf("line %d %q: parse error %v", i, line, err)
		}
		if off != cursor {
			t.Errorf("line %d: offset %d, want %d (non-contiguous)", i, off, cursor)
		}
		cursor = off + length
	}
	if cursor != 30 {
		t.Errorf("final cursor = %d, want 30", cursor)
	}
}

// stringScan is a small wrapper to make the contiguity test
// readable without pulling in fmt.Sscanf-with-error-vibes.
func stringScan(line, format string, args ...interface{}) (int, error) {
	// Trim trailing flag tokens we don't need to parse.
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return 0, errf("not enough fields in %q", line)
	}
	*(args[0].(*int)) = parseInt(fields[1])
	*(args[1].(*int)) = parseInt(fields[2])
	*(args[2].(*string)) = fields[3]
	return 3, nil
}

func errf(format string, args ...interface{}) error {
	return scanErr{msg: format}
}

type scanErr struct{ msg string }

func (e scanErr) Error() string { return e.msg }

func parseInt(s string) int {
	n := 0
	neg := false
	for i, c := range s {
		if i == 0 && c == '-' {
			neg = true
			continue
		}
		n = n*10 + int(c-'0')
	}
	if neg {
		n = -n
	}
	return n
}

// TestFormatSpansClipsToBody: a styled span past the body length
// is clipped to the body bound.
func TestFormatSpansClipsToBody(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 0, Length: 100, Italic: true}}, 5)
	want := "s 0 5 - italic\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansDropsOutOfRangeSpan: a styled span entirely past
// the body length is dropped.
func TestFormatSpansDropsOutOfRangeSpan(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 100, Length: 5, Italic: true}}, 10)
	want := "s 0 10 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
