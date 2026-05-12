# Frame rendering: behavior and invariants under variable line height

**Status:** spec, pre-implementation.

**Scope.** The behavior and invariants of `frame.frameimpl` once
variable line height (Phase B2.2 restart) lands. The current
implementation handles uniform line height correctly; this doc
defines what changes and what must continue to hold.

**Reading order.** Start with §1–2 (goals, data model). The
rest of the document spells out per-operation contracts and
algorithms with that model.

**Companion docs.**

- `frame-layout-invariants.md` — the running list of named
  invariants (I-1, I-2, I-5, I-10, I-11 today; this restart
  reintroduces I-3..I-9, I-12 with a stronger architectural
  basis).
- `unified-frame-spans.md` §12 Phase B2 — the slice's
  numbered requirements (R-B2.2.*).

---

## 1. Goals

Under variable line height:

1. **Glyphs on a line share a baseline.** A heading run and a
   body run on the same visual line must align at their
   baseline, not their tops. (Top alignment was attempt 1's
   visible mistake.)
2. **Every layout walk produces the same Y for the same box.**
   `drawtext`, `repaintBoxRange`, `drawsel0`, `ptofcharptb`,
   `charofptimpl`, the SetStyleRange walks, the cursor (tick)
   walk, the scroll walks — all of them. No exceptions.
3. **Click → rune position works inside any styled line.**
   Charofpt of a point inside a heading line returns a rune
   inside that heading, never the rune after the line.
4. **Paint stays inside `f.rect`.** No leaks into the tag bar
   or the next pane.
5. **Plain-text rendering is unchanged.** A frame with no
   styled runs must behave identically to upstream — same Y
   values for every box, same paint ops in the same order
   (up to additive instrumentation).
6. **Selection (`drawsel0`) rectangles cover the whole line
   they're on, including its baseline-aligned glyphs and the
   vertical padding above/below.**
7. **Tick (cursor) sits at the line's baseline-aligned ascent
   for the rune-position's containing line.**

Non-goals (defer):

- Sub-line scroll within a tall replaced element (Slice C C2).
- Continuous-scale glyph scaling — we always pick a
  pre-installed font slot.
- Mid-word character-level split for over-long words (B5
  exclusion; revisit when needed).

---

## 2. Data model

### 2.1 Box (`frbox`)

A box is one contiguous run of text with a single Style and no
embedded newline / tab. Fields:

```go
type frbox struct {
    // Identity / content.
    Ptr   []byte  // UTF-8 bytes; nil for special boxes
    Nrune int     // > 0 for content, -1 for tab/newline
    Bc    rune    // '\n' or '\t' for special boxes; 0 otherwise
    Style Style   // appearance attributes

    // Layout — populated by the layout pass (§5). All four
    // are valid after layout completes; layout walks READ
    // them and must NOT recompute pt from a walk-state
    // accumulator.
    X     int  // box's left-edge X (relative to rect.Min.X)
    Y     int  // top Y of the LINE containing this box
    Wid   int  // box width in pixels = font.BytesWidth(Ptr) for content;
                //   tabstop-driven for tabs; 10000 for newlines (a soft-
                //   wrap forcing sentinel — see §4.3)
    Minwid byte // minimum width for soft-wrap decisions (tab: width of "0")

    // Line-shared metrics — every box on the same line carries
    // the SAME line-derived values. Redundant storage, O(1)
    // lookup. Set by the layout pass from the line's max
    // (per §3.3, §3.4).
    LineH int  // height of the box's line in pixels
    LineA int  // ascent (baseline distance from line top) for the line
}
```

The redundancy in `LineH` / `LineA` per-box is intentional:
every layout walk is then a per-box read, never a re-scan.

### 2.2 Style (unchanged from B4 + KindScale)

```go
type Style struct {
    Kind  Kind
    Fg    draw.Image  // gated by KindColored
    Bg    draw.Image  // gated by KindColored
    Scale float32     // gated by KindScale (B2.2 addition)
}
```

