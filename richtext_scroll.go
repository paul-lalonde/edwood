package main

import "github.com/rjkroege/edwood/rich"

// richScrollModel adapts *RichText to the ScrollModel interface that
// the shared Scrollbar widget consumes. This is the rich-text mode
// counterpart of textScrollModel, completing the unification: both
// modes drive the same widget.
//
// All math is done in document-rendered Y space (line.Y values from
// the layout, which include inter-line gaps for paragraph/heading
// spacing and horizontal-scrollbar adjustments). This makes B1 and
// B3 exact inverses for any clickY, and makes B3 land the visually-
// clicked line at the viewport top — even on documents with mixed
// line heights or significant gaps between lines.
//
// Snap policy (see unified-scrollbar.md § "Scroll snap policy"):
//
//	B1 reveals earlier content -> SnapBottom (anchor bottom line).
//	B3 reveals later content   -> SnapTop (anchor new top line).
//	B2 jumps arbitrarily       -> SnapTop for predictability.
//
// SetOrigin/SetOriginYOffset reset snap to SnapTop, so SetScrollSnap
// must be called AFTER them.
type richScrollModel struct {
	rt *RichText
}

// Geometry returns (totalDocHeight, frameHeight, originDocY) — all
// in document-rendered pixel space (post-scrollbar adjustment, with
// inter-line gaps). The widget uses these proportionally for thumb
// position and size.
func (m *richScrollModel) Geometry() (totalPx, viewPx, originPx int) {
	rt := m.rt
	if rt == nil || rt.frame == nil {
		return 0, 0, 0
	}
	fr := rt.frame
	totalPx = fr.TotalDocumentHeight()
	viewPx = fr.Rect().Dy()

	lineYs := fr.LinePixelYs()
	lineStarts := fr.LineStartRunes()
	if len(lineYs) > 0 && len(lineStarts) > 0 {
		currentLine := findLineForOrigin(rt.Origin(), lineStarts)
		if currentLine < len(lineYs) {
			originPx = lineYs[currentLine] + rt.GetOriginYOffset()
		}
	}
	return totalPx, viewPx, originPx
}

// DragTopToPixel implements B1 (drag-the-top-line-down-to-clickY).
func (m *richScrollModel) DragTopToPixel(clickY int) {
	m.scrollByClickY(1, clickY)
}

// DragPixelToTop implements B3 (drag-the-line-here-up-to-the-top).
func (m *richScrollModel) DragPixelToTop(clickY int) {
	m.scrollByClickY(3, clickY)
}

// JumpToFraction implements B2 (jump to fraction of total document).
func (m *richScrollModel) JumpToFraction(f float64) {
	rt := m.rt
	if rt == nil || rt.frame == nil {
		return
	}
	scrollHeight := rt.lastScrollRect.Dy()
	if scrollHeight <= 0 {
		return
	}
	clickY := int(f * float64(scrollHeight))
	if clickY < 0 {
		clickY = 0
	}
	if clickY > scrollHeight {
		clickY = scrollHeight
	}
	m.scrollByClickY(2, clickY)
}

// scrollByClickY computes a new origin from button + clickY (in
// scrollbar-relative pixels) and applies it to the underlying
// rich.Frame. Encapsulates the gap-aware math (LinePixelYs,
// asymmetric line-boundary rounding) and the snap policy.
//
// Returns the new rune origin so that the legacy ScrollClick API
// can preserve its return type.
func (m *richScrollModel) scrollByClickY(button, clickY int) int {
	rt := m.rt
	if rt == nil || rt.content == nil || rt.frame == nil {
		return 0
	}
	if rt.content.Len() == 0 {
		return 0
	}

	lineHeights := rt.frame.LinePixelHeights()
	lineYs := rt.frame.LinePixelYs()
	lineStarts := rt.frame.LineStartRunes()
	if len(lineHeights) == 0 || len(lineYs) != len(lineHeights) {
		return 0
	}

	totalPixelHeight := rt.frame.TotalDocumentHeight()
	frameHeight := rt.frame.Rect().Dy()
	if totalPixelHeight <= frameHeight {
		return 0
	}

	scrollHeight := rt.lastScrollRect.Dy()
	if scrollHeight <= 0 {
		return rt.Origin()
	}
	if clickY < 0 {
		clickY = 0
	}
	if clickY > scrollHeight {
		clickY = scrollHeight
	}
	clickProportion := float64(clickY) / float64(scrollHeight)

	currentOrigin := rt.Origin()
	currentLine := findLineForOrigin(currentOrigin, lineStarts)
	currentTopDocY := lineYs[currentLine] + rt.GetOriginYOffset()

	// targetTopDocY is the document-rendered Y where the new
	// viewport top should sit. No upper clamp: B3 / B2 may set an
	// origin past the "last-line-at-bottom" position so the last
	// line ends up at the viewport top with empty space below
	// (matches acme's text-mode B3, never clamped). A previous
	// upper clamp broke B1/B3 round-trip exactness.
	var targetTopDocY int
	var snap rich.ScrollSnap
	switch button {
	case 1:
		targetTopDocY = currentTopDocY - clickY
		if targetTopDocY < 0 {
			targetTopDocY = 0
		}
		snap = rich.SnapBottom
	case 2:
		targetTopDocY = int(float64(totalPixelHeight) * clickProportion)
		if targetTopDocY < 0 {
			targetTopDocY = 0
		}
		snap = rich.SnapTop
	case 3:
		targetTopDocY = currentTopDocY + clickY
		snap = rich.SnapTop
	default:
		return rt.Origin()
	}

	// B1 uses round-up line mapping (smallest M with lineYs[M] >=
	// target), B3 and B2 use round-down (largest L with lineYs[L]
	// <= target). The asymmetry is what makes B3+B1 round-trips
	// exact on text-only documents: B3 may snap *down* to a line
	// boundary, leaving a residual; B1 then rounds *up* across the
	// same residual to land back on the original line.
	//
	// For tall lines (images, large code blocks) the asymmetry is
	// wrong for B1: round-up would skip past the image to the line
	// below, leaving the image unreachable as a viewport top. When
	// targetTopDocY falls within a tall line, override with a
	// sub-line offset on that line. For B3/B2 this override is a
	// no-op (round-down already lands inside the tall line).
	fontH := rt.frame.DefaultFontHeight()
	var newLine, newOffset int
	if line, off, ok := landWithinTallLine(targetTopDocY, lineYs, lineHeights, fontH); ok {
		newLine, newOffset = line, off
	} else if button == 1 {
		newLine, newOffset = lineAtDocYRoundUp(targetTopDocY, lineYs)
	} else {
		newLine, newOffset = lineAtDocY(targetTopDocY, lineYs, lineHeights)
	}
	newOffset = snapOffset(newLine, newOffset, fontH, lineHeights)

	newOrigin := lineStarts[newLine]
	rt.frame.SetOrigin(newOrigin)
	rt.frame.SetOriginYOffset(newOffset)
	rt.frame.SetScrollSnap(snap)
	// Repaint the frame body so each latch tick is visible. Text
	// mode gets this for free via Text.SetOrigin's internal
	// fill/ScrDraw; rich mode's frame.SetOrigin only mutates state,
	// so without this call the body stays static throughout a held
	// scrollbar press while only the thumb moves. frame.Redraw is
	// idempotent and no-ops if the frame has no display attached
	// (the test path), so it's safe to call unconditionally here.
	rt.frame.Redraw()
	return newOrigin
}
