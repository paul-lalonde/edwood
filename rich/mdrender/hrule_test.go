package mdrender

import (
	"image"
	"strings"
	"testing"

	"github.com/rjkroege/edwood/edwoodtest"
	"github.com/rjkroege/edwood/rich"
)

// hruleContent returns a small content blob with one horizontal-rule
// span. Used by the tests below.
func hruleContent() rich.Content {
	return rich.Content{
		{Text: "before\n", Style: rich.Style{Scale: 1.0}},
		{Text: "rule", Style: rich.Style{HRule: true, Scale: 1.0}},
		{Text: "\nafter", Style: rich.Style{Scale: 1.0}},
	}
}

// countRuleLikeFillOps counts draw operations that fill a rectangle
// exactly 1 pixel tall (height = y1 - y0 == 1). The horizontal rule
// is the only painter that emits 1px-tall rect fills in this
// rendering path; matching by height keeps the assertion robust
// against changes in color formatting.
func countRuleLikeFillOps(ops []string) int {
	n := 0
	for _, op := range ops {
		if !strings.HasPrefix(op, "fill") && !strings.Contains(op, "<- draw r:") {
			continue
		}
		if hasFillRectHeight(op, 1) {
			n++
		}
	}
	return n
}

// hasFillRectHeight scans an op string for a "(x0,y0)-(x1,y1)"
// substring and returns true if (y1 - y0) equals the supplied
// height. Mirror of hasFillRectWidth in blockquote_test.go.
// Conservative: returns false on any parse failure.
func hasFillRectHeight(op string, want int) bool {
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
	first := op[open1+1 : open1+dash]                // "x0,y0"
	second := op[open1+dash+3 : open1+dash+3+close2] // "x1,y1"
	y0, ok := parseSecondInt(first)
	if !ok {
		return false
	}
	y1, ok := parseSecondInt(second)
	if !ok {
		return false
	}
	return y1-y0 == want
}

func parseSecondInt(s string) (int, bool) {
	comma := strings.Index(s, ",")
	if comma < 0 {
		return 0, false
	}
	t := s[comma+1:]
	n := 0
	neg := false
	for i, c := range t {
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

// TestRendererPaintsHRule covers R3 / R4 of
// rich/mdrender/hrule.design.md: when the wrapped frame's content
// contains an HRule-styled span, Renderer.Redraw produces a 1px-tall
// rule fill. Frame.Redraw alone produces no such op (R1 — paint
// moved out of rich.Frame).
func TestRendererPaintsHRule(t *testing.T) {
	rect := image.Rect(0, 0, 200, 200)
	d := edwoodtest.NewDisplay(rect)
	f := buildFrameWithContent(t, d, rect, hruleContent())
	r := New(f, d)

	// Direct frame.Redraw must NOT produce rule-shaped ops.
	d.(edwoodtest.GettableDrawOps).Clear()
	f.Redraw()
	if got := countRuleLikeFillOps(d.(edwoodtest.GettableDrawOps).DrawOps()); got != 0 {
		t.Errorf("frame.Redraw alone produced %d rule-like fill ops; want 0 (paint must have moved to wrapper)", got)
	}

	// Renderer.Redraw must produce at least one rule.
	d.(edwoodtest.GettableDrawOps).Clear()
	r.Redraw()
	if got := countRuleLikeFillOps(d.(edwoodtest.GettableDrawOps).DrawOps()); got < 1 {
		t.Errorf("Renderer.Redraw on HRule content produced %d rule-like fill ops; want >= 1", got)
	}
}

// TestRendererPaintsHRuleAtFullWidth covers R4: the rule spans the
// full frame width (X from frameRect.Min.X to frameRect.Max.X).
// Mirrors the test that used to live as TestHorizontalRuleFullWidth
// in rich/frame_test.go before the paint move.
func TestRendererPaintsHRuleAtFullWidth(t *testing.T) {
	rect := image.Rect(0, 0, 500, 300)
	d := edwoodtest.NewDisplay(rect)
	font := edwoodtest.NewFont(10, 14)
	bgImage := edwoodtest.NewImage(d, "frame-background", image.Rect(0, 0, 1, 1))
	textImage := edwoodtest.NewImage(d, "text-color", image.Rect(0, 0, 1, 1))

	f := rich.NewFrame()
	f.Init(rect,
		rich.WithDisplay(d),
		rich.WithFont(font),
		rich.WithBackground(bgImage),
		rich.WithTextColor(textImage),
	)
	f.SetContent(rich.Content{
		{Text: string(rich.HRuleRune) + "\n", Style: rich.StyleHRule},
	})
	r := New(f, d)

	d.(edwoodtest.GettableDrawOps).Clear()
	r.Redraw()
	ops := d.(edwoodtest.GettableDrawOps).DrawOps()

	// The frame's background fill spans the full rect; ignore it.
	// The rule fill should span x ∈ [0, 500) at some y, with
	// height 1.
	frameBackgroundRect := "(0,0)-(500,300)"
	foundFullWidthLine := false
	for _, op := range ops {
		if strings.Contains(op, frameBackgroundRect) {
			continue
		}
		if !strings.HasPrefix(op, "fill") && !strings.Contains(op, "<- draw r:") {
			continue
		}
		if !hasFillRectHeight(op, 1) {
			continue
		}
		// Look for a fill spanning x=0..500.
		if strings.Contains(op, "(0,") && strings.Contains(op, "-(500,") {
			foundFullWidthLine = true
		}
	}
	if !foundFullWidthLine {
		t.Errorf("Renderer.Redraw did not paint a full-width (500px) rule\nops: %v", ops)
	}
}

// TestRendererPaintsNoRulesForNonHRuleContent covers R5: content
// without HRule styling produces no rule drawing.
func TestRendererPaintsNoRulesForNonHRuleContent(t *testing.T) {
	rect := image.Rect(0, 0, 200, 200)
	d := edwoodtest.NewDisplay(rect)
	f := buildFrameWithContent(t, d, rect, rich.Plain("plain text, no rule"))
	r := New(f, d)

	d.(edwoodtest.GettableDrawOps).Clear()
	r.Redraw()
	if got := countRuleLikeFillOps(d.(edwoodtest.GettableDrawOps).DrawOps()); got != 0 {
		t.Errorf("Renderer.Redraw on non-HRule content produced %d rule-like fill ops; want 0", got)
	}
}
