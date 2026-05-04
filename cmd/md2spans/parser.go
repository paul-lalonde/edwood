package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// SpanKind discriminates the four shapes a Span can take.
// Each Span is exactly one kind; downstream consumers
// (FormatSpans, fillGaps, isDefaultFill) switch on Kind
// instead of inspecting which legacy fields are set.
//
// Wire format is unchanged — Kind is in-memory only.
// Phase 3 round 6.5.
type SpanKind int

const (
	// SpanStyled: the default kind. A rune-covering span
	// with foreground / weight / italic / scale / family /
	// hrule attributes. Zero value, so no migration needed
	// for existing styled-span construction sites.
	SpanStyled SpanKind = iota
	// SpanBox: an inline-image box (Phase 3 round 4).
	// IsBox / BoxWidth / BoxHeight / BoxPayload /
	// BoxPlacement carry the box's payload.
	SpanBox
	// SpanRegionBegin: a `begin region <kind>` directive
	// (Phase 3 round 5/6). Length is 0; RegionBegin holds
	// the kind name; RegionParams holds optional key=value.
	SpanRegionBegin
	// SpanRegionEnd: an `end region` directive. Length is
	// 0; the consumer pops the most recent open begin.
	SpanRegionEnd
)

// Span is a styled rune range in the body. Offset and Length are
// in runes (matching the spans-protocol convention used by
// cmd/edcolor and consumed by spanparse.go in the main package).
type Span struct {
	// Kind discriminates the span's shape (Phase 3 round
	// 6.5). Zero value SpanStyled matches existing
	// styled-span construction; box / region producers must
	// set Kind explicitly.
	Kind SpanKind
	// Offset is the rune index at which this span begins (0-based).
	Offset int
	// Length is the number of runes covered.
	Length int
	// Fg is a CSS-style hex color "#rrggbb" or "" for "use the
	// default foreground" (renders as "-" in the spans protocol).
	Fg string
	// Bold and Italic are flag bits. Both may be set
	// simultaneously (bold-italic).
	Bold   bool
	Italic bool
	// Scale is the font-size multiplier (1.0 = body baseline).
	// 0 is the unset sentinel — emit() omits the `scale=` flag.
	// Headings emit Scale=2.0 (H1) through 1.0 (H6); emphasis
	// inside a heading inherits the heading's Scale per the
	// Parse-time merge decision (see phase3-r1-font-scale.md).
	Scale float64
	// Family is the semantic font-family name (e.g., "code" for
	// monospace). Empty string is the unset sentinel — emit()
	// omits the `family=` flag. Inline code (`` `text` ``)
	// emits Family="code"; inside a heading, the merge attaches
	// the heading's Scale alongside (see phase3-r2-font-family.md).
	Family string
	// HRule signals that this span represents a horizontal-rule
	// line (`---` / `***` / `___` markdown form). emit() formats
	// it as the `hrule` flag; the consumer suppresses the
	// span's text and draws a rule line. See phase3-r3-hrule.md.
	HRule bool
	// Box fields (Phase 3 round 4). When IsBox is true, emit
	// formats this Span as a `b` directive instead of `s`.
	// BoxPlacement maps to the wire `placement=NAME` flag
	// (v1 values: "below" for non-replacing inline images;
	// "" or "replace" for the existing replacing semantic —
	// md2spans v1 only emits "below"). BoxPayload carries
	// the URL plus optional `key=value` params (e.g.
	// "image:./pic.png width=200"). BoxWidth/BoxHeight are
	// zero in v1 to use the renderer-probes sentinel; the
	// payload's `width=N` provides any explicit override.
	IsBox        bool
	BoxWidth     int
	BoxHeight    int
	BoxPayload   string
	BoxPlacement string
	// Region directive sentinels (Phase 3 round 5). Mutually
	// exclusive with each other and with IsBox / styled-span
	// fields. When RegionBegin != "", this Span represents a
	// `begin region <kind>` wire directive (kind is the
	// string value); RegionParams carries optional key=value
	// params. When RegionEnd is true, this Span represents
	// `end region`. Length is 0 for both — the directives
	// slot between styled runs without consuming runes.
	RegionBegin  string
	RegionEnd    bool
	RegionParams map[string]string
}

// Parse takes the markdown source and returns the list of styled
// runs that should be emitted to the spans file. Plain text
// produces no spans (default styling suffices); styled runs come
// from emphasis, inline links, and ATX headings.
//
// Offsets and lengths are in runes (R7 of md2spans.design.md).
func Parse(src string) []Span {
	var spans []Span
	for _, p := range scanParagraphs(src) {
		switch {
		case p.IsBlockquote:
			spans = append(spans, parseBlockquoteRange(src, p)...)
		case p.IsCodeBlock:
			spans = append(spans, parseCodeBlockParagraph(src, p)...)
		case p.IsHRule:
			spans = append(spans, parseHRuleParagraph(src, p)...)
		case p.IsListItem:
			spans = append(spans, parseListItemParagraph(src, p)...)
		case p.HeadingLevel > 0:
			spans = append(spans, parseHeadingParagraph(src, p)...)
		default:
			spans = append(spans, parseParagraph(src, p)...)
		}
	}
	return spans
}

