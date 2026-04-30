// Package mdrender is the in-tree wrapper around rich.Frame that
// owns markdown-specific paint phases. Phase 1 of the
// markdown-externalization work moves blockquote borders, horizontal
// rules, and slide-break handling out of rich.Frame and into this
// package; Renderer wraps a frame and adds those phases on top of
// the frame's own paint pass.
//
// As of Phase 1.2, the wrapper paints blockquote bars after the
// frame's paint pass returns. Subsequent rows add horizontal
// rules (1.3) and slide-break handling (1.4). The package is
// transitional — it will be deleted in Phase 4 once the external
// md2spans tool produces equivalent output via spans-protocol
// primitives.
//
// Import direction: mdrender imports rich. rich must NEVER import
// mdrender. The Go compiler enforces this via the import cycle
// check.
package mdrender

import (
	edwooddraw "github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/rich"
)

// Renderer wraps a rich.Frame and applies markdown-specific paint
// phases on top of the frame's own paint pass. See the package doc
// for the larger context.
type Renderer struct {
	frame   rich.Frame
	display edwooddraw.Display

	// colorCache mirrors rich.frameImpl.colorCache: caches 1x1
	// replicated images by packed RGBA so repeated calls during
	// scrollbar latches / live edits don't leak Plan 9 image
	// handles. Lazily initialized in allocColorImage.
	colorCache map[edwooddraw.Color]edwooddraw.Image
}

// New returns a Renderer wrapping frame with display as the draw
// target for wrapper-side decoration phases. Both frame and
// display must be non-nil; New panics on either being nil with a
// message identifying which.
func New(frame rich.Frame, display edwooddraw.Display) *Renderer {
	if frame == nil {
		panic("mdrender.New: frame must not be nil")
	}
	if display == nil {
		panic("mdrender.New: display must not be nil")
	}
	return &Renderer{frame: frame, display: display}
}

// Redraw paints the wrapped frame and runs wrapper-side paint
// phases.
//
// Order is fixed: frame paints first (text + images via scratch +
// blit to screen), then the wrapper draws decorations directly on
// top of the screen image. No new flicker — it's a layered draw
// above already-blitted pixels.
//
// The wrapper-side phases run in a fixed order. Currently these
// don't overlap geometrically (bars go in the left-edge indent
// zone; rules span the line vertically-centered in the line's
// height), so the order is for predictability rather than
// correctness. Future Phase-1 rows that add overlapping decorations
// must revisit.
func (r *Renderer) Redraw() {
	r.frame.Redraw()
	r.paintBlockquoteBorders()
	r.paintHorizontalRules()
}

// Frame returns the wrapped rich.Frame for callers that need to
// drive the frame directly during the Phase 1 transition (e.g. to
// call SetContent, SetRect, SetOrigin while wrapper-side methods
// don't yet exist). Transitional affordance; expected to shrink.
func (r *Renderer) Frame() rich.Frame {
	return r.frame
}
