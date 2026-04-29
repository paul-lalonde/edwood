package main

import (
	"image"
	"testing"

	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/edwoodtest"
	"github.com/rjkroege/edwood/rich"
)

// scrollSnapHarness builds a RichText with content larger than the
// viewport so scroll actions actually move the origin. Returns the
// RichText and the resulting scrollbar rectangle so tests can pick
// click points.
func scrollSnapHarness(t *testing.T) (*RichText, image.Rectangle) {
	t.Helper()
	displayRect := image.Rect(0, 0, 800, 600)
	display := edwoodtest.NewDisplay(displayRect)
	font := edwoodtest.NewFont(10, 14)
	bg, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.White)
	text, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Black)
	scrBg, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Palebluegreen)
	scrThumb, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Medblue)

	rt := NewRichText()
	rt.Init(display, font,
		WithRichTextBackground(bg),
		WithRichTextColor(text),
		WithScrollbarColors(scrBg, scrThumb),
	)
	var content rich.Content
	for i := 0; i < 30; i++ {
		if i > 0 {
			content = append(content, rich.Plain("\n")...)
		}
		content = append(content, rich.Plain("line")...)
	}
	rt.SetContent(content)
	rt.Render(image.Rect(0, 0, 400, 300))
	return rt, rt.ScrollRect()
}

// TestScrollClickB1SetsSnapBottom: B1 reveals earlier content; the
// last line of the new viewport should anchor cleanly at the
// bottom. The snap is set on the underlying frame before the
// origin update.
func TestScrollClickB1SetsSnapBottom(t *testing.T) {
	rt, scrollRect := scrollSnapHarness(t)
	rt.SetOrigin(75) // mid-document
	rt.Frame().SetScrollSnap(rich.SnapTop)
	rt.ScrollClick(1, image.Pt(scrollRect.Min.X+5, scrollRect.Min.Y+50))
	if got := rt.Frame().ScrollSnap(); got != rich.SnapBottom {
		t.Errorf("after B1 click: ScrollSnap = %v, want SnapBottom", got)
	}
}

// TestScrollClickB3SetsSnapTop: B3 advances through content; the
// new top line should be a clean line edge.
func TestScrollClickB3SetsSnapTop(t *testing.T) {
	rt, scrollRect := scrollSnapHarness(t)
	rt.SetOrigin(0)
	rt.Frame().SetScrollSnap(rich.SnapBottom)
	rt.ScrollClick(3, image.Pt(scrollRect.Min.X+5, scrollRect.Min.Y+50))
	if got := rt.Frame().ScrollSnap(); got != rich.SnapTop {
		t.Errorf("after B3 click: ScrollSnap = %v, want SnapTop", got)
	}
}

// TestScrollClickB2SetsSnapTop: B2 jumps to an arbitrary fraction;
// default to a clean top edge for predictability.
func TestScrollClickB2SetsSnapTop(t *testing.T) {
	rt, scrollRect := scrollSnapHarness(t)
	rt.SetOrigin(0)
	rt.Frame().SetScrollSnap(rich.SnapBottom)
	middle := (scrollRect.Min.Y + scrollRect.Max.Y) / 2
	rt.ScrollClick(2, image.Pt(scrollRect.Min.X+5, middle))
	if got := rt.Frame().ScrollSnap(); got != rich.SnapTop {
		t.Errorf("after B2 click: ScrollSnap = %v, want SnapTop", got)
	}
}