// parseBlockquoteRange emits the spans for a Markdown
// blockquote group (`>` lines) — a `begin region
// blockquote` sentinel at the group's start, a recursive
// Parse of the inner content (with `>` markers stripped)
// re-anchored to the original body's rune offsets, and an
// `end region` sentinel at the group's end.
//
// Nested blockquotes (`>>` or `> > `) work via recursion:
// stripping the OUTER `>` leaves the inner content with
// its own `>` markers intact; the recursive Parse call
// detects another blockquote group there and emits another
// nested begin/end pair.
//
// Headings, fenced code blocks, HRules etc. inside a
// blockquote work the same way — the recursive Parse call
// dispatches to the appropriate handler. Phase 3 round 6.
func parseBlockquoteRange(src string, p paragraphRange) []Span {
	stripped, mapping, lineStarts := stripBlockquoteSource(src, p)
	begin := Span{
		Kind:        SpanRegionBegin,
		Offset:      p.RuneStart,
		Length:      0,
		RegionBegin: "blockquote",
	}
	groupRuneEnd := p.RuneStart + utf8.RuneCountInString(src[p.ByteStart:p.ByteEnd])
	end := Span{
		Kind:      SpanRegionEnd,
		Offset:    groupRuneEnd,
		Length:    0,
		RegionEnd: true,
	}
	out := []Span{begin}
	for _, s := range Parse(stripped) {
		// Recursive Parse must return offsets within the
		// stripped source's rune range. mapping has length
		// stripped_rune_count + 1; an offset past the end
		// indicates a sub-parser arithmetic bug (e.g., the
		// EOF-fence opener that originally added 1 for a
		// trailing \n that wasn't there). Assert here so
		// the failure surfaces with context instead of as
		// a generic mapping panic.
		if s.Offset >= len(mapping) {
			panic(fmt.Sprintf("parseBlockquoteRange: subspan offset %d past stripped end (mapping length %d, kind %q)",
				s.Offset, len(mapping), s.RegionBegin))
		}
		// Span offsets have two semantics:
		//   - Inclusive (begin, body content): the rune AT
		//     position N is INSIDE the span. Map by rune-at:
		//     mapping[N] gives the position in the parent
		//     source AT which this rune now sits.
		//   - Exclusive (region end): position N is "the
		//     boundary just BEFORE rune N" — the region
		//     EXCLUDES rune N. Map by boundary-before:
		//     mapping[N-1]+1 gives the position in the parent
		//     source just after the region's last included
		//     rune. The two coincide when no parent runes
		//     were stripped at the boundary; they diverge
		//     when the boundary lands at a stripped-marker
		//     line (e.g., a fenced code block's closer line
		//     inside a blockquote — its `>>` markers were
		//     stripped, so rune-at lookup lands AFTER them
		//     instead of at the line start).
		if s.Kind == SpanRegionEnd && s.Offset > 0 {
			s.Offset = mapping[s.Offset-1] + 1
		} else {
			s.Offset = mapping[s.Offset]
		}
		// Some region kinds need their begin offset snapped
		// to the source line's start so the line's first box
		// belongs to the deepest region (and the layout's
		// first-box-determines-indent rule produces the
		// correct per-line indent). Membership is in
		// kindsAnchorAtLineStart. Phase 3 round 6 added
		// "blockquote"; round 7 will add "listitem".
		// Other kinds (e.g., "code", whose body anchors
		// after the opener's `\n`, ALREADY at a line start)
		// must NOT be snapped — snapping would move the
		// begin onto the fence opener line.
		if kindsAnchorAtLineStart[s.RegionBegin] {
			s.Offset = snapToLineStart(s.Offset, lineStarts)
		}
		out = append(out, s)
	}
	out = append(out, end)
	return out
}

// kindsAnchorAtLineStart lists region kinds whose begin
// directive must be snapped to the source line's start
// when produced by recursive parsing. The line's first
// box must belong to the deepest region of these kinds so
// the layout's first-box-determines-indent rule indents
// the line correctly.
//
// Member criteria: the kind covers contiguous source
// LINES (column 0 to end of line), not just a sub-range
// within a line. "blockquote" qualifies (round 6).
//
// "listitem" does NOT qualify — when a list line lives
// inside a blockquote (`> - item`), the listitem region
// begins AFTER the `>` markers, leaving the line's first
// box (the `>`) with Blockquote flags only. That's the
// VISUAL we want: the `>` aligns with non-list
// blockquote lines (same column), and the list content
// appears just after the `>` at blockquote indent. If we
// snapped listitem begins to line-start, the layout's
// first-box-determines-indent rule would add ListIndent
// to the line, pushing `>` past where it belongs (a
// regression flagged in round 7 smoke testing).
//
// "code" does NOT qualify — its body anchors AFTER the
// opener fence's `\n`, which is already a line start;
// snapping a code begin would move it onto the fence-
// opener line, mis-anchoring the region.
var kindsAnchorAtLineStart = map[string]bool{
	"blockquote": true,
}

// snapToLineStart returns the largest line-start offset in
// `lineStarts` that is <= offset. lineStarts is the sorted
// slice of original-rune-positions where each line of the
// blockquote group begins. Used by parseBlockquoteRange to
// snap inner blockquote begin directives to the original
// line's start.
func snapToLineStart(offset int, lineStarts []int) int {
	snapped := offset
	for _, ls := range lineStarts {
		if ls > offset {
			break
		}
		snapped = ls
	}
	return snapped
}

// stripBlockquoteSource builds the inner source for a
// blockquote group: each line in [ByteStart, ByteEnd) has
// its leading `>` (with optional space after) removed.
//
// Returns: (stripped source, mapping, lineStarts).
//
// `mapping` is indexed by stripped rune position and gives
// the corresponding original rune position; its length is
// one past the stripped content's rune count (so
// `mapping[rune_count]` gives the position just past the
// last rune, useful for end-of-region offsets).
//
// `lineStarts` is the sorted list of original rune positions
// where each line of the group begins (i.e., the rune just
// after the previous line's \n, or p.RuneStart for line 1).
// Used by parseBlockquoteRange to snap nested blockquote
// `begin region` directives to the line start.
//
// Per CommonMark: a blockquote line starts with `>`
// optionally followed by a single space; the leading
// content (including any subsequent text) is the line's
// inner content. v1 of round 6 strictly requires `>` at
// column 0; CommonMark's lazy continuation and 1-3 leading
// spaces are deferred.
func stripBlockquoteSource(src string, p paragraphRange) (string, []int, []int) {
	var b strings.Builder
	mapping := make([]int, 0, p.ByteEnd-p.ByteStart) // upper bound
	var lineStarts []int

	// Walk the group line by line, stripping the leading
	// `>` (and optional space). Track the original rune
	// position per stripped rune so emitted span offsets
	// can be re-anchored.
	lineStart := p.ByteStart
	origRune := p.RuneStart
	for lineStart < p.ByteEnd {
		lineStarts = append(lineStarts, origRune)
		// Find the end of this line.
		lineEnd := lineStart
		for lineEnd < p.ByteEnd && src[lineEnd] != '\n' {
			lineEnd++
		}
		hasNewline := lineEnd < p.ByteEnd
		// Skip the leading `>` (always present for blockquote
		// lines, by construction in scanParagraphs).
		inner := lineStart
		if inner < lineEnd && src[inner] == '>' {
			origRune++ // skip the `>` rune
			inner++
		}
		if inner < lineEnd && src[inner] == ' ' {
			origRune++ // skip the optional space rune
			inner++
		}
		// Emit the inner content, building the mapping.
		for i := inner; i < lineEnd; {
			r, size := utf8.DecodeRuneInString(src[i:])
			b.WriteRune(r)
			mapping = append(mapping, origRune)
			origRune++
			i += size
		}
		// Emit the trailing newline if present.
		if hasNewline {
			b.WriteByte('\n')
			mapping = append(mapping, origRune)
			origRune++
			lineStart = lineEnd + 1
		} else {
			lineStart = lineEnd
		}
	}
	// Append a final mapping entry for end-of-content (the
	// position just past the last rune).
	mapping = append(mapping, origRune)
	return b.String(), mapping, lineStarts
}

