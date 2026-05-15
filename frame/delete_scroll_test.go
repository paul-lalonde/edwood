package frame

import (
	"image"
	"strings"
	"testing"

	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/edwoodtest"
)

// Phase B2.2 R7 — Delete's blit math uses per-line heights.
// Pre-R7 it used f.defaultfontheight as the line stride for
// every line in the shift; that breaks when the line at pt0
// is taller than defaultfontheight (a scaled heading) or vice
// versa.

// TestR7_Delete_BlitHeight_PlainLines confirms regression: a
// plain-text Delete uses per-line heights for its blit work.
// B2.3 R6 replaced the per-line blit chain with R5's
// diff-driven blit: adjacent same-ΔY shifted lines compose
// into one blit op whose Dy is the *total* shifted-run height,
// not a single line. The constant-height case here is two
// shifted lines (bbb\n + ccc), so Dy == 2 * defaultfontheight.
func TestR7_Delete_BlitHeight_PlainLines(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 200, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("aaa\nbbb\nccc"), 0)

	g := gdo(t, fr)
	g.Clear()
	// Delete "aaa\n" → blits "bbb\nccc" up to start at top.
	fr.Delete(0, 4)

	dh := fr.(*frameimpl).defaultfontheight
	got := firstBlitDy(g.DrawOps())
	if got != 2*dh {
		t.Errorf("plain Delete blit Dy = %d, want %d (2 × defaultfontheight = total shifted-run height)",
			got, 2*dh)
	}
}

// TestR7_Delete_BlitHeight_ScaledLine confirms that deleting
// the first line when the SECOND line is scaled produces a
// blit whose Dy matches the scaled line's height — that
// scaled line is what's being copied up into the vacated top
// row.
func TestR7_Delete_BlitHeight_ScaledLine(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(0, 0),
		textarea:  image.Rect(0, 0, 400, 200),
	}
	display := edwoodtest.NewDisplay(iv.textarea)
	tallFont := edwoodtest.NewFont(10, 26)

	var textcolors [NumColours]draw.Image
	textcolors[ColBack] = display.AllocImageMix(draw.Paleyellow, draw.White)
	textcolors[ColHigh], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Darkyellow)
	textcolors[ColBord], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Yellowgreen)
	textcolors[ColText] = display.Black()
	textcolors[ColHText] = display.Black()
	baseFont, _ := display.OpenFont("helvetica")
	fr := NewFrame(iv.textarea, baseFont, display.ScreenImage(), textcolors,
		OptScaleFonts(map[float32]draw.Font{1.5: tallFont}))

	scaled := Style{Kind: KindScale, Scale: 1.5}
	plain := Style{}
	// "ab\nHH\ncd" — line 1 plain, line 2 scaled (HH), line 3 plain.
	fr.InsertWithStyle([]rune("ab\nHH\ncd"), 0, []StyleRun{
		{Len: 3, Style: plain},  // "ab\n"
		{Len: 3, Style: scaled}, // "HH\n"
		{Len: 2, Style: plain},  // "cd"
	})

	g := gdo(t, fr)
	g.Clear()
	// Delete "ab\n" → blits both surviving lines (HH scaled +
	// cd plain) up. Under B2.3 R6's diff-driven blit, adjacent
	// same-ΔY shifted lines compose into one blit op whose Dy
	// is the total shifted-run height (= scaled.LineH +
	// plain.LineH).
	fr.Delete(0, 3)

	got := firstBlitDy(g.DrawOps())
	want := tallFont.Height() + fr.(*frameimpl).defaultfontheight
	if got != want {
		t.Errorf("scaled Delete blit Dy = %d, want %d (scaled LineH + plain LineH)",
			got, want)
	}
}

// firstBlitDy returns the Dy of the first "blit (x0,y0)-(x1,y1) to ..."
// op in ops, or -1 if not found.
func firstBlitDy(ops []string) int {
	for _, op := range ops {
		if !strings.HasPrefix(op, "blit (") {
			continue
		}
		rest := op[len("blit ("):]
		var x0, y0, x1, y1 int
		if _, err := fmtSscan(rest, &x0, &y0, &x1, &y1); err != nil {
			continue
		}
		return y1 - y0
	}
	return -1
}
