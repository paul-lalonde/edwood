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
		{Offset: 3, Length: 5, Scale: 1.5},                                  // "Use `"
		{Offset: 8, Length: 4, Family: "code", Scale: 1.5},                  // "make"
		{Offset: 12, Length: 6, Scale: 1.5},                                 // "` here"
	}
	assertSpansEqual(t, Parse(src), want)
}

// TestParseEmptyInlineCode: ` ` / `` is empty; produces no
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
		{Offset: 3, Length: 1, Scale: 1.5},  // "*"
		{Offset: 4, Length: 9, Scale: 1.5, Italic: true}, // "important"
		{Offset: 13, Length: 7, Scale: 1.5}, // "* title"
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
		if got[i] != want[i] {
			t.Errorf("span[%d]:\n  got:  %+v\n  want: %+v", i, got[i], want[i])
		}
	}
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
			IsBox:        true,
			BoxPayload:   "image:x.png",
			BoxPlacement: "below",
		},
		{
			Offset:       16,
			Length:       11,
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

// TestParseImageURLWithSpace: a URL containing a space is
// terminated at the space (the title attr starts after).
// Without a title attr but with a space, `)` is the
// terminator.
func TestParseImageURLWithSpace(t *testing.T) {
	src := `![alt](path/to/file.png "width=100px")`
	// 38 runes total
	want := []Span{
		{
			Offset:       0,
			Length:       38,
			IsBox:        true,
			BoxPayload:   "image:path/to/file.png width=100",
			BoxPlacement: "below",
		},
	}
	assertSpansEqual(t, Parse(src), want)
}
