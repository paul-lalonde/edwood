package frame

import (
	"image"
	"strings"
	"testing"

	"github.com/rjkroege/edwood/draw"
)

// Frame-side tests for the "Box" and "Spans" debug overlays.
//
// The mock display records `Draw` rect-fills as plain `fill (x0,y0)-(x1,y1)`
// without the color name (only `Bytes` glyph ops log `fill: <colorname>`),
// so these tests assert on rect shape — outline edges are exactly 1 pixel
// thick in one dimension — rather than scanning the op text for "Medblue".

// TestToggleBoxOutlines_FlipsState confirms ToggleBoxOutlines returns
// true on first call, false on second.
func TestToggleBoxOutlines_FlipsState(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 200, 200),
	}
	fr := setupFrame(t, iv)

	if got := fr.ToggleBoxOutlines(); got != true {
		t.Errorf("first toggle = %v, want true", got)
	}
	if got := fr.ToggleBoxOutlines(); got != false {
		t.Errorf("second toggle = %v, want false", got)
	}
}

// TestBoxOutlines_DrawsOutlinePerPaintedBox confirms that enabling
// outlines causes paintBox to emit four extra 1-pixel-thick fill ops
// per painted content box (top, bottom, left, right edges). We drive
// painting via Insert (which propagates outline state through bxscan
// to the nframe).
func TestBoxOutlines_DrawsOutlinePerPaintedBox(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 200, 60),
	}
	fr := setupFrame(t, iv)
	fr.ToggleBoxOutlines()

	g := gdo(t, fr)
	g.Clear()

	fr.Insert([]rune("abc"), 0)

	if got := countOutlineEdgeFills(g.DrawOps()); got < 4 {
		t.Errorf("expected ≥4 1-px-thick fills (one outlined box), got %d; ops:\n%s",
			got, strings.Join(g.DrawOps(), "\n"))
	}
}

// TestBoxOutlines_NoOutlinesWhenOff confirms paint produces no 1-px-thick
// fill ops when outlines are off (the default).
func TestBoxOutlines_NoOutlinesWhenOff(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 200, 60),
	}
	fr := setupFrame(t, iv)

	g := gdo(t, fr)
	g.Clear()
	fr.Insert([]rune("abc def"), 0)

	if n := countOutlineEdgeFills(g.DrawOps()); n != 0 {
		t.Errorf("expected 0 outline fills with feature off, got %d; ops:\n%s",
			n, strings.Join(g.DrawOps(), "\n"))
	}
}

// TestSetAfterPaintHook_FiresOnInsert confirms the hook fires once
// per Insert that actually merges content into f.box. The hook is
// the seam Text uses to paint the Spans overlay after each Insert
// during Text.fill (the repaint after a "Spans" toggle).
func TestSetAfterPaintHook_FiresOnInsert(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 200, 60),
	}
	fr := setupFrame(t, iv)

	calls := 0
	fr.SetAfterPaintHook(func() { calls++ })

	fr.Insert([]rune("hello"), 0)

	if calls != 1 {
		t.Errorf("hook fired %d times after Insert; want 1", calls)
	}
}

// TestSetAfterPaintHook_FiresOnRepaintBoxRange confirms that a hook
// registered via SetAfterPaintHook runs after a repaintBoxRange pass.
// SetStyleRange invokes repaintBoxRange on the parent frame directly.
func TestSetAfterPaintHook_FiresOnRepaintBoxRange(t *testing.T) {
	fr, _ := setupStyledFrame(t)
	fr.Insert([]rune("hello"), 0)

	calls := 0
	fr.SetAfterPaintHook(func() { calls++ })

	red, _ := fr.(*frameimpl).display.AllocImage(image.Rect(0, 0, 1, 1), fr.(*frameimpl).display.ScreenImage().Pix(), true, draw.Medblue)
	fr.SetStyleRange(0, 5, []StyleRun{{Len: 5, Style: Style{Kind: KindColored, Fg: red}}})

	if calls == 0 {
		t.Errorf("hook never fired after SetStyleRange (repaintBoxRange path)")
	}
}

