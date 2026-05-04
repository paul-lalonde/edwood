package main

import (
	"fmt"
	"sort"
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
func FormatSpans(input []Span, totalRunes int) string {
	if totalRunes <= 0 {
		return ""
	}
	// Separate region directives (sentinels) from
	// styled/box spans. Region directives have Length=0 and
	// don't represent rune coverage; they slot between
	// styled spans at their offsets. Phase 3 round 5;
	// switched to Kind discrimination in round 6.5.
	var styled []Span
	var directives []Span
	for _, s := range input {
		switch s.Kind {
		case SpanRegionBegin, SpanRegionEnd:
			directives = append(directives, s)
		default:
			styled = append(styled, s)
		}
	}
	// Stable-sort directives by Offset so the merge below
	// can use a single forward walk. Stable preserves input
	// order for same-offset directives (e.g., a parser-emitted
	// [begin@N, end@N] empty region must emit begin-then-end
	// in that order). Round 5 producers naturally emit
	// directives in offset order; the sort makes the merge
	// robust against future producers that don't.
	sort.SliceStable(directives, func(i, j int) bool {
		return directives[i].Offset < directives[j].Offset
	})
	// Anchors: offsets where region directives sit. fillGaps
	// splits default-fill spans at these offsets so the
	// interleaver can slot directives in. Without anchor
	// splits, an empty region between two default-styled
	// regions would have nowhere to insert its begin/end.
	anchors := uniqueDirectiveOffsets(directives)
	contiguous := fillGapsWithAnchors(styled, totalRunes, anchors)
	var b strings.Builder
	// Merge contiguous spans and directives by offset,
	// preserving input order among same-offset directives.
	// Rule: a directive at offset O is emitted IMMEDIATELY
	// before any styled span starting at offset O. (For an
	// `end region` after a styled span ending at O, this
	// places the end at the boundary between the closing
	// span and the next opening — which is the correct
	// position regardless of which "side" you frame it as.)
	emitSpan := func(s Span) {
		if s.Kind == SpanBox {
			writeBoxLine(&b, s)
		} else {
			writeSpanLine(&b, s)
		}
	}
	emitDirective := func(d Span) {
		if d.Kind == SpanRegionBegin {
			writeBeginRegionLine(&b, d)
		} else {
			writeEndRegionLine(&b)
		}
	}
	si, di := 0, 0
	for si < len(contiguous) || di < len(directives) {
		if di >= len(directives) {
			emitSpan(contiguous[si])
			si++
			continue
		}
		if si >= len(contiguous) {
			emitDirective(directives[di])
			di++
			continue
		}
		if directives[di].Offset <= contiguous[si].Offset {
			emitDirective(directives[di])
			di++
		} else {
			emitSpan(contiguous[si])
			si++
		}
	}
	return b.String()
}

// writeBeginRegionLine emits a `begin region <kind>
// [param=value...]` directive. Phase 3 round 5.
func writeBeginRegionLine(b *strings.Builder, s Span) {
	fmt.Fprintf(b, "begin region %s", s.RegionBegin)
	for k, v := range s.RegionParams {
		fmt.Fprintf(b, " %s=%s", k, v)
	}
	b.WriteByte('\n')
}

// writeEndRegionLine emits an `end region` directive. v1
// `end region` carries no kind or params; the receiver pops
// the most recent open begin. Phase 3 round 5.
func writeEndRegionLine(b *strings.Builder) {
	b.WriteString("end region\n")
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

// uniqueDirectiveOffsets returns a sorted, deduplicated
// list of offsets at which region directives sit. Used by
// FormatSpans to force splits in the default-fill output so
// directives can slot in. Phase 3 round 5.
func uniqueDirectiveOffsets(directives []Span) []int {
	seen := map[int]bool{}
	for _, d := range directives {
		seen[d.Offset] = true
	}
	out := make([]int, 0, len(seen))
	for o := range seen {
		out = append(out, o)
	}
	// Insertion sort (small N typical for region anchors).
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// fillGapsWithAnchors is fillGaps + an extra constraint:
// default-fill spans are SPLIT at every anchor offset so
// directives can be interleaved at those exact points.
// Phase 3 round 5.
func fillGapsWithAnchors(styled []Span, totalRunes int, anchors []int) []Span {
	contiguous := fillGaps(styled, totalRunes)
	if len(anchors) == 0 {
		return contiguous
	}
	out := make([]Span, 0, len(contiguous)+len(anchors))
	for _, s := range contiguous {
		// Only default-fill spans get split — they have no
		// styled fields. Styled spans either align with
		// anchors (parser-emitted body spans) or don't,
		// and don't need splitting.
		if !isDefaultFill(s) {
			out = append(out, s)
			continue
		}
		cursor := s.Offset
		end := s.Offset + s.Length
		for _, a := range anchors {
			if a <= cursor || a >= end {
				continue
			}
			out = append(out, Span{Offset: cursor, Length: a - cursor})
			cursor = a
		}
		if cursor < end {
			out = append(out, Span{Offset: cursor, Length: end - cursor})
		}
	}
	return out
}

// isDefaultFill reports whether s is a default-styled span
// (one that fillGaps inserts between styled spans). These
// are the only spans that fillGapsWithAnchors splits.
func isDefaultFill(s Span) bool {
	if s.Kind != SpanStyled {
		return false
	}
	return s.Fg == "" && !s.Bold && !s.Italic && s.Scale == 0 &&
		s.Family == "" && !s.HRule
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
			Kind:         s.Kind,
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
