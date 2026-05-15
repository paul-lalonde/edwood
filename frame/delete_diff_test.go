package frame

import (
	"image"
	"testing"
)

// B2.3 R6 — deleteimpl uses snapshotLines + diffLines per
// frame-layout-design.md §6.2. Tests pin the observable
// contract; the bulk of behavioral coverage stays in the
// existing TestDelete suite (insert_test / delete_test).
//
// Numbered requirements:
//
//   R6.1  After Delete inside a single line, f.nchars / f.lines /
//         f.box are consistent.
//   R6.2  After Delete spanning lines, the surviving content
//         shifts up; lines[i].TopY values are correct.
//   R6.3  After Delete that vacates the bottom, lastlinefull
//         flips to false (delegated to R2's defer; regression
//         guard that R6's flow preserves it).
//   R6.4  Delete-all produces an empty frame: f.box and
//         f.lines both empty; lastlinefull == false.
//   R6.5  Delete preserves I-LAYOUT-2 / I-LAYOUT-3 / I-LAYOUT-6
//         on the post-mutation state.
//   R6.6  Delete with overflow content above the cut: the
//         content shifts up correctly (the historical
//         scroll-overlap bug).

// TestDelete_Diff_SingleLine — R6.1.
func TestDelete_Diff_SingleLine(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("hello world"), 0)
	fimpl := fr.(*frameimpl)

	fr.Delete(5, 6) // remove " "
	if fimpl.nchars != 10 {
		t.Errorf("nchars=%d, want 10", fimpl.nchars)
	}
	if len(fimpl.lines) != 1 {
		t.Errorf("lines=%d, want 1", len(fimpl.lines))
	}
}

// TestDelete_Diff_AcrossLines — R6.2 / R6.6.
func TestDelete_Diff_AcrossLines(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("a\nb\nc"), 0)
	fimpl := fr.(*frameimpl)
	dh := fimpl.defaultfontheight

	fr.Delete(0, 2) // remove "a\n"
	if len(fimpl.lines) != 2 {
		t.Fatalf("after Delete: lines=%d, want 2", len(fimpl.lines))
	}
	// "b" is now on line 0 at rect.Min.Y.
	if fimpl.lines[0].TopY != fimpl.rect.Min.Y {
		t.Errorf("line[0].TopY=%d, want %d", fimpl.lines[0].TopY, fimpl.rect.Min.Y)
	}
	// "c" is on line 1 at rect.Min.Y + lineH.
	wantLine1Y := fimpl.rect.Min.Y + dh
	if fimpl.lines[1].TopY != wantLine1Y {
		t.Errorf("line[1].TopY=%d, want %d", fimpl.lines[1].TopY, wantLine1Y)
	}
}

// TestDelete_Diff_VacatesBottom — R6.3.
func TestDelete_Diff_VacatesBottom(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 30), // 2-line height
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("a\nb\nc"), 0)
	fimpl := fr.(*frameimpl)
	if !fimpl.lastlinefull {
		t.Fatalf("test premises broken: want lastlinefull=true before Delete")
	}
	fr.Delete(0, 4) // remove "a\nb\n"
	if fimpl.lastlinefull {
		t.Errorf("after Delete vacating bottom: lastlinefull=true, want false; nchars=%d, lines=%+v, box=%+v",
			fimpl.nchars, fimpl.lines, fimpl.box)
	}
}

// TestDelete_Diff_All — R6.4.
func TestDelete_Diff_All(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("hello"), 0)
	fimpl := fr.(*frameimpl)
	fr.Delete(0, fimpl.nchars)
	if len(fimpl.box) != 0 {
		t.Errorf("Delete-all: len(box)=%d, want 0", len(fimpl.box))
	}
	if len(fimpl.lines) != 0 {
		t.Errorf("Delete-all: len(lines)=%d, want 0", len(fimpl.lines))
	}
	if fimpl.lastlinefull {
		t.Errorf("Delete-all: lastlinefull=true, want false")
	}
}

// TestDelete_Diff_InvariantsHold — R6.5.
func TestDelete_Diff_InvariantsHold(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("hello world\nfoo bar baz\nqux"), 0)
	fimpl := fr.(*frameimpl)

	fr.Delete(6, 17) // remove "world\nfoo b"
	// I-LAYOUT-2: every line's metrics match its FirstBox.
	for i, line := range fimpl.lines {
		fb := fimpl.box[line.FirstBox]
		if fb.Y != line.TopY {
			t.Errorf("post-Delete I-LAYOUT-2: line[%d].TopY=%d, box[FirstBox].Y=%d",
				i, line.TopY, fb.Y)
		}
	}
	// I-LAYOUT-3: monotone TopY.
	for i := 1; i < len(fimpl.lines); i++ {
		prev := fimpl.lines[i-1]
		cur := fimpl.lines[i]
		if cur.TopY != prev.TopY+prev.LineH {
			t.Errorf("post-Delete I-LAYOUT-3: line[%d].TopY=%d, want %d",
				i, cur.TopY, prev.TopY+prev.LineH)
		}
	}
	// I-LAYOUT-6: no layout-only fragmentation. Use the helper
	// from line_summary_test.go.
	checkNoFragmentation(t, fimpl)
}
