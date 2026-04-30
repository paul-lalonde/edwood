# `rich/mdrender` blockquote-border painting — Design

## Purpose

Phase 1.2 of the markdown-externalization Phase 1 plan
([../../docs/plans/PLAN_markdown-externalization.md](../../docs/plans/PLAN_markdown-externalization.md)).
Move the blockquote-border painting (left-edge vertical bars
indicating blockquote nesting depth) from `rich.Frame` into the
`rich/mdrender` wrapper. After this row, `rich.Frame.drawTextTo`
no longer contains a blockquote paint phase; the bars are drawn
by `Renderer.Redraw` after `frame.Redraw` returns.

The visual output must be identical: a markdown document
containing blockquotes renders pixel-for-pixel the same way
through the wrapper as it did when the phase lived in
`rich.Frame`.

This row supersedes the `New(frame)` signature from row 1.1 with
a `New(frame, display)` signature. The wrapper needs the display
to draw decorations on top of the frame's already-blitted output.
Row 1.1's `renderer.design.md` will be updated in lockstep.

## Requirements

R1. `rich.Frame.drawTextTo` no longer calls
    `paintPhaseBlockquoteBorders`. The function and its helper
    `drawBlockquoteBorders` are removed from `rich/frame.go`. The
    "Phase 3a" entry in `drawTextTo`'s phase-ordering comment
    block is removed (or replaced with a note that blockquote
    bars are now drawn by `mdrender`). The constants
    `BlockquoteBorderColor` and `BlockquoteBorderWidth` move
    with the function.

R2. `rich.Frame` interface gains a new method:

    ```go
    // LayoutLines returns a fresh copy of the layout lines from
    // the current origin. Transitional accessor consumed by
    // rich/mdrender for post-paint decoration; goes away in Phase 4.
    LayoutLines() []Line
    ```

    The implementation calls `layoutFromOrigin` and returns the
    cloned line slice (the layout cache returns a clone already;
    no extra copy needed). Empty-content frames return nil.

R3. `mdrender.New` signature changes to
    `New(frame rich.Frame, display draw.Display) *Renderer`.
    `display` must be non-nil; `New` panics on nil display with
    the same convention as nil frame (R1 of `renderer.design.md`).
    Row 1.1's design + tests update to match.

R4. `Renderer.Redraw` calls `frame.Redraw()` and then
    `r.paintBlockquoteBorders()`. The order is fixed: frame paint
    first (text + images via scratch + blit to screen), then
    decoration on top of the screen image. No new flicker —
    it's a layered draw above already-blitted pixels.

R5. `Renderer.paintBlockquoteBorders` produces the same draw
    operations the old `paintPhaseBlockquoteBorders` produced,
    for the same content and frame state. Specifically:
    - For each visible line (Y < frameHeight), find the maximum
      `Style.BlockquoteDepth` across the line's boxes.
    - For each depth level 1..maxDepth, draw a 2-pixel-wide
      vertical bar at X = `frame.Rect().Min.X + (level-1)*ListIndentWidth + 2`,
      from Y = line.Y to Y = line.Y + line.Height, clipped to
      the frame rect.
    - Bar color: `BlockquoteBorderColor`.

    The verbatim transplant of the old draw loop produces this
    by construction; tests pin draw-op-by-draw-op equivalence.

R6. Lines with no blockquote-styled boxes produce no bar drawing.
    A line with `BlockquoteDepth == 0` on every box is skipped
    cleanly.

R7. Bars are clipped to the frame rect. A bar whose vertical
    extent partially exits the frame is drawn for the in-frame
    portion only; a bar entirely outside the frame is skipped.
    The existing `target.Draw(barRect.Intersect(clipRect), ...)`
    pattern preserves this.

R8. The `Renderer.allocColorImage` helper used for the bar fill
    color owns its own per-Renderer color cache (mirroring
    `frameImpl.colorCache`'s purpose: avoid per-Redraw
    `display.AllocImage` leaks). Cache size bounded by unique
    colors used; lifetime is the Renderer's lifetime.

R9. The full test suite (24+ packages) remains green after the
    move. Specifically: any existing test that exercises
    blockquote rendering still passes — those tests get a
    `Renderer` constructed around the frame and check the same
    output paths.

## Signatures

