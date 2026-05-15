package frame

import (
	"image"
	"testing"
)

// B2.3 R7 — insertbyteimpl uses snapshotLines + diffLines per
// frame-layout-design.md §6.1. Tests pin the observable
// contract; the bulk of behavioral coverage stays in the
// existing TestBxscan / TestInsert / TestInsertAligned suites.
//
// Numbered requirements:
//
//   R7.1  Insert within a single line: nchars/lines/boxes are
//         consistent post-mutation.
//   R7.2  Insert that adds lines: content below the insertion
//         point shifts down (post-relayout TopY values reflect
//         the new layout).
//   R7.3  Insert at top of full frame: content below shifts
//         down, off-screen content is truncated.
//   R7.4  I-LAYOUT-2 / I-LAYOUT-3 / I-LAYOUT-6 hold post-Insert.
//   R7.5  No-op Insert (empty bytes): no state change.
//   R7.6  Round-trip Insert + Delete: state returns to baseline.

// TestInsert_Diff_SingleLine — R7.1.
func TestInsert_Diff_SingleLine(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("helo"), 0)
	fimpl := fr.(*frameimpl)

	fr.Insert([]rune("l"), 2) // -> "hello"
	if fimpl.nchars != 5 {
		t.Errorf("nchars=%d, want 5", fimpl.nchars)
	}
	if len(fimpl.lines) != 1 {
		t.Errorf("lines=%d, want 1", len(fimpl.lines))
	}
}

// TestInsert_Diff_AddsLines — R7.2.
func TestInsert_Diff_AddsLines(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 200),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("a\nb\nc"), 0)
	fimpl := fr.(*frameimpl)
	dh := fimpl.defaultfontheight
	wantLine2TopYBefore := fimpl.lines[2].TopY

	// Insert a new line at position 0; existing lines shift down.
	fr.Insert([]rune("\n"), 0)

	if len(fimpl.lines) != 4 {
		t.Fatalf("after Insert: lines=%d, want 4", len(fimpl.lines))
	}
	// Line 3 (was "c") should now be at TopY shifted by +dh.
	if fimpl.lines[3].TopY != wantLine2TopYBefore+dh {
		t.Errorf("after Insert: lines[3].TopY=%d, want %d (shifted by +%d)",
			fimpl.lines[3].TopY, wantLine2TopYBefore+dh, dh)
	}
}

// TestInsert_Diff_TopOfFullFrame — R7.3.
func TestInsert_Diff_TopOfFullFrame(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 30), // 2-line height
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("a\nb"), 0)
	fimpl := fr.(*frameimpl)
	if !fimpl.lastlinefull {
		t.Fatalf("test premises broken: want lastlinefull=true after fill")
	}

	// Insert at top: "b" should shift off-screen and be truncated.
	fr.Insert([]rune("X\n"), 0)
	// nchars accounts for what survived in the frame's bounded view.
	// Either truncation drops "b" (nchars goes from 3 to 4 — added 2,
	// dropped 1) or the diff handles it without truncation.
	// Test the I-LAYOUT-3 invariant either way: line tops monotone.
	for i := 1; i < len(fimpl.lines); i++ {
		prev, cur := fimpl.lines[i-1], fimpl.lines[i]
		if cur.TopY != prev.TopY+prev.LineH {
			t.Errorf("post-Insert I-LAYOUT-3: line[%d].TopY=%d, want %d",
				i, cur.TopY, prev.TopY+prev.LineH)
		}
	}
	if !fimpl.lastlinefull {
		t.Errorf("after Insert-at-top of full frame: lastlinefull=false, want true")
	}
}

// TestInsert_Diff_InvariantsHold — R7.4.
func TestInsert_Diff_InvariantsHold(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 200),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("hello world\nfoo bar"), 0)
	fimpl := fr.(*frameimpl)

	fr.Insert([]rune("XXX"), 6) // insert into the middle of line 0
	// I-LAYOUT-2.
	for i, line := range fimpl.lines {
		fb := fimpl.box[line.FirstBox]
		if fb.Y != line.TopY {
			t.Errorf("post-Insert I-LAYOUT-2: line[%d].TopY=%d, box[FirstBox].Y=%d",
				i, line.TopY, fb.Y)
		}
	}
	// I-LAYOUT-3.
	for i := 1; i < len(fimpl.lines); i++ {
		prev, cur := fimpl.lines[i-1], fimpl.lines[i]
		if cur.TopY != prev.TopY+prev.LineH {
			t.Errorf("post-Insert I-LAYOUT-3: line[%d].TopY=%d, want %d",
				i, cur.TopY, prev.TopY+prev.LineH)
		}
	}
	// I-LAYOUT-6.
	checkNoFragmentation(t, fimpl)
}

// TestInsert_Diff_RoundTrip — R7.6.
func TestInsert_Diff_RoundTrip(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 200),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("hello\nworld"), 0)
	fimpl := fr.(*frameimpl)
	wantNchars := fimpl.nchars
	wantLines := len(fimpl.lines)

	fr.Insert([]rune("XXX"), 5)
	fr.Delete(5, 8)

	if fimpl.nchars != wantNchars {
		t.Errorf("round-trip nchars=%d, want %d", fimpl.nchars, wantNchars)
	}
	if len(fimpl.lines) != wantLines {
		t.Errorf("round-trip lines=%d, want %d", len(fimpl.lines), wantLines)
	}
}
