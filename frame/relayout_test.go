package frame

import (
	"image"
	"testing"
)

// Phase B2.2 R2 — relayoutFrom populates X/Y/LineH/LineA on
// every box. The pass runs at the end of Insert / Delete /
// SetStyleRange after the box model is consistent. No walk
// consumer reads these fields yet (R3 lands that); these tests
// pin the pass's output directly.

// TestRelayout_SingleLine_X_AccumulatesFromRectMin confirms
// every box's X equals rect.Min.X plus the sum of preceding
// boxes' Wid on the same line.
func TestRelayout_SingleLine_X_AccumulatesFromRectMin(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("abc def ghi"), 0)

	fimpl := fr.(*frameimpl)
	wantX := fimpl.rect.Min.X
	for i, b := range fimpl.box {
		if b.Y != fimpl.rect.Min.Y {
			t.Fatalf("test premises broken: box[%d] is on a wrapped line; widen the frame", i)
		}
		if b.X != wantX {
			t.Errorf("box[%d] (%q).X = %d, want %d", i, string(b.Ptr), b.X, wantX)
		}
		wantX += b.Wid
	}
}

// TestRelayout_SingleLine_Y_RectMin confirms every box on a
// single (non-wrapped) line shares Y = rect.Min.Y.
func TestRelayout_SingleLine_Y_RectMin(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("abc def"), 0)

	fimpl := fr.(*frameimpl)
	wantY := fimpl.rect.Min.Y
	for i, b := range fimpl.box {
		if b.Y != wantY {
			t.Errorf("box[%d] (%q).Y = %d, want %d", i, string(b.Ptr), b.Y, wantY)
		}
	}
}

// TestRelayout_NewlineSplitsY confirms that a newline box
// belongs to the line *before* the break (its Y matches that
// line's top), and the next box's Y advances by
// defaultfontheight.
func TestRelayout_NewlineSplitsY(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("a\nb"), 0)

	fimpl := fr.(*frameimpl)
	rectMinY := fimpl.rect.Min.Y
	dh := fimpl.defaultfontheight

	// Find the three logical positions.
	var aBox, nlBox, bBox *frbox
	for _, b := range fimpl.box {
		switch {
		case b.Nrune > 0 && string(b.Ptr) == "a":
			aBox = b
		case b.Nrune < 0 && b.Bc == '\n':
			nlBox = b
		case b.Nrune > 0 && string(b.Ptr) == "b":
			bBox = b
		}
	}
	if aBox == nil || nlBox == nil || bBox == nil {
		t.Fatalf("expected boxes for 'a', newline, 'b'; got box model:\n%+v", fimpl.box)
	}

	if aBox.Y != rectMinY {
		t.Errorf("'a' box Y = %d, want %d", aBox.Y, rectMinY)
	}
	if nlBox.Y != rectMinY {
		t.Errorf("newline box Y = %d, want %d (newline is last on its line)", nlBox.Y, rectMinY)
	}
	if bBox.Y != rectMinY+dh {
		t.Errorf("'b' box Y = %d, want %d", bBox.Y, rectMinY+dh)
	}
}

// TestRelayout_SoftWrap_AdvancesY confirms a soft wrap (line
// too long) advances Y by defaultfontheight for boxes past the
// wrap point. Frame is narrow enough to force wrap at the
// space.
func TestRelayout_SoftWrap_AdvancesY(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 60, 100), // 40 px wide → ~3 chars
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("aaa bbb"), 0)

	fimpl := fr.(*frameimpl)
	rectMinY := fimpl.rect.Min.Y
	dh := fimpl.defaultfontheight

	// We expect "aaa" / " " on line 1, "bbb" on line 2 (or
	// some split that produces ≥ 2 visual lines). Capture
	// line 1 Y vs line 2 Y.
	var line1Y, line2Y int = -1, -1
	for _, b := range fimpl.box {
		if line1Y < 0 {
			line1Y = b.Y
			continue
		}
		if b.Y != line1Y {
			line2Y = b.Y
			break
		}
	}
	if line1Y != rectMinY {
		t.Errorf("line 1 Y = %d, want %d", line1Y, rectMinY)
	}
	if line2Y < 0 {
		t.Fatalf("expected at least one wrapped line; box model:\n%+v", fimpl.box)
	}
	if line2Y != rectMinY+dh {
		t.Errorf("line 2 Y = %d, want %d", line2Y, rectMinY+dh)
	}
}