```go
// rich/frame.go (interface change)
type Frame interface {
    // ... existing methods ...
    LayoutLines() []Line // R2
}

// rich/mdrender/renderer.go (signature change, supersedes 1.1)
func New(frame rich.Frame, display draw.Display) *Renderer

// rich/mdrender/blockquote.go (new file)
package mdrender

// paintBlockquoteBorders walks the frame's visible lines and
// draws left-edge vertical bars for blockquote-depth-styled
// content. Verbatim transplant of the logic that used to live
// in rich.Frame's paintPhaseBlockquoteBorders.
func (r *Renderer) paintBlockquoteBorders()
```

The exported color constant and width survive the move:

```go
// rich/mdrender/blockquote.go
var BlockquoteBorderColor = color.RGBA{R: 200, G: 200, B: 200, A: 255}
const BlockquoteBorderWidth = 2
```

These were exported on `rich/` before the move; consumers
referencing `rich.BlockquoteBorderColor` (if any) will need to
update to `mdrender.BlockquoteBorderColor`. A grep before the
move identifies the impact.

## Edge cases

- **Empty content**: `frame.LayoutLines()` returns nil →
  `paintBlockquoteBorders` no-ops.
- **No blockquotes anywhere**: line walk runs, finds depth=0
  for every line, skips all draws. No allocation needed.
- **Maximum nesting depth not bounded**: the existing code
  walks `level := 1; level <= depth` with no upper limit. The
  layout indent for level N is `(N-1)*ListIndentWidth + 2`; deep
  nesting can push bars off-screen, where the clip handles them.
  Behavior unchanged; not adding a limit at this row.
- **Frame width zero or negative**: `clipRect` is empty, all
  intersections empty, no draws. Existing behavior preserved.
- **Mixed content (some boxes blockquote, some not, on the same
  line)**: max-depth wins (existing behavior). No change.
- **Repeated Redraw calls**: each call recomputes layout via
  `LayoutLines` (cheap; layout cache hits on unchanged
  content/width) and repaints bars. Idempotent for unchanged
  state.

## Not in scope

- **Removing `Style.Blockquote` / `Style.BlockquoteDepth`** from
  `rich.Style`. These fields stay until their Phase 3 round
  introduces a protocol primitive that replaces them. The
  wrapper continues to read them via `line.Boxes[i].Box.Style`.
- **Generalizing the bar-drawing API** (e.g. supporting other
  left-edge decorations beyond blockquote). The wrapper's
  `paintBlockquoteBorders` is markdown-blockquote-specific.
  Generalization happens at Phase 3 round 6 alongside the
  protocol primitive design.
- **Other paint phase moves** (HRule → row 1.3; slide-break →
  row 1.4; preview wiring → row 1.5; geometry ownership →
  row 1.6).
- **Behavior changes**. This row is a verbatim move with
  signature plumbing; the rendered output and the conditions
  under which bars are drawn are identical to before.

## Lifecycle

After this row:
- `rich.Frame` no longer paints blockquote bars internally.
  Constructing a frame and calling `Redraw` on it directly
  produces NO bars even for blockquote-styled content.
- Production code (`RichText` preview mode) gets bars only if
  it goes through `mdrender.Renderer`.

**Decision (option a)**: this row also wires `RichText` preview
mode through `Renderer`, in lockstep with the paint-phase move.
No regression window at any commit on the branch. Row 1.5
(originally "route preview mode through wrapper") shrinks to
"confirm the wiring works for HRule + slide-break and any
non-blockquote affordances we deferred", since the wiring itself
already lives by then.

Specifics:
- `RichText.Init` constructs a `mdrender.Renderer` wrapping its
  frame when in preview mode (or unconditionally — preview vs.
  styled mode flag plumbing settles at row 1.5; for this row
  unconditional construction is fine because `Renderer.Redraw`
  is still effectively a pass-through plus the new blockquote
  paint, and styled mode never has blockquote-styled content).
- `RichText.Redraw` calls `rt.renderer.Redraw()` instead of
  `rt.frame.Redraw()` directly.
- The styled-mode invariant ("Style.Blockquote* never set in
  styled mode") is what makes the unconditional wrapper safe;
  if a future change sends Blockquote-styled content via the
  spans protocol, the wrapper will draw bars for it. That's
  consistent with the long-term intent (everything goes through
  the wrapper).

## Status

Design — drafted. Awaiting review. Lifecycle question resolved
(option a — wire `RichText` through wrapper at this row).