// parseCodeBlockParagraph emits the spans for a fenced code
// block. v1 (round 5) emitted ONE region covering the
// whole body; v2 (round 6.5) emits ONE REGION PER BODY LINE
// (begin / body span / end per line), so when this code
// block is recursively remapped through a blockquote, the
// blockquote markers BETWEEN body lines fall OUTSIDE any
// code region. Without per-line emission, those markers
// land inside a contiguous region [begin, end) — which the
// renderer styles as code-block content (visible as a
// double indent on body line 2+).
//
// At the top level (no recursive remap), per-line emission
// produces multiple abutting regions that render identically
// to one big region — the layout per-rune lookup composes
// the same flags for adjacent identical kinds.
//
// Per-line spans:
//   - `begin region code` at line's content start. The
//     `lang=NAME` param appears only on the FIRST line's
//     begin (the language belongs to the code BLOCK, not
//     each line; emitting it once is consistent with the
//     v1 protocol).
//   - body span (family=code) covering the line's content
//     plus its trailing `\n` (if any).
//   - `end region` at the line's end (one past the `\n`).
//
// Empty body emits a single begin/end pair at the body
// position so consumers still see the `code` region exist
// (matches round-5 behavior).
func parseCodeBlockParagraph(src string, p paragraphRange) []Span {
	bodyByteStart := p.ByteStart
	bodyByteEnd := p.ByteEnd
	bodyRuneStart := p.CodeBodyRuneStart
	bodyRuneEnd := p.CodeBodyRuneEnd

	// Empty body: emit just the begin/end pair.
	if bodyRuneStart >= bodyRuneEnd {
		begin := Span{
			Kind:        SpanRegionBegin,
			Offset:      bodyRuneStart,
			RegionBegin: "code",
		}
		if p.CodeLang != "" {
			begin.RegionParams = map[string]string{"lang": p.CodeLang}
		}
		end := Span{
			Kind:      SpanRegionEnd,
			Offset:    bodyRuneEnd,
			RegionEnd: true,
		}
		return []Span{begin, end}
	}

	var spans []Span
	lineRuneStart := bodyRuneStart
	runePos := bodyRuneStart
	firstLine := true

	emitLine := func(lineRuneEnd int) {
		begin := Span{
			Kind:        SpanRegionBegin,
			Offset:      lineRuneStart,
			RegionBegin: "code",
		}
		if firstLine && p.CodeLang != "" {
			begin.RegionParams = map[string]string{"lang": p.CodeLang}
		}
		body := Span{
			Offset: lineRuneStart,
			Length: lineRuneEnd - lineRuneStart,
			Family: "code",
		}
		end := Span{
			Kind:      SpanRegionEnd,
			Offset:    lineRuneEnd,
			RegionEnd: true,
		}
		spans = append(spans, begin, body, end)
		firstLine = false
	}

	for i := bodyByteStart; i < bodyByteEnd; {
		r, size := utf8.DecodeRuneInString(src[i:])
		i += size
		runePos++
		if r == '\n' {
			emitLine(runePos)
			lineRuneStart = runePos
		}
	}
	// Final line without trailing \n (e.g., unclosed fence
	// at EOF).
	if lineRuneStart < bodyRuneEnd {
		emitLine(bodyRuneEnd)
	}
	return spans
}

// parseHRuleParagraph emits a single Span over the HRule
// marker runes with HRule=true. The wrapper renderer
// (rich/mdrender) suppresses the span's text and draws a
// horizontal line spanning the frame width on the line.
//
// Trailing whitespace (allowed after the markers) is NOT part
// of the emitted span — the rule visually overlays the marker
// region; the trailing whitespace gets a default-styled fill
// from emit.go's fillGaps.
func parseHRuleParagraph(src string, p paragraphRange) []Span {
	n := detectHRule(src, p.ByteStart, p.ByteEnd)
	if n <= 0 {
		// Defensive: scanParagraphs only sets IsHRule when
		// detectHRule returns > 0, so this shouldn't happen.
		return nil
	}
	return []Span{{
		Offset: p.RuneStart,
		Length: n,
		HRule:  true,
	}}
}

// parseListItemParagraph emits the spans for ONE list-
// item line as a self-contained begin/content/end triple.
// Items at deeper indent depths are emitted as SIBLINGS,
// not nested — the visual nesting in the rendered view
// comes from the SOURCE LEADING WHITESPACE that the
// markup-stays-visible stance preserves in the body.
//
// Concretely, for a depth-N item with `2*(N-1)` source
// leading spaces:
//
//   - Each list line's first box has `ListItem=true,
//     ListIndent=1` (one ancestor — the item's own
//     region). Layout's first-box-determines-indent rule
//     applies one ListIndentWidth of indent to every
//     list line, regardless of depth.
//   - The source leading whitespace renders as plain
//     text after that, contributing `2*(N-1)*charWidth`
//     of additional offset.
//   - Net: depth-N item's marker lands at column
//     `N * ListIndentWidth`, matching the in-tree path.
//
// If the regions were instead nested (outer covers
// sub-list), the bridge's ancestor walk would set
// `ListIndent=N` for depth-N items, and the layout would
// add `N * Width` of indent ON TOP of the source
// whitespace — visually doubling the indent. That's
// wrong for markup-stays-visible, so v1.1 keeps each
// item as its own non-nested region. Wire format is
// flat siblings; depth lives in the source whitespace.
//
// Phase 3 round 7 v1; round 7.x kept the per-item shape
// after the nesting analysis showed source whitespace
// already provides the visual depth.
func parseListItemParagraph(src string, p paragraphRange) []Span {
	begin := Span{
		Kind:        SpanRegionBegin,
		Offset:      p.RuneStart,
		RegionBegin: "listitem",
	}
	if p.ListMarker != 0 {
		begin.RegionParams = map[string]string{
			"marker": string(p.ListMarker),
		}
	} else {
		begin.RegionParams = map[string]string{
			"number": strconv.Itoa(p.ListNumber),
		}
	}
	itemRuneEnd := p.RuneStart + utf8.RuneCountInString(src[p.ByteStart:p.ByteEnd])
	end := Span{
		Kind:      SpanRegionEnd,
		Offset:    itemRuneEnd,
		RegionEnd: true,
	}
	out := []Span{begin}
	contentBytes := contentBytePos(src, p)
	if contentBytes < p.ByteEnd {
		runes := []rune(src[contentBytes:p.ByteEnd])
		out = append(out, parseInlineSpans(runes, p.ListContentRuneStart)...)
	}
	out = append(out, end)
	return out
}

// contentBytePos returns the byte position in src where
// a list item's CONTENT begins — i.e., just past any
// leading whitespace, the marker, and the required space.
// Mirrors the contentByte computed by isListLine.
//
// Round 7 v1 ignored leading whitespace (column-0 only);
// round 7.x adds nesting via leading spaces / tabs, so
// the helper now skips that whitespace first.
func contentBytePos(src string, p paragraphRange) int {
	// Skip leading whitespace (matches isListLine's
	// indent-counting prefix).
	i := p.ByteStart
	for i < p.ByteEnd && (src[i] == ' ' || src[i] == '\t') {
		i++
	}
	if p.ListMarker != 0 {
		// Unordered: 1 marker byte + 1 space byte.
		return i + 2
	}
	// Ordered: digits + ('.' | ')') + space.
	for i < p.ByteEnd && src[i] >= '0' && src[i] <= '9' {
		i++
	}
	// Skip `.` or `)` then space. Defensive: scanParagraphs
	// only sets IsListItem after isListLine validated.
	if i < p.ByteEnd {
		i++ // `.` or `)`
	}
	if i < p.ByteEnd {
		i++ // space
	}
	return i
}

