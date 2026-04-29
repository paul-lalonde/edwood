package main

import "image"

// richScrollModel adapts *RichText to the ScrollModel interface that
// the shared Scrollbar widget consumes. This is the rich-text mode
// counterpart of textScrollModel, completing the unification: both
// modes drive the same widget.
//
// The adapter delegates the heavy lifting (B1/B3 drag-line math,
// B2 fraction jump, gap-aware line mapping, asymmetric line-boundary
// rounding) to scrollClickAt, which already encapsulates that
// behavior for the legacy ScrollClick API. Both legacy and unified
// paths share that single source of truth, so the visual experience
// is consistent across the migration.
//
// All quantities at the widget boundary are document-rendered pixels
// (LinePixelYs), with inter-line gaps and scrollbar-height
// adjustments included. This matches the gap-aware fix from
// `5543f81` in the unified-scrollbar branch — see
// docs/designs/features/unified-scrollbar.md § "Scroll snap policy
// (rich mode)".
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
// Delegates to scrollClickAt with the appropriate button code and a
// synthesized click point.
func (m *richScrollModel) DragTopToPixel(clickY int) {
	m.dispatchButton(1, clickY)
}

// DragPixelToTop implements B3 (drag-the-line-here-up-to-the-top).
func (m *richScrollModel) DragPixelToTop(clickY int) {
	m.dispatchButton(3, clickY)
}

// JumpToFraction implements B2 (jump to fraction of total document).
// The widget passes a fraction in [0, 1]; we convert to clickY and
// route through scrollClickAt's existing B2 path.
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
	m.dispatchButton(2, clickY)
}

// dispatchButton synthesizes a click point at the scrollbar's left
// edge (X is irrelevant for vertical scroll math) and the requested
// clickY, then delegates to scrollClickAt.
func (m *richScrollModel) dispatchButton(button, clickY int) {
	rt := m.rt
	if rt == nil || rt.frame == nil {
		return
	}
	scrollRect := rt.lastScrollRect
	if scrollRect.Empty() {
		return
	}
	pt := image.Point{X: scrollRect.Min.X, Y: scrollRect.Min.Y + clickY}
	rt.scrollClickAt(button, pt, scrollRect)
}
