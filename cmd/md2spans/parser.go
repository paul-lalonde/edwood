package main

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Span is a styled rune range in the body. Offset and Length are
// in runes (matching the spans-protocol convention used by
// cmd/edcolor and consumed by spanparse.go in the main package).
type Span struct {
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
		case p.IsCodeBlock:
			spans = append(spans, parseCodeBlockParagraph(src, p)...)
		case p.IsHRule:
			spans = append(spans, parseHRuleParagraph(src, p)...)
		case p.HeadingLevel > 0:
			spans = append(spans, parseHeadingParagraph(src, p)...)
		default:
			spans = append(spans, parseParagraph(src, p)...)
		}
	}
	return spans
}

// parseCodeBlockParagraph emits the spans for a fenced code
// block: a RegionBegin sentinel at the body's start, a
// styled span over the body runes (family=code so the
// renderer uses the monospace font even before the region
// machinery is consulted), and a RegionEnd sentinel at the
// body's end. The opening / closing fence runes are not
// covered by any span — they render as default-styled text
// via emit-time gap-fill, consistent with markup-stays-
// visible. Phase 3 round 5.
func parseCodeBlockParagraph(src string, p paragraphRange) []Span {
	begin := Span{
		Offset:      p.CodeBodyRuneStart,
		Length:      0,
		RegionBegin: "code",
	}
	if p.CodeLang != "" {
		begin.RegionParams = map[string]string{"lang": p.CodeLang}
	}
	end := Span{
		Offset:    p.CodeBodyRuneEnd,
		Length:    0,
		RegionEnd: true,
	}
	bodyLen := p.CodeBodyRuneEnd - p.CodeBodyRuneStart
	if bodyLen == 0 {
		// Empty body: emit just the begin/end pair.
		return []Span{begin, end}
	}
	body := Span{
		Offset: p.CodeBodyRuneStart,
		Length: bodyLen,
		Family: "code",
	}
	return []Span{begin, body, end}
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
// runes[i] (which must be `` ` ``). On match, returns a span
// over the inner text with Family="code", and the number of
// runes consumed (`` `…` `` end-to-end). On no-match (no
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
	IsCodeBlock      bool
	CodeBodyRuneStart, CodeBodyRuneEnd int
	CodeLang         string
}

// headingScale maps an ATX heading level (1-6) to its font
// scale multiplier. Values for H1-H3 mirror rich/style.go's
// StyleH1/H2/H3; H4-H6 extrapolate gently rather than reverting
// to body size at H4.
var headingScale = [7]float64{
	0:   0,    // plain paragraph (sentinel; not used)
	1:   2.0,  // H1
	2:   1.5,  // H2
	3:   1.25, // H3
	4:   1.1,  // H4
	5:   1.05, // H5
	6:   1.0,  // H6 (visually distinct via bold; same scale as body)
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
	inFence := false
	fenceStartByte := 0
	fenceStartRune := 0
	fenceLang := ""

	commit := func(byteEnd int) {
		if inParagraph {
			cur.ByteEnd = byteEnd
			out = append(out, cur)
			inParagraph = false
		}
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
		// inside code blocks).
		if inFence {
			if isFenceLine(src, lineStart, lineEnd) {
				emitFencedBlock(lineStart, lineRuneStart)
			}
			return
		}
		// Open a new fenced block when this line is a fence.
		if lang, ok := parseOpenFence(src, lineStart, lineEnd); ok {
			commit(lineStart)
			inFence = true
			fenceStartByte = lineEnd + 1 // skip the \n
			// Body's rune-start = next line's rune-start. We
			// don't know yet exactly which rune that is until
			// the outer loop crosses the \n. Compute as
			// (current line's rune-start) + (rune count from
			// lineStart to lineEnd) + 1 (the \n).
			fenceStartRune = lineRuneStart + utf8.RuneCountInString(src[lineStart:lineEnd]) + 1
			fenceLang = lang
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
	return out
}

// isFenceLine reports whether [start, end) is a fenced
// code-block delimiter line: 3+ consecutive backticks
// optionally followed by whitespace. Used for closing
// fences (opening fences also satisfy this; parseOpenFence
// is the higher-level wrapper that also extracts the info
// string). Phase 3 round 5.
func isFenceLine(src string, start, end int) bool {
	n := 0
	for start+n < end && src[start+n] == '`' {
		n++
	}
	if n < 3 {
		return false
	}
	// Anything after must be whitespace.
	for i := start + n; i < end; i++ {
		switch src[i] {
		case ' ', '\t', '\r':
		default:
			return false
		}
	}
	return true
}

// parseOpenFence reports whether [start, end) is an opening
// fenced code-block delimiter and extracts the language
// hint from the info string after the backticks. Returns
// (lang, true) on a fence; ("", false) otherwise.
//
// CommonMark allows a fence's info string to be any text
// after the backticks. v1 takes the first
// whitespace-delimited token as the language hint and
// drops the rest.
func parseOpenFence(src string, start, end int) (string, bool) {
	n := 0
	for start+n < end && src[start+n] == '`' {
		n++
	}
	if n < 3 {
		return "", false
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
	return src[langStart:i], true
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
