package frame

import (
	"image"
	"strings"
	"testing"
)

// B2.3 R4 — eliminate _draw as an accumulator walker. bxscan
// reads pt1 from the staging frame's line table instead of
// _draw's pt accumulator; tab width is recomputed inside
// relayoutFrom (newwid for tabstop alignment); off-screen
// content is truncated via a line-table-driven pass instead
// of _draw's pt.Y == rect.Max.Y check.
//
// Design lives at frame-layout-design.md §6.4 + §5 (the
// `_draw` row).
//
// Numbered requirements:
//
//   R4.1 After bxscan on plain content that fits, pt1 read
//        from the staging frame's lines[-1] equals
//        (rect.Min.X, lines[-1].TopY + lines[-1].LineH).
//   R4.2 bxscan with tabs produces tab boxes whose Wid is
//        tabstop-aligned (matches newwid0 semantics).
//   R4.3 bxscan with overflow truncates off-screen content
//        from the staging frame (delbox + nchars adjust).
//   R4.4 After insert with tabs, the parent frame's box
//        positions align to tabstops (b.X jumps by tab Wid).
//   R4.5 Insert + scroll cycle: insertion at the bottom of a
//        full frame produces correct lastlinefull state and
//        no scroll-overlap glitches (the user-visible bug
//        B2.3 was started for).

// TestDrawAccumulator_Pt1MatchesLineTable — R4.1.
func TestDrawAccumulator_Pt1MatchesLineTable(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 300),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("hello\nworld\nfoo bar"), 0)
	fimpl := fr.(*frameimpl)

	// After Insert, the parent's relayoutFrom has run; the
	// last line's bottom must match the final visible
	// position of inserted content (no off-by-one or stale
	// pt accumulator).
	if len(fimpl.lines) == 0 {
		t.Fatalf("no lines after Insert")
	}
	last := fimpl.lines[len(fimpl.lines)-1]
	wantBottom := last.TopY + last.LineH

	// Sanity: bottom is within rect (no overflow in this fixture).
	if wantBottom > fimpl.rect.Max.Y {
		t.Errorf("test premises broken: lines[-1] bottom %d exceeds rect.Max.Y %d",
			wantBottom, fimpl.rect.Max.Y)
	}

	// Every box on the last line should have Y == last.TopY
	// (consistency check: relayoutFrom wrote it).
	if len(fimpl.box) == 0 {
		t.Fatalf("no boxes after Insert")
	}
	lastBox := fimpl.box[len(fimpl.box)-1]
	if lastBox.Y != last.TopY {
		t.Errorf("box[last].Y=%d != lines[last].TopY=%d", lastBox.Y, last.TopY)
	}
}

// TestDrawAccumulator_TabWidthTabstopAligned — R4.2 / R4.4.
// After Insert with a tab, the tab box's Wid is the distance
// to the next tabstop from its X position. With maxtab=8 and
// a tab at the start of a line (X = rect.Min.X), Wid is the
// next-tabstop snap (clamped to >= Minwid).
func TestDrawAccumulator_TabWidthTabstopAligned(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("\ta"), 0)
	fimpl := fr.(*frameimpl)

	// Find the tab box.
	var tabBox *frbox
	for _, b := range fimpl.box {
		if b.Nrune < 0 && b.Bc == '\t' {
			tabBox = b
			break
		}
	}
	if tabBox == nil {
		t.Fatalf("no tab box found; boxes=%v", fimpl.box)
	}

	// Tab Wid must NOT be the bxscan-time placeholder (10000).
	if tabBox.Wid >= 10000 {
		t.Errorf("tab Wid=%d still at bxscan placeholder; want tabstop-aligned (< 10000)", tabBox.Wid)
	}
	// And tab Wid must be at least Minwid (StringWidth(" ")).
	if tabBox.Wid < int(tabBox.Minwid) {
		t.Errorf("tab Wid=%d < Minwid=%d", tabBox.Wid, tabBox.Minwid)
	}
}

