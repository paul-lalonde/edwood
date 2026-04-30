package main

import (
	"strings"
	"testing"
)

// TestFormatSpansEmpty: empty span list emits a single `c\n` line
// (clear). The clear is unconditional so each render fully replaces
// the prior styling, matching the spans-protocol convention used
// by cmd/edcolor.
func TestFormatSpansEmpty(t *testing.T) {
	got := FormatSpans(nil)
	want := "c\n"
	if got != want {
		t.Errorf("FormatSpans(nil) = %q, want %q", got, want)
	}
}

// TestFormatSpansItalic: a single italic span produces
// `c\ns OFFSET LENGTH - italic\n`.
func TestFormatSpansItalic(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 1, Length: 5, Italic: true}})
	want := "c\ns 1 5 - italic\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansBold: bold flag.
func TestFormatSpansBold(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 2, Length: 4, Bold: true}})
	want := "c\ns 2 4 - bold\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansBoldItalic: both flags.
func TestFormatSpansBoldItalic(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 0, Length: 3, Bold: true, Italic: true}})
	want := "c\ns 0 3 - bold italic\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansFg: an Fg color is emitted in place of the
// default `-`.
func TestFormatSpansFg(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 7, Length: 8, Fg: "#0000cc"}})
	want := "c\ns 7 8 #0000cc\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansFgPlusFlags: color and flags coexist.
func TestFormatSpansFgPlusFlags(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 0, Length: 4, Fg: "#0000cc", Italic: true}})
	want := "c\ns 0 4 #0000cc italic\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansMultiple: multiple spans emit one `s` line each,
// in order, after the leading `c\n`.
func TestFormatSpansMultiple(t *testing.T) {
	spans := []Span{
		{Offset: 1, Length: 3, Italic: true},
		{Offset: 7, Length: 3, Fg: "#0000cc"},
		{Offset: 16, Length: 4, Italic: true},
	}
	got := FormatSpans(spans)
	want := "c\ns 1 3 - italic\ns 7 3 #0000cc\ns 16 4 - italic\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansChunkable: the formatter doesn't preemptively
// chunk for 9P-msize limits — that's the writer's responsibility
// (see writeSpans in main.go). Here we just verify the formatter
// doesn't artificially split lines.
func TestFormatSpansChunkable(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 0, Length: 1, Italic: true}})
	if strings.Count(got, "\n") != 2 {
		t.Errorf("expected 2 newlines (clear + 1 span), got %d in %q", strings.Count(got, "\n"), got)
	}
}
