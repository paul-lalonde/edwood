# `rich/mdrender` horizontal-rule painting — Design

## Purpose

Phase 1.3 of the markdown-externalization Phase 1 plan. Same shape
as 1.2 (blockquote): move horizontal-rule painting out of
`rich.Frame.drawTextTo` into the `rich/mdrender` wrapper. After
this row, frame.Redraw produces no horizontal-rule drawing; the
wrapper draws rules after frame.Redraw on top of the screen image.

The infrastructure for the move (LayoutLines accessor, wrapper
construction in RichText.Init, color cache on Renderer) all
landed in 1.2; this row just plugs HRule into the same machinery.

## Requirements

R1. `rich.Frame.drawTextTo` no longer calls
    `paintPhaseHorizontalRules`. The function and its helper
    `drawHorizontalRuleTo` are removed from `rich/frame.go`.
    The "Phase 3 horizontal rules" entry in `drawTextTo`'s phase-
    ordering comment is removed. The constant `HRuleColor` moves
    with the function.

R2. The HRule-skip in `paintPhaseText` (`rich/frame.go` around the
    text-rendering loop) stays. Boxes whose Style.HRule is true
    must not render their placeholder text regardless of who
    paints the rule line. The corresponding "Phase 3" comment is
    updated to point at `mdrender` rather than at the (now
    removed) phase number.

R3. `Renderer.Redraw` calls a new `paintHorizontalRules()` after
    `paintBlockquoteBorders()`. Order between the two wrapper-
    side phases doesn't matter functionally (rules and bars
    don't overlap geometrically), but for predictability we
    keep the same order the frame used (rules before
    blockquote bars in the old phase numbering, but bars
    landed first in 1.2's wrapper — keep that order; HRule
    runs second).

R4. `Renderer.paintHorizontalRules()` produces the same draw
    operations the old `paintPhaseHorizontalRules` produced.
    For each visible line whose boxes contain at least one
    HRule-styled box, draw a 1-pixel-tall horizontal line at
    Y = line.Y + line.Height/2, X spanning [frameRect.Min.X,
    frameRect.Min.X + frameWidth), color = HRuleColor.

R5. Lines without any HRule-styled box produce no rule drawing.

R6. Rules clip to the frame rect. A rule whose vertical extent
    falls outside the frame is skipped; a rule that fits is
    drawn for its in-frame portion only. The existing
    `Intersect(clipRect)` pattern preserves this.

R7. The `HRuleColor` allocation goes through the existing
    `Renderer.allocColorImage` helper (introduced in 1.2 for
    BlockquoteBorderColor). Cache size grows by one entry on
    first HRule encounter.

R8. The full test suite remains green after the move.

## Signatures

```go
// rich/mdrender/hrule.go (new file)
package mdrender

// HRuleColor is the gray color used for horizontal rule lines.
// Moved from rich.HRuleColor in Phase 1.3.
var HRuleColor = color.RGBA{R: 180, G: 180, B: 180, A: 255}

// paintHorizontalRules walks the wrapped frame's layout lines
// and draws a 1px horizontal line for each line containing an
// HRule-styled box. Verbatim transplant of the logic that used
// to live in rich.Frame's paintPhaseHorizontalRules.
func (r *Renderer) paintHorizontalRules()

// drawHorizontalRuleTo draws a single rule for one line.
// Mirrors the same-named helper that used to live on
// rich.frameImpl.
func (r *Renderer) drawHorizontalRuleTo(target edwooddraw.Image, line rich.Line, offset image.Point, frameWidth, frameHeight int)
```

## Edge cases

- **Empty content**: `frame.LayoutLines()` returns nil →
  `paintHorizontalRules` no-ops.
- **No HRules anywhere**: line walk runs, no box matches,
  no draws.
- **Multiple HRule boxes on the same line**: existing
  behavior is `break // one rule per line`. Preserved
  verbatim.
- **HRule line height zero or 1**: existing math centers the
  rule with `line.Height/2`. With height 0 or 1, the rule
  lands at the line's top edge — same as before. Edge case
  not changed at this row.
- **Frame zero/negative dim**: clipRect empty, no draws.

## Not in scope

- **Removing `Style.HRule`** from `rich.Style`. The field
  stays until its Phase 3 round adds an inline-rule protocol
  primitive. The wrapper continues to read it via `box.Style`.
- **Generalizing rule rendering** (e.g. configurable color or
  thickness). The existing 1px gray rule is preserved
  verbatim.
- **Other paint phase moves** (slide-break → row 1.4;
  geometry ownership → row 1.6).
- **Behavior changes**. This row is a verbatim move; rules
  appear in the same places at the same colors.

## Lifecycle

After this row:
- `rich.Frame.Redraw` directly produces NO rule drawing for
  HRule-styled content. Bars come only from
  `mdrender.Renderer`.
- Production preview already routes through Renderer (wired
  in 1.2), so production rule rendering is unchanged.
- `Style.HRule` stays in `rich.Style` per Phase 1 scope.

## Status

Design — drafted. Awaiting review.
