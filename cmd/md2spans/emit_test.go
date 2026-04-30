package main

import (
	"strings"
	"testing"
)

// TestFormatSpansEmpty: empty span list emits empty string.
// The clear (`c\n`) is now a separate write — see writeSpans in
// main.go and the doc on FormatSpans.
func TestFormatSpansEmpty(t *testing.T) {
	got := FormatSpans(nil)
	if got != "" {
		t.Errorf("FormatSpans(nil) = %q, want empty", got)
	}
}

// TestFormatSpansItalic: a single italic span produces
// `s OFFSET LENGTH - italic\n`. No leading clear.
func TestFormatSpansItalic(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 1, Length: 5, Italic: true}})
	want := "s 1 5 - italic\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansBold: bold flag.
func TestFormatSpansBold(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 2, Length: 4, Bold: true}})
	want := "s 2 4 - bold\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansBoldItalic: both flags.
func TestFormatSpansBoldItalic(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 0, Length: 3, Bold: true, Italic: true}})
	want := "s 0 3 - bold italic\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansFg: an Fg color is emitted in place of the
// default `-`.
func TestFormatSpansFg(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 7, Length: 8, Fg: "#0000cc"}})
	want := "s 7 8 #0000cc\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansFgPlusFlags: color and flags coexist.
func TestFormatSpansFgPlusFlags(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 0, Length: 4, Fg: "#0000cc", Italic: true}})
	want := "s 0 4 #0000cc italic\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansMultiple: multiple spans emit one `s` line each
// in order. No leading clear; the writer prepends a separate
// clear write.
func TestFormatSpansMultiple(t *testing.T) {
	spans := []Span{
		{Offset: 1, Length: 3, Italic: true},
		{Offset: 7, Length: 3, Fg: "#0000cc"},
		{Offset: 16, Length: 4, Italic: true},
	}
	got := FormatSpans(spans)
	want := "s 1 3 - italic\ns 7 3 #0000cc\ns 16 4 - italic\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansChunkable: the formatter doesn't preemptively
// chunk for 9P-msize limits — that's the writer's responsibility
// (see writeSpans in main.go).
func TestFormatSpansChunkable(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 0, Length: 1, Italic: true}})
	if strings.Count(got, "\n") != 1 {
		t.Errorf("expected 1 newline (1 span), got %d in %q", strings.Count(got, "\n"), got)
	}
}

// TestFormatSpansNoLeadingClear documents the bug-fix: the
// formatter must NOT emit a leading `c\n` because the
// spans-protocol parser rejects a write mixing `c` with `s`
// directives. Callers issue the clear as a separate write.
func TestFormatSpansNoLeadingClear(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 0, Length: 1, Italic: true}})
	if strings.HasPrefix(got, "c") {
		t.Errorf("FormatSpans must not emit a leading `c` line; got %q", got)
	}
}
