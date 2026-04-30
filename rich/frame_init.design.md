# `rich.Frame.Init` — drop `rect` parameter — Design

## Purpose

Phase 1.4 (renumbered) of the markdown-externalization Phase 1
plan. Resolves the architect-review finding P1-6 (geometry
ownership ambiguity).

Today, `rich.Frame` has two paths to set its rectangle:
- `Init(r image.Rectangle, opts ...Option)` accepts a rect as a
  positional first argument.
- `SetRect(r image.Rectangle)` updates it later.

Production code (`richtext.go:148`) already calls
`Init(image.Rectangle{}, frameOpts...)` and then `SetRect` from
`Render` once the actual rectangle is known. The Init-time rect is
a no-op zero in production. Tests use the two-arg form mostly out
of habit — they could equally well call `Init` then `SetRect`.

This row drops the rect parameter from `Init`. After this row,
geometry has exactly one setter (`SetRect`) and the
RichText/wrapper layer is the sole driver. The architect's "pick
one" call lands on D — RichText owns geometry; frame is passive.

## Requirements

R1. `rich.Frame.Init` signature changes from
    `Init(r image.Rectangle, opts ...Option)` to
    `Init(opts ...Option)`. The rect field on `frameImpl` is left
    at its zero value after `Init`. Callers that need a non-zero
    rect call `SetRect(r)` afterward.

R2. The `Frame` interface declaration in `rich/frame.go` updates
    in lockstep. Any existing or future implementation of the
    interface must match the new signature; the Go compiler
    enforces this.

R3. `richtext.go:Init` drops the now-empty
    `image.Rectangle{}` argument. The subsequent `Render(rect)`
    call still drives `SetRect`, unchanged.

R4. All test call sites update from
    `f.Init(rect, opts...)` to
    `f.Init(opts...); f.SetRect(rect)` (when rect is non-zero) or
    just `f.Init(opts...)` (when rect was zero anyway). Roughly
    25 sites across `rich/`, `rich/mdrender/`, and possibly a
    few others.

R5. The Init godoc gains a doc note: "Use `SetRect` to set the
    frame's rectangle after `Init`. The rect is left at zero by
    `Init`; geometry is the caller's responsibility (typically
    a wrapper like `RichText` that knows the parent layout's
    coordinate system)."

R6. Full test suite remains green after the changes. The full
    suite runs in CI with `go test ./...`; behavior under
    every existing test must be preserved.

## Signatures

```go
// rich/frame.go (interface)
type Frame interface {
    // ... existing methods ...
    Init(opts ...Option)
    SetRect(r image.Rectangle)
    // ... existing methods ...
}

// rich/frame.go (implementation)
func (f *frameImpl) Init(opts ...Option) {
    for _, opt := range opts {
        opt(f)
    }
}
```

`Init` no longer touches `f.rect`. The default zero value is what
the field carries until `SetRect` is called.

## Edge cases

- **Caller that never calls SetRect**: `f.rect` stays
  `image.Rectangle{}`. `f.rect.Dx() == 0`,
  `f.rect.Dy() == 0`. `Redraw` early-exits on zero
  dimensions (`rich/frame.go` line ~1015), so no rendering
  happens. Same behavior as today's
  `Init(image.Rectangle{}, ...)` followed by no SetRect.
- **Caller that calls Init multiple times**: each call re-runs
  options. Behavior unchanged from today (already worked
  this way except the rect was reset to whatever the second
  Init's first argument was).
- **Test that calls Init then SetRect to set a rect**: this is
  the new pattern. SetRect already invalidates
  `layoutDirty`, so subsequent renders re-layout for the new
  width.

## Not in scope

- Removing the `rect` field from `frameImpl`. The field stays;
  it's set via `SetRect`.
- Touching `SetRect`'s contract in any way. Same behavior as
  today.
- `WithRect` as an Option. We could add `Init(WithRect(r), ...)`
  to keep one-call construction possible, but that re-introduces
  the same dual-ownership problem at the option layer. Stick
  with one path: `SetRect`.
- Changing the call site in `frame/frame.go` (text-mode `Frame`).
  That's a different package with its own `Init` signature;
  unaffected by this work.

## Migration touch surface

Production:
- `richtext.go:148`: drop `image.Rectangle{}` arg.

Tests (rough enumeration; final count confirmed at iterate
stage):
- `rich/frame_test.go`: ~10 sites.
- `rich/scroll_test.go`: ~3 sites.
- `rich/select_test.go`: ~2 sites.
- `rich/scroll_snap_test.go`: ~2 sites.
- `rich/image_test.go`: ~4 sites.
- `rich/mdrender/blockquote_test.go`: 2 sites.
- `rich/mdrender/hrule_test.go`: 1 site.
- `rich/mdrender/renderer_test.go`: 2 sites (helper).

Each site updates from `f.Init(rect, opts...)` to one of:
- `f.Init(opts...); f.SetRect(rect)` — when rect is meaningful.
- `f.Init(opts...)` — when rect was `image.Rectangle{}` already.

## Status

Design — drafted. Awaiting review.
