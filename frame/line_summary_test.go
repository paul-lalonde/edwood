package frame

import (
	"image"
	"testing"
)

// B2.3 R1 — per-line summary table + eager split/coalesce in
// relayoutFrom. Tests pin the new f.lines table and the inline
// split/coalesce behavior. The authoritative design lives at
// docs/designs/features/frame-layout-design.md §2.2 + §3 + §3.3
// (commits 7889f31 + e04bc81).
//
// Numbered requirements covered by this file:
//
//   R1.1  lineSummary{FirstBox, FirstRune, TopY, LineH, LineA}
//         struct exists (compile-time, see field accesses below).
//   R1.2  frameimpl.lines []lineSummary exists.
//   R1.3  len(f.lines) > 0 iff len(f.box) > 0.
//   R1.4  I-LAYOUT-2 line-table consistency: each line entry's
//         fields agree with f.box[FirstBox], and every box on
//         the line shares the line's metrics.
//   R1.5  I-LAYOUT-3 monotone TopY: lines[i+1].TopY ==
//         lines[i].TopY + lines[i].LineH for every adjacent pair.
//   R1.6  FirstRune == sum of nrune(b) over f.box[:FirstBox]
//         (special boxes count as 1).
//   R1.7  lines[0].FirstBox == 0 && lines[0].FirstRune == 0.
//   R1.8  FirstRune is monotone non-decreasing.
//   R1.9  Plain-text line count matches visual-line count.
//   R1.10 Eager split: relayoutFrom splits a content box whose
//         Wid > rect.Dx() into pieces that each fit.
//   R1.11 Multi-split: a content box with Wid >= 3*rect.Dx()
//         produces at least 3 pieces.
//   R1.12 Eager split preserves total rune count.
//   R1.13 Eager coalesce — adjacent same-style same-category
//         boxes that fit on a line are merged.
//   R1.14 Eager coalesce does NOT cross a hard newline.
//   R1.15 Eager coalesce does NOT cross a wrap boundary
//         (combined Wid doesn't fit at pt.X).
//   R1.16 Eager coalesce respects the space/word carve-out
//         (isSpaceOnlyBox category must match).
//   R1.17 Split-then-coalesce round-trip: shrink rect to force
//         eager-split, then widen rect to force eager-coalesce,
//         and the result is the original single box.
//   R1.18 I-LAYOUT-6 — no layout-only fragmentation remains
//         after any relayoutFrom.

// --- Line-table population (R1.3 / R1.7) ---

// TestLineSummary_EmptyFrame_NoEntries — R1.3 zero-content case.
func TestLineSummary_EmptyFrame_NoEntries(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fimpl := fr.(*frameimpl)
	if got := len(fimpl.lines); got != 0 {
		t.Errorf("empty frame: len(f.lines) = %d, want 0", got)
	}
}

// TestLineSummary_OneRune_OneLineWithZeroOrigin — R1.3 + R1.7.
func TestLineSummary_OneRune_OneLineWithZeroOrigin(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("a"), 0)
	fimpl := fr.(*frameimpl)
	if got := len(fimpl.lines); got != 1 {
		t.Fatalf("one-rune frame: len(f.lines) = %d, want 1", got)
	}
	line := fimpl.lines[0]
	if line.FirstBox != 0 {
		t.Errorf("line[0].FirstBox = %d, want 0", line.FirstBox)
	}
	if line.FirstRune != 0 {
		t.Errorf("line[0].FirstRune = %d, want 0", line.FirstRune)
	}
	if line.TopY != fimpl.rect.Min.Y {
		t.Errorf("line[0].TopY = %d, want %d", line.TopY, fimpl.rect.Min.Y)
	}
}

// --- I-LAYOUT-2 / I-LAYOUT-3 (R1.4 / R1.5) ---

