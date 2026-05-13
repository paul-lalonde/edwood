# Frame layout & paint invariants

Canonical list of invariants the frame package must satisfy.
Each invariant names its purpose, the failure mode it prevents,
and the test(s) that pin it. Add to this list when a new
invariant is discovered; add the test alongside the code that
uses the invariant.

The frame's job is to render a buffer of styled boxes inside a
fixed rect. Most invariants here are general — they apply
regardless of whether line heights are uniform or variable. The
variable-line-height work (Phase B2.2) added more invariants
specific to that case; those have been removed pending a
restart with a cleaner per-box-Y architecture.

## I-1: paintBox stays inside f.rect

Every paint op `paintBox` emits resolves to pixels entirely
within `f.rect`. The mechanism differs by op type:

- A box whose `pt.Y >= f.rect.Max.Y` OR `pt.Y + LineH <=
  f.rect.Min.Y` produces no ops at all — fully off-screen.
- A partially-visible box (line top inside the rect, bottom
  past `rect.Max.Y` — or vice versa) IS painted, but only
  its visible slice survives:
  - The bg rect's Y edges are clamped to `[rect.Min.Y,
    rect.Max.Y]` before the Draw call.
  - The glyph (`Bytes`) paint is NOT suppressed for partial
    visibility; instead `f.background` is bounded to
    `f.rect` (sub-image or equivalent), so glyphs that
    extend past `rect.Max.Y` are clipped by the underlying
    image rather than dropped entirely.
  - The `KindHRule` decoration is suppressed when its
    vertical center is past `rect.Max.Y` (a stylistic
    accent that's acceptable to lose on a partial line).

**Failure mode:** content leaks into the tag bar below the
frame or the next pane in the column (image not bounded to
rect); OR partial lines disappear entirely when they should
be partially visible (overly-aggressive suppression).

**Tests:** `TestPaintWithinBounds_*` (to be re-added during
B2.2 restart — they referenced KindScale).

## I-2: layout-walking functions break at f.rect.Max.Y

`drawtext`, `repaintBoxRange`, and any future paint walk break
out of their box loop as soon as `cklinewrap` returns
`pt.Y >= f.rect.Max.Y`. Off-screen boxes never enter `paintBox`.

**Failure mode:** the "row of overlapping glyphs" effect at the
frame's bottom when content from the buffer extends past the
visible area and every off-screen box clamps to
`pt.Y = rect.Max.Y`.

**Tests:** covered by I-1 (same fixtures).

## I-5: paint walks produce the same pt as ptofcharptb

For every box the paint walk hands to `paintBox`, the pt must
equal `ptofcharptb(firstRune(box))`. Any divergence means the
walks disagree on layout — repaint will land at a different Y
from the position-query walk, leaving stale pixels behind.

**Failure mode:** doubled lines after `SetStyleRange` re-styles
across heading lines (the bug class that drove B2.2-attempt-1's
ten-fix cascade).

**Tests:** `-validatelayout` flag (to be re-added). Runtime
audit in `repaintBoxRange` that panics on drift; frame test
suite runs cleanly under `go test -args -validatelayout`.

## I-10: boxWid is the single content-box width helper

`f.boxWid(b)` is the only place in the package that computes
the width of a content box from `(Style, Ptr)`. Every site that
needs "the right width for this box" calls `boxWid`. Special
boxes (`Nrune < 0`) keep their tabstop/metric-driven widths and
must NOT be routed through `boxWid` (it panics).

`validateboxmodel` asserts `b.Wid == f.boxWid(b)` for content
boxes under `-validateboxes`. This structurally prevents the
SetStyleRange-forgot-Wid bug class.

**Failure mode:** bold glyphs that are wider than regular get
clipped at their right edge by the next box's bg paint —
because layout sized the box with the regular font's
`BytesWidth` but rendered with the bold font.

**Tests:** `TestBoxWid_PlainBoxMatchesBaseFont`,
`TestBoxWid_BoldBoxUsesBoldFont`,
`TestValidateBoxModel_PanicsOnWidthMismatch`.

## I-11: paintBox is the single per-box paint point

`(*frameimpl).paintBox(b, pt, text, back, clearBg)` is the only
function in `frame/draw.go` that resolves a content box's font
(`fontFor`), resolves its effective fg/bg, paints background,
paints glyphs, and applies per-box decorations (`KindHidden`
suppression, `KindHRule`). Adding a new decoration is a
one-site edit.

**Failure mode:** historical — `drawtext` and `repaintBoxRange`
had duplicated paint logic; adding `KindHRule` would otherwise
require touching both.

**Tests:** `TestPaintParity_DrawtextAndRepaintAgreeOnFont`,
`TestKindHidden_SkipsGlyphPaintInRepaintBoxRange`.

## I-12: Spans overlay emits one rect per visual line

The "Spans" debug overlay (`Text.paintSpansOverlay` →
`Frame.DrawOutlineRect`) draws one outlined rectangle per
**visual line** of every non-plain styled region — not one
hull rectangle per styled region. When a styled region soft-
wraps across multiple visual lines, each line gets its own
outline.

Per-line geometry:
- **First line of the region:** left edge = `Ptofchar(s)`.X;
  right edge = `f.rect.Max.X` if the region continues onto a
  next line, else `Ptofchar(e)`.X (one past the region's last
  rune).
- **Intermediate lines:** left edge = `f.rect.Min.X`; right
  edge = `f.rect.Max.X`.
- **Last line:** left edge = `f.rect.Min.X`; right edge =
  `Ptofchar(e)`.X. Top edge = the line's top Y; bottom edge =
  top + `DefaultFontHeight()`.

**Failure mode:** a single hull rectangle from `Ptofchar(s)` to
`Ptofchar(e)` produces a visually wrong outline whenever the
region wraps — its left edge cuts into mid-line continuation
text on line 2+ and its right edge clips the original line's
right-side glyphs. The user-visible result is a box that does
not enclose the styled glyphs.

**Tests:** `TestPaintSpansOverlay_*` cover single-line,
multi-line, and edge cases.

## Architectural notes (carried over from B2.2-attempt-1)

These weren't "invariants" but lessons learned. Document them
so the restart can build on them.

- **Walk-time state mutation is fragile.** Attempt 1 stored
  `f.curLineH` (the per-line max-height accumulator) on the
  frame and required every layout walk to call
  `resetWalkState` at its start. Each missed walk produced a
  visible bug. **Lesson:** in the restart, store per-box Y on
  each box itself, computed once during
  Insert/Delete/SetStyleRange and read by all subsequent
  layout walks. Walks become trivial readers, agreeing by
  construction.
- **Box-paint divergence between walks is the root bug
  class.** `drawtext` and `repaintBoxRange` historically used
  a manual `pt.X += b.Wid` advance that didn't match
  `ptofcharptb`'s `advance(qt, b)`. Under uniform line height
  the two happened to agree; under variable, they diverged.
  **Lesson:** there must be ONE pt-advance function used by
  all walks. After attempt 1 we factored `advance()` as the
  single newline handler, but other divergences remained
  (paintBox vs ptofcharptb, narrow vs full repaint).
- **Mid-frame layout walks need prefix state.** A narrow
  repaint that starts at box N can't know the line height of
  the line containing N without re-walking from box 0. The
  walk-from-0-paint-only-in-range pattern preserves
  on-the-wire parsimony while keeping pt accurate — but only
  if walks agree on per-line state, which I-5 covers.
- **Cursor and scroll paths under variable line height were
  never addressed in attempt 1.** The tick rect uses
  `defaultfontheight`; the scrolling `Delete` uses it for
  blit rect arithmetic; `insertbyteimpl`'s pt0/pt1 parallel
  walk shares one `curLineH` across two parallel positions.
  **Lesson:** factor these explicitly into the restart's
  plan as named subrows.
- **Baseline alignment, not top alignment.** Attempt 1 had
  paintBox draw every glyph at `pt.Y` (the line's top) — so
  short-font runs sat at the top of a tall line with empty
  space hanging below them. Visually this means heading-text
  runs and adjacent body-text runs don't share a baseline,
  which is wrong for any typographically sane rendering.
  **Lesson:** for the restart, the renderer must paint each
  glyph at `pt.Y + (line.baseline - font.ascent)` (or
  equivalent), so all glyphs on a line share a baseline. The
  line's baseline is `line.maxAscent` (max ascent of any
  font used on the line). Each font's ascent comes from
  `draw.Font.Ascent()` (or computed from `font.Height()` if
  no Ascent method). Layout walks need: per-line ascent in
  addition to per-line height. paintBox uses ascent for
  pt.Y adjustment.

## Debug tools (from B2.2-attempt-1, to be restored)

- `go test ./frame/ -args -validateboxes` — assert the box
  model is internally consistent (Nrune matches Ptr, Wid
  matches boxWid).
- `go test ./frame/ -args -validatelayout` — assert each
  paint walk's pt for box `nb` equals `ptofcharptb` for that
  box's first rune.
- `Box` tag command — toggles a Medblue 1-pixel outline
  around every painted box. Useful for debugging layout.
- `Spans` tag command — outlines each non-plain region from
  the spans store after every paint. Useful for confirming
  wire→layout flow.
- `visibleGlyphPaintsContaining(disp, rect, needle)` test
  helper — scans recorded draw ops, tracks overpaints by
  fill rects in both X and Y, and returns only the paints
  surviving in the visible state. The authoritative "what
  does the user see?" predicate for tests.