// parseHeadingParagraph emits scaled spans for an ATX heading
// paragraph. Per the Phase 3 round 1 design, the entire heading
// content (the runes after the `# ` opener) is covered by a
// span at the heading's scale; emphasis runs inside the heading
// emit additional spans that carry BOTH their flag and the
// heading's scale (the Parse-time merge).
//
// The `#` opener runes themselves are NOT covered by any span
// — they remain at body baseline, consistent with v1's
// "markup runes visible at body scale" stance. (CommonMark hides
// the opener in rendered HTML; v1 leaves it visible to make
// Markdown editing self-consistent. Hiding via the `Hidden`
// protocol flag is future work.)
func parseHeadingParagraph(src string, p paragraphRange) []Span {
	scale := headingScale[p.HeadingLevel]
	if scale == 0 {
		// Defensive: H6's scale is 1.0, which we still want as
		// a rendered scale to keep the line height computation
		// consistent. headingScale[6] == 1.0 already; the only
		// path to scale==0 here is HeadingLevel out of range.
		return nil
	}

	runes := []rune(src[p.ByteStart:p.ByteEnd])
	// Locate the heading content: skip the leading `#`s and the
	// single space (if present). detectHeadingLevel guarantees
	// 1-6 `#` runes followed by a space or end-of-line.
	contentStart := p.HeadingLevel
	if contentStart < len(runes) && runes[contentStart] == ' ' {
		contentStart++
	}
	if contentStart >= len(runes) {
		// Empty heading (`#` with nothing after). No span.
		return nil
	}

	// Run the inline tokenizer over the heading content. Each
	// emphasis / link span gets the heading's Scale layered on
	// top of its existing flags. Default-content gaps are filled
	// with Scale-only spans so the line height is consistent.
	contentRuneStart := p.RuneStart + contentStart
	innerSpans := parseInlineSpans(runes[contentStart:], contentRuneStart)

	// Compose the contiguous heading-content output. Walk the
	// inner spans (sorted by Offset, non-overlapping) and emit
	// a Scale-only span for each gap, then the inner span with
	// Scale added.
	var out []Span
	cursor := contentRuneStart
	contentEnd := p.RuneStart + len(runes)
	for _, s := range innerSpans {
		if s.Offset > cursor {
			out = append(out, Span{
				Offset: cursor,
				Length: s.Offset - cursor,
				Scale:  scale,
			})
		}
		s.Scale = scale
		out = append(out, s)
		cursor = s.Offset + s.Length
	}
	if cursor < contentEnd {
		out = append(out, Span{
			Offset: cursor,
			Length: contentEnd - cursor,
			Scale:  scale,
		})
	}
	return out
}

// parseInlineSpans runs the emphasis, link, image, and
// inline-code tokenizers over `runes` (a paragraph's content),
// returning Span values whose Offsets are body-absolute
// (runeStart + per-paragraph index). Reused by both
// parseParagraph and parseHeadingParagraph.
//
// Image syntax (`![alt](url)`) is recognized BEFORE the link
// tokenizer; the discriminating character is `!`. Phase 3
// round 4.
func parseInlineSpans(runes []rune, runeStart int) []Span {
	var spans []Span
	for i := 0; i < len(runes); {
		switch c := runes[i]; {
		case c == '!':
			s, advance, ok := tryImage(runes, i, runeStart)
			if ok {
				spans = append(spans, s)
			}
			i += advance
		case c == '*' || c == '_':
			s, advance, ok := tryEmphasis(runes, i, runeStart)
			if ok {
				spans = append(spans, s)
			}
			i += advance
		case c == '[':
			s, advance, ok := tryLink(runes, i, runeStart)
			if ok {
				spans = append(spans, s)
			}
			i += advance
		case c == '`':
			s, advance, ok := tryCode(runes, i, runeStart)
			if ok {
				spans = append(spans, s)
			}
			i += advance
		default:
			i++
		}
	}
	return spans
}

// tryCode attempts to parse an inline-code run starting at
// runes[i] (which must be “ ` “). On match, returns a span
// over the inner text with Family="code", and the number of
// runes consumed (“ `…` “ end-to-end). On no-match (no
// closing backtick or zero-length content), returns ok=false
// and 1 (skip past the opening backtick as literal).
//
// v1: only single-backtick form; double-backtick form (for
// code containing single backticks) is deferred.
func tryCode(runes []rune, i, runeStart int) (Span, int, bool) {
	closeIdx := indexRune(runes, i+1, '`')
	if closeIdx < 0 {
		return Span{}, 1, false
	}
	textLen := closeIdx - (i + 1)
	if textLen <= 0 {
		// Empty content `` `` — skip the pair without emitting
		// (zero-length span is protocol noise).
		return Span{}, closeIdx + 1 - i, false
	}
	return Span{
		Offset: runeStart + i + 1,
		Length: textLen,
		Family: "code",
	}, closeIdx + 1 - i, true
}

// paragraphRange records a paragraph's bounds in the source.
// ByteStart / ByteEnd are byte offsets into src (used by the
// scanner internally); RuneStart is the paragraph's rune
// position in the body (the unit emitted to the spans protocol).
//
// HeadingLevel is 0 for plain paragraphs, 1-6 for ATX headings
// (`# h1` through `###### h6`). Heading paragraphs are exactly
// one source line; scanParagraphs splits a heading line into
// its own paragraph regardless of surrounding blank lines.
//
// IsHRule is true for horizontal-rule lines (`---` / `***` /
// `___` per phase3-r3-hrule.md). Like headings, HRule lines
// are split into their own one-line paragraphs by
// scanParagraphs regardless of surrounding blank lines.
type paragraphRange struct {
	ByteStart, ByteEnd int
	RuneStart          int
	HeadingLevel       int
	IsHRule            bool
	// IsCodeBlock is true for fenced code blocks (` ``` `
	// per phase3-r5-blockcode-regions.md). The body's rune
	// bounds (excluding the opening/closing fence lines)
	// are CodeBodyRuneStart and CodeBodyRuneEnd. CodeLang is
	// the optional language hint from the info string after
	// the opening fence (e.g., "go", "python"); empty if
	// absent. Phase 3 round 5.
	IsCodeBlock                        bool
	CodeBodyRuneStart, CodeBodyRuneEnd int
	CodeLang                           string
	// IsBlockquote is true for `>`-prefixed line groups
	// (Phase 3 round 6). The group's bounds [ByteStart,
	// ByteEnd) cover all consecutive blockquote lines;
	// parseBlockquoteRange strips the `> ` markers and
	// recursively calls Parse on the inner content, so
	// nested blockquotes (`>>` or `> > `), headings inside,
	// fenced code inside, etc. all work via recursion.
	IsBlockquote bool
	// IsListItem is true for a single list line (Phase 3
	// round 7). v1 emits one paragraphRange per list line
	// — runs of consecutive list lines produce a sequence
	// of IsListItem ranges, NOT one range per group. The
	// item's content (after the marker + space) lives at
	// runes [ListContentRuneStart, RuneStart + rune count
	// of [ByteStart, ByteEnd)). ListMarker holds the
	// bullet character ('-', '*', '+') for unordered, or
	// 0 for ordered. ListNumber holds the item's decimal
	// number for ordered, or 0 for unordered.
	//
	// ListDepth is the indent level (1 = column-0 top-
	// level, 2 = one level nested, etc.). Computed from
	// leading whitespace per the in-tree
	// `tabCount + spaceCount/2` formula. Round 7 v1
	// implicitly emitted ListDepth=1; round 7.x carries
	// the depth so nested begin/end region emission can
	// build the right tree.
	IsListItem           bool
	ListMarker           byte
	ListNumber           int
	ListContentRuneStart int
	ListDepth            int
}

