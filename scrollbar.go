package main

// MinThumbHeightPx is the minimum on-screen height of the scrollbar
// thumb. Below this the thumb becomes hard to grab on hi-DPI displays
// and visually disappears against the track. Chosen empirically to be
// reliably grabbable; not tied to font height because the scrollbar
// must remain usable in extremely large documents where a strictly
// proportional thumb height would be sub-pixel.
const MinThumbHeightPx = 10

// ScrollModel is the mode-specific adapter that the Scrollbar widget
// calls to ask "where is the document, and where should it move
// next?"
//
// All quantities are in document-space pixels: the document is
// treated as a vertical strip of total height TotalPx, of which
// ViewPx pixels are visible starting at OriginPx from the document
// top. Modes that operate in non-pixel units (e.g. text mode in
// runes) may return any unit triple from Geometry as long as the
// three values share the same unit; the widget uses them only to
// size and position the thumb proportionally.
//
// The widget never sees runes, lines, or any mode-specific concept.
// Three operations, all expressed in viewport-relative pixels.
type ScrollModel interface {
	// Geometry returns the current scroll state.
	//   totalPx:  total document height (>= viewPx).
	//   viewPx:   viewport height.
	//   originPx: offset from document top to viewport top.
	Geometry() (totalPx, viewPx, originPx int)

	// DragTopToPixel implements B1: the line currently at the top of
	// the viewport must end up at viewport pixel clickY. Equivalent
	// to subtracting clickY pixels from origin (clamped to
	// [0, totalPx-viewPx]).
	DragTopToPixel(clickY int)

	// DragPixelToTop implements B3: the line currently at viewport
	// pixel clickY must end up at the top of the viewport.
	// Equivalent to adding clickY pixels to origin (clamped).
	DragPixelToTop(clickY int)

	// JumpToFraction implements B2: set origin so its position
	// within [0, totalPx-viewPx] is at fraction f in [0, 1].
	JumpToFraction(f float64)
}
