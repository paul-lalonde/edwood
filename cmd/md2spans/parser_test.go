package main

import "testing"

// TestParseEmpty: empty input produces no spans (R3).
func TestParseEmpty(t *testing.T) {
	if got := Parse(""); len(got) != 0 {
		t.Errorf("Parse(\"\") = %v, want empty", got)
	}
}

// TestParseSinglePlainParagraph: paragraph of plain text, no
// emphasis or links, produces no spans. v1 inherits default
// styling for unstyled runs (R3).
func TestParseSinglePlainParagraph(t *testing.T) {
	src := "Hello, world. This is plain text."
	if got := Parse(src); len(got) != 0 {
		t.Errorf("Parse(plain) = %v, want empty", got)
	}
}

// TestParseMultipleParagraphs: paragraphs separated by blank
// lines. v1 emits no spans for plain paragraphs.
func TestParseMultipleParagraphs(t *testing.T) {
	src := "First paragraph.\n\nSecond paragraph.\n\nThird."
	if got := Parse(src); len(got) != 0 {
		t.Errorf("Parse(multi-paragraph) = %v, want empty", got)
	}
}

// TestParseTrailingWhitespace: leading / trailing blank lines
// shouldn't cause crashes or unexpected spans. R3.
func TestParseTrailingWhitespace(t *testing.T) {
	cases := []string{
		"\n\nHello.\n\n",
		"\nHello.\n",
		"Hello.\n",
		"Hello.",
		"\n\n\n",
		"   \n\n   \n",
	}
	for _, src := range cases {
		if got := Parse(src); len(got) != 0 {
			t.Errorf("Parse(%q) = %v, want empty", src, got)
		}
	}
}

// TestParseUTF8SafePlainText: parser must not crash on multi-byte
// UTF-8 plain text. R7 (rune offsets): unit of measure is runes,
// not bytes.
func TestParseUTF8SafePlainText(t *testing.T) {
	cases := []string{
		"日本語のテキスト",
		"emoji: 🎉🚀",
		"mixed: hello 世界 world",
	}
	for _, src := range cases {
		if got := Parse(src); len(got) != 0 {
			t.Errorf("Parse(%q) = %v, want empty (no styled runs in plain text)", src, got)
		}
	}
}

// --- Emphasis tests (R4) -------------------------------------------------

// TestParseItalicAsterisk covers R4: *text* emits an italic span
// at rune offsets that EXCLUDE the markers.
func TestParseItalicAsterisk(t *testing.T) {
	spans := Parse("*hello*")
	want := []Span{{Offset: 1, Length: 5, Italic: true}}
	assertSpansEqual(t, spans, want)
}

// TestParseItalicUnderscore: _text_ also italic.
func TestParseItalicUnderscore(t *testing.T) {
	spans := Parse("_hi_")
	want := []Span{{Offset: 1, Length: 2, Italic: true}}
	assertSpansEqual(t, spans, want)
}

// TestParseBoldAsterisk: **text** emits bold.
func TestParseBoldAsterisk(t *testing.T) {
	spans := Parse("**bold**")
	want := []Span{{Offset: 2, Length: 4, Bold: true}}
	assertSpansEqual(t, spans, want)
}

// TestParseBoldUnderscore: __text__ also bold.
func TestParseBoldUnderscore(t *testing.T) {
	spans := Parse("__strong__")
	want := []Span{{Offset: 2, Length: 6, Bold: true}}
	assertSpansEqual(t, spans, want)
}

// TestParseBoldItalicAsterisk: ***text*** emits bold + italic.
func TestParseBoldItalicAsterisk(t *testing.T) {
	spans := Parse("***both***")
	want := []Span{{Offset: 3, Length: 4, Bold: true, Italic: true}}
	assertSpansEqual(t, spans, want)
}

// TestParseEmphasisInSentence: emphasis embedded in normal text;
// rune offsets correctly skip the surrounding plain text.
func TestParseEmphasisInSentence(t *testing.T) {
	src := "Hello *world* today."
	// Runes: H=0 e=1 l=2 l=3 o=4 ' '=5 *=6 w=7 o=8 r=9 l=10 d=11 *=12 ...
	// Italic span covers "world" → offset 7, length 5.
	want := []Span{{Offset: 7, Length: 5, Italic: true}}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseMultipleEmphasis: two emphasis groups in one
// paragraph yield two spans, each with the correct offset.
func TestParseMultipleEmphasis(t *testing.T) {
	src := "*a* and **bcd**"
	// Runes: *=0 a=1 *=2 ' '=3 a=4 n=5 d=6 ' '=7 *=8 *=9 b=10 c=11 d=12 *=13 *=14
	// Italic "a" at offset 1, len 1.
	// Bold "bcd" at offset 10, len 3.
	want := []Span{
		{Offset: 1, Length: 1, Italic: true},
		{Offset: 10, Length: 3, Bold: true},
	}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseUnclosedEmphasisFallsThrough: an opener with no
// matching closer in the paragraph emits no span (R4 final
// paragraph). The marker character is left literal.
func TestParseUnclosedEmphasisFallsThrough(t *testing.T) {
	for _, src := range []string{
		"*hello world",
		"**bold without close",
		"_italic without close",
	} {
		if got := Parse(src); len(got) != 0 {
			t.Errorf("Parse(%q) = %v, want empty (unclosed → literal)", src, got)
		}
	}
}

// TestParseAsymmetricEmphasisBlocks pins the post-review
// behavior: a different-count delimiter run encountered while
// searching for a closer BLOCKS the match, returning literal
// text rather than producing a surprising partial match.
//
// Earlier behavior (skip-past-different-count) produced
// italic("a**b") for `*a**b*c*` — confusing. New behavior:
// the inner `**` blocks the outer `*` from finding a closer,
// so the outer markers fall through as literal text. The
// inner `**b**` is then matched on its own pass.
func TestParseAsymmetricEmphasisBlocks(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []Span
	}{
		{
			name: "outer single blocked by inner double",
			src:  "*a**b**c*",
			// First pass: opener `*` at 0 looks for `*`. Hits
			// `**` (count 2) at 2 → blocks. No span.
			// Then position 1 is `a`, advance.
			// Position 2: `**` opener (count 2). Looks for `**`.
			// Finds `**` at 5 → match. Bold "b". offset 4 length 1.
			// Position 7: `c`, advance.
			// Position 8: `*` opener. No closer remaining. No span.
			want: []Span{{Offset: 4, Length: 1, Bold: true}},
		},
		{
			name: "single delimiter blocked by triple",
			src:  "*a***b**",
			// `*` at 0 looks for `*`. Hits `***` at 2 (count 3) → blocks.
			// `***` at 2 (count 3) looks for `***`. None remaining → no span.
			// `**` at 6 (count 2) looks for `**` → none → no span.
			want: nil,
		},
		{
			name: "double blocked by single",
			src:  "**a*b**",
			// `**` at 0 (count 2) looks for `**`. First sees `*` at 3 (count 1) → blocks.
			// `*` at 3 (count 1) looks for `*`. None remaining (the `**` is count 2) → no span.
			want: nil,
		},
		{
			name: "mixed delimiter chars don't match",
			src:  "*x_",
			// `*` opener cannot pair with `_`. No closer → no span.
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertSpansEqual(t, Parse(tc.src), tc.want)
		})
	}
}