// TestScrollClickB1MagnitudeScalesWithClickY pins down the corrected
// B1 semantic: clicking near the top of the scrollbar should produce
// a small scroll-back; clicking near the bottom should produce a
// large scroll-back. This mirrors acme text-mode B1
// (BackNL(t.org, clickY/fontH)) and makes B1/B3 inverse operations
// at any click position.
//
// A previous implementation used (1.0 - clickProportion) which
// inverted this relationship; clicking near the top produced a
// full-screen scroll-back. The user observed this as "B1 doesn't
// behave as the inverse of B3" in rich-text mode.
func TestScrollClickB1MagnitudeScalesWithClickY(t *testing.T) {
	rt, scrollRect := scrollSnapHarness(t)
	rt.SetOrigin(75) // mid-document, with room to scroll back
	originAtMid := rt.Origin()

	// Click near the TOP of the scrollbar -> small scroll back.
	rt.SetOrigin(originAtMid)
	rt.ScrollClick(1, image.Pt(scrollRect.Min.X+5, scrollRect.Min.Y+1))
	smallBack := originAtMid - rt.Origin()

	// Click near the BOTTOM of the scrollbar -> large scroll back.
	rt.SetOrigin(originAtMid)
	rt.ScrollClick(1, image.Pt(scrollRect.Min.X+5, scrollRect.Max.Y-1))
	largeBack := originAtMid - rt.Origin()

	if smallBack < 0 || largeBack < 0 {
		t.Fatalf("expected origin to decrease in both cases, got smallBack=%d largeBack=%d",
			smallBack, largeBack)
	}
	if smallBack >= largeBack {
		t.Errorf("B1 near top should scroll back LESS than B1 near bottom; "+
			"got smallBack=%d largeBack=%d (formula likely inverted)",
			smallBack, largeBack)
	}
}

// gappyScrollHarness builds a RichText whose content has heading
// boxes (Scale > 1.0). Layout adds half-fontH spacing before each
// heading, so TotalDocumentHeight > sum(LinePixelHeights). This is
// the content shape that exposes the gap-mismatch bug: scrolls
// computed in lineHeights-only space drift relative to what the
// user sees in screen-space.
func gappyScrollHarness(t *testing.T) (*RichText, image.Rectangle) {
	t.Helper()
	displayRect := image.Rect(0, 0, 800, 600)
	display := edwoodtest.NewDisplay(displayRect)
	font := edwoodtest.NewFont(10, 14)
	bg, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.White)
	text, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Black)
	scrBg, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Palebluegreen)
	scrThumb, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Medblue)

	rt := NewRichText()
	rt.Init(display, font,
		WithRichTextBackground(bg),
		WithRichTextColor(text),
		WithScrollbarColors(scrBg, scrThumb),
	)
	// Mixed body / heading content. Headings (Scale > 1.0) trigger
	// half-fontH spacing in the layout (rich/layout.go:585). 20
	// iterations produces ~980 px of content, comfortably more than
	// the 300-px viewport.
	var content rich.Content
	for i := 0; i < 20; i++ {
		content = append(content, rich.Span{Text: "body line", Style: rich.Style{Scale: 1.0}})
		content = append(content, rich.Span{Text: "\n", Style: rich.Style{Scale: 1.0}})
		content = append(content, rich.Span{Text: "Heading", Style: rich.Style{Scale: 2.0, Bold: true}})
		content = append(content, rich.Span{Text: "\n", Style: rich.Style{Scale: 2.0}})
	}
	rt.SetContent(content)
	rt.Render(image.Rect(0, 0, 400, 300))
	return rt, rt.ScrollRect()
}

// TestScrollClick_GapsExistInHarness sanity-checks that the harness
// does produce content with inter-line gaps. If this regresses (e.g.
// the layout stops adding heading spacing) the inverse-B1/B3 test
// below would silently lose its discriminating power.
func TestScrollClick_GapsExistInHarness(t *testing.T) {
	rt, _ := gappyScrollHarness(t)
	heights := rt.Frame().LinePixelHeights()
	total := rt.Frame().TotalDocumentHeight()
	sumH := 0
	for _, h := range heights {
		sumH += h
	}
	if total <= sumH {
		t.Errorf("expected gaps in harness (total %d > sum-heights %d); "+
			"layout may have stopped adding heading spacing — harness "+
			"is no longer testing what it claims", total, sumH)
	}
}

