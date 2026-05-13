package frame

import (
	"image"
	"testing"
)

// Phase B2.2 R1 — per-box layout fields. The frbox struct gains
// X, Y, LineH, LineA. R1 ships only the fields and their
// initialization; the layout pass that populates Y/LineH/LineA
// from per-line metrics lands in R2. Tests below pin the
// post-bxscan defaults so R2 has a known starting point.

// TestFrbox_LayoutFields_DefaultsAfterInsert confirms that
// every box produced by Insert carries LineH = the frame's
// defaultfontheight and LineA = the same (Ascent stand-in
// until real Ascent is plumbed in R5).
func TestFrbox_LayoutFields_DefaultsAfterInsert(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 200, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("abc def"), 0)

	fimpl := fr.(*frameimpl)
	dh := fimpl.defaultfontheight
	for i, b := range fimpl.box {
		if b == nil {
			t.Errorf("box[%d] is nil", i)
			continue
		}
		if b.LineH != dh {
			t.Errorf("box[%d].LineH = %d, want defaultfontheight = %d", i, b.LineH, dh)
		}
		if b.LineA != dh {
			t.Errorf("box[%d].LineA = %d, want %d (Ascent stand-in)", i, b.LineA, dh)
		}
	}
}

// TestFrbox_LayoutFields_TabAndNewline confirms tab and newline
// boxes also carry the default line metrics. These special
// boxes are constructed inline in bxscan, not via
// addifnonempty, so they need their own init site.
func TestFrbox_LayoutFields_TabAndNewline(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 200, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("a\tb\nc"), 0)

	fimpl := fr.(*frameimpl)
	dh := fimpl.defaultfontheight

	sawTab, sawNewline := false, false
	for i, b := range fimpl.box {
		if b == nil || b.Nrune >= 0 {
			continue
		}
		switch b.Bc {
		case '\t':
			sawTab = true
		case '\n':
			sawNewline = true
		default:
			continue
		}
		if b.LineH != dh {
			t.Errorf("special box[%d] (Bc=%q).LineH = %d, want %d", i, b.Bc, b.LineH, dh)
		}
		if b.LineA != dh {
			t.Errorf("special box[%d] (Bc=%q).LineA = %d, want %d", i, b.Bc, b.LineA, dh)
		}
	}
	if !sawTab {
		t.Errorf("expected a tab box in the layout; found none")
	}
	if !sawNewline {
		t.Errorf("expected a newline box in the layout; found none")
	}
}

// (R1's "Y is zero pre-R2" pin was removed in R2; relayoutFrom
// now populates Y. See relayout_test.go for the Y assertions.)