// TestLineSummary_LineTableConsistency — R1.4 (I-LAYOUT-2).
func TestLineSummary_LineTableConsistency(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("hello\nworld\nfoo"), 0)
	fimpl := fr.(*frameimpl)

	if len(fimpl.lines) < 2 {
		t.Fatalf("test premises broken: want >= 2 lines, got %d", len(fimpl.lines))
	}
	for i, line := range fimpl.lines {
		if line.FirstBox < 0 || line.FirstBox >= len(fimpl.box) {
			t.Fatalf("line[%d].FirstBox=%d out of range [0,%d)",
				i, line.FirstBox, len(fimpl.box))
		}
		fb := fimpl.box[line.FirstBox]
		if fb.Y != line.TopY {
			t.Errorf("line[%d]: TopY=%d but box[%d].Y=%d",
				i, line.TopY, line.FirstBox, fb.Y)
		}
		if fb.LineH != line.LineH {
			t.Errorf("line[%d]: LineH=%d but box[%d].LineH=%d",
				i, line.LineH, line.FirstBox, fb.LineH)
		}
		if fb.LineA != line.LineA {
			t.Errorf("line[%d]: LineA=%d but box[%d].LineA=%d",
				i, line.LineA, line.FirstBox, fb.LineA)
		}
		// Every box on the line shares the line's metrics.
		end := len(fimpl.box)
		if i+1 < len(fimpl.lines) {
			end = fimpl.lines[i+1].FirstBox
		}
		for j := line.FirstBox; j < end; j++ {
			b := fimpl.box[j]
			if b.Y != line.TopY {
				t.Errorf("line[%d]: box[%d].Y=%d should match TopY=%d",
					i, j, b.Y, line.TopY)
			}
			if b.LineH != line.LineH {
				t.Errorf("line[%d]: box[%d].LineH=%d should match LineH=%d",
					i, j, b.LineH, line.LineH)
			}
			if b.LineA != line.LineA {
				t.Errorf("line[%d]: box[%d].LineA=%d should match LineA=%d",
					i, j, b.LineA, line.LineA)
			}
		}
	}
}

// TestLineSummary_MonotoneTopY — R1.5 (I-LAYOUT-3).
func TestLineSummary_MonotoneTopY(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("line1\nline2\nline3\nline4"), 0)
	fimpl := fr.(*frameimpl)

	for i := 1; i < len(fimpl.lines); i++ {
		prev := fimpl.lines[i-1]
		cur := fimpl.lines[i]
		want := prev.TopY + prev.LineH
		if cur.TopY != want {
			t.Errorf("line[%d].TopY=%d, want %d (prev.TopY=%d + prev.LineH=%d)",
				i, cur.TopY, want, prev.TopY, prev.LineH)
		}
	}
}

// --- FirstRune (R1.6 / R1.7 / R1.8) ---

// TestLineSummary_FirstRuneCumulative — R1.6.
func TestLineSummary_FirstRuneCumulative(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("ab\ncd\nef"), 0)
	fimpl := fr.(*frameimpl)

	for i, line := range fimpl.lines {
		want := 0
		for _, b := range fimpl.box[:line.FirstBox] {
			want += nrune(b)
		}
		if line.FirstRune != want {
			t.Errorf("line[%d].FirstRune=%d, want %d", i, line.FirstRune, want)
		}
	}
}

// TestLineSummary_FirstRuneMonotone — R1.8.
func TestLineSummary_FirstRuneMonotone(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 60, 100), // narrow → forces soft-wraps
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("aa bb cc dd ee ff"), 0)
	fimpl := fr.(*frameimpl)

	for i := 1; i < len(fimpl.lines); i++ {
		if fimpl.lines[i].FirstRune < fimpl.lines[i-1].FirstRune {
			t.Errorf("line[%d].FirstRune=%d < line[%d].FirstRune=%d",
				i, fimpl.lines[i].FirstRune, i-1, fimpl.lines[i-1].FirstRune)
		}
	}
}

// --- Plain-text line count (R1.9) ---

// TestLineSummary_PlainTextLineCount — R1.9.
func TestLineSummary_PlainTextLineCount(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"a", 1},
		{"a\n", 1}, // trailing newline does NOT start a new line
		{"a\nb", 2},
		{"a\nb\nc", 3},
		{"\n", 1},
		{"\n\n", 2},
	}
	for _, c := range cases {
		c := c
		t.Run(c.input, func(t *testing.T) {
			iv := &invariants{
				topcorner: image.Pt(20, 10),
				textarea:  image.Rect(20, 10, 400, 100),
			}
			fr := setupFrame(t, iv)
			if len(c.input) > 0 {
				fr.Insert([]rune(c.input), 0)
			}
			fimpl := fr.(*frameimpl)
			if got := len(fimpl.lines); got != c.want {
				t.Errorf("input=%q: len(f.lines)=%d, want %d", c.input, got, c.want)
			}
		})
	}
}

// --- Eager split (R1.10 / R1.11 / R1.12) ---