// headingScale maps an ATX heading level (1-6) to its font
// scale multiplier. Values for H1-H3 mirror rich/style.go's
// StyleH1/H2/H3; H4-H6 extrapolate gently rather than reverting
// to body size at H4.
var headingScale = [7]float64{
	0: 0,    // plain paragraph (sentinel; not used)
	1: 2.0,  // H1
	2: 1.5,  // H2
	3: 1.25, // H3
	4: 1.1,  // H4
	5: 1.05, // H5
	6: 1.0,  // H6 (visually distinct via bold; same scale as body)
}

// detectHRule returns the rune-length of an HRule line, or 0
// if the line is not a horizontal rule. An HRule line consists
// of 3+ identical marker characters (`-`, `*`, or `_`) at the
// line start, optionally followed by trailing whitespace, with
// no other content.
//
// `start` and `end` are byte offsets bracketing the line (no
// trailing newline). The returned length is the number of
// marker runes (which equals byte count for these ASCII
// markers) — used by parseHRuleParagraph to size the emitted
// span.
func detectHRule(src string, start, end int) int {
	if start >= end {
		return 0
	}
	c := src[start]
	if c != '-' && c != '*' && c != '_' {
		return 0
	}
	// Count the run of identical markers.
	n := 0
	for start+n < end && src[start+n] == c {
		n++
	}
	if n < 3 {
		return 0
	}
	// Anything after the marker run must be whitespace.
	for i := start + n; i < end; i++ {
		switch src[i] {
		case ' ', '\t', '\r':
		default:
			return 0
		}
	}
	// Safe to return n as a rune count: the marker characters
	// (`-`, `*`, `_`) are all 1-byte ASCII, enforced by the
	// equality check on src[start+n] above. If the recognized
	// marker set is ever broadened to include a multi-byte rune,
	// this must convert n from bytes to runes before returning.
	return n
}

// detectHeadingLevel returns the ATX heading level (1-6) for a
// line whose [start, end) range in src spans the line's
// content (no trailing newline). Returns 0 if the line is not
// a heading.
//
// ATX heading rules per md2spans v1 / Phase 3 round 1:
//   - 1-6 leading `#` characters.
//   - Followed by a space character, OR end of line.
//   - No leading whitespace (CommonMark allows up to 3 leading
//     spaces; v1 doesn't bother).
//
// Trailing `#` stripping (CommonMark close-form) is NOT
// implemented in v1: `## title ##` produces a heading with
// content "title ##" rather than "title".
func detectHeadingLevel(src string, start, end int) int {
	n := 0
	for i := start; i < end && i < start+6 && src[i] == '#'; i++ {
		n++
	}
	if n == 0 {
		return 0
	}
	// Must be followed by a space or end-of-line.
	if start+n == end {
		return n
	}
	if src[start+n] == ' ' {
		return n
	}
	return 0
}

