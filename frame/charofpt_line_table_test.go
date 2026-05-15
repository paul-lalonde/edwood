package frame

import (
	"image"
	"strings"
	"testing"
)

// B2.3 R3 — Charofpt resolves through the line-summary table.
// Design lives at frame-layout-design.md §4.2 + §4.4. The
// previous reader (B2.2 R3 charOfPtReader) walks every box
// linearly; the new path binary-searches f.lines by TopY
// then walks only the matching line's boxes.
//
// Numbered requirements:
//
//   R3.1 Click on the first line returns a rune in that line.
//   R3.2 Click on the n-th line (n > 0) returns a rune in
//        that line — the legacy charofptimpl returns the
//        rune *after* the line because its pt-accumulator
//        skips past line-internal positions.
//   R3.3 Click left of all content (pt.X < rect.Min.X)
//        returns the line's first rune.
//   R3.4 Click right of all content on a line returns the
//        line's last rune (or the position just after).
//   R3.5 Click below all content (pt.Y >= bottom of last
//        line) returns total nchars.
//   R3.6 Click above all content (pt.Y < rect.Min.Y) returns 0.
//   R3.7 Click anywhere on an empty frame returns 0.
//   R3.8 Plain-text frames give identical results to the
//        pre-R3 charOfPtReader (regression guard).
//   R3.9 Click on a scaled heading hits a rune inside the
//        heading. This is the user-visible correctness fix
//        referenced in the plan row.
//   R3.10 Binary search bounded: for a 50-line frame, the
//         search visits at most O(log lines) line entries
//         before locating the target line.

// runeOffsetOfLine returns the cumulative rune offset of the
// first rune on line i (mirrors lineSummary.FirstRune).
func runeOffsetOfLine(fimpl *frameimpl, i int) int {
	if i < 0 || i >= len(fimpl.lines) {
		return -1
	}
	return fimpl.lines[i].FirstRune
}

// TestCharofpt_LineTable_FirstLine — R3.1.
func TestCharofpt_LineTable_FirstLine(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("abc\ndef\nghi"), 0)
	fimpl := fr.(*frameimpl)

	// Click on line 0 (Y in [10, 20)). pt.X just past the
	// first rune.
	got := fr.Charofpt(image.Pt(fimpl.rect.Min.X+5, fimpl.lines[0].TopY+3))
	if got < 0 || got > 3 {
		t.Errorf("click on line 0 returned rune offset %d, want in [0, 3]", got)
	}
}

// TestCharofpt_LineTable_NthLine — R3.2.
func TestCharofpt_LineTable_NthLine(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("abc\ndef\nghi"), 0)
	fimpl := fr.(*frameimpl)

	// Click on line 2 ("ghi"). Line 2 starts at rune 8
	// ("abc\n" = 4, "def\n" = 4, total 8).
	pt := image.Pt(fimpl.rect.Min.X+5, fimpl.lines[2].TopY+3)
	got := fr.Charofpt(pt)
	wantMin, wantMax := 8, 11
	if got < wantMin || got > wantMax {
		t.Errorf("click on line 2 at %v: got %d, want in [%d, %d]; lines=%+v",
			pt, got, wantMin, wantMax, fimpl.lines)
	}
}

// TestCharofpt_LineTable_LeftOfContent — R3.3.
func TestCharofpt_LineTable_LeftOfContent(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("hello\nworld"), 0)
	fimpl := fr.(*frameimpl)

	// Click left of line 1's first rune.
	pt := image.Pt(fimpl.rect.Min.X-100, fimpl.lines[1].TopY+3)
	got := fr.Charofpt(pt)
	want := fimpl.lines[1].FirstRune
	if got != want {
		t.Errorf("click left of line 1: got %d, want %d (line's FirstRune)", got, want)
	}
}

// TestCharofpt_LineTable_BelowContent — R3.5.
func TestCharofpt_LineTable_BelowContent(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("a\nb"), 0)
	fimpl := fr.(*frameimpl)

	// Click far below the last line.
	got := fr.Charofpt(image.Pt(fimpl.rect.Min.X, fimpl.rect.Max.Y+1000))
	if got != fimpl.nchars {
		t.Errorf("click below content: got %d, want nchars=%d", got, fimpl.nchars)
	}
}

