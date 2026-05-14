# Frame layout — first-principles design (B2.3)

**Status:** design, pre-implementation.

**Scope.** A clean-slate re-design of how `frame.frameimpl` stores
and recomputes per-box geometry under variable line height. This
document defines the **layout function** that owns all geometry,
the **per-line summary table** that lets consumers answer
line-shaped questions in O(1), and the **mutator flow** that
prevents the duplicate-calculation class of bug that caused B2.2
to thrash.

**Companion docs.**
- `frame-rendering-spec.md` — the variable-line-height behavior
  spec. Section 5 specifies the per-line math (boxHeight, LineH,
  LineA, two-phase pass); this doc cross-references that math
  rather than restating it.
- `frame-layout-invariants.md` — the running invariants list.
  This doc adds I-LAYOUT-* invariants (§7); the next pass on the
  invariants doc folds them in.
- `layout-once-invariant.md` — the audit/diagnosis that motivated
  this rewrite (the table of duplicate-recompute sites).
- `unified-frame-spans.md` — feature umbrella. Phase B2.3 (the
  set of work driven by this doc) lives there.
- `frame-scrollbar-spec.md` — scrollbar work is **out of scope
  here**. The reader API §4 surfaces the data the scrollbar will
  need; the snap behavior lives in that spec.

---

## 1. Goals

Single-sentence form: **`relayoutFrom` is the only function
that writes box.X / box.Y / box.LineH / box.LineA, and every
other function in the frame package reads them.**

Concretely:

1. **Layout-once.** For any mutation (Insert / Delete /
   SetStyleRange), geometry is recomputed exactly once via a
   single forward pass over the affected suffix of the box list.
2. **No accumulator walks outside the layout function.**
   `cklinewrap`, `cklinewrap0`, `advance`, `ptofcharptb`,
   `charofptimpl`, `_draw`'s pt-accumulator, the per-box blit
   loops in `deleteimpl` / `insertbyteimpl`, all the
   `lineHForAdvance` and `lineHAtPt` helpers — gone or contained
   to `relayoutFrom`'s internals.
3. **Pure-reader query API.** `Ptofchar`, `Charofpt`, `LineHAt`,
   tick, scroll, blit-extent — all of them resolve to box-field
   reads or per-line-table lookups.
4. **O(1) per-line lookups for paint and scroll.** Functions that
   need "the top Y of the line containing rune p" or "the height
   of the n-th visible line" do not walk the box list.
5. **The mutator flow is uniform.** Insert, Delete, and
   SetStyleRange follow one shape: pre-mutation read → mutate box
   list → relayoutFrom → blit shift → paint from box state.
6. **Plain-text parity is preserved.** A frame with no styled
   boxes produces byte-identical box geometry to the pre-B2.2
   implementation. (Spec §5.3.)

Non-goals (deferred):

- Scrollbar snap (`SnapTop` / `SnapBottom` / `SnapPixel`) — owned
  by `frame-scrollbar-spec.md`. This doc surfaces the reader
  primitives the scrollbar will use.
- Continuous-scale glyphs / live font hot-swap — Slice C.
- Sub-line scroll within a tall replaced element — Slice C C2.
- Mid-word character-level split for over-long words — B5.4
  follow-up, separate row.

---

## 2. Data model

### 2.1 Per-box fields (no change from spec §2.1)

```go
type frbox struct {
    // Identity / content.
    Ptr    []byte
    Nrune  int
    Bc     rune
    Style  Style

    // Layout — written ONLY by relayoutFrom; read everywhere else.
    X      int  // box's left-edge X
    Y      int  // top Y of the line containing this box
    Wid    int  // box width in pixels
    Minwid byte

    // Line-shared metrics — same value for every box on a line.
    LineH  int
    LineA  int
}
```

Pre-relayout the values are stale; post-relayout the I-LAYOUT
invariants (§7) hold. No layout consumer may read these fields
between a mutation and the relayout that follows it.

### 2.2 Per-line summary (new)

