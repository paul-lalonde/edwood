package main

import (
	"fmt"
	"strings"
)

// FormatSpans renders a list of styled spans as the bytes that
// should be written to the window's spans file. The output
// always begins with a `c\n` line that clears any prior
// styling, followed by one `s` line per Span:
//
//	s <offset> <length> <fg> [flags...]
//
// where <fg> is either the Span's Fg field (for example
// "#0000cc") or "-" for "use the default". Flags are "bold"
// and/or "italic".
//
// The format matches what spanparse.go:parseSpanLine in the
// main edwood package consumes for the spans file. Offsets and
// lengths are rune-indexed (R7).
//
// Chunking for 9P msize limits is the writer's responsibility,
// not this function's — FormatSpans returns one contiguous
// string and lets the caller split on '\n' boundaries to fit
// into Twrite messages.
func FormatSpans(spans []Span) string {
	var b strings.Builder
	b.WriteString("c\n")
	for _, s := range spans {
		fg := s.Fg
		if fg == "" {
			fg = "-"
		}
		fmt.Fprintf(&b, "s %d %d %s", s.Offset, s.Length, fg)
		if s.Bold {
			b.WriteString(" bold")
		}
		if s.Italic {
			b.WriteString(" italic")
		}
		b.WriteByte('\n')
	}
	return b.String()
}
