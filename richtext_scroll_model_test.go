package main

import (
	"image"
	"testing"

	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/edwoodtest"
	"github.com/rjkroege/edwood/rich"
)

// richScrollModelHarness builds a RichText with overflowing content
// (long enough to require scrolling) for adapter tests.
func richScrollModelHarness(t *testing.T) *RichText {
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
		content = append(content, rich.Plain("line content")...)
	}
	rt.SetContent(content)
	rt.Render(image.Rect(0, 0, 400, 300))
	return rt
}

func TestRichScrollModel_GeometryAtFileTop(t *testing.T) {
	rt := richScrollModelHarness(t)
	rt.SetOrigin(0)
	m := &richScrollModel{rt: rt}

	totalPx, viewPx, originPx := m.Geometry()
	if totalPx <= 0 {
		t.Errorf("totalPx = %d, want > 0", totalPx)
	}
	if viewPx != rt.Frame().Rect().Dy() {
		t.Errorf("viewPx = %d, want %d (frame height)", viewPx, rt.Frame().Rect().Dy())
	}
	if originPx != 0 {
		t.Errorf("originPx at origin=0 = %d, want 0", originPx)
	}
}

func TestRichScrollModel_GeometryMidDocument(t *testing.T) {
	rt := richScrollModelHarness(t)
	// Set origin partway through content. Each "line content\n" is
	// 13 runes; line 10 starts at rune 130.
	rt.SetOrigin(rt.Frame().LineStartRunes()[10])
	m := &richScrollModel{rt: rt}

	_, _, originPx := m.Geometry()
	if originPx <= 0 {
		t.Errorf("originPx mid-doc = %d, want > 0", originPx)
	}
}

func TestRichScrollModel_DragPixelToTopAdvancesOrigin(t *testing.T) {
	rt := richScrollModelHarness(t)
	rt.SetOrigin(0)
	m := &richScrollModel{rt: rt}

	before := rt.Origin()
	// Click halfway down a 300-px viewport ≈ 150-px clickY.
	m.DragPixelToTop(150)
	after := rt.Origin()

	if after <= before {
		t.Errorf("DragPixelToTop should advance origin from %d, got %d", before, after)
	}
}

func TestRichScrollModel_DragTopToPixelRetreatsOrigin(t *testing.T) {
	rt := richScrollModelHarness(t)
	// Start mid-document so we have room to scroll back.
	rt.SetOrigin(rt.Frame().LineStartRunes()[15])
	m := &richScrollModel{rt: rt}

	before := rt.Origin()
	m.DragTopToPixel(100)
	after := rt.Origin()

	if after >= before {
		t.Errorf("DragTopToPixel should retreat origin from %d, got %d", before, after)
	}
}

func TestRichScrollModel_JumpToFractionSetsRoughPosition(t *testing.T) {
	rt := richScrollModelHarness(t)
	rt.SetOrigin(0)
	m := &richScrollModel{rt: rt}

	m.JumpToFraction(0.0)
	if got := rt.Origin(); got != 0 {
		t.Errorf("JumpToFraction(0.0): origin = %d, want 0", got)
	}

	rt.SetOrigin(0)
	m.JumpToFraction(0.5)
	mid := rt.Origin()
	if mid <= 0 {
		t.Errorf("JumpToFraction(0.5): origin = %d, want > 0", mid)
	}
	contentLen := rt.Frame().LineStartRunes()
	maxOrigin := contentLen[len(contentLen)-1]
	if mid >= maxOrigin {
		t.Errorf("JumpToFraction(0.5): origin = %d, want < %d (max)", mid, maxOrigin)
	}
}

func TestRichScrollModel_NilFrameSafe(t *testing.T) {
	rt := NewRichText()
	// rt.frame is nil before Init.
	m := &richScrollModel{rt: rt}

	totalPx, viewPx, originPx := m.Geometry()
	if totalPx != 0 || viewPx != 0 || originPx != 0 {
		t.Errorf("nil-frame Geometry = (%d, %d, %d), want (0, 0, 0)",
			totalPx, viewPx, originPx)
	}
	// Drag/jump methods must not panic.
	m.DragTopToPixel(50)
	m.DragPixelToTop(50)
	m.JumpToFraction(0.5)
}

// TestRichScrollModel_B1B3Inverse pins the same exact-inverse
// property the user verified visually for ScrollClick on gappy
// content; here we exercise it through the ScrollModel adapter
// methods (which the Phase 3 unified widget will call).
func TestRichScrollModel_B1B3Inverse(t *testing.T) {
	rt := richScrollModelHarness(t)
	startOrigin := rt.Frame().LineStartRunes()[10]
	rt.SetOrigin(startOrigin)
	rt.SetOriginYOffset(0)
	m := &richScrollModel{rt: rt}

	// B3 forward, then B1 back at the same clickY.
	m.DragPixelToTop(120)
	if rt.Origin() == startOrigin {
		t.Fatal("DragPixelToTop should advance origin")
	}
	m.DragTopToPixel(120)
	if rt.Origin() != startOrigin {
		t.Errorf("after B1+B3 inverse pair: origin = %d, want %d (round-trip exact)",
			rt.Origin(), startOrigin)
	}
}
