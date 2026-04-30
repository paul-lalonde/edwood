// Package mdrender is the in-tree wrapper around rich.Frame that
// owns markdown-specific paint phases. Phase 1 of the
// markdown-externalization work moves blockquote borders, horizontal
// rules, and slide-break handling out of rich.Frame and into this
// package; Renderer wraps a frame and adds those phases on top of
// the frame's own paint pass.
//
// At Phase 1.1 (this row of the plan) the package contains only an
// empty skeleton: Renderer wraps a frame and Redraw delegates to it
// transparently. Subsequent rows add the moved paint phases. The
// package is transitional — it will be deleted in Phase 4 once the
// external md2spans tool produces equivalent output via spans-
// protocol primitives.
//
// Import direction: mdrender imports rich. rich must NEVER import
// mdrender. The Go compiler enforces this via the import cycle
// check.
package mdrender

import "github.com/rjkroege/edwood/rich"

// Renderer wraps a rich.Frame and applies markdown-specific paint
// phases on top of the frame's own paint pass. See the package doc
// for the larger context.
type Renderer struct {
	frame rich.Frame
}

// New returns a Renderer wrapping frame. frame must be non-nil; New
// panics on nil rather than constructing a silently-broken Renderer
// that would later nil-deref. The panic is intentional and matches
// the project convention for unrecoverable construction-time misuse.
func New(frame rich.Frame) *Renderer {
	if frame == nil {
		panic("mdrender.New: frame must not be nil")
	}
	return &Renderer{frame: frame}
}

// Redraw paints the wrapped frame. At Phase 1.1 this is a pure
// delegation to the frame's own Redraw; later rows add wrapper-side
// paint phases that run after the frame's paint pass.
func (r *Renderer) Redraw() {
	r.frame.Redraw()
}

// Frame returns the wrapped rich.Frame for callers that need to
// drive the frame directly during the Phase 1 transition (e.g. to
// call SetContent, SetRect, SetOrigin while wrapper-side methods
// don't yet exist). This is a transitional affordance and is
// expected to shrink as the wrapper grows; new code should prefer
// adding methods on Renderer over reaching through this getter.
func (r *Renderer) Frame() rich.Frame {
	return r.frame
}
