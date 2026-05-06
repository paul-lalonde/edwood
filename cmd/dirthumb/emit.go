package main

import (
	"fmt"
	"strings"
)

// Span is dirthumb's wire-format input record. Two shapes are
// used: SpanStyled (default-fill gap, no styling) and SpanBox
// (an image thumbnail at a directory entry's runes). Region
// directives are NOT used by dirthumb v1.
type Span struct {
	Kind         SpanKind
	Offset       int
	Length       int
	BoxWidth     int
	BoxHeight    int
	BoxPlacement string
	BoxPayload   string
}

// SpanKind discriminates Span shape. Subset of md2spans.
type SpanKind int

const (
	SpanStyled SpanKind = iota
	SpanBox
)

// FormatSpans renders the span list as bytes for a single
// write to the window's spans file. Output covers the full
// rune range [0, totalRunes) as a CONTIGUOUS sequence: gap
// fill spans interleave with the supplied SpanBox entries.
//
// dirthumb only emits two directive types (`s` default-fill
// and `b` image box), so this is a stripped-down variant of
// md2spans's FormatSpans — no regions, no styled flags.
//
// Preconditions: input is sorted by Offset and
// non-overlapping. Out-of-bounds spans are clipped.
func FormatSpans(input []Span, totalRunes int) string {
	if totalRunes <= 0 {
		return ""
	}
	contiguous := fillGaps(input, totalRunes)
	var b strings.Builder
	for _, s := range contiguous {
		switch s.Kind {
		case SpanBox:
			writeBoxLine(&b, s)
		default:
			writeSpanLine(&b, s)
		}
	}
	return b.String()
}

// writeSpanLine emits `s OFFSET LENGTH -` for a default-fill
// gap span. dirthumb has no styling so fg is always "-" and
// no flags follow.
func writeSpanLine(b *strings.Builder, s Span) {
	fmt.Fprintf(b, "s %d %d -\n", s.Offset, s.Length)
}

// writeBoxLine emits `b OFFSET LENGTH WIDTH HEIGHT - -
// placement=PLACEMENT PAYLOAD` for an image thumbnail.
func writeBoxLine(b *strings.Builder, s Span) {
	fmt.Fprintf(b, "b %d %d %d %d - -", s.Offset, s.Length, s.BoxWidth, s.BoxHeight)
	if s.BoxPlacement != "" {
		fmt.Fprintf(b, " placement=%s", s.BoxPlacement)
	}
	if s.BoxPayload != "" {
		fmt.Fprintf(b, " %s", s.BoxPayload)
	}
	b.WriteByte('\n')
}

// fillGaps returns a contiguous span list covering [0,
// totalRunes) by inserting default-styled spans between,
// before, and after the supplied input spans. Spans that
// fall outside [0, totalRunes) are clipped or dropped.
// Mirrors md2spans/emit.go:fillGaps minus the styled-field
// copy.
func fillGaps(input []Span, totalRunes int) []Span {
	out := make([]Span, 0, 2*len(input)+1)
	cursor := 0
	for _, s := range input {
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
			Kind:         s.Kind,
			Offset:       start,
			Length:       end - start,
			BoxWidth:     s.BoxWidth,
			BoxHeight:    s.BoxHeight,
			BoxPlacement: s.BoxPlacement,
			BoxPayload:   s.BoxPayload,
		})
		cursor = end
	}
	if cursor < totalRunes {
		out = append(out, Span{Offset: cursor, Length: totalRunes - cursor})
	}
	return out
}