`Kind` bits (B5-current order): `KindColored`, `KindBold`,
`KindItalic`, `KindHidden`, `KindHRule`, `KindCodeFamily`,
`KindScale`. Slice C extends; Bit positions stay stable.

### 2.3 Frame state

```go
type frameimpl struct {
    // Geometry / fonts (unchanged from B4).
    rect              image.Rectangle
    display           draw.Display
    background        draw.Image
    font              draw.Font
    fontBold          draw.Font
    fontItalic        draw.Font
    fontBoldItalic    draw.Font
    fontCode          draw.Font
    fontByScale       map[float32]draw.Font
    defaultfontheight int  // base font height; only used as a
                            // floor / sentinel value
    maxtab            int

    box        []*frbox

    // Layout-snapshot bookkeeping. After any operation that
    // changes box contents (Insert / Delete / SetStyleRange),
    // layoutFrom indicates the first box whose (X, Y, LineH,
    // LineA) may be stale and must be recomputed by the next
    // layout pass. 0 means "everything stale". len(box) means
    // "everything fresh".
    layoutFrom int

    nchars       int
    nlines       int
    maxlines     int  // unchanged: r.Dy() / defaultfontheight (a
                       //   base-height-count estimate; see I-fill)
    lastlinefull bool

    cols [NumColours]draw.Image

    sp0, sp1 int    // selection bounds (rune offsets)
    highlighton bool

    // Mid-line caret state (unchanged).
    ticked      bool
    tickback    draw.Image
    tickimage   draw.Image
    tickscale   int

    // No mutable layout-walk accumulator like attempt 1's
    // f.curLineH. State lives on boxes; walks are pure readers.
}
```

The crucial difference from attempt 1: **no `f.curLineH`**, no
`resetWalkState`. The layout pass mutates `box[i].X/Y/LineH/LineA`
and updates `layoutFrom`. Walks read the fields directly.

---

## 3. Per-line metrics

### 3.1 What is a "line"?

A line is a maximal contiguous run of boxes whose visual rows
overlap. Two boxes are on the same line iff `box[i].Y ==
box[j].Y`. Line boundaries occur at:

1. **Hard newline.** A special box with `Bc == '\n'`. The
   newline box itself is the LAST box on its line (its X is
   wherever the line content ended; its Y is the line top).
2. **Soft wrap.** A content box that would extend past
   `rect.Max.X`. The wrap happens at a word boundary (Phase
   B5 — see §4.3) or, if no word boundary fits, at the box's
   left edge.

### 3.2 Line height (`LineH`)

For a line containing boxes `[i, j]`:

```
LineH(line) = max(boxHeight(box[k])) for k in [i, j]
```

where `boxHeight(b)`:

- Content box (`Nrune > 0`): `fontFor(b.Style).Height()`.
- Special box (`Nrune < 0`): `defaultfontheight`. Special
  boxes are vertically neutral and inherit the line's height
  via the max.

Every box on the line carries the same `LineH`.

### 3.3 Line ascent (`LineA`) and the baseline

For a line containing boxes `[i, j]`:

```
LineA(line) = max(fontAscent(box[k])) for k in [i, j], Nrune > 0
            (special boxes contribute 0; if line is all special,
             LineA(line) = fontAscent(base font))
```

where `fontAscent(b)`:

- `draw.Font.Ascent()` if the font exposes ascent; or
- A computed proxy: `font.Height() * 8 / 10` (a 0.8 ratio that
  matches typical Latin fonts and matches Plan 9 bitmap font
  conventions). Implementer picks whichever is exposed.

The **line baseline** is at `line.Y + line.LineA`. Each glyph
paints at `pt.Y = line.Y + (line.LineA - fontAscent(box))`.

Every box on the line carries the same `LineA`.

### 3.4 Layout invariant

For every adjacent pair `(box[i], box[i+1])`:

