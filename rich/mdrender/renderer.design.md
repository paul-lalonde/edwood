# `rich/mdrender.Renderer` — Design

## Purpose

`rich/mdrender` is the in-tree wrapper introduced by Phase 1 of the
markdown-externalization work
([docs/designs/features/markdown-externalization.md](../../docs/designs/features/markdown-externalization.md)).
This row (Phase 1.1) sets up the package skeleton: an empty
`Renderer` type that wraps a `rich.Frame` and delegates `Redraw` to
it transparently. No paint phases have moved into the wrapper yet —
those land in rows 1.2 (blockquote bars), 1.3 (horizontal rules),
and 1.4 (slide-break handling).

The skeleton's value is structural: it establishes the package
boundary (`mdrender` imports `rich`, never the reverse), the
`Renderer` API shape that subsequent rows extend, and the test
layout. It must not change rendering behavior. Any visible
difference in rendered output between "frame.Redraw() directly"
and "renderer.Redraw() through this wrapper" at this row is a bug.

## Requirements

R1. `New(frame rich.Frame) *Renderer` returns a non-nil `*Renderer`
    that holds the supplied frame. The frame must not be nil; if it
    is, `New` panics with a clear message. (No silent-nil
    misbehavior — Phase 1 is high-risk; failing loudly at
    construction is preferable to a confusing later nil-deref.)

R2. `Renderer.Redraw()` calls the underlying frame's `Redraw()` and
    returns. No additional drawing happens at this row. Calling
    `Redraw` on a wrapper produces output indistinguishable from
    calling `Redraw` on the underlying frame directly, for any
    content the frame can render today.

R3. `Renderer.Frame()` returns the underlying `rich.Frame` for
    callers that need direct access during the Phase 1 transition
    (for example, to call `SetContent`, `SetRect`, `SetOrigin` etc.
    that the wrapper does not yet re-export). This is a transitional
    affordance and is expected to shrink as later rows in Phase 1
    add wrapper-side methods. It is documented as such in the
    method's godoc.

R4. The package compiles and passes `go vet ./rich/mdrender/...`
    cleanly. No unused imports, no shadowed names, no documentation
    typos that block `go doc`.

R5. Import direction: `rich/mdrender` may import `rich`. `rich`
    must NOT import `rich/mdrender`. A test (or a `go list` check)
    asserts this.

## Signatures

```go
package mdrender

import "github.com/rjkroege/edwood/rich"

// Renderer wraps a rich.Frame and adds markdown-specific paint
// phases on top of the frame's own paint pass. Phase 1.1 is the
// empty skeleton; subsequent rows of the markdown-externalization
// Phase 1 plan add blockquote borders, horizontal rules, and
// slide-break handling. After Phase 4, Renderer is deleted entirely
// and edwood drives rich.Frame from spans-protocol output produced
// by the external md2spans tool.
type Renderer struct {
    frame rich.Frame
}

// New constructs a Renderer wrapping frame. frame must be non-nil.
func New(frame rich.Frame) *Renderer

// Redraw paints the wrapped frame. At Phase 1.1 this is a pure
// pass-through to frame.Redraw(); later rows add markdown-specific
// post-paint passes.
func (r *Renderer) Redraw()

// Frame returns the wrapped frame for callers that need to drive it
// directly during the Phase 1 transition. Going forward, prefer
// adding methods to Renderer over reaching through this getter.
func (r *Renderer) Frame() rich.Frame
```

## Edge cases

- **Nil frame**: `New(nil)` panics. The argument is required; there
  is no useful behavior for a nil frame and silently constructing
  an unusable Renderer would mask configuration mistakes.
- **Repeated Redraw calls**: same as repeated `frame.Redraw()`
  calls — idempotent in the sense that they produce the same
  output for the same frame state.
- **Frame mutation between Redraw calls**: not the wrapper's
  concern. The wrapper holds a `rich.Frame` interface; whatever
  mutations the caller performs on that interface are visible on
  the next `Redraw`. The wrapper does not cache or shadow frame
  state.

## Not in scope

This row deliberately defers (each lands in a later Phase 1 row):

- **Blockquote-border painting** (Phase 1.2). `rich.Frame` still
  owns `paintPhaseBlockquoteBorders` after this row.
- **Horizontal-rule painting** (Phase 1.3). `rich.Frame` still
  owns `paintPhaseHorizontalRules` after this row.
- **Slide-break detection / fill** (Phase 1.4). `findSlideRegions`,
  `adjustLayoutForSlides`, and the slide-fill paint logic stay in
  `rich.Frame` after this row.
- **Preview-mode wiring** (Phase 1.5). `RichText` does not yet
  construct a `Renderer`; this row produces the type but does not
  use it.
- **Wrapper-side `SetContent`, `SetRect`, etc.** Callers go through
  `Renderer.Frame()` for those at this row. Wrapper-side methods
  may be added in later rows as the wrapper grows; not now.
- **Markdown content interpretation.** The wrapper does not yet
  inspect `rich.Style` fields or own any markdown-semantic logic.
  That is Phase 1.2-1.4's work.
- **Geometry ownership** (Phase 1.6). The wrapper does not yet
  own `SetRect`. Frame still does.

## Status

Design — drafted. Awaiting review.
