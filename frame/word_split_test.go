package frame

import (
	"image"
	"testing"

	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/edwoodtest"
)

// Phase B5: bxscan splits content at U+0020 SPACE boundaries.
// Each contiguous non-space run is a word box; each contiguous
// run of spaces is a space box. Tabs and newlines remain
// special boxes. Style boundaries still trigger splits.
//
// Tests pin R-B5.1 (the split) and R-B5.2 (clean's merge
// predicate preserves the word/space boundary).

// buildPlainFrame is a small helper for word-split tests: a
// frame wide enough to hold the test content on one line, base
// font only.
func buildPlainFrame(t *testing.T) Frame {
	t.Helper()
	rect := image.Rect(0, 0, 400, 100)
	display := edwoodtest.NewDisplay(rect)
	var textcolors [NumColours]draw.Image
	textcolors[ColBack] = display.AllocImageMix(draw.Paleyellow, draw.White)
	textcolors[ColHigh], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Darkyellow)
	textcolors[ColBord], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Yellowgreen)
	textcolors[ColText] = display.Black()
	textcolors[ColHText] = display.Black()

	f := new(frameimpl)
	f.Init(rect,
		OptColors(textcolors),
		OptFont(edwoodtest.NewFont(10, 13)),
		OptBackground(display.ScreenImage()),
		OptMaxTab(8),
	)
	return f
}

// boxContents returns the byte contents of each content box in
// the frame, in order. Special boxes (Nrune < 0) render as
// "\\n" / "\\t" for readability.
func boxContents(f *frameimpl) []string {
	var out []string
	for _, b := range f.box {
		switch {
		case b.Nrune < 0 && b.Bc == '\n':
			out = append(out, "\\n")
		case b.Nrune < 0 && b.Bc == '\t':
			out = append(out, "\\t")
		default:
			out = append(out, string(b.Ptr))
		}
	}
	return out
}

func TestBxscan_SplitsAtSpaces_PlainContent(t *testing.T) {
	fr := buildPlainFrame(t)
	f := fr.(*frameimpl)

	fr.Insert([]rune("one two three"), 0)

	got := boxContents(f)
	want := []string{"one", " ", "two", " ", "three"}
	if !equalSlices(got, want) {
		t.Errorf("box contents = %q, want %q", got, want)
	}
}

func TestBxscan_SplitsAtSpaces_MultipleSpaces(t *testing.T) {
	// Adjacent spaces stay in one space box.
	fr := buildPlainFrame(t)
	f := fr.(*frameimpl)

	fr.Insert([]rune("hi   there"), 0)

	got := boxContents(f)
	want := []string{"hi", "   ", "there"}
	if !equalSlices(got, want) {
		t.Errorf("box contents = %q, want %q", got, want)
	}
}

func TestBxscan_SplitsAtSpaces_LeadingAndTrailing(t *testing.T) {
	fr := buildPlainFrame(t)
	f := fr.(*frameimpl)

	fr.Insert([]rune("  word  "), 0)

	got := boxContents(f)
	want := []string{"  ", "word", "  "}
	if !equalSlices(got, want) {
		t.Errorf("box contents = %q, want %q", got, want)
	}
}

func TestBxscan_SplitsAtSpaces_AcrossNewlines(t *testing.T) {
	// Newlines remain their own special boxes; word/space
	// splitting happens within each line.
	fr := buildPlainFrame(t)
	f := fr.(*frameimpl)

	fr.Insert([]rune("a b\nc d"), 0)

	got := boxContents(f)
	want := []string{"a", " ", "b", "\\n", "c", " ", "d"}
	if !equalSlices(got, want) {
		t.Errorf("box contents = %q, want %q", got, want)
	}
}

func TestBxscan_SplitsAtSpaces_AcrossTabs(t *testing.T) {
	fr := buildPlainFrame(t)
	f := fr.(*frameimpl)

	fr.Insert([]rune("a b\tc d"), 0)

	got := boxContents(f)
	want := []string{"a", " ", "b", "\\t", "c", " ", "d"}
	if !equalSlices(got, want) {
		t.Errorf("box contents = %q, want %q", got, want)
	}
}

func TestBxscan_SplitsAtSpaces_StyleBoundaryInsideWord(t *testing.T) {
	// A style change mid-word splits the word into two boxes,
	// each carrying its own Style. Spaces still split too.
	fr := buildPlainFrame(t)
	f := fr.(*frameimpl)

	src := []rune("ab cd")
	// First two runes ("ab") plain; next rune (" ") plain;
	// remaining ("cd") bold-styled.
	styles := []StyleRun{
		{Len: 3, Style: Style{}},
		{Len: 2, Style: Style{Kind: KindBold}},
	}
	fr.InsertWithStyle(src, 0, styles)

	got := boxContents(f)
	want := []string{"ab", " ", "cd"}
	if !equalSlices(got, want) {
		t.Errorf("box contents = %q, want %q", got, want)
	}
	// The bold "cd" must carry KindBold.
	if f.box[2].Style.Kind&KindBold == 0 {
		t.Errorf("box[2] should be bold; got Style.Kind = %v", f.box[2].Style.Kind)
	}
}

// R-B5.2: clean's merge predicate is relaxed — it does NOT
// merge two adjacent same-Style content boxes if exactly one is
// space-only. (Two adjacent space-only boxes can still merge;
// two adjacent non-space same-style boxes can still merge.)