- If `box[i+1].Y == box[i].Y` (same line):
  - `box[i+1].X == box[i].X + box[i].Wid` (or `0` if `box[i]`
    is a hard-newline box with `box[i+1].Y == box[i].Y` —
    which is impossible; newlines end their line, so
    `box[i+1].Y > box[i].Y`).
  - `box[i+1].LineH == box[i].LineH`.
  - `box[i+1].LineA == box[i].LineA`.
- If `box[i+1].Y > box[i].Y` (new line — hard or soft wrap):
  - `box[i+1].X == rect.Min.X`.
  - `box[i+1].Y == box[i].Y + box[i].LineH`.
  - `box[i+1].LineH`, `box[i+1].LineA` reflect the new line's
    own max.

These are I-LAYOUT invariants. `validateboxmodel` (under
`-validateboxes`) asserts them for every pair.

---

## 4. Operations

Each operation declares its pre- and post-conditions in terms
of the data-model invariants above. Mutations are followed by
a layout pass (§5).

### 4.1 `Insert(r, p0)` / `InsertWithStyle(r, p0, styles)`

**Pre:** layout fresh up through `box[len(box))`; `f.box` has
`f.nchars` runes total covering visible content.

**Action:** runs `bxscan` on the new bytes to produce a fresh
sub-sequence of boxes (each contiguous non-space content run is
one word box, each space run is one space box; newlines and
tabs are their own boxes; Phase B5). Inserts that sequence at
the correct position. Sets `layoutFrom = nb` (first affected
box).

**Post (after layout pass):** every box has fresh X / Y /
LineH / LineA. `f.nchars` is updated. `f.lastlinefull` is set
iff the layout pass ran out of vertical space.

### 4.2 `Delete(p0, p1)`

**Pre:** as above.

**Action:** removes the boxes covering `[p0, p1)`. Closes
gaps if the removal splits a box. Sets `layoutFrom = nb` of
the deletion start.

**Post:** layout pass recomputes downstream box positions.

### 4.3 `bxscan` (Phase B5 + B2.2)

Splits input bytes into boxes at:

- **Newlines.** Each `'\n'` → its own newline box.
- **Tabs.** Each `'\t'` → its own tab box.
- **Spaces.** A maximal run of U+0020 spaces → its own space
  box. The space box's Wid is `font.BytesWidth(Ptr)` (the
  actual space-character width times the run length).
- **Style boundaries.** When `runeStyles != nil`, a style
  change between consecutive runes flushes the current box and
  starts a new one. A space straddling a style change becomes
  two boxes — one per style.

A "word" box is a maximal contiguous run of non-space non-tab
non-newline characters of one Style. A "space" box is a
maximal contiguous run of U+0020 of one Style.

Soft-wrap rule (in `cklinewrap`, unchanged from upstream):
when the box being placed has `Wid > rect.Max.X - pt.X`, it
wraps to the next line. The previous box on the same line is
typically a space — the natural word boundary. Long words
(Wid > rect.Dx()) wrap to a fresh line and overflow per
R-B5.4 (deferred fix).

### 4.4 `SetStyleRange(p0, p1, styles)`

**Pre:** as above.

**Action:** find boxes covering `[p0, p1)`, split at the
boundaries via `splitbox` if needed, write the new Style on
each affected box. Recompute `Wid` via `boxWid(b)`. Track
whether any affected box's `boxHeight` or `Wid` changed (this
sets `layoutShifted`).

If `layoutShifted`: `layoutFrom = lineStartBox(nb0)` (the
first box on the line containing the change). The layout
pass recomputes from that line forward.

If `!layoutShifted`: `layoutFrom = nb0`. The layout pass
recomputes within the affected range only. (X is unchanged for
boxes after nb1 since Wid didn't change, so a narrow recompute
suffices.)

**Post:** layout pass updates affected boxes. SetStyleRange
then triggers a repaint over `[lineStart, end of frame]` if
`layoutShifted`, or `[nb0, nb1)` if not.

