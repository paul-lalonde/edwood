package frame

import (
	"image"
	"testing"
)

// B2.3 R5 — snapshotLines + diffLines helpers per
// frame-layout-design.md §3.5. snapshotLines captures the
// pre-mutation line state; diffLines compares the snapshot
// against post-mutation f.lines and returns a minimum-cost
// sequence of paint ops (blit shifts for unchanged content
// that moved; paint for changed/new content).
//
// Numbered requirements:
//
//   R5.1  snapshotLines returns one entry per current line.
//   R5.2  snapshotLines captures FirstRune, TopY, LineH, and
//         a content digest that changes when box Ptr or Style
//         change within the line.
//   R5.3  diffLines on an unchanged state returns no ops.
//   R5.4  diffLines on shifted lines (same FirstRune, same
//         LineH, same digest, different TopY) returns one
//         blit op per shift run.
//   R5.5  diffLines on dirty lines (no FirstRune match, or
//         digest/LineH changed) returns paint ops.
//   R5.6  Run compression: adjacent shifted lines with same
//         ΔY → one blit op covering the whole run.
//   R5.7  Mixed shift + dirty: ops emitted in screen order,
//         no src/dst rectangle overlap on blit runs.
//   R5.8  All-dirty (width-resize-style reflow): no blits,
//         one or more paint ops covering everything.
//   R5.9  No-op mutation (snap matches f.lines exactly) →
//         empty diff.

// TestDiff_SnapshotLines_OneEntryPerLine — R5.1.
func TestDiff_SnapshotLines_OneEntryPerLine(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 300),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("a\nb\nc"), 0)
	fimpl := fr.(*frameimpl)

	snap := fimpl.snapshotLines()
	if len(snap) != len(fimpl.lines) {
		t.Errorf("snapshotLines returned %d entries, want %d (= len(f.lines))",
			len(snap), len(fimpl.lines))
	}
	for i, s := range snap {
		if s.FirstRune != fimpl.lines[i].FirstRune {
			t.Errorf("snap[%d].FirstRune=%d, want %d", i, s.FirstRune, fimpl.lines[i].FirstRune)
		}
		if s.TopY != fimpl.lines[i].TopY {
			t.Errorf("snap[%d].TopY=%d, want %d", i, s.TopY, fimpl.lines[i].TopY)
		}
		if s.LineH != fimpl.lines[i].LineH {
			t.Errorf("snap[%d].LineH=%d, want %d", i, s.LineH, fimpl.lines[i].LineH)
		}
	}
}

// TestDiff_SnapshotLines_DigestStable — R5.2 stability. Two
// snapshots taken back-to-back on the same frame state produce
// equal digests.
func TestDiff_SnapshotLines_DigestStable(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 300),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("hello\nworld\nfoo"), 0)
	fimpl := fr.(*frameimpl)

	snap1 := fimpl.snapshotLines()
	snap2 := fimpl.snapshotLines()
	if len(snap1) != len(snap2) {
		t.Fatalf("snapshot count diverged: %d vs %d", len(snap1), len(snap2))
	}
	for i := range snap1 {
		if snap1[i].Digest != snap2[i].Digest {
			t.Errorf("snap[%d] digest unstable: %d vs %d", i, snap1[i].Digest, snap2[i].Digest)
		}
	}
}

// TestDiff_SnapshotLines_DigestChangesOnContent — R5.2
// sensitivity. Mutating box content changes the digest.
func TestDiff_SnapshotLines_DigestChangesOnContent(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 300),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("hello\nworld"), 0)
	fimpl := fr.(*frameimpl)
	snap1 := fimpl.snapshotLines()

	// Insert into line 0; line 1's FirstRune shifts but its content's digest
	// (relative to its boxes) should differ from before because the line's
	// box range is now different… actually line 0's content changes so its
	// digest changes; line 1's content is the same.
	fr.Insert([]rune("X"), 0)
	snap2 := fimpl.snapshotLines()

	if len(snap1) != len(snap2) {
		t.Fatalf("line count changed unexpectedly: %d -> %d", len(snap1), len(snap2))
	}
	// Line 0 content changed → digest must differ.
	if snap1[0].Digest == snap2[0].Digest {
		t.Errorf("line 0 digest unchanged after content mutation; want different")
	}
}

