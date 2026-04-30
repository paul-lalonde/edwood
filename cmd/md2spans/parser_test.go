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

// TestParseEmphasisDoesNotSpanParagraphs: emphasis is intra-
// paragraph (R4); openers in one paragraph don't pair with
// closers in another.
func TestParseEmphasisDoesNotSpanParagraphs(t *testing.T) {
	src := "*opener\n\ncloser*"
	if got := Parse(src); len(got) != 0 {
		t.Errorf("Parse(%q) = %v, want empty (emphasis cannot span paragraphs)", src, got)
	}
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