// TestRelayout_LineH_DefaultFontHeight confirms LineH equals
// defaultfontheight on every line (pre-R4: constant height).
func TestRelayout_LineH_DefaultFontHeight(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 60, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("aaa\nbbb ccc\nddd"), 0)

	fimpl := fr.(*frameimpl)
	dh := fimpl.defaultfontheight
	for i, b := range fimpl.box {
		if b.LineH != dh {
			t.Errorf("box[%d].LineH = %d, want %d (constant height pre-R4)", i, b.LineH, dh)
		}
		if b.LineA != dh {
			t.Errorf("box[%d].LineA = %d, want %d (Ascent stand-in)", i, b.LineA, dh)
		}
	}
}

// TestRelayout_MonotonicY_AcrossMultiLine confirms that as
// we walk box[i] forward, Y never decreases.
func TestRelayout_MonotonicY_AcrossMultiLine(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 60, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("aaa\nbbb ccc ddd\neee"), 0)

	fimpl := fr.(*frameimpl)
	prevY := -1
	for i, b := range fimpl.box {
		if b.Y < prevY {
			t.Errorf("box[%d].Y = %d < previous %d (must be monotonic)", i, b.Y, prevY)
		}
		prevY = b.Y
	}
}

// TestRelayout_AfterDelete_FieldsConsistent confirms relayout
// runs at the end of Delete and produces consistent fields on
// the remaining boxes.
func TestRelayout_AfterDelete_FieldsConsistent(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("aaa\nbbb\nccc"), 0)
	fr.Delete(0, 4) // remove "aaa\n"; remaining: "bbb\nccc"

	fimpl := fr.(*frameimpl)
	rectMinY := fimpl.rect.Min.Y
	dh := fimpl.defaultfontheight

	// First content box now at top of frame.
	first := fimpl.box[0]
	if first.Y != rectMinY {
		t.Errorf("after Delete, first box Y = %d, want %d (line 1 at top)", first.Y, rectMinY)
	}

	// Find the 'ccc' box; it should be on line 2.
	var cccBox *frbox
	for _, b := range fimpl.box {
		if b.Nrune > 0 && string(b.Ptr) == "ccc" {
			cccBox = b
			break
		}
	}
	if cccBox == nil {
		t.Fatalf("expected 'ccc' box after Delete; box model:\n%+v", fimpl.box)
	}
	if cccBox.Y != rectMinY+dh {
		t.Errorf("'ccc' box Y = %d, want %d (now on line 2)", cccBox.Y, rectMinY+dh)
	}
}

// TestRelayout_AfterSetStyleRange_FieldsStillConsistent
// confirms that SetStyleRange triggers relayout. Style changes
// can change box widths (R-B4.7) so X needs recompute.
func TestRelayout_AfterSetStyleRange_FieldsStillConsistent(t *testing.T) {
	fr, _ := setupStyledFrame(t)
	fr.Insert([]rune("hello"), 0)
	fr.SetStyleRange(0, 5, []StyleRun{{Len: 5, Style: Style{Kind: KindBold}}})

	fimpl := fr.(*frameimpl)
	wantX := fimpl.rect.Min.X
	for i, b := range fimpl.box {
		if b.X != wantX {
			t.Errorf("after SetStyleRange, box[%d].X = %d, want %d", i, b.X, wantX)
		}
		wantX += b.Wid
	}
}
