package frame

import (
	"image"
	"testing"
)

// B2.3 R2 — lastlinefull is owned by relayoutFrom, derived
// from the line table:
//
//   f.lastlinefull == (lines[-1].TopY + lines[-1].LineH >= rect.Max.Y)
//
// Design lives at frame-layout-design.md §2.3 + I-LAYOUT-4.
// Tests pin the formula across the Insert / Delete /
// SetStyleRange mutator paths and assert relayoutFrom alone
// sets the field.
//
// Numbered requirements:
//
//   R2.1 Empty frame: lastlinefull == false.
//   R2.2 Content fits with room to spare:
//        lines[-1].TopY + LineH < rect.Max.Y → lastlinefull == false.
//   R2.3 Content exactly fills:
//        lines[-1].TopY + LineH == rect.Max.Y → lastlinefull == true.
//   R2.4 Content overflows (relayoutFrom continues past
//        rect.Max.Y; lines[-1].TopY + LineH > rect.Max.Y) →
//        lastlinefull == true.
//   R2.5 Insert that fills past rect.Max.Y sets lastlinefull = true.
//   R2.6 Delete that vacates the bottom (was-full → not-full)
//        flips lastlinefull to false. This is the regression
//        guard for commit 677ab5e's explicit reset, now folded
//        back into the layout pass.
//   R2.7 SetStyleRange that does not reflow leaves
//        lastlinefull unchanged. SetStyleRange that does
//        reflow updates lastlinefull per the formula.
//   R2.8 relayoutFrom is the sole writer: forcibly setting
//        f.lastlinefull to a wrong value before relayoutFrom
//        always results in the formula's value after.

// expectedLastLineFull computes the I-LAYOUT-4 formula from
// the line table.
func expectedLastLineFull(f *frameimpl) bool {
	if len(f.lines) == 0 {
		return false
	}
	last := f.lines[len(f.lines)-1]
	return last.TopY+last.LineH >= f.rect.Max.Y
}

// TestLastLineFull_EmptyFrame — R2.1.
func TestLastLineFull_EmptyFrame(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fimpl := fr.(*frameimpl)
	if fimpl.lastlinefull {
		t.Errorf("empty frame: lastlinefull = true, want false")
	}
}

// TestLastLineFull_FitsWithRoomToSpare — R2.2.
func TestLastLineFull_FitsWithRoomToSpare(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 200), // tall frame
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("a\nb\nc"), 0) // 3 lines at ~13 px each → 39 px, well under 200
	fimpl := fr.(*frameimpl)
	if fimpl.lastlinefull {
		t.Errorf("3 lines in 200-px frame: lastlinefull = true, want false")
	}
	if got := expectedLastLineFull(fimpl); fimpl.lastlinefull != got {
		t.Errorf("lastlinefull = %v, want formula value %v", fimpl.lastlinefull, got)
	}
}

// TestLastLineFull_ContentOverflows — R2.4. With defaultfontheight=13
// and rect height 30, inserting 5 lines overflows.
func TestLastLineFull_ContentOverflows(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 40), // height 30 → fits ~2 lines
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("a\nb\nc\nd\ne"), 0) // 5 lines × 13 = 65, far over 30
	fimpl := fr.(*frameimpl)
	if !fimpl.lastlinefull {
		t.Errorf("overflowing content: lastlinefull = false, want true (rect.Max.Y=%d, last line bottom=%d)",
			fimpl.rect.Max.Y,
			func() int {
				if len(fimpl.lines) == 0 {
					return -1
				}
				l := fimpl.lines[len(fimpl.lines)-1]
				return l.TopY + l.LineH
			}())
	}
	if got := expectedLastLineFull(fimpl); fimpl.lastlinefull != got {
		t.Errorf("lastlinefull = %v, want formula value %v", fimpl.lastlinefull, got)
	}
}