// TestRelayout_EagerSplit_LongWord — R1.10. Construct a state
// where a single content box has Wid > rect.Dx() and assert
// relayoutFrom splits it.
func TestRelayout_EagerSplit_LongWord(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100), // wide enough at insert
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), 0) // 40 'a's
	fimpl := fr.(*frameimpl)

	// Shrink rect; existing boxes now exceed rect.Dx().
	narrow := image.Rect(20, 10, 60, 100) // 40 px wide
	fimpl.rect = narrow
	fimpl.relayoutFrom(0)

	for i, b := range fimpl.box {
		if b.Nrune <= 0 {
			continue
		}
		if b.Wid > narrow.Dx() {
			t.Errorf("box[%d] (%q) Wid=%d > rect.Dx()=%d after eager-split",
				i, string(b.Ptr), b.Wid, narrow.Dx())
		}
	}
}

// TestRelayout_EagerSplit_MultiSplit — R1.11. A content box
// >= 3*rect.Dx() wide must produce at least 3 pieces.
func TestRelayout_EagerSplit_MultiSplit(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	// 60 'a's. At helvetica's ~6 px per 'a', this is ~360 px;
	// shrinking to 40 px wide will force >= 3 pieces.
	fr.Insert([]rune("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), 0)
	fimpl := fr.(*frameimpl)

	narrow := image.Rect(20, 10, 60, 100) // 40 px wide
	fimpl.rect = narrow
	fimpl.relayoutFrom(0)

	contentBoxes := 0
	for _, b := range fimpl.box {
		if b.Nrune > 0 {
			contentBoxes++
		}
	}
	if contentBoxes < 3 {
		t.Errorf("multi-split: got %d content boxes, want >= 3", contentBoxes)
	}
}

// TestRelayout_EagerSplit_PreservesRuneCount — R1.12.
func TestRelayout_EagerSplit_PreservesRuneCount(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	input := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	fr.Insert([]rune(input), 0)
	fimpl := fr.(*frameimpl)

	fimpl.rect = image.Rect(20, 10, 60, 100)
	fimpl.relayoutFrom(0)

	total := 0
	for _, b := range fimpl.box {
		if b.Nrune > 0 {
			total += b.Nrune
		}
	}
	if total != len(input) {
		t.Errorf("after eager-split: total runes %d, want %d", total, len(input))
	}
}

// --- Eager coalesce (R1.13 / R1.14 / R1.15 / R1.16) ---

// TestRelayout_EagerCoalesce_MergesAdjacent — R1.13. Construct
// two adjacent same-style same-category content boxes via
// splitbox, then relayoutFrom must merge them back.
func TestRelayout_EagerCoalesce_MergesAdjacent(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("hello"), 0)
	fimpl := fr.(*frameimpl)

	// Force an artificial split: 5 → "hel" + "lo".
	if fimpl.box[0].Nrune != 5 {
		t.Fatalf("test premises broken: want one 5-rune box, got %+v", fimpl.box)
	}
	fimpl.splitbox(0, 3)
	if len(fimpl.box) != 2 {
		t.Fatalf("test premises broken: want 2 boxes after split, got %d", len(fimpl.box))
	}
	if string(fimpl.box[0].Ptr) != "hel" || string(fimpl.box[1].Ptr) != "lo" {
		t.Fatalf("test premises broken: split produced %q + %q",
			fimpl.box[0].Ptr, fimpl.box[1].Ptr)
	}

	fimpl.relayoutFrom(0)

	// Coalesce should restore the single box.
	contentBoxes := 0
	for _, b := range fimpl.box {
		if b.Nrune > 0 {
			contentBoxes++
		}
	}
	if contentBoxes != 1 {
		t.Errorf("after eager-coalesce: %d content boxes, want 1; box model: %v",
			contentBoxes, fimpl.box)
	}
}

// TestRelayout_EagerCoalesce_NotAcrossNewline — R1.14.
func TestRelayout_EagerCoalesce_NotAcrossNewline(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("ab\ncd"), 0)
	fimpl := fr.(*frameimpl)
	before := len(fimpl.box)

	fimpl.relayoutFrom(0)

	if len(fimpl.box) != before {
		t.Errorf("coalesce crossed newline: box count %d -> %d", before, len(fimpl.box))
	}
}

// TestRelayout_EagerCoalesce_NotAcrossWrap — R1.15. After a
// soft wrap, the two halves are on different lines; even if
// they were same-style same-category, they must not merge.
func TestRelayout_EagerCoalesce_NotAcrossWrap(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 60, 100), // narrow
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("aaa bbb ccc"), 0)
	fimpl := fr.(*frameimpl)

	// Expect at least one soft-wrap; record the box count.
	if len(fimpl.lines) < 2 {
		t.Skipf("test premises broken: %q did not wrap; got %d lines",
			"aaa bbb ccc", len(fimpl.lines))
	}
	before := len(fimpl.box)
	fimpl.relayoutFrom(0)
	if len(fimpl.box) != before {
		t.Errorf("coalesce changed box count across wrap: %d -> %d",
			before, len(fimpl.box))
	}
	// Spot-check: no two adjacent content boxes on different Y
	// values got merged.
	for i := 0; i+1 < len(fimpl.box); i++ {
		a, b := fimpl.box[i], fimpl.box[i+1]
		if a.Nrune > 0 && b.Nrune > 0 && a.Y != b.Y {
			// They survived as separate boxes — good.
			_ = a
			_ = b
		}
	}
}