// scanParagraphs walks src and returns one paragraphRange per
// paragraph. A plain paragraph is a maximal run of consecutive
// non-blank lines; a blank line is a line whose contents are
// whitespace-only. ATX heading lines (`# title` … `###### `)
// each form their own one-line paragraph regardless of
// surrounding blank lines, with HeadingLevel set 1-6.
//
// Tracking RuneStart in lockstep with the byte cursor lets later
// per-paragraph processing emit rune-indexed spans without
// re-walking the source.
func scanParagraphs(src string) []paragraphRange {
	var out []paragraphRange
	var cur paragraphRange
	inParagraph := false
	runePos := 0
	lineStart := 0
	lineRuneStart := 0

	// inFence: true while we're inside an open fenced code
	// block (between the opening and closing ` ``` ` lines).
	// fenceStartByte / fenceStartRune: the body start (just
	// after the opening fence's newline).
	// fenceLang: language hint from the opening fence's info
	// string.
	// fenceOpenLen: number of backticks in the opening fence;
	// closing fence must have at least this many (per
	// CommonMark — a 4-backtick opener cannot be closed by a
	// 3-backtick body line).
	inFence := false
	fenceStartByte := 0
	fenceStartRune := 0
	fenceLang := ""
	fenceOpenLen := 0

	// inQuote: true while we're inside a blockquote group
	// (consecutive `>` lines). bqStartByte / bqStartRune /
	// bqEndByte: the bounds of the current group.
	// Phase 3 round 6.
	inQuote := false
	bqStartByte := 0
	bqStartRune := 0
	bqEndByte := 0

	commit := func(byteEnd int) {
		if inParagraph {
			cur.ByteEnd = byteEnd
			out = append(out, cur)
			inParagraph = false
		}
	}

	// emitBlockquote closes the current blockquote group
	// (if any) and emits a paragraphRange for it. Phase 3
	// round 6.
	emitBlockquote := func() {
		if !inQuote {
			return
		}
		out = append(out, paragraphRange{
			ByteStart:    bqStartByte,
			ByteEnd:      bqEndByte,
			RuneStart:    bqStartRune,
			IsBlockquote: true,
		})
		inQuote = false
	}

	// emitFencedBlock builds a paragraphRange for the
	// just-closed fenced code block. bodyEndByte is the byte
	// position just before the closing fence's start (or the
	// EOF position if the fence is unclosed). bodyEndRune is
	// the corresponding rune count.
	emitFencedBlock := func(bodyEndByte, bodyEndRune int) {
		out = append(out, paragraphRange{
			ByteStart:         fenceStartByte,
			ByteEnd:           bodyEndByte,
			RuneStart:         fenceStartRune,
			IsCodeBlock:       true,
			CodeBodyRuneStart: fenceStartRune,
			CodeBodyRuneEnd:   bodyEndRune,
			CodeLang:          fenceLang,
		})
		inFence = false
		fenceLang = ""
	}

	flushLine := func(lineEnd int) {
		// Inside an open fence: only check for the closing
		// fence on this line; skip all other paragraph logic
		// (headings, HRule, blank-line etc. don't apply
		// inside code blocks). The closer must have at least
		// as many backticks as the opener.
		if inFence {
			if n := fenceLineLen(src, lineStart, lineEnd); n >= fenceOpenLen {
				emitFencedBlock(lineStart, lineRuneStart)
			}
			return
		}
		// Blockquote line check (Phase 3 round 6): a line
		// starting with `>` (at column 0) joins or starts a
		// blockquote group. Inside a group, all line types
		// are deferred to the recursive Parse call in
		// parseBlockquoteRange — scanParagraphs at this
		// level just bounds the group. A blank line or a
		// non-`>` line ends the group.
		if isBlockquoteLine(src, lineStart, lineEnd) {
			if !inQuote {
				commit(lineStart)
				inQuote = true
				bqStartByte = lineStart
				bqStartRune = lineRuneStart
			}
			bqEndByte = lineEnd
			return
		}
		if inQuote {
			emitBlockquote()
			// Fall through to handle the current line as a
			// regular paragraph / heading / etc.
		}
		// Open a new fenced block when this line is a fence.
		if lang, openLen, ok := parseOpenFence(src, lineStart, lineEnd); ok {
			commit(lineStart)
			inFence = true
			// Body starts just after the opener's terminating
			// `\n`. If the opener is the last line of input
			// with no trailing `\n` (unclosed fence at EOF),
			// the body is empty and starts at lineEnd itself.
			fenceStartByte = lineEnd
			fenceStartRune = lineRuneStart + utf8.RuneCountInString(src[lineStart:lineEnd])
			if lineEnd < len(src) && src[lineEnd] == '\n' {
				fenceStartByte++
				fenceStartRune++
			}
			fenceLang = lang
			fenceOpenLen = openLen
			return
		}
		// Detect blank-line: only whitespace between lineStart..lineEnd.
		blank := true
		for i := lineStart; i < lineEnd; i++ {
			c := src[i]
			if c != ' ' && c != '\t' && c != '\r' {
				blank = false
				break
			}
		}
		if blank {
			commit(lineStart)
			return
		}
		// Heading line ends any prior plain paragraph and is its
		// own one-line paragraph. Trailing newline (if any) is
		// not part of the heading paragraph's bounds.
		if level := detectHeadingLevel(src, lineStart, lineEnd); level > 0 {
			commit(lineStart)
			out = append(out, paragraphRange{
				ByteStart:    lineStart,
				ByteEnd:      lineEnd,
				RuneStart:    lineRuneStart,
				HeadingLevel: level,
			})
			return
		}
		// HRule line: same handling as a heading — own
		// one-line paragraph, ends any prior plain paragraph.
		if detectHRule(src, lineStart, lineEnd) > 0 {
			commit(lineStart)
			out = append(out, paragraphRange{
				ByteStart: lineStart,
				ByteEnd:   lineEnd,
				RuneStart: lineRuneStart,
				IsHRule:   true,
			})
			return
		}
		// List item line: own one-line paragraph (Phase 3
		// round 7 v1). Detection runs AFTER HRule so that
		// `---` parses as HRule, not as a `-` list line.
		if ok, marker, number, contentByte, depth := isListLine(src, lineStart, lineEnd); ok {
			commit(lineStart)
			contentRune := lineRuneStart + utf8.RuneCountInString(src[lineStart:contentByte])
			out = append(out, paragraphRange{
				ByteStart:            lineStart,
				ByteEnd:              lineEnd,
				RuneStart:            lineRuneStart,
				IsListItem:           true,
				ListMarker:           marker,
				ListNumber:           number,
				ListContentRuneStart: contentRune,
				ListDepth:            depth,
			})
			return
		}
		if !inParagraph {
			cur = paragraphRange{
				ByteStart: lineStart,
				RuneStart: lineRuneStart,
			}
			inParagraph = true
		}
	}

	for i := 0; i < len(src); {
		r, size := utf8.DecodeRuneInString(src[i:])
		if r == '\n' {
			flushLine(i)
			i += size
			runePos++
			lineStart = i
			lineRuneStart = runePos
			continue
		}
		i += size
		runePos++
	}
	// Final line: no trailing newline.
	if lineStart < len(src) {
		flushLine(len(src))
		commit(len(src))
	} else {
		commit(lineStart)
	}
	// Unclosed fence: emit the block covering everything
	// from fence start to EOF (matches CommonMark behavior:
	// no closing fence runs to end-of-document).
	if inFence {
		emitFencedBlock(len(src), runePos)
	}
	// Open blockquote at EOF: close it.
	if inQuote {
		emitBlockquote()
	}
	return out
}

// isBlockquoteLine reports whether [start, end) is a
// blockquote-marker line — `>` at column 0 (no leading
// whitespace; v1 of round 6 doesn't accept the
// CommonMark-allowed 1-3 leading spaces). Phase 3 round 6.
func isBlockquoteLine(src string, start, end int) bool {
	if start >= end {
		return false
	}
	return src[start] == '>'
}

// isListLine reports whether [start, end) opens a list
// item, detecting the indent depth from leading
// whitespace before the marker. Phase 3 round 7 v1
// emitted at column 0 only; round 7.x adds nesting via
// 2-space-or-tab-per-level indent.
//
// Returns:
//   - ok: true if the line is a list line.
//   - marker: '-', '*', or '+' for unordered; 0 for ordered.
//   - number: item number for ordered; 0 for unordered.
//   - contentByte: byte position right after the marker
//     and required space — the start of the item's content
//     (skipping any leading whitespace).
//   - depth: indent level, 1 for column-0 (top-level), 2
//     for one nesting level (2 spaces or 1 tab), etc.
//     Computed as `1 + tabCount + spaceCount/2` (matches
//     the in-tree markdown/parse.go semantics; odd
//     trailing spaces round down).
//
// Detection rules:
//   - Optional leading whitespace (spaces and/or tabs).
//   - Unordered: marker `-` / `*` / `+` followed by SPACE.
//     The space is required so emphasis (`*x*`), plain
//     lines starting with `-foo`, and HRule (`---`) do
//     NOT match.
//   - Ordered: digits followed by `.` or `)` followed by
//     SPACE.
//
// Callers must check HRule precedence BEFORE calling
// this — `---` is HRule, not a list line, even though
// detectHRule's marker char overlaps with the bullet.
func isListLine(src string, start, end int) (ok bool, marker byte, number, contentByte, depth int) {
	if start >= end {
		return false, 0, 0, 0, 0
	}
	// Count leading whitespace and compute depth.
	spaceCount := 0
	tabCount := 0
	i := start
	for i < end {
		switch src[i] {
		case ' ':
			spaceCount++
			i++
		case '\t':
			tabCount++
			i++
		default:
			goto afterIndent
		}
	}
afterIndent:
	if i >= end {
		// Whitespace-only line is not a list.
		return false, 0, 0, 0, 0
	}
	depth = 1 + tabCount + spaceCount/2
	c := src[i]
	// Unordered: marker + SPACE.
	if c == '-' || c == '*' || c == '+' {
		if i+1 < end && src[i+1] == ' ' {
			return true, c, 0, i + 2, depth
		}
		return false, 0, 0, 0, 0
	}
	// Ordered: digits + ('.' | ')') + SPACE.
	if c < '0' || c > '9' {
		return false, 0, 0, 0, 0
	}
	digitStart := i
	for i < end && src[i] >= '0' && src[i] <= '9' {
		i++
	}
	// Must have at least one digit (already true) and a
	// trailing `.` or `)` followed by space.
	if i >= end {
		return false, 0, 0, 0, 0
	}
	if src[i] != '.' && src[i] != ')' {
		return false, 0, 0, 0, 0
	}
	if i+1 >= end || src[i+1] != ' ' {
		return false, 0, 0, 0, 0
	}
	// Parse the digit run. We've already validated each
	// rune is 0-9; convert to int defensively. Walk from
	// digitStart (where the digits actually began, after
	// any leading whitespace).
	n := 0
	for j := digitStart; j < i; j++ {
		n = n*10 + int(src[j]-'0')
	}
	return true, 0, n, i + 2, depth
}