// TestParseDelimiterCountAboveThree: runs of 4+ delimiters are
// not recognized as emphasis (delimRunLen caps at 4). v1 treats
// them as literal text.
func TestParseDelimiterCountAboveThree(t *testing.T) {
	for _, src := range []string{
		"****x****",
		"____x____",
	} {
		if got := Parse(src); len(got) != 0 {
			t.Errorf("Parse(%q) = %v, want empty (4-run not recognized)", src, got)
		}
	}
}

// TestParseEmphasisDoesNotSpanParagraphs: emphasis is intra-
// paragraph (R4); openers in one paragraph don't pair with
// closers in another.
func TestParseEmphasisDoesNotSpanParagraphs(t *testing.T) {
	src := "*opener\n\ncloser*"
	if got := Parse(src); len(got) != 0 {
		t.Errorf("Parse(%q) = %v, want empty (emphasis cannot span paragraphs)", src, got)
	}
}

// --- Horizontal rule tests (Phase 3 round 3) ---------------------------

// TestParseHRuleDash covers basic `---` recognition: a span over
// the marker runes with HRule=true, no other styling.
func TestParseHRuleDash(t *testing.T) {
	src := "---"
	want := []Span{{Offset: 0, Length: 3, HRule: true}}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseHRuleAsterisk and Underscore: the other two ATX
// rule markers also work.
func TestParseHRuleAsterisk(t *testing.T) {
	want := []Span{{Offset: 0, Length: 3, HRule: true}}
	assertSpansEqual(t, Parse("***"), want)
}

func TestParseHRuleUnderscore(t *testing.T) {
	want := []Span{{Offset: 0, Length: 3, HRule: true}}
	assertSpansEqual(t, Parse("___"), want)
}

// TestParseHRuleLongerRun: 4+ markers also count as a rule.
func TestParseHRuleLongerRun(t *testing.T) {
	src := "-----"
	want := []Span{{Offset: 0, Length: 5, HRule: true}}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseHRuleTrailingWhitespace: trailing whitespace allowed.
func TestParseHRuleTrailingWhitespace(t *testing.T) {
	src := "---   "
	// The Span covers only the marker runes (not the trailing
	// spaces). HRule=true; mdrender draws the rule across the
	// whole frame width regardless of span length.
	want := []Span{{Offset: 0, Length: 3, HRule: true}}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseHRuleCRLF: `\r` is allowed as trailing whitespace.
// Pins the CRLF-input case so a Windows-edited markdown file
// still recognizes its rule lines. The `\r` is the line
// terminator's first byte; scanParagraphs strips the `\n` and
// detectHRule's whitespace check accepts the trailing `\r`.
func TestParseHRuleCRLF(t *testing.T) {
	src := "---\r\nafter"
	// Paragraph rune offsets:
	//   --- = 0..2 (3)   \r = 3   \n = 4
	//   after = 5..9
	// HRule span over the marker runes only.
	want := []Span{{Offset: 0, Length: 3, HRule: true}}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseHRuleNotAList: `- item` is a list (later round),
// NOT an HRule. v1 leaves it as plain text — emphasis tokenizer
// ignores `-`. No spans.
func TestParseHRuleNotAList(t *testing.T) {
	if got := Parse("- item"); len(got) != 0 {
		t.Errorf("Parse(\"- item\") = %v, want empty (list, not HRule)", got)
	}
}

// TestParseHRuleNotMixedMarkers: `--*` (mixed) is not a rule.
func TestParseHRuleNotMixedMarkers(t *testing.T) {
	// `--*` has 2 dashes then a star. The HRule detector
	// requires 3+ same character. So no rule. The trailing `*`
	// is a single-rune emphasis opener with nothing to close →
	// no emphasis span either. Result: empty.
	if got := Parse("--*"); len(got) != 0 {
		t.Errorf("Parse(\"--*\") = %v, want empty", got)
	}
}

// TestParseHRuleNotShort: `--` (2 markers) is not an HRule.
func TestParseHRuleNotShort(t *testing.T) {
	if got := Parse("--"); len(got) != 0 {
		t.Errorf("Parse(\"--\") = %v, want empty", got)
	}
}

// TestParseHRuleNotWithContent: `--- title` is not an HRule
// per v1 (markers followed by content). v1 leaves as plain.
func TestParseHRuleNotWithContent(t *testing.T) {
	if got := Parse("--- title"); len(got) != 0 {
		t.Errorf("Parse(\"--- title\") = %v, want empty", got)
	}
}

// TestParseHRuleBetweenParagraphs: an HRule line ends the prior
// paragraph and is its own one-line paragraph. Subsequent
// non-blank lines start a new paragraph.
func TestParseHRuleBetweenParagraphs(t *testing.T) {
	src := "intro\n\n---\n\nafter"
	// Runes:
	//   intro=0..4 (5)  \n=5
	//   blank=6 (\n)
	//   ---=7..9 (3)    \n=10
	//   blank=11 (\n)
	//   after=12..16 (5)
	// HRule span at offset 7, length 3. No other spans.
	want := []Span{{Offset: 7, Length: 3, HRule: true}}
	assertSpansEqual(t, Parse(src), want)
}

// --- Fenced code block tests (Phase 3 round 5) -------------------------

// TestParseFencedCodeBasic: a simple fenced block produces
// a RegionBegin sentinel + a styled span over the body +
// a RegionEnd sentinel. The opening/closing fence runes
// stay visible in the body and render via emit-time
// gap-fill (default styling).
func TestParseFencedCodeBasic(t *testing.T) {
	src := "```\nfoo\n```"
	// Runes: ` ` ` \n f o o \n ` ` `
	//        0 1 2 3 4 5 6 7 8 9 10 (11 total)
	// Opening fence: 0..3 (then \n at 3). Body: 4..8
	// ("foo\n" — body includes the trailing \n before the
	// closing fence, matching CommonMark). Closing fence:
	// 8..10. Body span has family=code so the renderer's
	// monospace font applies even before the region machinery
	// is consulted.
	got := Parse(src)
	if len(got) != 3 {
		t.Fatalf("got %d spans, want 3 (begin + body + end); spans: %+v", len(got), got)
	}
	if got[0].RegionBegin != "code" {
		t.Errorf("got[0].RegionBegin = %q, want %q", got[0].RegionBegin, "code")
	}
	if got[0].Offset != 4 || got[0].Length != 0 {
		t.Errorf("got[0] offset/length = (%d, %d), want (4, 0)", got[0].Offset, got[0].Length)
	}
	if got[1].Offset != 4 || got[1].Length != 4 {
		t.Errorf("body span offset/length = (%d, %d), want (4, 4)", got[1].Offset, got[1].Length)
	}
	if got[1].Family != "code" {
		t.Errorf("body span Family = %q, want %q", got[1].Family, "code")
	}
	if !got[2].RegionEnd {
		t.Error("got[2].RegionEnd should be true")
	}
	if got[2].Offset != 8 || got[2].Length != 0 {
		t.Errorf("got[2] offset/length = (%d, %d), want (8, 0)", got[2].Offset, got[2].Length)
	}
}

// TestParseFencedCodeWithLang: an info string after the
// opening fence becomes a `lang=NAME` param on the begin
// region directive.
func TestParseFencedCodeWithLang(t *testing.T) {
	src := "```go\nfmt\n```"
	got := Parse(src)
	if len(got) != 3 {
		t.Fatalf("got %d spans, want 3; spans: %+v", len(got), got)
	}
	if got[0].RegionBegin != "code" {
		t.Errorf("got[0].RegionBegin = %q, want %q", got[0].RegionBegin, "code")
	}
	if got[0].RegionParams["lang"] != "go" {
		t.Errorf("got[0].RegionParams[lang] = %q, want %q", got[0].RegionParams["lang"], "go")
	}
}

// TestParseFencedCodeMultilineBody: body spans multiple
// lines; the body span covers from after the opening
// fence's newline to before the closing fence.
func TestParseFencedCodeMultilineBody(t *testing.T) {
	src := "```\nline1\nline2\nline3\n```"
	// Opening fence: 0..3, \n at 3. Body: 4..21 ("line1\nline2\nline3\n"
	// is 18 runes — but we want body up to BUT NOT INCLUDING the
	// closing fence's start. Closing fence starts at... let me count:
	// `(0) `(1) `(2) \n(3) l(4) i(5) n(6) e(7) 1(8) \n(9)
	// l(10) i(11) n(12) e(13) 2(14) \n(15) l(16) i(17) n(18) e(19) 3(20) \n(21)
	// `(22) `(23) `(24)
	// So body is 4..22 (18 runes including trailing \n before closing).
	got := Parse(src)
	if len(got) != 3 {
		t.Fatalf("got %d spans, want 3; spans: %+v", len(got), got)
	}
	if got[1].Offset != 4 || got[1].Length != 18 {
		t.Errorf("body span offset/length = (%d, %d), want (4, 18)", got[1].Offset, got[1].Length)
	}
	if got[2].Offset != 22 {
		t.Errorf("end region offset = %d, want 22", got[2].Offset)
	}
}

// TestParseFencedCodeEmpty: ` ``` ` immediately followed by
// closing ` ``` ` — body has zero rune content. Emit just
// the begin/end pair with no body span.
func TestParseFencedCodeEmpty(t *testing.T) {
	src := "```\n```"
	got := Parse(src)
	if len(got) != 2 {
		t.Fatalf("got %d spans, want 2 (begin + end, no body); spans: %+v", len(got), got)
	}
	if got[0].RegionBegin != "code" {
		t.Errorf("got[0].RegionBegin = %q, want %q", got[0].RegionBegin, "code")
	}
	if !got[1].RegionEnd {
		t.Error("got[1].RegionEnd should be true")
	}
	if got[0].Offset != got[1].Offset {
		t.Errorf("empty body: begin offset (%d) != end offset (%d)", got[0].Offset, got[1].Offset)
	}
}

// TestParseFencedCodeBetweenParagraphs: a fenced block
// embedded in a document with surrounding plain text. The
// surrounding paragraphs produce no spans (default styling
// via emit-time gap-fill).
func TestParseFencedCodeBetweenParagraphs(t *testing.T) {
	src := "intro\n\n```\nbody\n```\n\nafter"
	got := Parse(src)
	// Intro and after produce no spans. The fenced block
	// produces 3 spans (begin + body + end).
	if len(got) != 3 {
		t.Fatalf("got %d spans, want 3 (just the fenced block); spans: %+v", len(got), got)
	}
}

// TestParseFencedCodeCloserMustMatchOpenerCount: the
// closing fence must have at least as many backticks as
// the opening fence (CommonMark rule). A 4-backtick opener
// with 3-backtick lines in the body must NOT close on those
// 3-backtick lines — they're part of the body.
func TestParseFencedCodeCloserMustMatchOpenerCount(t *testing.T) {
	src := "````\nthree backticks below should not close:\n```\nstill body\n````"
	got := Parse(src)
	if len(got) != 3 {
		t.Fatalf("got %d spans, want 3 (begin + body + end); spans: %+v", len(got), got)
	}
	if got[0].RegionBegin != "code" {
		t.Errorf("got[0].RegionBegin = %q, want %q", got[0].RegionBegin, "code")
	}
	// Body should INCLUDE the 3-backtick line in the middle.
	// Source: ````\nthree backticks below should not close:\n```\nstill body\n````
	// Runes: ````=0..3 \n=4 ...
	// The body must span past the inner ``` line.
	body := got[1]
	if body.Length < 50 {
		t.Errorf("body length = %d, want >= 50 (body should include the inner ``` line)", body.Length)
	}
}

// TestParseFencedCodeUnclosed: an opening fence with no
// matching close — treat the rest of the document as the
// code body (matches CommonMark behavior: no closing fence
// → block runs to EOF).
func TestParseFencedCodeUnclosed(t *testing.T) {
	src := "```\ndangling"
	got := Parse(src)
	if len(got) < 2 {
		t.Fatalf("got %d spans, want at least 2 (begin + body); spans: %+v", len(got), got)
	}
	if got[0].RegionBegin != "code" {
		t.Errorf("got[0].RegionBegin = %q, want %q", got[0].RegionBegin, "code")
	}
	// Final span should be the end region — even unclosed,
	// the parser closes the region at EOF.
	last := got[len(got)-1]
	if !last.RegionEnd {
		t.Error("unclosed fenced block: last span should be RegionEnd at EOF")
	}
}

// --- Blockquote tests (Phase 3 round 6) --------------------------------

// TestParseBlockquoteSingleLine: a single `> a` line emits
// the begin/end region pair around the body content.
func TestParseBlockquoteSingleLine(t *testing.T) {
	src := "> a quote"
	// Runes: > ' ' a ' ' q u o t e
	//        0  1  2  3  4 5 6 7 8  (9 total)
	got := Parse(src)
	if len(got) != 2 {
		t.Fatalf("got %d spans, want 2 (begin + end); spans: %+v", len(got), got)
	}
	if got[0].RegionBegin != "blockquote" {
		t.Errorf("got[0].RegionBegin = %q, want %q", got[0].RegionBegin, "blockquote")
	}
	if got[0].Offset != 0 {
		t.Errorf("got[0].Offset = %d, want 0 (group start)", got[0].Offset)
	}
	if !got[1].RegionEnd {
		t.Error("got[1].RegionEnd should be true")
	}
	if got[1].Offset != 9 {
		t.Errorf("got[1].Offset = %d, want 9 (group end)", got[1].Offset)
	}
}

// TestParseBlockquoteMultiLine: multiple consecutive `>`
// lines form one blockquote group.
func TestParseBlockquoteMultiLine(t *testing.T) {
	src := "> line one\n> line two"
	got := Parse(src)
	if len(got) != 2 {
		t.Fatalf("got %d spans, want 2 (begin + end); spans: %+v", len(got), got)
	}
	if got[0].RegionBegin != "blockquote" {
		t.Errorf("got[0].RegionBegin = %q, want %q", got[0].RegionBegin, "blockquote")
	}
}

// TestParseBlockquoteNested: `>> ` produces a nested
// blockquote (recursive — strip outer `>`, recursive Parse
// sees inner `>` as another blockquote).
func TestParseBlockquoteNested(t *testing.T) {
	src := "> outer\n>> inner\n> outer again"
	got := Parse(src)
	// Expect: outer begin, inner begin (around inner line),
	// inner end, outer end. 4 sentinels minimum.
	beginCount := 0
	endCount := 0
	for _, s := range got {
		if s.RegionBegin == "blockquote" {
			beginCount++
		}
		if s.RegionEnd {
			endCount++
		}
	}
	if beginCount != 2 || endCount != 2 {
		t.Errorf("nested: got %d begins / %d ends, want 2/2; spans: %+v", beginCount, endCount, got)
	}
}

// TestParseBlockquoteContainingHeading: `> # heading` —
// the inside of the blockquote contains a heading. The
// recursive parser should emit the heading's scaled spans
// inside the blockquote region.
func TestParseBlockquoteContainingHeading(t *testing.T) {
	src := "> # title"
	got := Parse(src)
	// Expect: begin blockquote, heading span(s), end.
	beginFound := false
	endFound := false
	headingFound := false
	for _, s := range got {
		if s.RegionBegin == "blockquote" {
			beginFound = true
		}
		if s.RegionEnd {
			endFound = true
		}
		if s.Scale > 1.0 {
			headingFound = true
		}
	}
	if !beginFound || !endFound {
		t.Errorf("missing begin/end region; spans: %+v", got)
	}
	if !headingFound {
		t.Errorf("expected a heading-scaled span inside the blockquote; spans: %+v", got)
	}
}

// TestParseBlockquoteContainingFencedCode: `> ```\n> body\n> ``` `
// — blockquote contains a fenced code block. Two nested
// regions, both kinds.
func TestParseBlockquoteContainingFencedCode(t *testing.T) {
	src := "> ```\n> body\n> ```"
	got := Parse(src)
	bqBegin := 0
	codeBegin := 0
	for _, s := range got {
		switch s.RegionBegin {
		case "blockquote":
			bqBegin++
		case "code":
			codeBegin++
		}
	}
	if bqBegin != 1 {
		t.Errorf("got %d blockquote begins, want 1; spans: %+v", bqBegin, got)
	}
	if codeBegin != 1 {
		t.Errorf("got %d code begins, want 1; spans: %+v", codeBegin, got)
	}
}

// TestParseBlockquoteNestedInnerBeginAtLineStart: bug fix
// regression. For source `>Quoted\n>>Double Quoted`, the
// inner blockquote begin must anchor at the START OF
// LINE 2 (rune 8 = the first `>`), not at the position of
// the second `>` (rune 9). Otherwise line 2's first box
// (the first `>` rune) would be inside the outer
// blockquote ONLY, with BlockquoteDepth=1, and the
// layout's first-box-determines-indent rule would produce
// a 20px indent for line 2 instead of the expected 40px.
//
// Per-line indent depends on the first box's depth
// (rich/layout.go:614 — `if box.Style.Blockquote
// indentPixels += BlockquoteDepth * ListIndentWidth`); the
// inner blockquote must claim the line from rune 0 of the
// line so the line's first box is at depth=2.
func TestParseBlockquoteNestedInnerBeginAtLineStart(t *testing.T) {
	src := ">Quoted\n>>Double Quoted"
	got := Parse(src)
	begins := []int{}
	for _, s := range got {
		if s.RegionBegin == "blockquote" {
			begins = append(begins, s.Offset)
		}
	}
	if len(begins) != 2 {
		t.Fatalf("expected 2 blockquote begins, got %d at %v; spans: %+v", len(begins), begins, got)
	}
	// begins[0] = outer at offset 0.
	// begins[1] = inner — must be 8 (line 2 start, before
	// the outer marker on line 2). If it's 9, the inner
	// begins AFTER the outer's marker, leaving line 2's
	// first box at depth=1 and producing the wrong per-line
	// indent.
	if begins[0] != 0 {
		t.Errorf("outer begin offset = %d, want 0", begins[0])
	}
	if begins[1] != 8 {
		t.Errorf("inner begin offset = %d, want 8 (line 2 start)", begins[1])
	}
}

// TestParseCodeBlockBeginNotSnapped pins the negative
// invariant for the round-6.5 line-anchor registry: a
// `code` region's begin offset is NOT snapped to the
// source line's start, because the body of a fenced code
// block begins AFTER the opening fence's `\n`, not at
// the fence line's column 0. Snapping would put the begin
// directive on the fence line (visually anchored to the
// “ ``` “ markup) and the body span would render with
// the wrong starting position.
//
// Source:
//
//	> ```           // outer blockquote line 1; fence opener
//	> body
//	> ```           // closer
//
// After the outer-blockquote strip, the inner content has
// a fence at line start. The recursive Parse emits a
// `code` region begin at the body-start rune (after the
// opener line's `\n`), which IS already a line-start in
// the inner stripped source. The blockquote-snap logic
// must NOT also snap the code begin (no-op there), but
// more importantly must not snap a code begin that is mid-
// line in the future. Phase 3 round 6.5.
func TestParseCodeBlockBeginNotSnapped(t *testing.T) {
	src := "> ```\n> body\n> ```"
	got := Parse(src)
	var codeBeginOffset int
	var foundCodeBegin bool
	for _, s := range got {
		if s.RegionBegin == "code" {
			foundCodeBegin = true
			codeBeginOffset = s.Offset
		}
	}
	if !foundCodeBegin {
		t.Fatalf("no code region begin emitted; spans: %+v", got)
	}
	// The code body begins after the opener line's `\n` —
	// in the original source that is rune 6 (after "> ```\n").
	// The blockquote-snap path would snap it to rune 0 (line
	// 1 start), which would be wrong.
	if codeBeginOffset == 0 {
		t.Errorf("code begin offset = 0; the line-start snap accidentally fired on the code kind")
	}
}

// TestParseBlockquoteUnclosedFenceInside reproduces a
// smoke-test crash. While typing, a user can produce a
// state where a nested blockquote contains an UNCLOSED
// fenced code block:
//
//	>Quoted
//	>>Double Quoted
//	>> 'inline code'
//	>> Some text
//	>> ```
//
// The recursive Parse should not panic — it should treat
// the unclosed fence as code-to-EOF inside the inner
// blockquote.
func TestParseBlockquoteUnclosedFenceInside(t *testing.T) {
	src := ">Quoted\n>>Double Quoted\n>> 'inline code'\n>> Some text\n>> ```"
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Parse panicked: %v", r)
		}
	}()
	_ = Parse(src)
}

// TestParseBlockquoteCodeBlockEndsBeforeCloserMarkers
// pins the rune-at-vs-boundary-before bug: a fenced code
// block inside a nested blockquote had its END region
// directive mapped via per-rune lookup, which after the
// `>>` strip on the closer line landed AT the first
// backtick of the closer instead of AT the closer's line
// start. The closer's `>>` markers then appeared INSIDE
// the code region, which the renderer treated as
// extra-indented code-block content (visible as a wide
// gap between `>>` and `` ``` `` on the closer line).
//
// The fix maps RegionEnd offsets via boundary-before
// semantics (mapping[N-1]+1 for an exclusive end), which
// gives the closer line's start in the original source.
//
// Pinning: the `code` end region must land at the start
// of the closer line in the original source — i.e., at
// the offset of the `>` that opens the closer line, not
// the first backtick.
func TestParseBlockquoteCodeBlockEndsBeforeCloserMarkers(t *testing.T) {
	src := ">>```go\n>>testing\n>> ```"
	got := Parse(src)
	// In the original source, the closer line begins at
	// rune 18 (`>` of ">> ```"). Counts:
	//   0..7   ">>```go\n"
	//   8..17  ">>testing\n"
	//   18..23 ">> ```"
	var codeEnd int = -1
	for _, s := range got {
		if s.RegionEnd && codeEnd == -1 {
			// The first RegionEnd we see is the inner code
			// region's end (followed by the inner blockquote
			// end, then the outer's). All inner ends emit
			// before outer's appended end — order is begin
			// outer, begin inner, [code begin/body/end],
			// end inner, end outer.
			codeEnd = s.Offset
		}
	}
	if codeEnd != 18 {
		t.Errorf("code region end offset = %d, want 18 (closer line start in original)", codeEnd)
	}
}

// TestParseBlockquoteEndsAtBlankLine: a blank line ends
// the blockquote group; subsequent content is outside.
func TestParseBlockquoteEndsAtBlankLine(t *testing.T) {
	src := "> a\n\nplain text"
	got := Parse(src)
	beginCount := 0
	endCount := 0
	for _, s := range got {
		if s.RegionBegin == "blockquote" {
			beginCount++
		}
		if s.RegionEnd {
			endCount++
		}
	}
	if beginCount != 1 || endCount != 1 {
		t.Errorf("got %d/%d begins/ends, want 1/1; spans: %+v", beginCount, endCount, got)
	}
}

// TestParseBlockquoteMidLineGreaterIgnored: `>` mid-line is
// not a blockquote marker (markers must be at column 0).
func TestParseBlockquoteMidLineGreaterIgnored(t *testing.T) {
	src := "x > y"
	got := Parse(src)
	for _, s := range got {
		if s.RegionBegin == "blockquote" {
			t.Errorf("mid-line `>` should not produce a blockquote region; spans: %+v", got)
		}
	}
}

// --- Inline code tests (Phase 3 round 2) -------------------------------

// TestParseInlineCode covers basic backtick-delimited inline
// code: `text` produces one span with Family="code" over the
// inner text. The backtick markup runes remain in the body.
func TestParseInlineCode(t *testing.T) {
	src := "`hello`"
	// Runes: `=0 h=1 e=2 l=3 l=4 o=5 `=6
	// Code span: offset 1, length 5.
	want := []Span{{Offset: 1, Length: 5, Family: "code"}}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseInlineCodeInSentence: code in plain text gets the
// right offsets relative to the body.
func TestParseInlineCodeInSentence(t *testing.T) {
	src := "Run `make build` first."
	// Runes: R=0 u=1 n=2 ' '=3 `=4 m=5 a=6 k=7 e=8 ' '=9 b=10 u=11 i=12 l=13 d=14 `=15 ...
	want := []Span{{Offset: 5, Length: 10, Family: "code"}}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseInlineCodeAdjacentToEmphasis: backticks coexist with
// emphasis markers in the same paragraph; each produces its
// own span.
func TestParseInlineCodeAdjacentToEmphasis(t *testing.T) {
	src := "*pre* `mid` *post*"
	// Runes:
	//   *=0 p=1 r=2 e=3 *=4 ' '=5 `=6 m=7 i=8 d=9 `=10 ' '=11 *=12 p=13 o=14 s=15 t=16 *=17
	want := []Span{
		{Offset: 1, Length: 3, Italic: true},
		{Offset: 7, Length: 3, Family: "code"},
		{Offset: 13, Length: 4, Italic: true},
	}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseUnclosedInlineCodeFallsThrough: no closing backtick
// → no span; the opening backtick remains literal.
func TestParseUnclosedInlineCodeFallsThrough(t *testing.T) {
	if got := Parse("`unclosed code"); len(got) != 0 {
		t.Errorf("Parse(unclosed) = %v, want empty", got)
	}
}

// TestParseInlineCodeInsideHeading: code inside a heading
// carries BOTH Family="code" AND the heading's Scale (the
// Parse-time merge for inline runs in heading context).
func TestParseInlineCodeInsideHeading(t *testing.T) {
	src := "## Use `make` here"
	// Runes:
	//   #=0 #=1 ' '=2 U=3 s=4 e=5 ' '=6 `=7 m=8 a=9 k=10 e=11 `=12 ' '=13 h=14 e=15 r=16 e=17
	// Heading content: offset 3..18 (length 15). Scale=1.5.
	// Code span: offset 8, length 4 ("make"), Family="code", Scale=1.5.
	want := []Span{
		{Offset: 3, Length: 5, Scale: 1.5},                 // "Use `"
		{Offset: 8, Length: 4, Family: "code", Scale: 1.5}, // "make"
		{Offset: 12, Length: 6, Scale: 1.5},                // "` here"
	}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseEmptyInlineCode: ` ` / “ is empty; produces no
// span (zero-length code is protocol noise, same rule as
// link with empty text).
func TestParseEmptyInlineCode(t *testing.T) {
	if got := Parse("``"); len(got) != 0 {
		t.Errorf("Parse(``) = %v, want empty (zero-length code)", got)
	}
}

// --- Heading tests (Phase 3 round 1) -----------------------------------

// TestParseATXHeadingH1 covers basic H1 detection: `# title`
// produces one scaled span over the heading text. The `# `
// markup runes remain in the body (not part of the span).
func TestParseATXHeadingH1(t *testing.T) {
	src := "# Hello"
	// Runes: #=0 ' '=1 H=2 e=3 l=4 l=5 o=6
	// Heading span: offset 2, length 5, Scale=2.0.
	want := []Span{{Offset: 2, Length: 5, Scale: 2.0}}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseATXHeadingLevels: H1-H6 produce the documented
// scale values (mirrors rich/style.go's StyleH1/H2/H3 and
// extrapolates for H4-H6).
func TestParseATXHeadingLevels(t *testing.T) {
	cases := []struct {
		src   string
		scale float64
	}{
		{"# H1", 2.0},
		{"## H2", 1.5},
		{"### H3", 1.25},
		{"#### H4", 1.1},
		{"##### H5", 1.05},
		{"###### H6", 1.0},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			spans := Parse(tc.src)
			if len(spans) != 1 {
				t.Fatalf("got %d spans, want 1: %+v", len(spans), spans)
			}
			if spans[0].Scale != tc.scale {
				t.Errorf("Scale = %v, want %v", spans[0].Scale, tc.scale)
			}
		})
	}
}

// TestParseHeadingNeedsSpaceAfterHash: `#abc` is NOT a heading
// (no space after #). v1 treats it as plain text.
func TestParseHeadingNeedsSpaceAfterHash(t *testing.T) {
	if got := Parse("#nospace"); len(got) != 0 {
		t.Errorf("Parse(%q) = %v, want empty (no heading without space)", "#nospace", got)
	}
}

// TestParseHeadingTooManyHashes: 7+ `#`s is not a heading
// (CommonMark caps at 6); v1 treats as plain text.
func TestParseHeadingTooManyHashes(t *testing.T) {
	if got := Parse("####### too many"); len(got) != 0 {
		t.Errorf("Parse(7-hash line) = %v, want empty", got)
	}
}

// TestParseHeadingMidParagraph: `#` mid-line isn't a heading.
// Heading detection is line-anchored.
func TestParseHeadingMidParagraph(t *testing.T) {
	if got := Parse("foo # bar"); len(got) != 0 {
		t.Errorf("Parse(mid-line #) = %v, want empty", got)
	}
}

// TestParseHeadingBreaksPriorParagraph: a heading line ends the
// prior paragraph even without a blank line between. After the
// heading, a non-blank line is its own new paragraph.
func TestParseHeadingBreaksPriorParagraph(t *testing.T) {
	src := "intro\n# Title\nafter"
	// Runes: i=0 n=1 t=2 r=3 o=4 \n=5 #=6 ' '=7 T=8 i=9 t=10 l=11 e=12 \n=13 a=14 f=15 t=16 e=17 r=18
	// Heading span: offset 8, length 5 ("Title"), Scale=2.0.
	want := []Span{{Offset: 8, Length: 5, Scale: 2.0}}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseEmphasisInsideHeading: emphasis runs inside a
// heading carry both their flag and the heading's Scale (the
// Parse-time merge decision from the design doc).
func TestParseEmphasisInsideHeading(t *testing.T) {
	src := "## *important* title"
	// Runes: #=0 #=1 ' '=2 *=3 i=4 m=5 p=6 o=7 r=8 t=9 a=10 n=11 t=12 *=13 ' '=14 t=15 i=16 t=17 l=18 e=19
	// Heading content: offset 3..20 (length 17). Scale=1.5 over the whole.
	// Emphasis: italic over "important" at offset 4 length 9.
	// Per the merge decision: the heading-scaled regions
	// outside the emphasis emit ONE span with Scale only;
	// the emphasis emits with both Scale AND italic.
	want := []Span{
		{Offset: 3, Length: 1, Scale: 1.5},               // "*"
		{Offset: 4, Length: 9, Scale: 1.5, Italic: true}, // "important"
		{Offset: 13, Length: 7, Scale: 1.5},              // "* title"
	}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseEmphasisAcrossParagraphs: spans in separate
// paragraphs preserve correct body-relative offsets (rune
// counts include the blank-line newlines between paragraphs).
func TestParseEmphasisAcrossParagraphs(t *testing.T) {
	src := "*a*\n\n**bc**"
	// Paragraph 1: *=0 a=1 *=2  → italic "a" at offset 1.
	// Paragraph 2 starts after "*a*\n\n" = 5 runes; *=5 *=6 b=7 c=8 *=9 *=10
	//   → bold "bc" at offset 7, len 2.
	want := []Span{
		{Offset: 1, Length: 1, Italic: true},
		{Offset: 7, Length: 2, Bold: true},
	}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseEmphasisUTF8: italic over multi-byte runes uses rune
// counts, not byte counts (R7).
func TestParseEmphasisUTF8(t *testing.T) {
	// "*世界*" : *=0 世=1 界=2 *=3
	// Italic span covers rune offsets [1, 3), length 2.
	want := []Span{{Offset: 1, Length: 2, Italic: true}}
	assertSpansEqual(t, Parse("*世界*"), want)
}

// --- Link tests (R5) ----------------------------------------------------

// TestParseLinkBasic covers R5: [text](url) emits a Fg-colored
// span over "text"; the URL is dropped.
func TestParseLinkBasic(t *testing.T) {
	src := "[link](https://example.com)"
	// Runes: [=0 l=1 i=2 n=3 k=4 ]=5 (=6 ...
	want := []Span{{Offset: 1, Length: 4, Fg: linkBlue}}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseLinkInSentence: link embedded in plain text; offsets
// are correct within the body.
func TestParseLinkInSentence(t *testing.T) {
	src := "Visit [our site](https://example.com) today"
	// Runes: V=0 i=1 s=2 i=3 t=4 ' '=5 [=6 o=7 u=8 r=9 ' '=10 s=11 i=12 t=13 e=14 ]=15 ...
	// Link text "our site" at offset 7, length 8.
	want := []Span{{Offset: 7, Length: 8, Fg: linkBlue}}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseLinkAdjacentToEmphasis: a link next to emphasis in
// the same paragraph yields two distinct spans.
func TestParseLinkAdjacentToEmphasis(t *testing.T) {
	src := "*pre* [mid](u) *post*"
	// Runes: *=0 p=1 r=2 e=3 *=4 ' '=5 [=6 m=7 i=8 d=9 ]=10 (=11 u=12 )=13 ' '=14 *=15 p=16 o=17 s=18 t=19 *=20
	// Italic "pre" → offset 1 len 3.
	// Link "mid" → offset 7 len 3, Fg=blue.
	// Italic "post" → offset 16 len 4.
	want := []Span{
		{Offset: 1, Length: 3, Italic: true},
		{Offset: 7, Length: 3, Fg: linkBlue},
		{Offset: 16, Length: 4, Italic: true},
	}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseMalformedLinksFallThrough: links missing required
// pieces emit no spans (R5: malformed cases are literal text).
// Each case has a documented reason for the no-span outcome.
func TestParseMalformedLinksFallThrough(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"unclosed bracket", "[unclosed"},
		{"bracket then non-paren", "[text] no paren"},
		{"open paren no close", "[text](no close"},
		{"orphan close bracket", "]orphan close"},
		{"empty link text and url", "[](u)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Parse(tc.src); len(got) != 0 {
				t.Errorf("Parse(%q) = %v, want empty (malformed link → literal)", tc.src, got)
			}
		})
	}
}

// TestParseLinkTextEmpty: `[](url)` is a degenerate but valid
// inline link with no link text. v1 emits an empty-length span
// at the bracket position; or, equivalently, it emits no span
// (zero-length spans are protocol-noise). v1's choice: skip.
func TestParseLinkTextEmpty(t *testing.T) {
	src := "[](u)"
	if got := Parse(src); len(got) != 0 {
		t.Errorf("Parse(%q) = %v, want empty (zero-length link text)", src, got)
	}
}

// TestParseLinkUTF8: link over multi-byte runes uses rune
// counts (R7).
func TestParseLinkUTF8(t *testing.T) {
	// "[世界](u)": [=0 世=1 界=2 ]=3 (=4 u=5 )=6
	want := []Span{{Offset: 1, Length: 2, Fg: linkBlue}}
	assertSpansEqual(t, Parse("[世界](u)"), want)
}

// assertSpansEqual fails the test if got != want.
func assertSpansEqual(t *testing.T, got, want []Span) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("got %d spans, want %d\n  got:  %+v\n  want: %+v", len(got), len(want), got, want)
		return
	}
	for i := range got {
		if !spansFieldEqual(got[i], want[i]) {
			t.Errorf("span[%d]:\n  got:  %+v\n  want: %+v", i, got[i], want[i])
		}
	}
}

// spansFieldEqual compares two Spans field-by-field. Plain
// `==` doesn't work because Span contains a RegionParams
// map (Phase 3 round 5).
func spansFieldEqual(a, b Span) bool {
	if a.Kind != b.Kind {
		return false
	}
	if a.Offset != b.Offset || a.Length != b.Length {
		return false
	}
	if a.Fg != b.Fg || a.Bold != b.Bold || a.Italic != b.Italic {
		return false
	}
	if a.Scale != b.Scale || a.Family != b.Family || a.HRule != b.HRule {
		return false
	}
	if a.IsBox != b.IsBox || a.BoxWidth != b.BoxWidth || a.BoxHeight != b.BoxHeight {
		return false
	}
	if a.BoxPayload != b.BoxPayload || a.BoxPlacement != b.BoxPlacement {
		return false
	}
	if a.RegionBegin != b.RegionBegin || a.RegionEnd != b.RegionEnd {
		return false
	}
	if len(a.RegionParams) != len(b.RegionParams) {
		return false
	}
	for k, v := range a.RegionParams {
		if w, ok := b.RegionParams[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// --- Image syntax tests (Phase 3 round 4) ------------------------------

// TestParseImageBasic covers the `![alt](url)` syntax: emits a
// single box record (IsBox=true, BoxPlacement="below",
// BoxPayload="image:URL") covering the source runes
// [offset, offset+length). The renderer renders those source
// markers as text in the normal way AND paints the image
// below the line; emit-time gap-fill is not involved (the
// box's covered runes ARE the source text).
func TestParseImageBasic(t *testing.T) {
	src := "![alt](pic.png)"
	// Runes: !=0 [=1 a=2 l=3 t=4 ]=5 (=6 p=7 i=8 c=9 .=10 p=11 n=12 g=13 )=14
	want := []Span{
		{
			Offset:       0,
			Length:       15,
			Kind:         SpanBox,
			IsBox:        true,
			BoxPayload:   "image:pic.png",
			BoxPlacement: "below",
		},
	}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseImageWithTitleNoWidth: title attr without
// width=Npx → payload is just `image:URL` (no width param).
func TestParseImageWithTitleNoWidth(t *testing.T) {
	src := `![alt](p.png "no width here")`
	// 29 runes total (![alt](p.png "no width here"))
	want := []Span{
		{
			Offset:       0,
			Length:       29,
			Kind:         SpanBox,
			IsBox:        true,
			BoxPayload:   "image:p.png",
			BoxPlacement: "below",
		},
	}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseImageWithWidth: title attr with `width=Npx` flows
// into the box's payload as `width=N` (px suffix dropped).
func TestParseImageWithWidth(t *testing.T) {
	src := `![alt](p.png "width=200px")`
	// 27 runes (![alt](p.png "width=200px"))
	want := []Span{
		{
			Offset:       0,
			Length:       27,
			Kind:         SpanBox,
			IsBox:        true,
			BoxPayload:   "image:p.png width=200",
			BoxPlacement: "below",
		},
	}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseImageEmptyAlt: `![](url)` is valid — alt is
// optional in CommonMark. The box is still emitted.
func TestParseImageEmptyAlt(t *testing.T) {
	src := "![](pic.png)"
	// 12 runes (![](pic.png))
	want := []Span{
		{
			Offset:       0,
			Length:       12,
			Kind:         SpanBox,
			IsBox:        true,
			BoxPayload:   "image:pic.png",
			BoxPlacement: "below",
		},
	}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseImageMidParagraph: image syntax mid-paragraph
// anchors at its start position; covers the source runes.
func TestParseImageMidParagraph(t *testing.T) {
	src := "see ![cat](c.png) here"
	// Runes: s=0 e=1 e=2 ' '=3 ![cat](c.png)=4..16 (13 runes) ' '=17 here=18..21
	want := []Span{
		{
			Offset:       4,
			Length:       13,
			Kind:         SpanBox,
			IsBox:        true,
			BoxPayload:   "image:c.png",
			BoxPlacement: "below",
		},
	}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseImageMultiplePerParagraph: two images in one
// paragraph emit two box records, anchored at their
// respective start positions; each covers its own source
// runes.
func TestParseImageMultiplePerParagraph(t *testing.T) {
	src := "![a](x.png) and ![b](y.png)"
	// First image: !=0 ... )=10 (11 runes)
	// Second image at position 16: !=16 ... )=26 (11 runes)
	want := []Span{
		{
			Offset:       0,
			Length:       11,
			Kind:         SpanBox,
			IsBox:        true,
			BoxPayload:   "image:x.png",
			BoxPlacement: "below",
		},
		{
			Offset:       16,
			Length:       11,
			Kind:         SpanBox,
			IsBox:        true,
			BoxPayload:   "image:y.png",
			BoxPlacement: "below",
		},
	}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseImageAdjacentToLink: an image followed by a
// link produces two distinct spans (one box, one link).
// The image takes precedence over the link tokenizer
// (image discriminator is `!`).
func TestParseImageAdjacentToLink(t *testing.T) {
	src := "![a](x.png) [b](y)"
	// Image: !=0 ...)=10 (11 runes). Link: ' '=11, [=12, b=13, ]=14, (=15, y=16, )=17.
	// Link text "b" → offset 13 length 1.
	want := []Span{
		{
			Offset:       0,
			Length:       11,
			Kind:         SpanBox,
			IsBox:        true,
			BoxPayload:   "image:x.png",
			BoxPlacement: "below",
		},
		{Offset: 13, Length: 1, Fg: linkBlue},
	}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseImageMalformed: malformed image syntax falls
// through as literal text, no span emitted. Same fallback
// as the in-tree path's recognizer.
func TestParseImageMalformed(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"unclosed bracket", "![alt"},
		{"bracket no paren", "![alt] no paren"},
		{"open paren no close", "![alt](no close"},
		{"bang only", "!alone"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Parse(tc.src); len(got) != 0 {
				t.Errorf("Parse(%q) = %v, want empty (malformed image → literal)", tc.src, got)
			}
		})
	}
}

// TestParseImageURLWithTitleSeparator: a URL followed by
// a `"title"` is correctly separated; the URL excludes the
// space-quote separator and the title runs through the
// width=Npx parser.
func TestParseImageURLWithTitleSeparator(t *testing.T) {
	src := `![alt](path/to/file.png "width=100px")`
	// 38 runes total
	want := []Span{
		{
			Offset:       0,
			Length:       38,
			Kind:         SpanBox,
			IsBox:        true,
			BoxPayload:   "image:path/to/file.png width=100",
			BoxPlacement: "below",
		},
	}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseImageURLWithEmbeddedSpace: a URL containing a
// raw space (no title attr) is preserved verbatim through
// to the closing `)`. CommonMark technically requires URL
// escaping for spaces, but v1 doesn't enforce that — the
// payload tokenizer would mis-parse a space-containing URL
// (it splits on whitespace), so this test pins the v1
// behavior: parsed but consumer-side tokenization will see
// `image:pa` as the URL and `th.png` as a "param". Worth
// pinning so the limitation is documented executable.
func TestParseImageURLWithEmbeddedSpace(t *testing.T) {
	src := "![alt](pa th.png)"
	// 17 runes
	want := []Span{
		{
			Offset:       0,
			Length:       17,
			Kind:         SpanBox,
			IsBox:        true,
			BoxPayload:   "image:pa th.png",
			BoxPlacement: "below",
		},
	}
	assertSpansEqual(t, Parse(src), want)
}

// TestSpanKindStyledFromInline pins that an emphasis span
// (the typical styled producer) carries Kind == SpanStyled.
// Phase 3 round 6.5.
func TestSpanKindStyledFromInline(t *testing.T) {
	got := Parse("a *b* c")
	if len(got) == 0 {
		t.Fatalf("Parse produced no spans for emphasis input")
	}
	for _, s := range got {
		if s.Italic && s.Kind != SpanStyled {
			t.Errorf("italic span has Kind %v, want SpanStyled", s.Kind)
		}
	}
}

// TestSpanKindBoxFromImage pins that an image-syntax span
// carries Kind == SpanBox alongside the legacy IsBox flag.
// Phase 3 round 6.5.
func TestSpanKindBoxFromImage(t *testing.T) {
	got := Parse("![alt](pic.png)")
	var found bool
	for _, s := range got {
		if s.IsBox {
			found = true
			if s.Kind != SpanBox {
				t.Errorf("box span has Kind %v, want SpanBox", s.Kind)
			}
		}
	}
	if !found {
		t.Fatalf("Parse produced no IsBox span for image input")
	}
}

// TestSpanKindRegionBeginFromCode pins that a fenced-code
// region begin span carries Kind == SpanRegionBegin.
// Phase 3 round 6.5.
func TestSpanKindRegionBeginFromCode(t *testing.T) {
	got := Parse("```\nx\n```")
	var found bool
	for _, s := range got {
		if s.RegionBegin != "" {
			found = true
			if s.Kind != SpanRegionBegin {
				t.Errorf("region-begin span has Kind %v, want SpanRegionBegin", s.Kind)
			}
		}
	}
	if !found {
		t.Fatalf("Parse produced no RegionBegin span for fenced code input")
	}
}

// TestSpanKindRegionEndFromCode pins that a fenced-code
// region end span carries Kind == SpanRegionEnd.
// Phase 3 round 6.5.
func TestSpanKindRegionEndFromCode(t *testing.T) {
	got := Parse("```\nx\n```")
	var found bool
	for _, s := range got {
		if s.RegionEnd {
			found = true
			if s.Kind != SpanRegionEnd {
				t.Errorf("region-end span has Kind %v, want SpanRegionEnd", s.Kind)
			}
		}
	}
	if !found {
		t.Fatalf("Parse produced no RegionEnd span for fenced code input")
	}
}

// TestSpanKindBlockquoteRegions pins Kind on blockquote
// region directives. Phase 3 round 6.5.
func TestSpanKindBlockquoteRegions(t *testing.T) {
	got := Parse("> a\n")
	var begin, end bool
	for _, s := range got {
		if s.RegionBegin == "blockquote" {
			begin = true
			if s.Kind != SpanRegionBegin {
				t.Errorf("blockquote begin Kind %v, want SpanRegionBegin", s.Kind)
			}
		}
		if s.RegionEnd {
			end = true
			if s.Kind != SpanRegionEnd {
				t.Errorf("region-end Kind %v, want SpanRegionEnd", s.Kind)
			}
		}
	}
	if !begin || !end {
		t.Fatalf("missing begin/end (begin=%v end=%v)", begin, end)
	}
}
