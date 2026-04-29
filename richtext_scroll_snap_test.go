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

// TestScrollClick_RoundTripFromTopLandsAtTop is the integration-
// level regression test for the user-reported bug. Starting at
// origin=0 (first line at top), B3 forward then B1 back to
// origin=0 must leave the first line at Y=0. Without the fix,
// SnapBottom from B1 would shift the first line up by the residual
// frameHeight%lineHeight.
func TestScrollClick_RoundTripFromTopLandsAtTop(t *testing.T) {
	rt, scrollRect := scrollSnapHarness(t)
	rt.SetOrigin(0)

	// B3 forward.
	rt.ScrollClick(3, image.Pt(scrollRect.Min.X+5, scrollRect.Min.Y+100))
	if rt.Origin() == 0 {
		t.Fatal("B3 should have advanced origin past 0")
	}

	// B1 back to origin=0. We may need multiple B1 clicks to fully
	// return, depending on viewport/click geometry. Loop until at
	// origin=0 (or bail after a sane number of attempts).
	for i := 0; i < 20 && rt.Origin() > 0; i++ {
		rt.ScrollClick(1, image.Pt(scrollRect.Min.X+5, scrollRect.Min.Y+1))
	}
	if rt.Origin() != 0 {
		t.Fatalf("could not return to origin=0 via B1 clicks; stuck at %d", rt.Origin())
	}

	// At origin=0, the first line must be at Y=0 — the file-top
	// override should kick in and force SnapTop, defeating any
	// SnapBottom set by the last B1.
	fi := rt.Frame()
	// Re-render to compute layout after the scroll round-trip.
	rt.Render(image.Rect(0, 0, 400, 300))
	if got := fi.GetOriginYOffset(); got != 0 {
		t.Errorf("after round-trip to origin=0: GetOriginYOffset = %d, want 0", got)
	}
	// The frame's layout should produce first line at Y=0.
	// Read via LinePixelHeights to confirm we're showing line 0:
	// LineStartRunes()[0] must be 0 (we're at the start of content).
	if starts := fi.LineStartRunes(); len(starts) == 0 || starts[0] != 0 {
		t.Errorf("LineStartRunes[0] = %v, want 0 (file top)", starts)
	}
}