// fenceLineLen reports the number of leading backticks on
// the line [start, end) IF the line is a closing-fence
// candidate (3+ backticks followed only by whitespace);
// returns 0 otherwise. The caller compares against the
// opener's count to decide whether the line actually closes
// the open fence (per CommonMark: closer must have ≥ as
// many backticks as opener). Phase 3 round 5.
func fenceLineLen(src string, start, end int) int {
	n := 0
	for start+n < end && src[start+n] == '`' {
		n++
	}
	if n < 3 {
		return 0
	}
	// Anything after must be whitespace.
	for i := start + n; i < end; i++ {
		switch src[i] {
		case ' ', '\t', '\r':
		default:
			return 0
		}
	}
	return n
}

// parseOpenFence reports whether [start, end) is an opening
// fenced code-block delimiter and extracts the language
// hint from the info string after the backticks. Returns
// (lang, openLen, true) on a fence; ("", 0, false)
// otherwise. The caller stashes openLen for use against the
// closing fence.
//
// CommonMark allows a fence's info string to be any text
// after the backticks. v1 takes the first
// whitespace-delimited token as the language hint and
// drops the rest.
func parseOpenFence(src string, start, end int) (string, int, bool) {
	n := 0
	for start+n < end && src[start+n] == '`' {
		n++
	}
	if n < 3 {
		return "", 0, false
	}
	// Skip leading whitespace before the info string.
	i := start + n
	for i < end && (src[i] == ' ' || src[i] == '\t') {
		i++
	}
	// Read up to the next whitespace as the language hint.
	langStart := i
	for i < end && src[i] != ' ' && src[i] != '\t' && src[i] != '\r' {
		i++
	}
	return src[langStart:i], n, true
}

// linkBlue is the v1 foreground color for inline-link text.
// Hard-coded per md2spans.design.md § R5.
const linkBlue = "#0000cc"

// parseParagraph turns one paragraph's source bytes into a list
// of styled spans. Handles emphasis (R4) and inline links (R5).
//
// Emphasis matcher: greedy and non-CommonMark-compliant. The
// matcher pairs delimiter runs by adjacency requiring the
// CLOSING run to have exactly the same delimiter character and
// the same count as the opener. `*x*` (count 1) → italic;
// `**x**` (count 2) → bold; `***x***` (count 3) → bold+italic.
// Runs of count > 3 are not recognized as emphasis. CommonMark's
// flanking-rune rules are not applied; `5*x*` is treated as
// emphasis on "x" the same as ` *x* `. Documented divergence
// (md2spans.design.md § R4).
//
// Link matcher: `[text](url)` emits a single span over "text"
// with `Fg = linkBlue`. The URL is dropped. Reference / autolink
// forms are not recognized (R5). Emphasis inside link text is
// not currently honored — `[**bold**](u)` styles only the link
// color, not the bold. Documented divergence.
func parseParagraph(src string, p paragraphRange) []Span {
	runes := []rune(src[p.ByteStart:p.ByteEnd])
	return parseInlineSpans(runes, p.RuneStart)
}

// tryEmphasis attempts to parse an emphasis run starting at
// runes[i] (which must be `*` or `_`). On match, returns the
// styled Span, the number of runes consumed (opener + content +
// closer), and ok=true. On no-match, returns ok=false and the
// number of runes to skip past as literal (typically the opener
// run length).
//
// runeStart is the body-relative rune offset of runes[0], so
// emitted Span offsets are body-absolute.
func tryEmphasis(runes []rune, i, runeStart int) (Span, int, bool) {
	c := runes[i]
	n := delimRunLen(runes, i, c)
	if n > 3 {
		return Span{}, n, false
	}
	closerIdx := findEmphasisCloser(runes, i+n, c, n)
	if closerIdx < 0 {
		return Span{}, n, false
	}
	return Span{
		Offset: runeStart + i + n,
		Length: closerIdx - (i + n),
		Bold:   n == 2 || n == 3,
		Italic: n == 1 || n == 3,
	}, closerIdx + n - i, true
}

// tryImage attempts to parse a CommonMark inline image
// `![alt](url)` (or `![alt](url "title")`) starting at
// runes[i] (which must be `!`). On match, returns a single
// IsBox=true span with BoxPlacement="below" and a payload
// of `image:URL` (plus optional ` width=N` if the title
// attribute contains `width=Npx`). The span's Length is the
// rune count of the entire `![alt](url ...)` source, and the
// span is anchored at the start of the syntax — the renderer
// renders those source runes as text in the normal way AND
// paints the image below the line on which they sit, so the
// user sees the markers AND the image.
//
// On no-match (no `[` after `!`, no `]`, no `(URL)`),
// returns ok=false and 1 (skip past the `!` as literal).
//
// runeStart is the body-relative rune offset of runes[0].
// Phase 3 round 4.
func tryImage(runes []rune, i, runeStart int) (Span, int, bool) {
	if i+1 >= len(runes) || runes[i+1] != '[' {
		return Span{}, 1, false
	}
	parts, ok := findInlineImage(runes, i)
	if !ok {
		return Span{}, 1, false
	}
	url := string(runes[parts.urlStart:parts.urlEnd])
	payload := "image:" + url
	if parts.titleStart > 0 && parts.titleEnd > 0 {
		title := string(runes[parts.titleStart:parts.titleEnd])
		if w := parseImageWidthPx(title); w > 0 {
			payload = fmt.Sprintf("%s width=%d", payload, w)
		}
	}
	advance := parts.closeParen + 1 - i
	return Span{
		Kind:   SpanBox,
		Offset: runeStart + i,
		// Length covers the source `![alt](url ...)` runes.
		// The renderer renders these source markers as text
		// (consistent with markup-stays-visible from rounds
		// 1-3) and ALSO paints the image below the line via
		// the placement=below contract.
		Length:       advance,
		IsBox:        true,
		BoxPayload:   payload,
		BoxPlacement: "below",
	}, advance, true
}