// TestSetAfterPaintHook_Nil_NoFire confirms clearing the hook with nil
// stops further firings.
func TestSetAfterPaintHook_Nil_NoFire(t *testing.T) {
	fr, _ := setupStyledFrame(t)
	fr.Insert([]rune("hello"), 0)

	called := false
	fr.SetAfterPaintHook(func() { called = true })
	fr.SetAfterPaintHook(nil)

	red, _ := fr.(*frameimpl).display.AllocImage(image.Rect(0, 0, 1, 1), fr.(*frameimpl).display.ScreenImage().Pix(), true, draw.Medblue)
	fr.SetStyleRange(0, 5, []StyleRun{{Len: 5, Style: Style{Kind: KindColored, Fg: red}}})

	if called {
		t.Errorf("hook fired after being cleared with nil")
	}
}

// TestDrawOutlineRect_DrawsFourEdges confirms DrawOutlineRect emits
// exactly four 1-pixel-thick fill ops at the requested rect's edges.
func TestDrawOutlineRect_DrawsFourEdges(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 200, 60),
	}
	fr := setupFrame(t, iv)

	g := gdo(t, fr)
	g.Clear()

	display := fr.(*frameimpl).display
	col, err := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Medblue)
	if err != nil {
		t.Fatalf("alloc Medblue: %v", err)
	}

	fr.DrawOutlineRect(image.Rect(30, 20, 90, 50), col)

	if got := countOutlineEdgeFills(g.DrawOps()); got != 4 {
		t.Errorf("DrawOutlineRect emitted %d 1-px fills, want 4 (one per edge); ops:\n%s",
			got, strings.Join(g.DrawOps(), "\n"))
	}
}

// TestDrawOutlineRect_ClipsToRect confirms DrawOutlineRect intersects
// the requested rect with f.rect before drawing — a rect entirely
// outside the frame produces no ops.
func TestDrawOutlineRect_ClipsToRect(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 200, 60),
	}
	fr := setupFrame(t, iv)
	display := fr.(*frameimpl).display
	col, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Medblue)

	g := gdo(t, fr)
	g.Clear()
	fr.DrawOutlineRect(image.Rect(500, 500, 600, 600), col) // outside f.rect

	if n := len(g.DrawOps()); n != 0 {
		t.Errorf("expected 0 ops for outside-rect, got %d:\n%s",
			n, strings.Join(g.DrawOps(), "\n"))
	}
}

// countOutlineEdgeFills counts fill ops that are exactly 1 pixel
// thick in either dimension — the shape of a DrawOutlineRect edge.
// Returns 0 if no such ops appear in `ops`.
func countOutlineEdgeFills(ops []string) int {
	n := 0
	for _, op := range ops {
		if !strings.HasPrefix(op, "fill (") {
			continue
		}
		var x0, y0, x1, y1 int
		// "fill (x0,y0)-(x1,y1) ..." — parse the rect.
		if _, err := parseFillRect(op, &x0, &y0, &x1, &y1); err != nil {
			continue
		}
		if (x1-x0) == 1 || (y1-y0) == 1 {
			n++
		}
	}
	return n
}

// parseFillRect extracts the rect from "fill (x0,y0)-(x1,y1) ..."
// into the supplied pointers. Returns the number of items parsed.
func parseFillRect(op string, x0, y0, x1, y1 *int) (int, error) {
	op = strings.TrimPrefix(op, "fill ")
	op = strings.TrimSpace(op)
	open := strings.Index(op, "(")
	close := strings.Index(op, ")")
	if open < 0 || close < 0 || close <= open {
		return 0, errBadFill
	}
	left := op[open+1 : close]
	rest := op[close+1:]
	openR := strings.Index(rest, "(")
	closeR := strings.Index(rest, ")")
	if openR < 0 || closeR < 0 {
		return 0, errBadFill
	}
	right := rest[openR+1 : closeR]
	lp := strings.Split(left, ",")
	rp := strings.Split(right, ",")
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

var errBadFill = stringErr("bad fill op format")

type stringErr string

func (s stringErr) Error() string { return string(s) }

func atoi(s string) (int, error) {
	s = strings.TrimSpace(s)
	n := 0
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	if s == "" {
		return 0, errBadFill
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errBadFill
		}
		n = n*10 + int(c-'0')
	}
	if neg {
		n = -n
	}
	return n, nil
}
