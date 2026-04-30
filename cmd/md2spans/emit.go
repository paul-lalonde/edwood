package main

import (
	"fmt"
	"strings"
)

// FormatSpans renders a list of styled spans as the bytes for
// a single write to the window's spans file:
//
//	s <offset> <length> <fg> [flags...]
//
// where <fg> is the Span's Fg field (e.g. "#0000cc") or "-" for
// "use the default". Flags are "bold" and/or "italic". One `s`
// line per Span; no leading `c\n`.
//
// **Why no leading clear**: the spans-protocol parser
// (spanparse.go:parseSpanMessage) rejects a write that mixes a
// `c` directive with `s` directives — clear must be the only
// command in a write. Callers wanting to fully replace prior
// styling do TWO writes: a single `c\n` first, then the output
// of FormatSpans. See cmd/md2spans/main.go:writeSpans for the
// production wiring.
//
// Format matches what spanparse.go:parseSpanLine consumes.
// Offsets and lengths are rune-indexed (R7). Chunking for 9P
// msize limits is the writer's responsibility, not this
// function's.
func FormatSpans(spans []Span) string {
	if len(spans) == 0 {
		return ""
	}
	var b strings.Builder
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