// TestLastLineFull_AfterDelete_FlipsToFalse — R2.6. The
// regression guard for commit 677ab5e: a Delete that vacates
// the bottom must clear lastlinefull.
func TestLastLineFull_AfterDelete_FlipsToFalse(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 40), // 30 px tall
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("a\nb\nc\nd\ne"), 0) // overflows
	fimpl := fr.(*frameimpl)
	if !fimpl.lastlinefull {
		t.Fatalf("test premises broken: lastlinefull = false pre-Delete, want true")
	}

	// Delete enough to bring content fully within rect.
	fr.Delete(0, fimpl.nchars-2) // leave at most "?\n?" or similar
	if fimpl.lastlinefull {
		t.Errorf("after Delete vacating the bottom: lastlinefull = true, want false")
	}
	if got := expectedLastLineFull(fimpl); fimpl.lastlinefull != got {
		t.Errorf("lastlinefull = %v, want formula value %v", fimpl.lastlinefull, got)
	}
}

// TestLastLineFull_AfterInsert_OverflowsSetsTrue — R2.5.
func TestLastLineFull_AfterInsert_OverflowsSetsTrue(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 40),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("a"), 0) // fits easily
	fimpl := fr.(*frameimpl)
	if fimpl.lastlinefull {
		t.Fatalf("test premises broken: single-rune frame already full")
	}

	fr.Insert([]rune("\nb\nc\nd\ne\nf"), 1) // overflows
	if !fimpl.lastlinefull {
		t.Errorf("after Insert that overflows: lastlinefull = false, want true")
	}
	if got := expectedLastLineFull(fimpl); fimpl.lastlinefull != got {
		t.Errorf("lastlinefull = %v, want formula value %v", fimpl.lastlinefull, got)
	}
}

// TestLastLineFull_AfterSetStyleRange_FormulaHolds — R2.7.
// Whether the style change reflows or not, lastlinefull must
// match the formula after relayoutFrom runs.
func TestLastLineFull_AfterSetStyleRange_FormulaHolds(t *testing.T) {
	fr, _ := setupStyledFrame(t)
	fr.Insert([]rune("hello\nworld"), 0)
	fr.SetStyleRange(0, 5, []StyleRun{{Len: 5, Style: Style{Kind: KindBold}}})

	fimpl := fr.(*frameimpl)
	if got := expectedLastLineFull(fimpl); fimpl.lastlinefull != got {
		t.Errorf("after SetStyleRange: lastlinefull = %v, want formula value %v",
			fimpl.lastlinefull, got)
	}
}

// TestLastLineFull_RelayoutFromIsSoleWriter — R2.8. Forcibly
// set lastlinefull to a wrong value, call relayoutFrom, and
// confirm the field gets corrected.
func TestLastLineFull_RelayoutFromIsSoleWriter(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("a\nb"), 0)
	fimpl := fr.(*frameimpl)

	// Frame is NOT full (2 lines × 13 = 26 < 90 px).
	// Force-set the wrong value.
	fimpl.lastlinefull = true
	fimpl.relayoutFrom(0)
	if fimpl.lastlinefull {
		t.Errorf("relayoutFrom did not override stale lastlinefull = true; still true")
	}

	// And the inverse: force false on an overflowed frame.
	narrowIv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 40),
	}
	fr2 := setupFrame(t, narrowIv)
	fr2.Insert([]rune("a\nb\nc\nd\ne"), 0)
	f2 := fr2.(*frameimpl)
	if !f2.lastlinefull {
		t.Fatalf("test premises broken")
	}
	f2.lastlinefull = false
	f2.relayoutFrom(0)
	if !f2.lastlinefull {
		t.Errorf("relayoutFrom did not override stale lastlinefull = false; still false")
	}
}

// TestLastLineFull_ExactlyFills — R2.3. Content whose last
// line's bottom sits exactly on rect.Max.Y triggers
// lastlinefull = true. Mock font defaultfontheight is 10
// (helvetica from edwoodtest), so a 10-pixel-tall rect holds
// exactly one line.
func TestLastLineFull_ExactlyFills(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 20), // height 10
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("a"), 0)
	fimpl := fr.(*frameimpl)
	if !fimpl.lastlinefull {
		t.Errorf("one line in single-line-tall rect: lastlinefull = false, want true (TopY=%d, LineH=%d, rect.Max.Y=%d)",
			fimpl.lines[0].TopY, fimpl.lines[0].LineH, fimpl.rect.Max.Y)
	}
	if got := expectedLastLineFull(fimpl); fimpl.lastlinefull != got {
		t.Errorf("lastlinefull = %v, want formula value %v", fimpl.lastlinefull, got)
	}
}
