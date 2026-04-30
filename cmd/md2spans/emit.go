package main

import (
	"fmt"
	"strings"
)

// FormatSpans renders the styled-span list as bytes for a single
// write to the window's spans file. Output covers the full body
// rune range [0, totalRunes) as a CONTIGUOUS sequence of `s`
// directives — gaps between the supplied styled spans are filled
// with default-styled spans (fg "-", no flags). The
// spans-protocol parser (spanparse.go:parseSpanMessage) rejects
// non-contiguous writes, so this contiguity is mandatory.
//
// Format per line:
//
//	s <offset> <length> <fg> [flags...]
//
// where <fg> is the Span's Fg field (e.g. "#0000cc") or "-" for
// "use the default". Flags are "bold" and/or "italic". One `s`
// line per emitted span; no leading `c\n` (the writer issues the
// clear as a separate Twrite — see writeSpans in main.go).
//
// Format matches spanparse.go:parseSpanLine. Offsets and lengths
// are rune-indexed (R7). If totalRunes <= 0, returns "".
//
// Preconditions: input styled spans must be sorted by Offset and
// non-overlapping. Out-of-bounds spans are silently clipped to
// [0, totalRunes).
func FormatSpans(styled []Span, totalRunes int) string {
	if totalRunes <= 0 {
		return ""
	}
	contiguous := fillGaps(styled, totalRunes)
	var b strings.Builder
	for _, s := range contiguous {
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
		// Scale==0 is the unset sentinel (renders at 1.0
		// baseline). Omit the flag in that case so plain text
		// produces minimal wire output. A producer that
		// chooses to emit explicit scale=1.0 (Span.Scale=1.0)
		// will get that on the wire — it round-trips through
		// the parser as Scale=1.0, distinct from unset.
		if s.Scale != 0 {
			fmt.Fprintf(&b, " scale=%g", s.Scale)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// fillGaps returns a contiguous span list covering [0, totalRunes)
// by inserting default-styled spans between, before, and after the
// supplied styled spans. Spans that fall outside [0, totalRunes)
// are clipped or dropped. Mirrors cmd/edcolor's colorize tail.
func fillGaps(styled []Span, totalRunes int) []Span {
	out := make([]Span, 0, 2*len(styled)+1)
	cursor := 0
	for _, s := range styled {
		// Clip to body bounds; drop spans entirely past the end.
		start := s.Offset
		end := s.Offset + s.Length
		if start < 0 {
			start = 0
		}
		if end > totalRunes {
			end = totalRunes
		}
		if start >= end || start >= totalRunes {
			continue
		}
		// Skip spans that overlap the cursor (defensive — input
		// is supposed to be non-overlapping; if not, we keep the
		// earlier styled run).
		if start < cursor {
			start = cursor
			if start >= end {
				continue
			}
		}
		if start > cursor {
			out = append(out, Span{Offset: cursor, Length: start - cursor})
		}
		out = append(out, Span{
			Offset: start,
			Length: end - start,
			Fg:     s.Fg,
			Bold:   s.Bold,
			Italic: s.Italic,
			Scale:  s.Scale,
		})
		cursor = end
	}
	if cursor < totalRunes {
		out = append(out, Span{Offset: cursor, Length: totalRunes - cursor})
	}
	return out
}