func TestIsSpaceOnlyBox(t *testing.T) {
	cases := []struct {
		ptr  string
		want bool
	}{
		{"  ", true},
		{" ", true},
		{"x", false},
		{"x ", false},
		{" x", false},
		{"", false}, // empty boxes aren't space-only by convention
	}
	for _, c := range cases {
		b := &frbox{Ptr: []byte(c.ptr), Nrune: len([]rune(c.ptr))}
		got := isSpaceOnlyBox(b)
		if got != c.want {
			t.Errorf("isSpaceOnlyBox(%q) = %v, want %v", c.ptr, got, c.want)
		}
	}
	// Special boxes are not space-only regardless of Bc.
	nl := &frbox{Nrune: -1, Bc: '\n'}
	if isSpaceOnlyBox(nl) {
		t.Errorf("newline should not be space-only")
	}
	tab := &frbox{Nrune: -1, Bc: '\t'}
	if isSpaceOnlyBox(tab) {
		t.Errorf("tab should not be space-only")
	}
}

func TestClean_DoesNotMergeWordAndSpace(t *testing.T) {
	// After Insert, clean runs at the end. With word/space
	// splits, the boxes should stay separate.
	fr := buildPlainFrame(t)
	f := fr.(*frameimpl)

	fr.Insert([]rune("alpha bravo"), 0)

	// Five-ish boxes expected: "alpha", " ", "bravo". clean
	// merging would collapse to one box.
	if len(f.box) < 3 {
		t.Errorf("expected at least 3 boxes after Insert+clean; got %d: %q", len(f.box), boxContents(f))
	}
	got := boxContents(f)
	want := []string{"alpha", " ", "bravo"}
	if !equalSlices(got, want) {
		t.Errorf("box contents after clean = %q, want %q", got, want)
	}
}

// Phase B5 row B5.2 — confirm that cklinewrap's existing wrap
// behavior produces word-boundary breaks now that content is
// split at spaces. R-B5.3 (word wrap) and R-B5.4 (long word
// fallback).

// buildNarrowFrame returns a frame just wide enough to fit
// exactly N base-font chars on a line. Useful for forcing
// soft-wraps at predictable positions.
func buildNarrowFrame(t *testing.T, charsPerLine int) Frame {
	t.Helper()
	const charW, charH = 10, 13
	rect := image.Rect(0, 0, charsPerLine*charW, 200)
	display := edwoodtest.NewDisplay(rect)
	var textcolors [NumColours]draw.Image
	textcolors[ColBack] = display.AllocImageMix(draw.Paleyellow, draw.White)
	textcolors[ColHigh], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Darkyellow)
	textcolors[ColBord], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Yellowgreen)
	textcolors[ColText] = display.Black()
	textcolors[ColHText] = display.Black()

	f := new(frameimpl)
	f.Init(rect,
		OptColors(textcolors),
		OptFont(edwoodtest.NewFont(charW, charH)),
		OptBackground(display.ScreenImage()),
		OptMaxTab(8),
	)
	return f
}

// R-B5.3: a paragraph that doesn't fit on one line wraps at
// the rightmost in-line space — Ptofchar of a rune just past
// the wrap point lands at X=rect.Min.X on the next visual
// row.
func TestCklinewrap_WrapsAtWordBoundary(t *testing.T) {
	// 15-char-wide frame; "one two three four" = 18 chars
	// doesn't fit. "one two three " = 14 chars fits; "four"
	// wraps. The "f" of "four" lands at X=0 on the second
	// visual row.
	fr := buildNarrowFrame(t, 15)

	fr.Insert([]rune("one two three four"), 0)

	// "four" starts at rune index 14 (after "one two three ").
	pt := fr.Ptofchar(14)
	if pt.X != 0 {
		t.Errorf("Ptofchar('f' of 'four').X = %d, want 0 (rightmost space caused wrap)", pt.X)
	}
	// Ptofchar should be on the second visual row (Y > first
	// line's top, which is rect.Min.Y = 0).
	if pt.Y == 0 {
		t.Errorf("Ptofchar('f' of 'four').Y = 0, want > 0 (wrapped to next row)")
	}
}

// R-B5.3: no mid-word wrap when a word boundary fits. After
// "one two three " (14 chars in a 15-char-wide frame), the
// space before "four" is the wrap point — NOT mid-"three" or
// mid-"four".
func TestCklinewrap_NoMidWordWrapWhenBoundaryFits(t *testing.T) {
	fr := buildNarrowFrame(t, 15)
	fr.Insert([]rune("one two three four"), 0)

	// The wrap should put "four" entirely on line 2. Check
	// that the last visible char of line 1 is the trailing
	// space (rune 13), not a mid-word character of "three"
	// or "four".
	// Line 1 ends with rune at offset 13 (= " " of "three ").
	// Ptofchar(13) should be on the first visual row.
	// Ptofchar(14) starts the next row (the "f" of "four").
	first := fr.Ptofchar(13)
	second := fr.Ptofchar(14)
	if first.Y >= second.Y {
		t.Errorf("expected Ptofchar(13).Y < Ptofchar(14).Y (wrap between them); got first=%v second=%v", first, second)
	}
}

// R-B5.4 fallback: a word longer than the line width wraps
// to a fresh line and extends past rect.Max.X. The wrap
// still happens — the long word doesn't infinite-loop the
// layout walks.
func TestCklinewrap_LongWordWrapsAndOverflows(t *testing.T) {
	// 5-char-wide frame; "ab supercalifragilistic cd" — the
	// long word is 21 chars, won't fit. Layout should still
	// terminate.
	fr := buildNarrowFrame(t, 5)
	fr.Insert([]rune("ab supercalifragilistic cd"), 0)

	// Layout must terminate — Ptofchar at end works. The
	// long word's "s" lands at X=0 on its own line.
	pt := fr.Ptofchar(3) // 's' of supercali...
	if pt.X != 0 {
		t.Errorf("long word should wrap to a fresh line; Ptofchar('s').X = %d", pt.X)
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