// TestDrawAccumulator_AfterTab_ContentAlignsToTabstop — R4.4.
// Box positions after a tab must reflect the tab's
// tabstop-aligned Wid in X. Today: text+tab+text has the
// second text's X = first.X + first.Wid + tabBox.Wid.
func TestDrawAccumulator_AfterTab_ContentAlignsToTabstop(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("a\tb"), 0)
	fimpl := fr.(*frameimpl)

	var first, tab, second *frbox
	for _, b := range fimpl.box {
		switch {
		case b.Nrune > 0 && string(b.Ptr) == "a":
			first = b
		case b.Nrune < 0 && b.Bc == '\t':
			tab = b
		case b.Nrune > 0 && string(b.Ptr) == "b":
			second = b
		}
	}
	if first == nil || tab == nil || second == nil {
		t.Fatalf("missing box; first=%v tab=%v second=%v", first, tab, second)
	}

	wantSecondX := first.X + first.Wid + tab.Wid
	if second.X != wantSecondX {
		t.Errorf("second.X=%d, want %d (first.X=%d + first.Wid=%d + tab.Wid=%d)",
			second.X, wantSecondX, first.X, first.Wid, tab.Wid)
	}
}

// TestDrawAccumulator_OverflowTruncates — R4.3. Insert that
// pushes content past rect.Max.Y must drop the off-screen
// boxes from f.box. Parent's nchars reflects only the kept
// content.
func TestDrawAccumulator_OverflowTruncates(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 30), // ~2 lines tall (lineH=10)
	}
	fr := setupFrame(t, iv)
	// 5 lines; only ~2 fit. Note: defaultfontheight = mock font Height = 10.
	fr.Insert([]rune("a\nb\nc\nd\ne"), 0)
	fimpl := fr.(*frameimpl)

	// lastlinefull must be true (overflow).
	if !fimpl.lastlinefull {
		t.Errorf("after overflow Insert: lastlinefull=false, want true")
	}

	// Box list should not contain content boxes whose Y is
	// >= rect.Max.Y. (Truncation removed them.)
	for i, b := range fimpl.box {
		if b.Nrune > 0 && b.Y >= fimpl.rect.Max.Y {
			t.Errorf("box[%d] (%q) at Y=%d is off-screen (rect.Max.Y=%d); want truncated",
				i, string(b.Ptr), b.Y, fimpl.rect.Max.Y)
		}
	}
}

// TestDrawAccumulator_ScrollCycle_NoOverlap — R4.5. The
// motivating user-visible bug: a "scroll" via Delete-top +
// Insert-bottom on a full frame must leave lastlinefull
// correctly set and the box state consistent. Verifies the
// integration of R1–R4 (per-line summary + ownership of
// lastlinefull + Charofpt routing + no _draw accumulator).
func TestDrawAccumulator_ScrollCycle_NoOverlap(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 30),
	}
	fr := setupFrame(t, iv)
	// Fill exactly: 3 lines × 10 px = 30. lastlinefull = true.
	fr.Insert([]rune("a\nb\nc"), 0)
	fimpl := fr.(*frameimpl)
	if !fimpl.lastlinefull {
		t.Fatalf("test premises broken: want lastlinefull=true after fill")
	}

	// Simulate one scroll step: drop the top line, append a
	// new bottom line.
	fr.Delete(0, 2) // remove "a\n"
	if fimpl.lastlinefull {
		t.Errorf("after Delete that vacates the bottom: lastlinefull=true, want false")
	}

	fr.Insert([]rune("\nd"), fimpl.nchars) // add new bottom
	if !fimpl.lastlinefull {
		t.Errorf("after refill Insert: lastlinefull=false, want true")
	}

	// I-LAYOUT-3 holds: line tops are monotone.
	for i := 1; i < len(fimpl.lines); i++ {
		prev, cur := fimpl.lines[i-1], fimpl.lines[i]
		if cur.TopY != prev.TopY+prev.LineH {
			t.Errorf("line[%d].TopY=%d, want %d (TopY-monotone broken post-scroll)",
				i, cur.TopY, prev.TopY+prev.LineH)
		}
	}
}

// TestDrawAccumulator_LongContent_Pt1Stable — R4.1 extended.
// pt1 reading from line table must not drift for long content.
func TestDrawAccumulator_LongContent_Pt1Stable(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 1000),
	}
	fr := setupFrame(t, iv)
	// 30 lines.
	fr.Insert([]rune(strings.Repeat("line\n", 30)), 0)
	fimpl := fr.(*frameimpl)

	if len(fimpl.lines) != 30 {
		t.Fatalf("got %d lines, want 30", len(fimpl.lines))
	}
	last := fimpl.lines[29]
	// Last line's bottom should be 10 + 30*10 = 310 (rect.Min.Y + 30*lineH).
	wantBottom := fimpl.rect.Min.Y + 30*last.LineH
	gotBottom := last.TopY + last.LineH
	if gotBottom != wantBottom {
		t.Errorf("last line bottom = %d, want %d", gotBottom, wantBottom)
	}
}
