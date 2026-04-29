# Unified Vertical Scrollbar

## Goal

Both text mode (`Text` in `text.go`) and rich-text mode (`RichText` in
`richtext.go`) currently render and operate their own vertical
scrollbars. The two implementations have diverged in visuals,
interaction, and code structure. This design extracts a single
**Scrollbar widget** that owns drawing, mouse handling, and the
click-and-hold latch loop. Each mode supplies a tiny **ScrollModel**
adapter that translates the widget's pixel-space requests into the
mode's native unit (runes for text mode, pixels with sub-line offsets
for rich-text mode).

The acme/edwood-style three-button scrollbar semantics, latch behavior,
visual style, and frame colors become canonical: a single source of
truth that cannot drift between modes.

## Why

`docs/richtext-design.md` §5 ("Scrollbar") originally posed the
question: "Reuse existing scrollbar code from `scrl.go` or implement
fresh?" The recommendation was "Extract scrollbar to shared utility if
possible, otherwise copy and adapt." The implementation took the
copy-and-adapt path. The result:

- **`scrl.go`** (~67 lines): rune-based, frame-colored, 2px thumb
  minimum, owns its latch loop with `ScrSleep` and cursor warping.
- **`richtext.go:273-507`** (~230 lines): pixel-based, configurable
  scrollbar colors, 10px thumb minimum, latch loop in
  `wind.go:previewVScrollLatch` and `previewHScrollLatch` with
  `previewScrSleep`.

Specific divergences:

| Axis | Text mode (`scrl.go`) | Rich mode (`richtext.go`) |
|---|---|---|
| Origin unit | rune offset | rune + pixel sub-line offset |
| Thumb minimum | 2px (`scrl.go:48-53`) | 10px (`richtext.go:463`) |
| Colors | `global.textcolors` only | `WithScrollbarColors()` option |
| B1 formula | `BackNL(org, (my-top)/fontH)` | `currentPx - frameH * (1 - clickProp)` |
| B3 formula | `org + Charofpt(maxX, my)` | `currentPx + frameH * clickProp` |
| Latch ownership | `Text.Scroll()` (`scrl.go:101-166`) | `wind.go:previewVScrollLatch` |
| Tall-image scroll | n/a | snaps to line boundary; jumps over images |

The first three are pure visual / configuration drift. Rows 4–5 are
*almost* the same idea but were re-derived independently in pixel space
and lost the precise acme drag semantics in the process. Row 6 is
duplication of a non-trivial mouse-warp + debounce loop. Row 7 is the
one actual bug this refactor fixes for free.

## Acme drag semantics (canonical)

Both modes must implement these exactly. From `scrl.go:142-146`:

```go
// Click at vertical pixel my, scrollbar top at s.Min.Y.
if but == 1 {
    p0 = t.BackNL(t.org, (my-s.Min.Y)/t.fr.DefaultFontHeight())
} else {
    p0 = t.org + t.fr.Charofpt(image.Pt(s.Max.X, my))
}
```

Stated as drag actions, both relative to the **current viewport**:

- **B1 (left)**: drag-the-top-line-down-to-here. The line currently at
  the top of the viewport moves *down* to the click pixel; earlier
  content slides into view above it.
- **B3 (right)**: drag-the-line-here-up-to-the-top. The line at the
  click pixel becomes the new top; later content slides into view
  below it.
- **B2 (middle)**: jump-to-fraction. Origin is set so the thumb's top
  edge sits at the click's fractional Y position in the track.

These are asymmetric: B1's reference line is the *current top*; B3's
reference line is the *line under the click*. The asymmetry is
intentional and must be preserved.

Auto-repeat: while the button is held, B1 and B3 re-fire on a
timer-based debounce (200ms first, 80ms thereafter) using the *current*
mouse Y, so holding the button at a fixed Y position page-scrolls
continuously. B2 re-fires per mouse event for live thumb dragging.

## Pixel-line alignment for rich mode

Text mode uses a uniform line height, so the integer division
`(my-top)/fontH` perfectly maps any pixel to a unique line, and an
origin expressed as "this line's first rune" is fully expressive.

Rich mode has variable line heights (a 600px image is one logical
line). If origin is "rune at start of a logical line", there is no way
to represent "the image is half-scrolled-off"; the viewport jumps from
"image at top" to "image gone" with no intermediate state. Holding B3
on a tall image is unresponsive.

