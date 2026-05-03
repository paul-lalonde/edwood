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
		if s.IsBox {
			writeBoxLine(&b, s)
		} else {
			writeSpanLine(&b, s)
		}
	}
	return b.String()
}

// writeSpanLine emits one `s OFFSET LENGTH FG flags...`
// line for a non-box styled run.
func writeSpanLine(b *strings.Builder, s Span) {
	fg := s.Fg
	if fg == "" {
		fg = "-"
	}
	fmt.Fprintf(b, "s %d %d %s", s.Offset, s.Length, fg)
	writeStyleFlags(b, s)
	b.WriteByte('\n')
}

// writeBoxLine emits one `b OFFSET LENGTH WIDTH HEIGHT FG
// BG flags... payload` line for a box. Round 4 producers
// emit IsBox spans for inline images; the placement= flag
// and the payload (including any width=N param) are
// formatted here.
func writeBoxLine(b *strings.Builder, s Span) {
	fg := s.Fg
	if fg == "" {
		fg = "-"
	}
	// Box format requires the BG slot. md2spans emits "-"
	// for default; future producers may set Bg.
	fmt.Fprintf(b, "b %d %d %d %d %s -", s.Offset, s.Length, s.BoxWidth, s.BoxHeight, fg)
	writeStyleFlags(b, s)
	if s.BoxPlacement != "" {
		fmt.Fprintf(b, " placement=%s", s.BoxPlacement)
	}
	if s.BoxPayload != "" {
		fmt.Fprintf(b, " %s", s.BoxPayload)
	}
	b.WriteByte('\n')
}

// writeStyleFlags emits the optional flag tokens shared by
// `s` and `b` lines: bold, italic, scale=N.N, family=NAME,
// hrule. Flags are emitted in a stable order (matching the
// spec examples) so producers / fixtures round-trip
// byte-exactly.
func writeStyleFlags(b *strings.Builder, s Span) {
	if s.Bold {
		b.WriteString(" bold")
	}
	if s.Italic {
		b.WriteString(" italic")
	}
	// Scale==0 is the unset sentinel (renders at 1.0
	// baseline). Omit the flag so plain text produces
	// minimal wire output. A producer that chooses to emit
	// explicit scale=1.0 (Span.Scale=1.0) gets that on the
	// wire — it round-trips through the parser as
	// Scale=1.0, distinct from unset.
	if s.Scale != 0 {
		fmt.Fprintf(b, " scale=%g", s.Scale)
	}
	// Family=="" is the unset sentinel; omit the flag.
	// v1's recognized values are {"code"}; the parser
	// rejects anything else, so emitting an unknown name
	// here would produce a write the consumer rejects.
	if s.Family != "" {
		fmt.Fprintf(b, " family=%s", s.Family)
	}
	if s.HRule {
		b.WriteString(" hrule")
	}
}

// fillGaps returns a contiguous span list covering [0, totalRunes)
// by inserting default-styled spans between, before, and after the
// supplied styled spans. Spans that fall outside [0, totalRunes)
// are clipped or dropped. Mirrors cmd/edcolor's colorize tail.
//
// Length-0 box spans (Phase 3 round 4 placement=below) are
// special-cased: they don't bound a text region and don't advance
// the cursor, so they sit "between" two text spans without
// splitting their coverage.
func fillGaps(styled []Span, totalRunes int) []Span {
	out := make([]Span, 0, 2*len(styled)+1)
	cursor := 0
	for _, s := range styled {
		// Length-0 boxes (placement=below): emit fill before,
		// emit the box itself, leave cursor unchanged. The box
		// can sit at offset == totalRunes (e.g., image at end
		// of body); clamp the offset for the fill check.
		if s.IsBox && s.Length == 0 {
			start := s.Offset
			if start < 0 {
				start = 0
			}
			if start > totalRunes {
				start = totalRunes
			}
			if start > cursor {
				out = append(out, Span{Offset: cursor, Length: start - cursor})
				cursor = start
			}
			boxOut := s
			boxOut.Offset = start
			out = append(out, boxOut)
			continue
		}
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
			Offset:       start,
			Length:       end - start,
			Fg:           s.Fg,
			Bold:         s.Bold,
			Italic:       s.Italic,
			Scale:        s.Scale,
			Family:       s.Family,
			HRule:        s.HRule,
			IsBox:        s.IsBox,
			BoxWidth:     s.BoxWidth,
			BoxHeight:    s.BoxHeight,
			BoxPayload:   s.BoxPayload,
			BoxPlacement: s.BoxPlacement,
		})
		cursor = end
	}
	if cursor < totalRunes {
		out = append(out, Span{Offset: cursor, Length: totalRunes - cursor})
	}
	return out
}
