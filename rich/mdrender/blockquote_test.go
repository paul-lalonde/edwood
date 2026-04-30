package mdrender

import (
	"image"
	"strings"
	"testing"

	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/edwoodtest"
	"github.com/rjkroege/edwood/rich"
)

// blockquoteContentDepth1 returns a small content blob with one
// blockquote-styled span at depth 1. Used by the tests below.
func blockquoteContentDepth1() rich.Content {
	return rich.Content{
		{Text: "quoted text", Style: rich.Style{Blockquote: true, BlockquoteDepth: 1, Scale: 1.0}},
	}
}

// blockquoteContentDepth3 returns content with one blockquote-styled
// span at depth 3 (max-nesting test for the depth iteration).
func blockquoteContentDepth3() rich.Content {
	return rich.Content{
		{Text: "deep", Style: rich.Style{Blockquote: true, BlockquoteDepth: 3, Scale: 1.0}},
	}
}

// buildFrameWithContent builds a rich.Frame on the supplied display
// with the supplied content. Mirrors the renderer_test.go helper but
// takes content as a parameter.
func buildFrameWithContent(t *testing.T, d draw.Display, rect image.Rectangle, content rich.Content) rich.Frame {
	t.Helper()
	font := edwoodtest.NewFont(10, 14)
	bg, _ := d.AllocImage(image.Rect(0, 0, 1, 1), d.ScreenImage().Pix(), true, 0xFFFFFFFF)
	fg, _ := d.AllocImage(image.Rect(0, 0, 1, 1), d.ScreenImage().Pix(), true, 0x000000FF)
	f := rich.NewFrame()
	f.Init(rect,
		rich.WithDisplay(d),
		rich.WithFont(font),
		rich.WithBackground(bg),
		rich.WithTextColor(fg),
	)
	f.SetContent(content)
	return f
}

// countBarLikeFillOps counts draw operations that fill a 2-pixel-wide
// rectangle. The blockquote bar is the only painter that emits
// 2px-wide rect fills in this rendering path; matching by width
// keeps the assertion robust against changes in color formatting.
func countBarLikeFillOps(ops []string) int {
	n := 0
	for _, op := range ops {
		if !strings.HasPrefix(op, "fill") && !strings.Contains(op, "<- draw r:") {
			continue
		}
		// Op format examples (see edwoodtest/draw.go):
		//   "fill (x0,y0)-(x1,y1) <name>"
		//   "<dst> <- draw r: (x0,y0)-(x1,y1) src: <s> mask <m> p1: <p>"
		// Width = (x1 - x0). Look for a rect with width 2.
		if hasFillRectWidth(op, 2) {
			n++
		}
	}
	return n
}

// hasFillRectWidth scans an op string for a "(x0,y0)-(x1,y1)"
// substring and returns true if (x1 - x0) equals the supplied
// width. Used by the bar-counting heuristic in the tests above.
// Conservative: returns false on any parse failure.
func hasFillRectWidth(op string, want int) bool {
	// Find first occurrence of "(x,y)-(x,y)".
	open1 := strings.Index(op, "(")
	if open1 < 0 {
		return false
	}
	dash := strings.Index(op[open1:], ")-(")
	if dash < 0 {
		return false
	}
	close2 := strings.Index(op[open1+dash+3:], ")")
	if close2 < 0 {
		return false
	}
	first := op[open1+1 : open1+dash]   // "x0,y0"
	second := op[open1+dash+3 : open1+dash+3+close2] // "x1,y1"
	x0, ok := parseFirstInt(first)
	if !ok {
		return false
	}
	x1, ok := parseFirstInt(second)
	if !ok {
		return false
	}
	return x1-x0 == want
}

func parseFirstInt(s string) (int, bool) {
	comma := strings.Index(s, ",")
	if comma < 0 {
		return 0, false
	}
	n := 0
	neg := false
	for i, c := range s[:comma] {
		if i == 0 && c == '-' {
			neg = true
			continue
		}
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	if neg {
		n = -n
	}
	return n, true
}

// TestRendererPaintsBlockquoteBar covers R4 / R5 of
// rich/mdrender/blockquote.design.md: when the wrapped frame's
// content contains a blockquote-styled span, Renderer.Redraw
// produces a 2px-wide vertical-bar fill operation. Frame.Redraw
// alone produces zero such ops (R1 — bar painting moved out of
// rich.Frame).
func TestRendererPaintsBlockquoteBar(t *testing.T) {
	rect := image.Rect(0, 0, 200, 200)
	d := edwoodtest.NewDisplay(rect)
	f := buildFrameWithContent(t, d, rect, blockquoteContentDepth1())
	r := New(f, d)

	// Direct frame.Redraw must NOT produce bar-shaped ops.
	d.(edwoodtest.GettableDrawOps).Clear()
	f.Redraw()
	if got := countBarLikeFillOps(d.(edwoodtest.GettableDrawOps).DrawOps()); got != 0 {
		t.Errorf("frame.Redraw alone produced %d bar-like fill ops; want 0 (paint must have moved to wrapper)", got)
	}

	// Renderer.Redraw must produce at least one bar (depth 1 → one bar).
	d.(edwoodtest.GettableDrawOps).Clear()
	r.Redraw()
	got := countBarLikeFillOps(d.(edwoodtest.GettableDrawOps).DrawOps())
	if got < 1 {
		t.Errorf("Renderer.Redraw on depth-1 blockquote produced %d bar-like fill ops; want >= 1", got)
	}
}

// TestRendererPaintsOneBarPerDepthLevel covers R5: depth N →
// exactly N bars per visible blockquote line.
func TestRendererPaintsOneBarPerDepthLevel(t *testing.T) {
	rect := image.Rect(0, 0, 400, 200)
	d := edwoodtest.NewDisplay(rect)
	f := buildFrameWithContent(t, d, rect, blockquoteContentDepth3())
	r := New(f, d)

	d.(edwoodtest.GettableDrawOps).Clear()
	r.Redraw()
	got := countBarLikeFillOps(d.(edwoodtest.GettableDrawOps).DrawOps())
	// One bar per level (1, 2, 3) on the single line of content. The
	// counting heuristic catches 2px-wide rect fills; depth 3 means
	// 3 bars stacked at successive indents.
	if got < 3 {
		t.Errorf("Renderer.Redraw on depth-3 blockquote produced %d bar-like fill ops; want >= 3", got)
	}
}

// TestRendererPaintsNoBarsForNonBlockquoteContent covers R6:
// content without blockquote styling produces no bar drawing,
// even after going through the wrapper.
func TestRendererPaintsNoBarsForNonBlockquoteContent(t *testing.T) {
	rect := image.Rect(0, 0, 200, 200)
	d := edwoodtest.NewDisplay(rect)
	f := buildFrameWithContent(t, d, rect, rich.Plain("plain text, no quote"))
	r := New(f, d)

	d.(edwoodtest.GettableDrawOps).Clear()
	r.Redraw()
	if got := countBarLikeFillOps(d.(edwoodtest.GettableDrawOps).DrawOps()); got != 0 {
		t.Errorf("Renderer.Redraw on non-blockquote content produced %d bar-like fill ops; want 0", got)
	}
}
