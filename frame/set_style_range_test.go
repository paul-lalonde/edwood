package frame

import (
	"image"
	"testing"

	"github.com/rjkroege/edwood/draw"
)

func TestSetStyleRange_SimpleRecolor(t *testing.T) {
	// Insert plain "hello", then re-style the whole range with a
	// colored Style. After the call, all glyph boxes covering
	// [0, 5) should carry the new Style.
	fr, display := setupStyledFrame(t)
	red, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Medblue)
	want := Style{Kind: KindColored, Fg: red}

	fr.Insert([]rune("hello"), 0)
	fr.SetStyleRange(0, 5, []StyleRun{{Len: 5, Style: want}})

	for i, b := range fr.(*frameimpl).box {
		if b == nil || b.Nrune <= 0 {
			continue
		}
		if b.Style != want {
			t.Errorf("box[%d].Style = %+v, want %+v", i, b.Style, want)
		}
	}
}

func TestSetStyleRange_PartialRange(t *testing.T) {
	// SetStyleRange on a sub-range. Boxes outside the range stay
	// plain; boxes inside the range carry the new Style.
	fr, display := setupStyledFrame(t)
	red, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Medblue)
	colored := Style{Kind: KindColored, Fg: red}

	fr.Insert([]rune("hello"), 0)
	fr.SetStyleRange(2, 4, []StyleRun{{Len: 2, Style: colored}}) // "ll" colored

	// Reconstruct rune offsets per box.
	off := 0
	for i, b := range fr.(*frameimpl).box {
		if b == nil {
			continue
		}
		nr := b.Nrune
		if nr < 0 {
			nr = 1 // tab/newline
		}
		boxStart, boxEnd := off, off+nr
		off = boxEnd
		if b.Nrune <= 0 {
			continue
		}
		// Box entirely inside [2,4) must be colored; entirely
		// outside must be plain. Splits should have separated any
		// partially-overlapping boxes.
		switch {
		case boxEnd <= 2 || boxStart >= 4:
			if !b.Style.IsPlain() {
				t.Errorf("box[%d] runes [%d,%d) outside [2,4): Style=%+v, want plain", i, boxStart, boxEnd, b.Style)
			}
		case boxStart >= 2 && boxEnd <= 4:
			if b.Style != colored {
				t.Errorf("box[%d] runes [%d,%d) inside [2,4): Style=%+v, want %+v", i, boxStart, boxEnd, b.Style, colored)
			}
		default:
			t.Errorf("box[%d] runes [%d,%d) partially overlaps [2,4); SetStyleRange should have split", i, boxStart, boxEnd)
		}
	}
}

func TestSetStyleRange_SplitsAtMidBoxBoundary(t *testing.T) {
	// Insert "hello" (one box). SetStyleRange covering "ell"
	// requires splitting the single box at both boundaries.
	// After the call there should be at least 3 boxes:
	// "h" plain, "ell" colored, "o" plain.
	fr, display := setupStyledFrame(t)
	red, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Medblue)
	colored := Style{Kind: KindColored, Fg: red}

	fr.Insert([]rune("hello"), 0)
	fr.SetStyleRange(1, 4, []StyleRun{{Len: 3, Style: colored}})

	boxes := fr.(*frameimpl).box
	if len(boxes) < 3 {
		t.Fatalf("expected at least 3 boxes after split, got %d: %v", len(boxes), boxes)
	}

	// First glyph box must be plain (covers rune 0).
	if !boxes[0].Style.IsPlain() {
		t.Errorf("box[0].Style = %+v, want plain", boxes[0].Style)
	}
	// Locate the colored box(es); they must carry the new Style.
	sawColored := false
	for i, b := range boxes {
		if b == nil || b.Nrune <= 0 {
			continue
		}
		if !b.Style.IsPlain() {
			if b.Style != colored {
				t.Errorf("box[%d].Style = %+v, want %+v", i, b.Style, colored)
			}
			sawColored = true
		}
	}
	if !sawColored {
		t.Errorf("no colored box found after SetStyleRange: %v", boxes)
	}
}

func TestSetStyleRange_PanicsOnLenMismatch(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on Len mismatch, got none")
		}
	}()
	fr, _ := setupStyledFrame(t)
	fr.Insert([]rune("hello"), 0)
	bad := []StyleRun{{Len: 2, Style: Style{}}} // sum 2 != p1-p0 = 3
	fr.SetStyleRange(0, 3, bad)
}

func TestSetStyleRange_PanicsOnOutOfRange(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on out-of-range, got none")
		}
	}()
	fr, _ := setupStyledFrame(t)
	fr.Insert([]rune("hello"), 0)
	// p1 = 99 > nchars = 5.
	fr.SetStyleRange(0, 99, []StyleRun{{Len: 99, Style: Style{}}})
}

func TestSetStyleRange_EmptyRangeIsNoOp(t *testing.T) {
	fr, _ := setupStyledFrame(t)
	fr.Insert([]rune("hello"), 0)
	before := len(fr.(*frameimpl).box)
	fr.SetStyleRange(2, 2, nil)
	after := len(fr.(*frameimpl).box)
	if before != after {
		t.Errorf("empty range changed box count from %d to %d", before, after)
	}
}

func TestSetStyleRange_DoesNotMoveSelection(t *testing.T) {
	// §5.4 "No effect on selection." sp0/sp1 must be untouched.
	fr, display := setupStyledFrame(t)
	red, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Medblue)
	colored := Style{Kind: KindColored, Fg: red}

	fr.Insert([]rune("hello world"), 0)
	fi := fr.(*frameimpl)

	// Establish a selection manually (skip the mouse loop).
	fi.sp0 = 1
	fi.sp1 = 4

	fr.SetStyleRange(6, 11, []StyleRun{{Len: 5, Style: colored}})

	if fi.sp0 != 1 || fi.sp1 != 4 {
		t.Errorf("selection moved: sp0=%d sp1=%d, want sp0=1 sp1=4", fi.sp0, fi.sp1)
	}
}