```go
type lineSummary struct {
    FirstBox int  // index into f.box of the first box on this line
    TopY     int  // line's top Y (== f.box[FirstBox].Y)
    LineH    int  // == f.box[FirstBox].LineH
    LineA    int  // == f.box[FirstBox].LineA
}

type frameimpl struct {
    // existing fields ...
    box   []*frbox
    lines []lineSummary   // index i is the i-th visible line
}
```

The summary table is **fully derived** from `f.box` and rebuilt
by `relayoutFrom` in the same pass that fills box fields. It
exists for O(1) answers to line-shaped questions:

- *Which visual line contains rune p?* — binary search
  `f.lines` by `FirstBox`'s rune-offset, or one pass over
  `f.box[lines[k].FirstBox : lines[k+1].FirstBox]`.
- *How tall is the n-th visible line?* — `lines[n].LineH`.
- *What's the bottom Y of the content area?* —
  `lines[len(lines)-1].TopY + lines[len(lines)-1].LineH`.
- *Is the frame full?* — `bottomY >= rect.Max.Y` is `lastlinefull`.
- *For a click at pt.Y, which line was clicked?* — binary search
  `f.lines` by `TopY` (lines are Y-sorted by construction).

Per-line metrics are also stored redundantly per-box (LineH,
LineA) so the existing per-box readers keep working without an
extra hop. The summary table is the **canonical** form; per-box
copies are denormalized for lookup speed.

The table is small. A typical frame fits ~50 visible lines; even a
1000-line frame is one cache line worth of summaries.

### 2.3 `lastlinefull` ownership

Set by `relayoutFrom` in one place: after the last line is laid
out, if `bottomY >= rect.Max.Y`, `lastlinefull = true`, else
`false`. No other function sets it. The Delete-time reset
currently in `frame/delete.go` (commit 677ab5e) is then folded
back into the layout pass — it falls out of the same rule.

---

## 3. The layout function (`relayoutFrom`)

### 3.1 Signature and contract

```go
// relayoutFrom recomputes geometry for the suffix of f.box
// starting at the line containing box[nb0], emitting:
//   - box[i].X, .Y, .LineH, .LineA for every i >= lineStart(nb0)
//   - f.lines rebuilt for the affected range
//   - f.lastlinefull set per §2.3
//
// Caller must ensure box content (Ptr, Nrune, Style, Wid) is
// correct for every affected box BEFORE calling. relayoutFrom
// does not split, merge, or otherwise mutate the box list — it
// only assigns positions.
//
// nb0 < 0 or > len(f.box) is treated as 0 / no-op respectively.
// nb0 not at a line start is normalized to the line's first box.
func (f *frameimpl) relayoutFrom(nb0 int)
```

### 3.2 Algorithm

Two-phase per line (spec §5.2):

1. **Phase A — line scan.** From `nb0` (normalized to the line
   start), walk boxes forward accumulating `pt.X`, tracking the
   line's max `boxHeight` and max `fontAscent` (the
   `updateLineMaxes` helper). Stop when:
   - the next box wouldn't fit (soft wrap), OR
   - the current box is a hard newline (it ends the line), OR
   - we reach `len(box)`.
2. **Phase B — line fill.** Write `X / Y / LineH / LineA` on
   every box in `[lineStart, nb)`. Append one `lineSummary`
   entry. Advance `pt = (rect.Min.X, lineStartY + lineH)`.
3. Loop to next line until `len(box)` is consumed.

Off-screen boxes (`pt.Y >= rect.Max.Y`): **still get geometry
written**. The paint side bails on visibility (`paintBox` returns
if `b.Y >= rect.Max.Y`); the layout side stays complete so
"any-line-shift" detection and the position readers can still
answer correctly for off-screen runes. (This matches what the R7
work was reaching for; we make it explicit.)

### 3.3 Splitbox is NOT in the layout function

Soft-wrap on a long word (Wid > rect.Dx()) requires splitting
the box. **Splitbox stays out of `relayoutFrom`.** Reasons:

1. `relayoutFrom` is a layout walker; box-list mutation belongs
   to the mutator path (Insert / bxscan).
2. The two-phase loop relies on each box's `Wid` being stable
   for the duration of the pass; an inline split would
   invalidate iterator state.
3. Long-word splits are rare; folding them into the layout
   pass costs every pass a branch that almost never fires.

