package main

import "unicode/utf8"

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
}

// Parse takes the markdown source and returns the list of styled
// runs that should be emitted to the spans file. Plain text
// produces no spans (default styling suffices); styled runs come
// from emphasis (`*x*`, `**x**`, ...) and inline links
// (`[text](url)`).
//
// At Phase 2.2 (paragraph parsing scaffold) Parse returns an
// empty slice for any input. The paragraph-walk machinery is in
// place for rows 2.3 (emphasis) and 2.4 (links) to plug into.
//
// Offsets and lengths are in runes (R7 of md2spans.design.md).
func Parse(src string) []Span {
	var spans []Span
	for _, p := range scanParagraphs(src) {
		spans = append(spans, parseParagraph(src, p)...)
	}
	return spans
}

// paragraphRange records a paragraph's bounds in the source.
// ByteStart / ByteEnd are byte offsets into src (used by the
// scanner internally); RuneStart is the paragraph's rune
// position in the body (the unit emitted to the spans protocol).
type paragraphRange struct {
	ByteStart, ByteEnd int
	RuneStart          int
}

// scanParagraphs walks src and returns one paragraphRange per
// paragraph (a maximal run of consecutive non-blank lines). A
// blank line is a line whose contents are whitespace-only.
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

	commit := func(byteEnd int) {
		if inParagraph {
			cur.ByteEnd = byteEnd
			out = append(out, cur)
			inParagraph = false
		}
	}

	flushLine := func(lineEnd int) {
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
	return out
}

// LinkBlue is the v1 foreground color for inline-link text.
// Hard-coded per md2spans.design.md § R5.
const LinkBlue = "#0000cc"

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
// with `Fg = LinkBlue`. The URL is dropped. Reference / autolink
// forms are not recognized (R5). Emphasis inside link text is
// not currently honored — `[**bold**](u)` styles only the link
// color, not the bold. Documented divergence.
func parseParagraph(src string, p paragraphRange) []Span {
	runes := []rune(src[p.ByteStart:p.ByteEnd])
	var spans []Span
	i := 0
	for i < len(runes) {
		c := runes[i]
		switch {
		case c == '*' || c == '_':
			n := delimRunLen(runes, i, c)
			if n > 3 {
				// Beyond v1's recognized counts; advance as literal.
				i += n
				continue
			}
			closerIdx := findEmphasisCloser(runes, i+n, c, n)
			if closerIdx < 0 {
				// No matching closer — leave opener as literal.
				i += n
				continue
			}
			spans = append(spans, Span{
				Offset: p.RuneStart + i + n,
				Length: closerIdx - (i + n),
				Bold:   n == 2 || n == 3,
				Italic: n == 1 || n == 3,
			})
			i = closerIdx + n
		case c == '[':
			closeBracket, closeParen, ok := findInlineLink(runes, i)
			if !ok {
				i++
				continue
			}
			textLen := closeBracket - (i + 1)
			if textLen > 0 {
				// Skip zero-length link text; emitting a 0-length
				// span is protocol-noise (R5 final paragraph).
				spans = append(spans, Span{
					Offset: p.RuneStart + i + 1,
					Length: textLen,
					Fg:     LinkBlue,
				})
			}
			i = closeParen + 1
		default:
			i++
		}
	}
	return spans
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
// `start`. Returns -1 if no such run exists in the remainder.
//
// Skip-rule: if a run of c is encountered with a different
// count, advance past it without matching. This is what makes
// the matcher "equal-count required". Documented divergence
// from CommonMark.
func findEmphasisCloser(runes []rune, start int, c rune, count int) int {
	i := start
	for i < len(runes) {
		if runes[i] != c {
			i++
			continue
		}
		n := delimRunLen(runes, i, c)
		if n == count {
			return i
		}
		i += n
	}
	return -1
}