// TestScrollClick_B1B3InverseOnGappyContent is the regression test
// for the gap-mismatch bug: B3 forward then B1 back at the same
// click position must return the viewport to its original state.
//
// The previous implementation computed scroll deltas in
// lineHeights-only space (`pixelToLineOffset(currentPixelY + clickY,
// lineHeights)`) but clickY was in screen pixels (which include
// inter-line gaps for paragraph and heading spacing). For each gap
// the user crossed during a scroll, the math advanced one extra line
// — B3-then-B1 round-trips drifted, and the click line wasn't the
// line the user actually saw at the click point.
//
// The fix introduces rich.Frame.LinePixelYs() (gap-aware Y values)
// and routes all scroll-click math through document-rendered Y space.
func TestScrollClick_B1B3InverseOnGappyContent(t *testing.T) {
	rt, scrollRect := gappyScrollHarness(t)
	frame := rt.Frame()
	if frame.TotalDocumentHeight() <= frame.Rect().Dy() {
		t.Fatalf("test fixture must produce overflowing content "+
			"(total %d > frame %d) for scroll-click to do anything",
			frame.TotalDocumentHeight(), frame.Rect().Dy())
	}

	// Start from a mid-document state (not origin=0) so the file-top
	// override in applyScrollSnap doesn't mask any drift in B1's
	// SnapBottom path. Find a line index that's well past the first
	// viewport-worth of content.
	lineStarts := frame.LineStartRunes()
	if len(lineStarts) < 10 {
		t.Fatalf("test fixture should have many lines; got %d", len(lineStarts))
	}
	startOrigin := lineStarts[len(lineStarts)/2] // mid-document
	rt.SetOrigin(startOrigin)
	rt.SetOriginYOffset(0)
	if rt.Origin() != startOrigin {
		t.Fatalf("SetOrigin precondition failed: got %d", rt.Origin())
	}

	// Click at mid-scrollbar, B3 forward then B1 back to round-trip.
	clickPt := image.Pt(scrollRect.Min.X+5,
		(scrollRect.Min.Y+scrollRect.Max.Y)/2)

	rt.ScrollClick(3, clickPt)
	if rt.Origin() == startOrigin {
		t.Fatalf("B3 should have advanced origin past %d; stayed at %d",
			startOrigin, rt.Origin())
	}
	originAfterB3 := rt.Origin()
	offsetAfterB3 := rt.GetOriginYOffset()

	rt.ScrollClick(1, clickPt)
	if rt.Origin() != startOrigin {
		t.Errorf("B1 after B3 with same click point: origin = %d, want %d "+
			"(B1/B3 must be exact inverses, even with inter-line gaps). "+
			"After B3: origin=%d offset=%d.",
			rt.Origin(), startOrigin, originAfterB3, offsetAfterB3)
	}
	if rt.GetOriginYOffset() != 0 {
		t.Errorf("B1 after B3: GetOriginYOffset = %d, want 0",
			rt.GetOriginYOffset())
	}
}

// TestScrollClick_RoundTripFromTopLandsAtTop is the integration-
// level regression test for the user-reported bug. Starting at
// origin=0 (first line at top), B3 forward then B1 back to
// origin=0 must leave the first line at Y=0. Without the fix,
// SnapBottom from B1 would shift the first line up by the residual
// frameHeight%lineHeight.
func TestScrollClick_RoundTripFromTopLandsAtTop(t *testing.T) {
	rt, scrollRect := scrollSnapHarness(t)
	rt.SetOrigin(0)

	// B3 forward then B1 back at the SAME click position. With B3's
	// round-down + B1's round-up line mapping, this is the
	// canonical inverse-pair operation; round-trip is exact even
	// when the click position falls between line boundaries. (See
	// scrollClickAt for the asymmetric-rounding rationale.)
	clickPt := image.Pt(scrollRect.Min.X+5, scrollRect.Min.Y+100)
	rt.ScrollClick(3, clickPt)
	if rt.Origin() == 0 {
		t.Fatal("B3 should have advanced origin past 0")
	}
	rt.ScrollClick(1, clickPt)

	// Re-render to compute layout after the round-trip.
	rt.Render(image.Rect(0, 0, 400, 300))

	// First line must be at Y=0 — the file-top override forces
	// SnapTop, defeating any SnapBottom set by the last B1.
	fi := rt.Frame()
	if got := fi.GetOriginYOffset(); got != 0 {
		t.Errorf("after round-trip to origin=0: GetOriginYOffset = %d, want 0", got)
	}
	if rt.Origin() != 0 {
		t.Errorf("after round-trip: Origin = %d, want 0", rt.Origin())
	}
	if starts := fi.LineStartRunes(); len(starts) == 0 || starts[0] != 0 {
		t.Errorf("LineStartRunes[0] = %v, want 0 (file top)", starts)
	}
}