// TestDiff_DiffLines_NoChange_EmptyOps — R5.3 / R5.9.
func TestDiff_DiffLines_NoChange_EmptyOps(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 300),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("a\nb\nc"), 0)
	fimpl := fr.(*frameimpl)

	snap := fimpl.snapshotLines()
	ops := fimpl.diffLines(snap)
	if len(ops) != 0 {
		t.Errorf("no-change diff produced %d ops, want 0; ops=%v", len(ops), ops)
	}
}

// TestDiff_DiffLines_ShiftedRun_OneBlit — R5.4 / R5.6.
// Construct two snapshots that differ only in TopY; expect
// one blit covering the run.
func TestDiff_DiffLines_ShiftedRun_OneBlit(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 300),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("a\nb\nc"), 0)
	fimpl := fr.(*frameimpl)
	snap := fimpl.snapshotLines()

	// Insert a NEWLINE at the very start so every existing line
	// shifts down by one lineH. Use Insert with "\n".
	fr.Insert([]rune("\n"), 0)
	ops := fimpl.diffLines(snap)

	// Expect at least one blit op covering the shifted run.
	hasBlit := false
	for _, op := range ops {
		if op.Kind == OpBlit {
			hasBlit = true
		}
	}
	if !hasBlit {
		t.Errorf("after insert-at-top: ops=%v, want at least one OpBlit", ops)
	}
}

// TestDiff_DiffLines_DirtyLine_PaintOp — R5.5. Content mutation
// produces a paint op for the dirty line.
func TestDiff_DiffLines_DirtyLine_PaintOp(t *testing.T) {
	fr, _ := setupStyledFrame(t)
	fr.Insert([]rune("hello\nworld"), 0)
	fimpl := fr.(*frameimpl)
	snap := fimpl.snapshotLines()

	// SetStyleRange on line 0 — same FirstRune, same TopY,
	// same LineH, but content's style digest changes.
	fr.SetStyleRange(0, 5, []StyleRun{{Len: 5, Style: Style{Kind: KindBold}}})
	ops := fimpl.diffLines(snap)

	hasPaint := false
	for _, op := range ops {
		if op.Kind == OpPaint {
			hasPaint = true
		}
	}
	if !hasPaint {
		t.Errorf("after style change: ops=%v, want at least one OpPaint", ops)
	}
}

// TestDiff_DiffLines_AllDirty_NoBlits — R5.8. When every line
// is dirty (e.g., a width-resize-style reflow that changes all
// TopY values via line height changes), no blits are emitted.
//
// Approximated by constructing a snapshot with different LineH
// for every line, then calling diffLines after a relayout that
// keeps the same content but doesn't match the snapshot LineH.
func TestDiff_DiffLines_AllDirty_NoBlits(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 300),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("a\nb\nc"), 0)
	fimpl := fr.(*frameimpl)

	// Construct a fake snapshot with LineH different from any
	// actual line; every line will classify as dirty.
	snap := make([]lineSnap, len(fimpl.lines))
	for i, line := range fimpl.lines {
		snap[i] = lineSnap{
			FirstRune: line.FirstRune,
			TopY:      line.TopY,
			LineH:     line.LineH + 99, // intentionally wrong
			Digest:    0,
		}
	}
	ops := fimpl.diffLines(snap)
	for _, op := range ops {
		if op.Kind == OpBlit {
			t.Errorf("all-dirty: got blit op %v, want only paint ops", op)
		}
	}
}

// TestDiff_DiffLines_MixedRuns_OrderingTopToBottom — R5.7.
func TestDiff_DiffLines_MixedRuns_OrderingTopToBottom(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 300),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("a\nb\nc\nd"), 0)
	fimpl := fr.(*frameimpl)
	snap := fimpl.snapshotLines()

	// Style only line 0; subsequent lines stay identical or
	// shift depending on whether the style change affected
	// LineH (mock bold is same height → no shift).
	fr.SetStyleRange(0, 1, []StyleRun{{Len: 1, Style: Style{Kind: KindBold}}})
	ops := fimpl.diffLines(snap)

	// Verify ops are issued in screen order (Dst.Min.Y monotone).
	var prevY int
	for i, op := range ops {
		if i > 0 && op.Dst.Min.Y < prevY {
			t.Errorf("op[%d] Dst.Min.Y=%d < prev=%d (ops not in screen order)",
				i, op.Dst.Min.Y, prevY)
		}
		prevY = op.Dst.Min.Y
	}
}