Fix: stop snapping. Rich mode's `frame.Frame` already exposes
`SetOriginYOffset(int)` / `GetOriginYOffset()` for sub-line pixel
offsets. The widget treats every pixel row in document space as
addressable; the rich-mode adapter converts a document-space pixel Y
to a `(lineIndex, pixelOffsetWithinLine)` pair via
`LinePixelHeights()` / `LineStartRunes()`.

Net effect: B1/B3 drag math becomes pure pixel arithmetic at the
widget boundary. Text mode's adapter rounds to `fontH` boundaries on
the way out (preserving today's line-granular feel); rich mode's
adapter passes pixels through.

## Scroll snap policy (rich mode)

### Problem

The legacy `snapBottomLine` flag on `rich.frameImpl` (added in
`3bb2bc0` originally to fix slide rendering) shifts every layout up so
that the last visible line ends exactly at the frame bottom. The first
visible line absorbs the overflow — its top is partially clipped.

This is correct mid-document but **wrong at the file top**: at
`origin=0, originYOffset=0` the user expects the first line's top
aligned with the viewport top. With `snapBottomLine=true` (always on
in styled mode) the first line is clipped by the residual
`frameHeight % lineHeight` pixels and the user can never see the first
line fully visible at top — the "can't scroll all the way back to the
top of the first line" bug.

The bug also applies to **tall lines** (e.g. an image taller than the
viewport): the always-on shift can move the tall line's Y arbitrarily,
breaking the sub-line pixel scrolling that `originYOffset` enables.

### Policy

Snap is a **per-scroll-action property** rather than a global flag:

| Trigger | Snap |
|---|---|
| B1 click (drag-top-down-to-here, scrolling up = revealing earlier) | **Bottom** |
| B3 click (drag-here-to-top, scrolling down = revealing later) | **Top** |
| B2 click (jump to fraction) | **Top** |
| Programmatic `SetOrigin` (Look, search, auto-scroll) | **Top** *(always reset)* |
| Origin = 0, Offset = 0 (file top) | **Top** *(override)* |
| Origin line height > frame height (tall line) | **Pixel** *(override, no snap)* |

Rationale:

- **B1** reveals content above the previous viewport. Anchoring the
  bottom line of the new viewport keeps the relationship to the
  previous frame visually obvious.
- **B3** advances through the document. Anchoring the top line lets
  the user read down without partial-line distractions.
- **B2 / programmatic** lands on an arbitrary spot. Default to a
  clean top edge for predictability and to match acme text-mode B2
  (which lands at a line start via the `BackNL(p0, 2)` snap in
  `scrl.go:127-131`).
- Programmatic scrolls *always* reset to `SnapTop`. A `B1 → Look`
  sequence puts the looked-up content at the top, not at the
  bottom-anchored state from the prior B1. Confirmed in design
  review.
- **Edge cases** (file top, tall line) override the user's last
  action because the snap that *would* apply has nowhere to anchor.

### API

New enum in `rich`:

```go
type ScrollSnap int

const (
    // SnapTop aligns the first visible line's top to the viewport
    // top. Default; matches a freshly-loaded document.
    SnapTop ScrollSnap = iota

    // SnapBottom aligns the last visible line's bottom to the
    // viewport bottom; the first line absorbs partial-line clipping.
    // Equivalent to legacy snapBottomLine=true.
    SnapBottom

    // SnapPixel honors originYOffset literally with no
    // line-boundary alignment. Used when the origin line is taller
    // than the viewport (e.g. a large image), where line-level
    // snapping would prevent the user scrolling within the line.
    SnapPixel
)
```

New method on `rich.Frame`:

```go
// SetScrollSnap configures snap behavior for subsequent layouts.
// Callers (typically scroll handlers) set this immediately before
// calling SetOrigin/SetOriginYOffset to record the user's intent.
// Edge cases (origin at file top, tall origin line) override this
// inside layoutFromOrigin.
SetScrollSnap(s ScrollSnap)
```

Replace `WithSnapBottomLine(bool)` with
`WithDefaultScrollSnap(ScrollSnap)`. The new default for steady
state is `SnapTop`, not the legacy `SnapBottom`. Mid-document the
last line may be partially clipped between scroll actions —
acceptable cost, confirmed in design review, because the next B1
immediately switches to bottom snap.

### Layout changes

`layoutFromOrigin` end:

```go
// Replaces f.applySnapBottomLine(visibleLines)
f.applyScrollSnap(visibleLines, startLineIdx, allLines)
```

```go
func (f *frameImpl) applyScrollSnap(visible []Line, startIdx int, all []Line) {
    if len(visible) == 0 {
        return
    }
    snap := f.scrollSnap

    // Edge-case overrides.
    switch {
    case f.origin == 0 && f.originYOffset == 0:
        snap = SnapTop
    case startIdx < len(all) && all[startIdx].Height > f.rect.Dy():
        snap = SnapPixel
    }

    switch snap {
    case SnapTop, SnapPixel:
        // Default layout already aligns the origin line's top to
        // the viewport top (with originYOffset for SnapPixel
        // sub-line scrolling). Nothing more to do.
        return
    case SnapBottom:
        // Existing applySnapBottomLine logic, unchanged.
        f.applyBottomShift(visible)
    }
}
```

`applyBottomShift` is the existing `applySnapBottomLine` body
verbatim (extracted with no logic changes, renamed because the snap
behavior is no longer a single flag).

### Caller changes

`richtext.go scrollClickAt` (`richtext.go:312`), at the top of each
button branch:

```go
case 1:
    rt.frame.SetScrollSnap(rich.SnapBottom)
    // existing pixelsToMove + clamp + SetOrigin/Offset
case 2:
    rt.frame.SetScrollSnap(rich.SnapTop)
    // existing jump
case 3:
    rt.frame.SetScrollSnap(rich.SnapTop)
    // existing pixelsToMove + clamp
```

Programmatic entry points (`SetOrigin`, `SetOriginYOffset`, used by
Look, address resolution, auto-scroll) reset snap to `SnapTop`
internally so they don't inherit a stale `SnapBottom` from a prior
B1.

The future Phase 3 `richScrollModel` adapter inherits this:
`DragTopToPixel` calls `SetScrollSnap(SnapBottom)`,
`DragPixelToTop` and `JumpToFraction` call `SetScrollSnap(SnapTop)`.

### Migration

- `rich/frame.go`: replace `snapBottomLine bool` with
  `scrollSnap ScrollSnap`, default `SnapTop`. Rename
  `applySnapBottomLine` → `applyBottomShift`; add `applyScrollSnap`.
  Add `SetScrollSnap` method.
- `rich/options.go`: replace `WithSnapBottomLine` with
  `WithDefaultScrollSnap`.
- `richtext.go`: replace `snapBottomLine bool` with the new option
  field; replace `WithRichTextSnapBottomLine` with
  `WithRichTextDefaultScrollSnap`. Add `SetScrollSnap` method.
  Update `scrollClickAt` to set per-button snap; update `SetOrigin`
  / `SetOriginYOffset` to reset snap to `SnapTop`.
- `wind.go`: `initStyledMode` and the preview-mode constructor:
  drop `WithRichTextSnapBottomLine(true)`. The new default is what
  we want for a freshly-displayed document.
- `docs/designs/features/spans-rendering.md`: update
  §"Interaction with Bottom-Line Snap" to point here.

### Slides — explicitly out of scope

The original `snapBottomLine` was added for slide rendering. Slide
display has had unintended side effects elsewhere; the user has
asked to defer slide-specific behavior. Slides are not addressed by
this design — `adjustLayoutForSlides` in `rich/frame.go:1597`
continues to run as before. If slide rendering regresses with
`SnapTop` as the new default, that's a separate fix.

### Risks

- **`originYOffset` clamping** in `layoutFromOrigin` (lines
  1556-1568) may interact with `SnapPixel` for tall lines. The
  clamp logic already produces a valid offset; verify it cooperates
  with `SnapPixel` (no shift on top of the clamp).
- The new default (`SnapTop`) means freshly-displayed styled
  documents look top-aligned rather than bottom-aligned. Acceptable
  per design review.

## ScrollModel interface

```go
// ScrollModel is the mode-specific adapter the Scrollbar widget calls
// to ask "where is the document, and where should it move next?"
//
// All quantities are in document-space pixels: the document is treated
// as a vertical strip of total height TotalPx, of which ViewPx pixels
// are visible starting at OriginPx from the document top.
type ScrollModel interface {
    // Geometry returns the current scroll state.
    //   totalPx:  total document pixel height (>= viewPx).
    //   viewPx:   viewport pixel height.
    //   originPx: pixel offset from document top to viewport top.
    Geometry() (totalPx, viewPx, originPx int)

    // DragTopToPixel implements B1: the line at originPx must end up at
    // viewport pixel clickY. Equivalent to subtracting clickY pixels
    // from origin (clamped to [0, totalPx-viewPx]).
    DragTopToPixel(clickY int)

    // DragPixelToTop implements B3: the line currently at viewport
    // pixel clickY must end up at originPx. Equivalent to adding
    // clickY pixels to origin (clamped).
    DragPixelToTop(clickY int)

    // JumpToFraction implements B2: set origin so its position within
    // [0, totalPx-viewPx] is at fraction f in [0,1].
    JumpToFraction(f float64)
}
```

The widget **never** sees runes, lines, or anything mode-specific.
Three operations, all expressed in viewport-relative pixels.

### Text-mode adapter

Implements `ScrollModel` over `*Text`. Internally:

- `Geometry`: `totalPx = file.Nr() * fontH`, `viewPx = scrollr.Dy()`,
  `originPx = (org_line_index_in_file) * fontH`. (Computing
  `org_line_index_in_file` requires a back-scan; in practice, text mode
  treats origin as the rune offset and converts via `BackNL` chains.
  See "Implementation note" below for how this avoids a full file
  scan.)
- `DragTopToPixel(clickY)`: `BackNL(t.org, clickY / fontH)`, then
  `t.SetOrigin(p0, true)`.
- `DragPixelToTop(clickY)`: `t.org + t.fr.Charofpt(image.Pt(scrollr.Max.X, scrollr.Min.Y + clickY))`,
  then `t.SetOrigin(p0, true)`.
- `JumpToFraction(f)`: `p0 = int(f * float64(t.file.Nr()))`; if
  `p0 >= t.q1` snap with `BackNL(p0, 2)`; `t.SetOrigin(p0, false)`.

These are the existing `scrl.go:127-148` formulas, lifted verbatim
into method form.

**Implementation note**: text mode does not need a true pixel origin
to satisfy `Geometry()`. The widget only uses the returned
`(totalPx, viewPx, originPx)` triple to size and position the thumb.
For text mode, returning `totalPx = file.Nr()`, `viewPx = nchars`,
`originPx = t.org` (i.e., reusing the rune-count proxy that `scrpos`
already uses) is sufficient because the thumb math is purely
proportional. The widget treats these as opaque "size in some unit"
values; only the drag methods commit to a unit.

### Rich-mode adapter

Implements `ScrollModel` over `*RichText`. Internally:

- `Geometry`: directly from `frame.TotalDocumentHeight()`,
  `frame.Rect().Dy()`, and `pixelYForOrigin()` (combining
  `LineStartRunes`, `LinePixelHeights`, and `GetOriginYOffset`).
- `DragTopToPixel(clickY)`:
  `newOriginPx = max(0, currentOriginPx - clickY)`; convert back to
  `(line, offset)` via `lineAndOffsetAtPixelY(newOriginPx)`; call
  `frame.SetOrigin(content[line])` and
  `frame.SetOriginYOffset(offset)`.
- `DragPixelToTop(clickY)`:
  `newOriginPx = min(maxOriginPx, currentOriginPx + clickY)`; same
  conversion.
- `JumpToFraction(f)`: `newOriginPx = int(f * float64(maxOriginPx))`;
  same conversion.

Where `maxOriginPx = max(0, totalPx - viewPx)`.

The conversion helpers (`pixelYForOrigin`, `lineAndOffsetAtPixelY`)
already exist in `richtext.go:650-671` and remain in place.

## Scrollbar widget

```go
package main // or a new "scrollbar" subpackage; see "Package layout"

const (
    // MinThumbHeightPx is the minimum on-screen height of the
    // scrollbar thumb. Below this the thumb becomes hard to grab on
    // hi-DPI displays and visually disappears against the track.
    // Chosen empirically to be reliably grabbable; not tied to font
    // height because the scrollbar must remain usable in extremely
    // large documents where a strictly proportional thumb height
    // would be sub-pixel.
    MinThumbHeightPx = 10
)

type Scrollbar struct {
    model    ScrollModel
    rect     image.Rectangle  // current scrollbar rectangle on screen
    lastSR   image.Rectangle  // last drawn thumb rect (dirty cache)
    tmp      draw.Image       // off-screen scratch image for flicker-free draw
    display  draw.Display
}

// SetRect updates the scrollbar's on-screen rectangle.
func (s *Scrollbar) SetRect(r image.Rectangle) { /* ... */ }

// Draw renders the scrollbar (track, thumb, edge) using the frame
// colors at global.textcolors[ColBord] and [ColBack]. Cheap: skips if
// the thumb rectangle hasn't changed since last draw.
func (s *Scrollbar) Draw() { /* ... */ }

// HandleClick runs the latch loop for a single button-down. Reads
// from global.mousectl until the button is released. Implements
// cursor warping to track centerline, debounce, and per-tick re-fire
// against the model. Returns when the button is released and any
// residual mouse events have been drained.
func (s *Scrollbar) HandleClick(button int) { /* ... */ }
```

### Drawing

Renders three rectangles into `tmp`, then blits to the screen image:

1. Track: `tmp.Draw(rect, global.textcolors[frame.ColBord], ...)`
2. Thumb: `tmp.Draw(thumbRect, global.textcolors[frame.ColBack], ...)`
3. Right edge: 1px slice of track color along `rect.Max.X-1`.

`thumbRect` is computed from `model.Geometry()`:

```
thumbTop    = trackTop + trackHeight * originPx / totalPx
thumbBottom = trackTop + trackHeight * (originPx + viewPx) / totalPx
if thumbBottom - thumbTop < MinThumbHeightPx { /* enforce min */ }
```

The downscale guard from `scrl.go:37-41` (right-shift by 10 when
`totalPx > 1<<20`) is preserved to avoid integer overflow on
multi-megabyte documents.

**Intentional deviation from legacy thumb math.** The minimum
thumb-height clamp (`clampThumbHeight`) uses `MinThumbHeightPx = 10`
where `scrl.go:48-53` used the literal `2`. As a consequence, in
the corner case where the legacy thumb just barely fits in the
remaining track (`q.Max.Y + 2 ≤ r.Max.Y`) but the new minimum
does not (`q.Max.Y + 10 > track.Max.Y`), the new code pins the
thumb to the bottom edge while the legacy would extend it down by
2 pixels. This is deliberate — the 10 px floor is the design's
fix for sub-grabbable thumbs on hi-DPI displays. Otherwise the
math is bit-for-bit equivalent to `scrpos`.

Dirty-cache check: if the new thumb rectangle equals
`lastDrawnThumb`, skip the blit. This matches `scrl.go:89-98`.

### Mouse latch

Lifted from `scrl.go:108-165`, with `previewScrSleep` (`wind.go:1481`)
folded in:

```
centerX = (rect.Min.X + rect.Max.X) / 2
firstTick = true
for {
    display.Flush()
    my = clamp(global.mouse.Y, rect.Min.Y, rect.Max.Y-1)
    if global.mouse.Point != (centerX, my) {
        display.MoveTo(centerX, my)
        global.mousectl.Read() // absorb synthetic move event
    }
    clickY = my - rect.Min.Y
    switch button {
    case 1: model.DragTopToPixel(clickY)
    case 3: model.DragPixelToTop(clickY)
    case 2:
        f = float64(my - rect.Min.Y) / float64(rect.Dy())
        model.JumpToFraction(f)
    }
    s.Draw()
    if button == 2 {
        global.mousectl.Read()
    } else {
        if firstTick { firstTick = false; sleep(200ms) }
        else         { scrSleep(80ms) }
    }
    if !buttonHeld(button) { break }
}
drainMouseEvents()
```

Mouse-warp (`display.MoveTo`) keeps the cursor pinned to the
scrollbar's centerline column while the button is held, matching
acme. The synthetic move event generated by `MoveTo` is absorbed via
the immediate `mousectl.Read()`.

### Colors

Hardcoded to `global.textcolors[frame.ColBord]` (track / border) and
`global.textcolors[frame.ColBack]` (thumb fill). The
`WithScrollbarColors()` option on `RichText` is removed. Decorations
should not change color across modes.

## Package layout

Two options:

**Option A — keep in `main`.** Add `scrollbar.go` next to `scrl.go`.
Smallest change. `Scrollbar` references `global.mousectl`,
`global.row.display`, and `global.textcolors` directly, matching the
existing style. `scrl.go` and the scrollbar code in `richtext.go` are
deleted; their adapters live in `text.go` and `richtext.go`
respectively.

**Option B — extract to `scrollbar/` subpackage.** Cleaner boundary,
testable in isolation, but requires threading `display`,
`mousectl`, and a `Colors` struct through the constructor since `main`
globals aren't visible.

**Recommendation**: Option A for the initial refactor. The
existing scrollbar code lives in `main` and uses globals freely; the
unification is already a sufficiently large change. A future PR can
hoist into a subpackage once the API is stable.

## Acceptance criteria

- Visual: text mode and rich mode scrollbars are pixel-identical
  (track color, thumb color, thumb border, minimum thumb size, edge
  pixel).
- Interaction: B1/B3/B2 all behave per the canonical drag semantics
  above in both modes; debounce timing matches `scrl.go` (200ms
  first, 80ms repeat); cursor warps to track centerline; releasing
  the button drains residual events.
- **Tall image regression test**: open a markdown document with an
  image taller than the viewport. Hold B3 over the scrollbar and
  scroll through. The image must scroll smoothly through the
  viewport (i.e., partial-image-visible states must be reachable),
  not jump from "image at top" to "image gone."
- `WithScrollbarColors()` no longer exists; rich-mode windows pick
  up frame color changes (e.g., new color scheme) without restart.
- `previewVScrollLatch`, `previewScrSleep`, the scroll loop in
  `Text.Scroll`, the duplicated scrollbar code in `richtext.go`, and
  `richtext.scrollBg` / `scrollThumb` fields are deleted. No copy
  remains.
- All existing scrollbar tests pass; new tests for the
  `ScrollModel` adapters cover edge cases (empty document,
  document equal to viewport, very large documents triggering the
  `>>10` downscale path, tall-image rich mode).

## Out of scope

- **Horizontal scrollbar unification.** The H-scrollbar currently
  lives in `rich/frame.go` and is invoked via
  `wind.go:previewHScrollLatch`. The latch pattern can be moved to
  the same widget by parameterizing axis (vertical / horizontal),
  but this is deferred to a follow-up. The H-scrollbar's
  per-block-region indexing model (see
  `docs/horizontal-scrollbar-design.md`) makes this a non-trivial
  separate effort.
  - **The V-scrollbar widget is not internally axis-agnostic.**
    The Phase 1 implementation bakes vertical assumptions into
    `clampMouseY`, `warpToCenter` (centerX/my split),
    `computeThumbRect`, `clampThumbHeight`, and `dispatch`'s
    pixel-Y semantics. An H-scrollbar variant would either share
    code via per-axis adapters or require a non-trivial
    parameterization pass. Treat the H-scrollbar follow-up as a
    cleanup phase that revisits this trade-off; do not assume a
    drop-in axis swap.
- **Scroll wheel.** `RichText.ScrollWheel` (`richtext.go:719`) and
  text mode's wheel handling continue to operate independently of
  the latch widget. They call into the same `ScrollModel` adapters,
  so behavior remains consistent, but the widget is not involved.
- **Keyboard scroll** (PgUp/PgDn). Same as scroll wheel.

## Risks

1. **`wind.go.orig` / `wind.go.rej`** are present in the working
   tree, indicating an unresolved merge. Resolve before starting
   PR3 (which edits `wind.go`).
2. **`Text.Scroll` is reentrant via `mousectl.Read()`.** The latch
   loop reads from `global.mousectl` while the body is being
   redrawn. Moving this loop into a shared widget must preserve
   exact ordering of `Flush` / `MoveTo` / `Read` / model-update.
   The existing test surface here is thin; manual verification is
   required.
3. **Preview-mode gating.** `Text.ScrDraw` short-circuits when
   `t.w.IsPreviewMode()` (`scrl.go:77-79`) so the text scrollbar
   doesn't draw on top of the rich-mode scrollbar. The unified
   widget must keep this guard at the *call site* (i.e., the
   `Text` adapter only invokes `s.Draw()` when not in preview
   mode), not inside the widget itself.
4. **Sub-line offset semantics in rich mode.** The fix for tall
   images relies on `SetOriginYOffset` already being honored
   throughout the rendering path. If any code path resets the
   offset (e.g., on relayout, on selection), scrolling through
   tall images will glitch. Audit `SetOrigin` call sites in
   `richtext.go` and `wind.go` before PR3.
