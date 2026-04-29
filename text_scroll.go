package main

import "image"

// textScrollModel adapts *Text to the ScrollModel interface that the
// shared Scrollbar widget consumes. The adapter operates on the
// rune-count proxy that scrl.go has historically used (totalPx →
// file.Nr(), viewPx → frame Nchars, originPx → t.org). The widget's
// thumb math is purely proportional, so the rune-count triple
// produces pixel-identical results to the legacy scrpos.
//
// The Drag* methods are bit-for-bit equivalent to the per-button
// branches at scrl.go:142-148. JumpToFraction mirrors scrl.go:127-131
// including the "if p0 >= q1, snap with BackNL(p0, 2)" guard that
// keeps the cursor in view after a B2 jump.
type textScrollModel struct {
	t *Text
}

// Geometry returns the rune-count triple expected by the widget.
// Called every Draw; cheap.
func (m *textScrollModel) Geometry() (totalPx, viewPx, originPx int) {
	if m.t.fr == nil {
		return 0, 0, 0
	}
	return m.t.file.Nr(), m.t.fr.GetFrameFillStatus().Nchars, m.t.org
}

// DragTopToPixel implements B1: the line currently at the top of the
// viewport must end up at viewport pixel clickY. clickY/fontH is the
// number of font-line-heights to scroll backwards, mirroring
// scrl.go:142-143.
func (m *textScrollModel) DragTopToPixel(clickY int) {
	if m.t.fr == nil {
		return
	}
	fontH := m.t.fr.DefaultFontHeight()
	if fontH <= 0 {
		return
	}
	p0 := m.t.BackNL(m.t.org, clickY/fontH)
	m.t.SetOrigin(p0, true)
}

// DragPixelToTop implements B3: the line currently at viewport pixel
// clickY must end up at the top of the viewport. The legacy code
// uses Charofpt at the rightmost track column to find the rune at
// the click row; we reproduce that exactly. clickY is relative to
// the inset track, so we add scrollr.Inset(1).Min.Y back to recover
// absolute screen coordinates for Charofpt.
func (m *textScrollModel) DragPixelToTop(clickY int) {
	if m.t.fr == nil {
		return
	}
	s := m.t.scrollr.Inset(1)
	pt := image.Pt(s.Max.X, s.Min.Y+clickY)
	p0 := m.t.org + m.t.fr.Charofpt(pt)
	m.t.SetOrigin(p0, true)
}

// JumpToFraction implements B2: set origin so its position within
// the document is at fraction f in [0, 1]. Mirrors scrl.go:127-131:
// p0 = file.Nr() * f, then if p0 >= q1 snap with BackNL(p0, 2) so
// the cursor doesn't disappear above the new viewport. SetOrigin is
// called with exact=false to allow the newline-search logic in
// setorigin to round to a clean line start.
func (m *textScrollModel) JumpToFraction(f float64) {
	p0 := int(f * float64(m.t.file.Nr()))
	if p0 >= m.t.q1 {
		p0 = m.t.BackNL(p0, 2)
	}
	m.t.SetOrigin(p0, false)
}
