package frame

import (
	"image"
	"strings"
	"testing"

	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/edwoodtest"
)

// Phase B2.2 R6 — Tick adapts to the line's height. On a
// plain line the caret is defaultfontheight tall; on a scaled
// heading line it's the heading's full line height.

// TestR6_Tick_PlainLine_DefaultHeight confirms the tick draws
// a Draw op whose Y extent equals defaultfontheight on a
// plain line.
func TestR6_Tick_PlainLine_DefaultHeight(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 200, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("abc"), 0)

	fimpl := fr.(*frameimpl)
	dh := fimpl.defaultfontheight

	g := gdo(t, fr)
	g.Clear()

	// Tick on the first character's position.
	pt := fr.Ptofchar(0)
	fr.(*frameimpl).Tick(pt, true)

	if got := tickDrawDy(g.DrawOps(), pt); got != dh {
		t.Errorf("tick Dy on plain line = %d, want %d (defaultfontheight)", got, dh)
	}
}

// TestR6_Tick_ScaledLine_LineHeight confirms the tick draws a
// Draw op whose Y extent equals the line's LineH (the scaled
// font's height) on a heading line.
func TestR6_Tick_ScaledLine_LineHeight(t *testing.T) {
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
	fr.InsertWithStyle([]rune("H"), 0, []StyleRun{{Len: 1, Style: scaled}})

	g := gdo(t, fr)
	g.Clear()

	pt := fr.Ptofchar(0)
	fr.(*frameimpl).Tick(pt, true)

	if got := tickDrawDy(g.DrawOps(), pt); got != tallFont.Height() {
		t.Errorf("tick Dy on scaled line = %d, want %d (scaled font Height)",
			got, tallFont.Height())
	}
}

// tickDrawDy scans the recorded ops for the alpha-blended
// Draw produced by tick — the one whose rect's top edge is at
// pt.Y — and returns its height (rect.Max.Y - rect.Min.Y).
// Returns -1 if not found. The tick produces ONE Draw op of
// the rect; finding it via top-edge-at-pt.Y is robust to
// changes in op formatting.
func tickDrawDy(ops []string, pt image.Point) int {
	for _, op := range ops {
		// Match either "fill (X0,Y0)-(X1,Y1) ..." or
		// "blit ..." — tick uses Draw which renders as fill
		// in the mock display recorder.
		idx := strings.Index(op, "fill (")
		if idx < 0 {
			continue
		}
		rest := op[idx+len("fill ("):]
		var x0, y0, x1, y1 int
		if _, err := fmtSscan(rest, &x0, &y0, &x1, &y1); err != nil {
			continue
		}
		if y0 == pt.Y && x0 <= pt.X && pt.X < x1 {
			return y1 - y0
		}
	}
	return -1
}

// fmtSscan reads "X0,Y0)-(X1,Y1)" prefix from s into the
// four int pointers. Returns the count of conversions and
// any error.
func fmtSscan(s string, x0, y0, x1, y1 *int) (int, error) {
	// Find "X0,Y0)-(X1,Y1)" tokens.
	end := strings.Index(s, ")")
	if end < 0 {
		return 0, errBadFill
	}
	leftPair := s[:end]
	rest := s[end+1:]
	if !strings.HasPrefix(rest, "-(") {
		return 0, errBadFill
	}
	rest = rest[2:]
	end = strings.Index(rest, ")")
	if end < 0 {
		return 0, errBadFill
	}
	rightPair := rest[:end]
	lp := strings.Split(leftPair, ",")
	rp := strings.Split(rightPair, ",")
	if len(lp) != 2 || len(rp) != 2 {
		return 0, errBadFill
	}
	var err error
	if *x0, err = atoi(lp[0]); err != nil {
		return 0, err
	}
	if *y0, err = atoi(lp[1]); err != nil {
		return 0, err
	}
	if *x1, err = atoi(rp[0]); err != nil {
		return 0, err
	}
	if *y1, err = atoi(rp[1]); err != nil {
		return 0, err
	}
	return 4, nil
}