Instead, the mutator (`bxscan`) pre-splits over-long boxes
before calling `relayoutFrom`. The split decision uses only
`b.Wid > rect.Dx()` — independent of layout state — so it can
be made up-front. (See §6.1.)

### 3.4 Seeding for partial relayout

For `nb0 == 0`: seed `pt = rect.Min`.

For `nb0 > 0`: read `box[nb0-1]` (the last box of the previous
line, which was already laid out). Seed:
- If previous box was a hard newline: `pt = (rect.Min.X,
  prev.Y + prev.LineH)`.
- Else (soft-wrap boundary): same formula. (Soft-wraps end the
  previous line, so the previous box's `Y + LineH` is the new
  line's top either way.)

The boundary normalization (caller passes any `nb` in the
target line; `relayoutFrom` walks back to the line's first box)
is the safety net so callers don't have to compute line starts.

Until partial relayout is wired through every mutator, callers
pass `nb0 = 0`. Box lists are short (frame contents at any one
time fit on screen), so the full pass is cheap.

---

## 4. The reader API

Every layout consumer routes through one of these primitives.
No consumer walks the box list with its own pt accumulator.

### 4.1 `Ptofchar(p int) image.Point`

Find `nb` containing rune `p` (rune-offset walk over `f.box`).
Return `(box[nb].X + glyphOffset, box[nb].Y)` where
`glyphOffset = font.BytesWidth(box[nb].Ptr[:byteOffset(p)])`.

### 4.2 `Charofpt(pt image.Point) int`

Binary-search `f.lines` by `TopY` to find the line containing
`pt.Y`. Walk that line's boxes by `X` to find the matching
box. Walk runes within the box's `Ptr` to find the rune at
`pt.X`.

### 4.3 `LineHAt(p int) int`, `LineAAt(p int) int`

Find the box containing `p`; return `box[nb].LineH` or
`.LineA`.

### 4.4 Line-shaped queries

```go
// Lines() returns the number of visible lines.
func (f *frameimpl) Lines() int { return len(f.lines) }

// LineRange(i) returns [firstBox, nextLineFirstBox).
func (f *frameimpl) LineRange(i int) (int, int)

// LineAt(i) returns f.lines[i].
func (f *frameimpl) LineAt(i int) lineSummary
```

These power the scrollbar (`Lines`, ratios), partial-clear
extent (line bounding rects), and paint walks (line iteration).

### 4.5 Selection geometry

`drawsel0` iterates the lines spanning the selection. For each
covered line `i`:
- `Y = lines[i].TopY`, `H = lines[i].LineH`.
- `Xstart` / `Xend` from the boxes inside the selection.
- Fill rectangle (Xstart, Y, Xend, Y+H); repaint glyphs at
  baseline `Y + LineA - fontAscent`.

No `cklinewrap` traversal.

### 4.6 Tick (caret)

`(pt, lineH) = (Ptofchar(sp0), LineHAt(sp0))`. Tick rect uses
`lineH` for its height (spec §4.8).

---

## 5. Removed code paths

The functions and call sites listed below get deleted as part
of the migration. Each row also lists the reader it routes to.

| Function | Status | Reader replacement |
|---|---|---|
| `(*frameimpl).cklinewrap` | DELETE | none — was an internal walker |
| `(*frameimpl).cklinewrap0` | DELETE | none |
| `(*frameimpl).advance` | DELETE | none |
| `(*frameimpl).ptofcharptb` | DELETE | `ptOfCharReader` |
| `(*frameimpl).ptofcharnb` | DELETE | `ptOfCharReader` |
| `(*frameimpl).charofptimpl` | DELETE | `charOfPtReader` |
| `(*frameimpl).lineHForAdvance` | DELETE | `LineHAt` / box-field read |
| `(*frameimpl).lineHAtPt` | DELETE | line-table lookup |
| `(*frameimpl)._draw` accumulator | DELETE | post-relayout box-field read |

Call sites currently in `deleteimpl`'s inner loop and
`insertbyteimpl`'s pt-walk loop are restructured per §6.

Names of internal helpers used by `relayoutFrom` may be kept
(`updateLineMaxes`, etc.); the public-shaped functions in the
table above go away.

---

## 6. Mutator flow

### 6.1 `Insert` / `InsertByte` / `InsertWithStyle`

```
1. Compute pre-mutation pt0 = Ptofchar(p).
2. Run bxscan on the new bytes to produce a slice of new boxes.
   - bxscan applies splitbox eagerly for over-long words.
   - bxscan does NOT set X/Y/LineH/LineA; those come from
     relayoutFrom.
3. Splice the new boxes into f.box at position p.
4. relayoutFrom(0)    // or relayoutFrom(nbStart) once partial works
5. Compute post-mutation pt1 = Ptofchar(p + insertedRunes).
6. Bulk-blit the tail content downward by (pt1.Y - pt0.Y).
7. Paint the inserted range plus any reflowed tail.
8. Update sp0/sp1 / Tick / nlines / lastlinefull (the last is
   already done by relayoutFrom — read it).
```

Step 4 may be `relayoutFrom(0)` in the first cut; tightening to
a partial pass is a follow-up after correctness is established.

### 6.2 `Delete`

```
1. Compute pt0 = Ptofchar(p0), pt1 = Ptofchar(p1) BEFORE
   deletion.
2. Splice boxes [n0, n1) out of f.box (gaps closed, splitbox
   for partial-box deletes).
3. relayoutFrom(0)
4. Bulk-blit the post-deletion tail upward by (pt1.Y - pt0.Y),
   then clear+repaint the variable-height case.
5. Update sp0/sp1 / Tick / nlines / lastlinefull (from
   relayoutFrom).
```

Per-box blit-and-walk loop (current `deleteimpl` inner body)
goes away.

### 6.3 `SetStyleRange`

```
1. Find boxes [nb0, nb1) covering [p0, p1).
2. Splitbox at the boundaries if needed.
3. Write Style on each box; recompute Wid via boxWid(b).
4. relayoutFrom(0)
5. Detect shift: if any line's TopY changed, repaint from the
   first changed line to end of frame; else repaint
   [nb0, nb1).
```

Shift detection is a per-line-table diff: snapshot
`f.lines` (or just the relevant TopY values) before step 4;
compare after. O(lines), no walk.

### 6.4 Scroll / fill (`Text` side)

The bxscan-driven path used by `Text.fill` and `Text.scroll`
becomes:

```
1. nframe = new frameimpl on the staging rect.
2. nframe.bxscan(bytes) populates nframe.box.
3. nframe.relayoutFrom(0) — one pass, complete.
4. Read pt1 = nframe.lastBox.Y + nframe.lastBox.LineH (or
    lines[-1].TopY + .LineH).
   No _draw walk, no second relayout.
5. Splice nframe.box into parent f.box.
6. f.relayoutFrom(spliceStart) — geometry recomputed for the
    parent in its own coords.
7. Blit + paint per §6.1.
```

This is the architectural fix for the bxscan→_draw→bulk-blit
divergence described in `layout-once-invariant.md` §"Why mostly
readers...".

---

## 7. Invariants

These add to `frame-layout-invariants.md`. Each is testable.

### I-LAYOUT-1: layout-once

Between any two consecutive `relayoutFrom` calls, every
non-`relayoutFrom` function reads `box[i].X / .Y / .LineH /
.LineA` and `f.lines[*]` without writing them.

**Test:** instrument `box[i].X` / `box[i].Y` with a write
counter inside `relayoutFrom` only; assert each mutator
operation increments the counter exactly once per affected
box.

### I-LAYOUT-2: line-table consistency

For every line `i` in `f.lines`:
- `lines[i].FirstBox` is a valid index into `f.box`.
- `box[lines[i].FirstBox].Y == lines[i].TopY`.
- `box[lines[i].FirstBox].LineH == lines[i].LineH`.
- `box[lines[i].FirstBox].LineA == lines[i].LineA`.
- For every `j` in `[lines[i].FirstBox, lines[i+1].FirstBox)`:
  `box[j].Y == lines[i].TopY`, `box[j].LineH == lines[i].LineH`,
  `box[j].LineA == lines[i].LineA`.

**Test:** `validateboxmodel` assertion under `-validateboxes`.

### I-LAYOUT-3: monotone line top

For every adjacent pair of lines:
`lines[i+1].TopY == lines[i].TopY + lines[i].LineH`.

**Test:** same fixture.

### I-LAYOUT-4: lastlinefull is layout-derived

`f.lastlinefull == (lines[-1].TopY + lines[-1].LineH >=
rect.Max.Y)` after any `relayoutFrom`.

**Test:** Insert / Delete / SetStyleRange scenarios assert
`lastlinefull` matches the formula.

### I-LAYOUT-5: paint matches layout

For every box painted via `paintBox`, the (X, Y) handed in
match `box[i].X, box[i].Y`. Equivalent: post-paint, the
visible rect of every painted box equals the box's layout
rect.

**Test:** `-validatelayout` flag — paint walks emit (box, pt)
tuples; comparator asserts pt == box.{X,Y}.

These supersede the looser I-5 (paint == ptofcharptb): once
the legacy walks are gone, ptofcharptb is gone too, and the
only meaningful agreement is paint-state vs box-state.

---

## 8. Migration order

Each row is one CODING-PROCESS pass; commits land independently.

1. **Per-line summary table.** Build it in `relayoutFrom`. No
   consumers yet. Tests assert I-LAYOUT-2 / I-LAYOUT-3.
2. **Move `lastlinefull` ownership into `relayoutFrom`.** Drop
   the explicit reset in `deleteimpl`; assert I-LAYOUT-4.
3. **Route `Charofpt` / position queries through the
   summary table.** Performance + correctness wash.
4. **Eliminate `_draw` as accumulator walker.** `bxscan`
   reads `pt1` from the staging frame's last box / line-table
   entry. Single biggest correctness gain — this is the
   suspected root of the scroll-overlap glitches.
5. **Restructure `deleteimpl`.** Pre-mutation reader,
   single bulk blit, single relayout, single post-relayout
   paint. Delete the inner per-box loop.
6. **Restructure `insertbyteimpl`.** Same pattern.
7. **Restructure `SetStyleRange`** to use the line-table diff
   for shift detection (replaces `contentBottomY` snapshot).
8. **Delete the legacy walkers** (`cklinewrap`, `cklinewrap0`,
   `advance`, `ptofcharptb`, `ptofcharnb`, `charofptimpl`,
   `lineHForAdvance`, `lineHAtPt`). Compile errors at the call
   sites are the migration checklist.
9. **Wire `-validatelayout` / `-validateboxes`** to assert the
   I-LAYOUT-* invariants. Should be green by construction at
   this point.

Each row commits independently with full regression.sh green.
A row may not introduce a regression of the manual test
fixture (`test-md-layout.md`), screen-tested before commit.

---

## 9. Test plan

### 9.1 Unit (`frame/`)

- **Layout pass.** Plain-text frames produce byte-identical
  geometry to the pre-B2.2 baseline (spec §5.3 / I-plain).
  Variable-height frames hit each of: heading-at-top,
  heading-mid-frame, heading-immediately-after-heading,
  wrapped-bold-span (soft-wrap inside styled run),
  long-word fallback, tab-after-heading.
- **Line table.** I-LAYOUT-2 / I-LAYOUT-3 fixtures for each
  of the scenarios above.
- **`lastlinefull`.** Insert / Delete / SetStyleRange across
  the rect.Max.Y boundary asserts I-LAYOUT-4.
- **Mutators.** Pre/post pt0/pt1 readers + a snapshot of the
  box geometry — assert one-and-only-one `relayoutFrom`
  fires per operation (I-LAYOUT-1).
- **Paint parity.** I-LAYOUT-5 fixture for each mutator.

### 9.2 Manual

`docs/designs/features/test-md-layout.md` opened in edwood
with `md2spans`, then exercise §1–§13's scenarios with click,
scroll, select, and tick placement. Sign-off criterion: every
section behaves correctly across one full scroll up + one
full scroll down, with no visible overlap or stale glyphs.

### 9.3 Regression

`regression.sh` green at every commit on the migration order
above.