### 4.5 `Redraw(enclosing)`

**Pre:** any state.

**Action:** clears `enclosing` to bg. The next paint walk
re-paints all visible boxes (via `drawtext`).

**Post:** layout state unchanged (Redraw doesn't mutate boxes).

### 4.6 `Ptofchar(p)` and `Charofpt(pt)`

Position queries — pure readers of the layout state. Implement
as `box[nb].X`/`box[nb].Y` lookups plus per-rune walks within
the matching box's `Ptr`.

`Ptofchar(p)`:
- Find `nb` containing rune `p`: walk box list accumulating
  `p`, stop when `p < firstRune(nb+1)`.
- Compute glyph pt: `box[nb].X` + per-rune-widths into
  `box[nb].Ptr` up to `p - firstRune(nb)`. `box[nb].Y` (top
  of line; glyph is at `Y + (LineA - fontAscent)`).

`Charofpt(pt)`:
- Find `nb` whose `[X, X+Wid) × [Y, Y+LineH)` contains pt.
- Walk runes within `box[nb].Ptr` accumulating per-rune
  widths to find the exact rune at `pt.X`.

Neither function calls `cklinewrap`, `advance`, or any walk
that mutates state. They are pure readers.

### 4.7 `DrawSel0(pt, p0, p1, back, text)`

Paints the selection rectangle over runes `[p0, p1)`.

For each visual line covered:
- Read the line's `Y` and `LineH` from the first box on the
  line.
- Compute the selection rectangle: `(X_start, Y, X_end,
  Y + LineH)`.
- Fill with `back`; re-paint each glyph on the line (within
  `[p0, p1)`) at the baseline `Y + LineA - fontAscent`.

Hidden boxes (`KindHidden`) are skipped only in clear-mode
(restoring the unselected state); in highlight mode they
still paint so the user sees what they selected.

### 4.8 Tick (caret)

The caret is rendered at `Ptofchar(sp0)` with rect
`(pt.X - tickscale, Y, pt.X + frtickw*tickscale, Y + LineH)`
where `Y` and `LineH` come from the box containing `sp0`. The
tick scales to the current line's height — so on a heading
line the caret is a tall bar; on a body line it's a short bar.

The tick paints over a backup of the underlying pixels
(`tickback`) so it can be erased cleanly when the selection
moves.

---

## 5. The layout pass

A single forward walk over `box[layoutFrom : ...]` that
computes each box's `X`, `Y`, `LineH`, `LineA`. Runs after any
mutation that changes box contents, Wid, or Style.

### 5.1 Algorithm

```
layoutFrom(start int):
  // Find the line containing box[start]. layout(start)
  // can't recompute correctly without the line's
  // previous boxes' contribution to LineH/LineA.
  start = lineStartBox(start)

  // Seed pt and per-line accumulators from the previous
  // line's end (or rect.Min if start == 0).
  if start == 0:
    pt = rect.Min
    lineH = defaultfontheight  // sentinel
    lineA = defaultAscent
  else:
    prev = box[start-1]
    if prev.Bc == '\n':
      pt = (rect.Min.X, prev.Y + prev.LineH)
    else:
      // start is at a soft-wrap boundary; previous box's
      // Y is one line up, but its LineH is the line we're
      // about to leave's max — same source.
      pt = (rect.Min.X, prev.Y + prev.LineH)
    lineH = defaultfontheight  // recomputed below
    lineA = defaultAscent

  // First pass: walk the affected line(s), computing
  // per-line max-h and max-asc. The first pass establishes
  // line boundaries (where soft-wraps land). The second
  // pass fills LineH/LineA on each box.

  ... see §5.2 for the two-phase algorithm ...
```

### 5.2 Two-phase per-line layout

For each line affected, we need to know `LineH`/`LineA`
BEFORE we know the line's range — but the line's range
depends on box widths (which may have changed).

Algorithm:

```
nb := layoutFrom (after lineStartBox normalization)
pt := the position of box[nb] (see seeding above)

while nb < len(box):
    // Phase A: scan boxes forward from nb until a hard or
    // soft line break, computing pt.X advances and the
    // line's max boxHeight / fontAscent.
    lineStartIdx := nb
    lineStartX   := pt.X
    lineStartY   := pt.Y
    lineH := defaultfontheight
    lineA := defaultAscent
    for nb < len(box):
        b := box[nb]
        // Decide: does b fit on the current line?
        if b is content and pt.X + b.Wid > rect.Max.X:
            // Soft wrap. b moves to the next line.
            break
        if b is newline:
            // Hard wrap — line ends AFTER b.
            // (b's X is the line's tail position; its LineH/
            // LineA inherit the line's max.)
            updateLineMaxes(b, &lineH, &lineA)
            nb++
            break
        // b stays on this line.
        updateLineMaxes(b, &lineH, &lineA)
        pt.X += b.Wid
        nb++
    // Phase B: fill X/Y/LineH/LineA for boxes
    // [lineStartIdx, nb).
    x := lineStartX
    for i := lineStartIdx; i < nb; i++:
        box[i].X     = x
        box[i].Y     = lineStartY
        box[i].LineH = lineH
        box[i].LineA = lineA
        x += box[i].Wid
    // Advance pt to the next line's top.
    pt = (rect.Min.X, lineStartY + lineH)
    // Bail if we're past the visible rect.
    if pt.Y >= rect.Max.Y:
        // Drop boxes beyond this point (matches upstream's
        // chop behavior). Or mark them off-screen.
        ...
        break
```

`updateLineMaxes(b, &lineH, &lineA)` is:
```
h := boxHeight(b)  // content: fontFor(b.Style).Height();
                    // special: defaultfontheight
a := fontAscent(b) // content: fontFor(b.Style).Ascent();
                    // special: 0 (contributes nothing)
if h > lineH: lineH = h
if a > lineA: lineA = a
```

### 5.3 Plain-text regression (I-plain)

A frame whose every box has plain `Style` produces layout
output byte-identical to upstream:

- Every line has `LineH == defaultfontheight`.
- Every line has `LineA == defaultAscent`.
- Every box's `Y` is a multiple of `defaultfontheight`
  starting at `rect.Min.Y`.

Tests for variable line height MUST also pass these plain-text
assertions when no scale fonts are loaded.

---

## 6. Paint walks

Walks that emit draw ops. All are pure readers of `box[i].X`,
`box[i].Y`, `box[i].LineH`, `box[i].LineA`.

### 6.1 `drawtext(pt, text, back)`

Initial full-frame paint. Iterates `f.box`, calling
`paintBox` for each visible box. `pt` argument is ignored
(boxes carry their own X/Y); kept for upstream signature
compatibility.

### 6.2 `repaintBoxRange(nb0, nb1, text, back)`

Repaints `[nb0, nb1)`. Same structure as `drawtext`, just a
narrower range.

### 6.3 `paintBox(b, text, back, clearBg)`

For one box at its layout-stored `b.X`, `b.Y`:

1. If `f.noredraw` or `b.Nrune < 0` or `b.Y >= rect.Max.Y`:
   return.
2. Resolve effective `fg` / `bg` (Style.Fg / Style.Bg if
   KindColored).
3. If `clearBg` or `hasBgOverride`: paint bg rect
   `(b.X, b.Y, b.X + b.Wid, b.Y + b.LineH)`. Clamped at
   bottom to `rect.Max.Y`.
4. If not `KindHidden`: paint glyph at
   `(b.X, b.Y + b.LineA - fontAscent(b))`. This is the
   baseline-alignment rule. Suppress if the glyph would
   extend past `rect.Max.Y`.
5. If `KindHRule`: paint a 1-px horizontal line at
   `(b.X, b.Y + b.LineH/2, b.X + b.Wid, b.Y + b.LineH/2 + 1)`.
   Suppress if past `rect.Max.Y`.

Note that the bg rect uses `LineH` (line height, not box
height) so the bg covers the entire line vertically. Body
text on a heading line gets its bg painted over the heading's
full vertical extent. Without this, the area below a short
glyph on a tall line would show stale pixels on repaint.

### 6.4 `drawsel0(pt, p0, p1, back, text)`

Selection paint. For each visual line in `[p0, p1)`:

1. Find the line via box list.
2. Compute selection rect: from `Ptofchar(start_of_selection_on_this_line).X`
   to `Ptofchar(end_of_selection_on_this_line).X`, Y from
   line.Y to line.Y + line.LineH.
3. Fill with `back`.
4. For each box overlapping the selection on this line,
   re-paint the glyph at its baseline-aligned position using
   `text` color.

---

## 7. Edge cases

### 7.1 Empty frame

`f.box` is empty. Ptofchar(0) returns `f.rect.Min`. Charofpt
of any point returns 0. Layout pass is a no-op.

### 7.2 Off-screen boxes

After the layout pass, any box whose `Y >= rect.Max.Y` is
off-screen. The pass marks the cutoff (sets `lastlinefull`
and truncates `f.box`). `Text.fill` then knows the frame is
full and stops inserting.

### 7.3 Scroll (setorigin / fill)

Each scroll Insert/Delete triggers a layout pass. Lines that
shift Y are correctly relocated. The blit operations in
`Delete` may need rect computations: those use `box[i].Y`
and `box[i].LineH` (NOT `defaultfontheight`). Specifically:

- The "shift up" blit after a deletion needs to know how
  many pixels the post-delete content needs to move up.
  Compute from the layout-pass-updated `Y` values.
- The "fill blank space at frame bottom" call needs to know
  the visible content's bottom: `box[last_visible].Y +
  box[last_visible].LineH`.

### 7.4 Mid-rune Style change in SetStyleRange

`findbox` + `splitbox` handle this. After splitbox the new
boxes have fresh X (recomputed by the layout pass). No
special handling beyond setting `layoutFrom`.

### 7.5 Boxes wider than `rect.Dx()`

Wraps to a fresh line; on that line, the box's content
extends past `rect.Max.X`. `paintBox`'s bg rect is clamped
at `rect.Max.Y`; the glyph paint extends horizontally and
is clipped by the screen image (not by the frame rect).
Acceptable v1 limitation.

---

## 8. Test surface

Tests live in `frame/`. Each pins one or more invariants.

### 8.1 Layout-data tests

- **`TestLayout_PlainTextMatchesUpstream`** — insert plain
  multi-line content, assert every `box.Y` is at a
  `defaultfontheight` step, every `box.LineH ==
  defaultfontheight`, every `box.LineA == defaultAscent`.
- **`TestLayout_HeadingLineUsesMaxHeight`** — insert a
  scaled heading line followed by body lines, assert the
  heading line's `LineH` equals the heading font's height,
  body lines' `LineH` equals the base font's height.
- **`TestLayout_BodyAfterHeadingShiftsByHeadingHeight`** —
  assert `box[firstBodyBox].Y == box[firstHeadingBox].Y +
  headingLineH`.
- **`TestLayout_MixedLineUsesMaxAscent`** — insert a line
  with a scaled run and a plain run, assert all boxes on
  the line share the same `LineA` (the larger of the two).

### 8.2 Baseline-alignment tests

- **`TestPaintBox_GlyphsShareBaselineOnMixedLine`** — insert
  "BIG body" where "BIG" is scale=2, "body" is plain. Capture
  the paint ops. Assert the `Bytes` op for "BIG" and the
  `Bytes` op for "body" have the SAME baseline y (their
  pt.Y differs by the ascent delta, not by zero).

### 8.3 Click-position tests

- **`TestCharofpt_InsideHeadingReturnsHeadingRune`** —
  click at mid-Y of a heading line; assert returned rune is
  inside the heading.
- **`TestCharofpt_InsideBodyReturnsBodyRune`** — click at
  any Y in a body line; assert correct.
- **`TestCharofpt_PastFrameBottomReturnsEndOfBuffer`** —
  click below all content; assert returns total rune count.

### 8.4 Walk-agreement tests

- **`TestAllWalksAgreeOn_pt`** — for every box in a frame
  with mixed content, assert `Ptofchar(firstRune(box)) ==
  (box.X, box.Y)`. Pins I-5.
- **`-validatelayout` flag** — runtime audit inside paint
  walks asserts the same. Test suite runs cleanly under
  `go test -args -validatelayout`.

### 8.5 Bounds tests

- **`TestPaintWithinBounds_PlainContentOverflows`** — fill
  with more lines than fit; assert no draw op extends past
  rect.Max.Y.
- **`TestPaintWithinBounds_ScaleReflowOverflows`** — insert
  plain content then re-style first line as scaled; assert
  no overflow.
- **`TestPaintWithinBounds_TickStaysOnTickLine`** — place
  caret on a heading line, then move to body line; assert
  the tick's height matches the destination line.

### 8.6 Layout-shift tests

- **`TestSetStyleRange_PreservesMarkersOnLine`** — given
  a heading line "## H", re-style the inner "H" to
  scale=2.0; assert "## " markers paint at their correct
  position and aren't erased.
- **`TestSetStyleRange_WidthChangeForcesWrap`** — re-style
  text to bold so the line wraps to two visual rows; assert
  the wrap happens at a word boundary (Phase B5) and that
  no stale pixels remain at the pre-wrap line position.

### 8.7 Scroll tests

- **`TestScroll_DeleteShiftsByLineH`** — scroll down through
  a buffer containing headings; assert no stale pixels
  remain on the lines that shifted.
- **`TestScroll_FillUnderVariableHeight`** — scroll to a
  position whose visible content includes mixed-height
  lines; assert `Text.fill` correctly tops up the frame.

---

## 9. Implementation order

The actual implementation (R-B2.2-restart) can land in this
order, each row staying green at HEAD:

1. **Add the layout-pass infrastructure** — the per-box
   fields, the two-phase algorithm, `lineStartBox`. No
   change in walks yet; walks still use the old pattern.
   Layout pass runs but its outputs aren't consumed.
2. **Migrate `Ptofchar` / `Charofpt`** to read box layout
   fields directly. No regressions expected — Charofpt
   becomes simpler.
3. **Migrate `drawtext` / `repaintBoxRange`** to read box
   fields. Remove the mutable `f.curLineH` accumulator and
   `resetWalkState`. Plain-text byte-identical regression
   (I-plain) must hold.
4. **Migrate `drawsel0`** to read box fields. Selection
   under variable line height starts working.
5. **Migrate the cursor (`tick`)** to read box fields.
6. **Migrate scroll paths** (`Delete`'s blit math; `fill`).
7. **Apply baseline alignment** in `paintBox` — glyphs at
   `Y + LineA - fontAscent` instead of just `Y`.

Each step has tests. Each step keeps the binary buildable
and the regression suite green.

---

## 10. Open design questions

For the implementer to settle, in order:

1. **Per-line storage** (4.1 currently has redundant
   per-box). Alternative: separate `[]lineMeta` indexed by
   line number, plus `box.lineIdx`. Trades memory for
   indirection. Decide based on profiling once the simple
   version works.
2. **Ascent computation** when the draw library doesn't
   expose Font.Ascent(). The proxy `height * 8 / 10` works
   for Latin scripts but might be wrong for CJK. The
   monospace Go fonts have a known ascent; can hard-code
   per family. Defer until needed.
3. **`f.maxlines` semantics under variable height** —
   currently a base-height-count estimate used by `Text.fill`
   for batching and `1/3-screen` scroll math. Keep as-is
   (estimate; not authoritative) or migrate to
   pixel-based. Defer.