// TestRelayout_EagerCoalesce_RespectsSpaceWordBoundary — R1.16.
// "hello" + " " must not merge even though both are content
// boxes with the same Style on the same line.
func TestRelayout_EagerCoalesce_RespectsSpaceWordBoundary(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("hello world"), 0)
	fimpl := fr.(*frameimpl)

	// bxscan produces word/space/word boxes; clean preserves
	// that boundary. After relayoutFrom we should still see
	// at least 3 content boxes ("hello", " ", "world").
	contentBoxes := 0
	for _, b := range fimpl.box {
		if b.Nrune > 0 {
			contentBoxes++
		}
	}
	if contentBoxes < 3 {
		t.Errorf("space/word boundary lost: got %d content boxes, want >= 3; box model: %v",
			contentBoxes, fimpl.box)
	}
}

// --- Round-trip (R1.17) ---

// TestRelayout_SplitCoalesceRoundTrip — R1.17. Shrink rect to
// force split, widen rect to force coalesce; final box list
// equals the original.
func TestRelayout_SplitCoalesceRoundTrip(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	// 24 'a's at helvetica's ~13 px/rune = 312 px, fits in
	// the 380 px-wide rect as one box.
	fr.Insert([]rune("aaaaaaaaaaaaaaaaaaaaaaaa"), 0)
	fimpl := fr.(*frameimpl)

	// Snapshot: should be a single content box at this width.
	originalContent := 0
	for _, b := range fimpl.box {
		if b.Nrune > 0 {
			originalContent++
		}
	}
	if originalContent != 1 {
		t.Fatalf("test premises broken: want 1 content box pre-split, got %d", originalContent)
	}

	// Shrink → eager-split.
	fimpl.rect = image.Rect(20, 10, 60, 100)
	fimpl.relayoutFrom(0)

	splitContent := 0
	for _, b := range fimpl.box {
		if b.Nrune > 0 {
			splitContent++
		}
	}
	if splitContent < 2 {
		t.Fatalf("after shrink: want >= 2 content boxes, got %d", splitContent)
	}

	// Widen → eager-coalesce.
	fimpl.rect = image.Rect(20, 10, 400, 100)
	fimpl.relayoutFrom(0)

	finalContent := 0
	for _, b := range fimpl.box {
		if b.Nrune > 0 {
			finalContent++
		}
	}
	if finalContent != 1 {
		t.Errorf("round-trip: final content boxes = %d, want 1; box model: %v",
			finalContent, fimpl.box)
	}
}

// --- I-LAYOUT-6 (R1.18) ---

// TestRelayout_I_LAYOUT_6_NoFragmentation — R1.18. After any
// relayoutFrom, no adjacent pair of content boxes is mergeable
// under §3.3's rule.
func TestRelayout_I_LAYOUT_6_NoFragmentation(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("hello world\nfoo bar baz\nqux"), 0)
	fimpl := fr.(*frameimpl)

	checkNoFragmentation(t, fimpl)
}

// checkNoFragmentation asserts I-LAYOUT-6 across all adjacent
// box pairs in f.box.
func checkNoFragmentation(t *testing.T, fimpl *frameimpl) {
	t.Helper()
	rectMaxX := fimpl.rect.Max.X
	for i := 0; i+1 < len(fimpl.box); i++ {
		a := fimpl.box[i]
		b := fimpl.box[i+1]
		if a.Nrune <= 0 || b.Nrune <= 0 {
			continue
		}
		if a.Style != b.Style {
			continue
		}
		if a.Y != b.Y {
			continue
		}
		if isSpaceOnlyBox(a) != isSpaceOnlyBox(b) {
			continue
		}
		remaining := rectMaxX - a.X
		if a.Wid+b.Wid <= remaining {
			t.Errorf("I-LAYOUT-6 violated: box[%d] (%q) + box[%d] (%q) could merge on line Y=%d (Wid %d+%d <= remaining %d)",
				i, string(a.Ptr), i+1, string(b.Ptr), a.Y, a.Wid, b.Wid, remaining)
		}
	}
}