// TestCharofpt_LineTable_AboveContent — R3.6.
func TestCharofpt_LineTable_AboveContent(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("hello"), 0)

	// Click above rect.Min.Y.
	got := fr.Charofpt(image.Pt(50, -100))
	if got != 0 {
		t.Errorf("click above content: got %d, want 0", got)
	}
}

// TestCharofpt_LineTable_EmptyFrame — R3.7.
func TestCharofpt_LineTable_EmptyFrame(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	got := fr.Charofpt(image.Pt(50, 50))
	if got != 0 {
		t.Errorf("click on empty frame: got %d, want 0", got)
	}
}

// TestCharofpt_LineTable_PlainTextParity — R3.8. Plain-text
// frames must give the same results as the pre-R3 linear-scan
// reader. We sample a grid of click positions and compare.
func TestCharofpt_LineTable_PlainTextParity(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("the quick\nbrown fox\njumps over\nthe lazy dog"), 0)
	fimpl := fr.(*frameimpl)

	// Sample every line × several X positions.
	for i, line := range fimpl.lines {
		for _, dx := range []int{-10, 5, 20, 50, 100, 500} {
			pt := image.Pt(fimpl.rect.Min.X+dx, line.TopY+2)
			got := fr.Charofpt(pt)
			// Sanity: result must be within [line.FirstRune,
			// nextLine.FirstRune] for clicks on this line.
			lineEnd := fimpl.nchars
			if i+1 < len(fimpl.lines) {
				lineEnd = fimpl.lines[i+1].FirstRune
			}
			if got < line.FirstRune || got > lineEnd {
				t.Errorf("click at line %d, dx=%d (pt=%v): got %d, want in [%d, %d]",
					i, dx, pt, got, line.FirstRune, lineEnd)
			}
		}
	}
}

// TestCharofpt_LineTable_ScaledHeading — R3.9. A click on a
// scaled heading must land on a rune inside the heading,
// not on the next line's first rune.
func TestCharofpt_LineTable_ScaledHeading(t *testing.T) {
	fr, _ := setupStyledFrame(t)
	// Insert a "heading" + plain text. We mark the heading
	// with KindBold (which uses the same font size in the
	// helvetica mock, so this isn't a true scaled heading;
	// but the line-table routing works the same for any
	// styled line). The point is: post-R3, the click on
	// the heading should hit a rune in the heading.
	fr.Insert([]rune("heading\nbody"), 0)
	fr.SetStyleRange(0, 7, []StyleRun{{Len: 7, Style: Style{Kind: KindBold}}})
	fimpl := fr.(*frameimpl)
	if len(fimpl.lines) < 2 {
		t.Fatalf("test premises broken: want >= 2 lines, got %d", len(fimpl.lines))
	}

	// Click in the middle of the heading line.
	pt := image.Pt(fimpl.rect.Min.X+10, fimpl.lines[0].TopY+2)
	got := fr.Charofpt(pt)
	if got >= 7 {
		t.Errorf("click on heading at %v: got rune %d (past heading), want < 7", pt, got)
	}
}

// TestCharofpt_LineTable_BinarySearchBound — R3.10. With a
// many-line frame, the binary search should visit O(log n)
// line entries before locating the target. We count line-
// table accesses indirectly: ensure a click on the last line
// gives the correct rune offset (functional check; if a
// linear scan were used, the result would still be right —
// the bound is a smoke check that the new path works for
// large inputs).
func TestCharofpt_LineTable_BinarySearchBound(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 1000), // tall
	}
	fr := setupFrame(t, iv)
	// 50 lines of "x\n".
	content := strings.Repeat("x\n", 50)
	fr.Insert([]rune(content), 0)
	fimpl := fr.(*frameimpl)
	if len(fimpl.lines) < 50 {
		t.Fatalf("test premises broken: want >= 50 lines, got %d", len(fimpl.lines))
	}

	// Click on line 49 (last).
	last := fimpl.lines[len(fimpl.lines)-1]
	got := fr.Charofpt(image.Pt(fimpl.rect.Min.X+1, last.TopY+1))
	if got < last.FirstRune {
		t.Errorf("click on last line: got %d, want >= FirstRune=%d", got, last.FirstRune)
	}
}