// imageParts captures the rune-index bounds of an
// `![alt](url)` (or with title) match. urlStart is the rune
// just after `(`; urlEnd is the rune at which the URL stops
// (either closeParen when there's no title, or the space
// preceding `"...` when there is). titleStart/titleEnd
// bracket the title's content (excluding quotes); both are 0
// when no title is present. closeParen is the closing `)`.
type imageParts struct {
	closeBracket int
	closeParen   int
	urlStart     int
	urlEnd       int
	titleStart   int
	titleEnd     int
}

// findInlineImage looks for the `![alt](url)` or
// `![alt](url "title")` pattern starting at runes[start]
// (which must be `!`). On match returns ok=true and the
// rune bounds in imageParts. On no-match (no `[`, no `]`,
// no `(URL)`, no closing `)`, malformed title quoting)
// returns ok=false.
func findInlineImage(runes []rune, start int) (imageParts, bool) {
	var p imageParts
	if start+1 >= len(runes) || runes[start] != '!' || runes[start+1] != '[' {
		return p, false
	}
	p.closeBracket = indexRune(runes, start+2, ']')
	if p.closeBracket < 0 {
		return p, false
	}
	if p.closeBracket+1 >= len(runes) || runes[p.closeBracket+1] != '(' {
		return p, false
	}
	p.urlStart = p.closeBracket + 2
	cp := indexRune(runes, p.urlStart, ')')
	if cp < 0 {
		return p, false
	}
	p.closeParen = cp
	p.urlEnd = cp // URL extends to closeParen unless a title is found
	// Optional title: scan for ` "` opener within [urlStart, closeParen).
	for j := p.urlStart; j < p.closeParen-1; j++ {
		if runes[j] == ' ' && runes[j+1] == '"' {
			tStart := j + 2
			tEnd := indexRune(runes, tStart, '"')
			if tEnd > 0 && tEnd < p.closeParen {
				p.urlEnd = j // URL ends at the space before `"`
				p.titleStart = tStart
				p.titleEnd = tEnd
				// Closing `)` must be after the closing `"`.
				cp2 := indexRune(runes, tEnd+1, ')')
				if cp2 < 0 {
					return imageParts{}, false
				}
				p.closeParen = cp2
				break
			}
		}
	}
	return p, true
}

// parseImageWidthPx parses a `width=Npx` substring out of a
// title attribute and returns N, or 0 if not present /
// malformed. Mirrors markdown/parse.go:parseImageWidth so
// md2spans honors the same authoring convention.
func parseImageWidthPx(title string) int {
	const prefix = "width="
	const suffix = "px"
	idx := strings.Index(title, prefix)
	if idx < 0 {
		return 0
	}
	rest := title[idx+len(prefix):]
	pxIdx := strings.Index(rest, suffix)
	if pxIdx <= 0 {
		return 0
	}
	numStr := rest[:pxIdx]
	n := 0
	for _, c := range numStr {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// tryLink attempts to parse an inline link `[text](url)`
// starting at runes[i] (which must be `[`). On match, returns
// the colored Span over the link text and the number of runes
// consumed (the entire `[text](url)`). On no-match (malformed
// shape, zero-length text), returns ok=false and 1 (skip past
// the `[` as literal).
//
// runeStart is the body-relative rune offset of runes[0].
func tryLink(runes []rune, i, runeStart int) (Span, int, bool) {
	closeBracket, closeParen, ok := findInlineLink(runes, i)
	if !ok {
		return Span{}, 1, false
	}
	textLen := closeBracket - (i + 1)
	if textLen <= 0 {
		// Zero-length link text — skip the whole `[](u)` form
		// without emitting (a 0-length span would be
		// protocol-noise per R5).
		return Span{}, closeParen + 1 - i, false
	}
	return Span{
		Offset: runeStart + i + 1,
		Length: textLen,
		Fg:     linkBlue,
	}, closeParen + 1 - i, true
}

// findInlineLink looks for the [text](url) pattern starting at
// runes[start] (which must be '['). Returns the rune indices of
// the closing ']' and the closing ')', plus ok=true on a match.
// Returns ok=false for any malformed shape (no ']', '(' missing,
// '(' not adjacent, no closing ')').
func findInlineLink(runes []rune, start int) (closeBracket, closeParen int, ok bool) {
	if start >= len(runes) || runes[start] != '[' {
		return 0, 0, false
	}
	closeBracket = indexRune(runes, start+1, ']')
	if closeBracket < 0 {
		return 0, 0, false
	}
	if closeBracket+1 >= len(runes) || runes[closeBracket+1] != '(' {
		return 0, 0, false
	}
	closeParen = indexRune(runes, closeBracket+2, ')')
	if closeParen < 0 {
		return 0, 0, false
	}
	return closeBracket, closeParen, true
}

// indexRune returns the rune index >= start of the next
// occurrence of c, or -1 if not found.
func indexRune(runes []rune, start int, c rune) int {
	for i := start; i < len(runes); i++ {
		if runes[i] == c {
			return i
		}
	}
	return -1
}

// delimRunLen returns the length of the maximal run of character
// c starting at runes[start], capped at 4 (callers handle counts
// of 1, 2, 3 specially; >3 is "beyond v1").
func delimRunLen(runes []rune, start int, c rune) int {
	n := 0
	for start+n < len(runes) && runes[start+n] == c && n < 4 {
		n++
	}
	return n
}

// findEmphasisCloser returns the rune index in runes of the next
// run of exactly count copies of c, starting search at index
// `start`. Returns -1 if no such run exists or if a run of c
// with a different count is encountered first.
//
// Block-rule: if the matcher encounters a run of c with a
// different count, it returns -1 immediately rather than
// skipping past. This treats the asymmetric run as a hard
// boundary that the opener cannot cross — matching what most
// users intuit from `*a**b*c*`-style inputs (where the inner
// `**` should not let the outer `*` quietly consume the closer
// `*` after `b`). The earlier "skip and continue" rule produced
// confusing partial matches; tests now pin the new behavior.
//
// This is still a divergence from CommonMark (which has full
// flanking-rune rules), but it's a more honest "v1 doesn't
// support nested emphasis at all" rather than "v1 sometimes
// matches across nested runs in surprising ways."
func findEmphasisCloser(runes []rune, start int, c rune, count int) int {
	for i := start; i < len(runes); i++ {
		if runes[i] != c {
			continue
		}
		n := delimRunLen(runes, i, c)
		if n == count {
			return i
		}
		// Different-count run of the same delimiter character
		// blocks the match. Return -1 (caller falls through to
		// literal text). Skipping past the run, as earlier
		// versions did, produced surprising matches like
		// italic("a**b") for `*a**b*c*`.
		return -1
	}
	return -1
}
