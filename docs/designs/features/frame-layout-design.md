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
   SetStyleRange follow one shape: snapshot line table → mutate
   box list → relayoutFrom → diff line table → issue blit/paint
   ops (§3.5).
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
    // Content — written by the mutator path (bxscan / Insert /
    // Delete / SetStyleRange) AND by relayoutFrom when it
    // performs eager split or eager coalesce (§3.3). Wid is
    // recomputed from Ptr+Style on each content write.
    Ptr    []byte
    Nrune  int
    Bc     rune
    Style  Style
    Wid    int  // box width in pixels
    Minwid byte

    // Layout — written ONLY by relayoutFrom; read everywhere
    // else. (X/Y/LineH/LineA + the parallel f.lines table.)
    X      int  // box's left-edge X
    Y      int  // top Y of the line containing this box

    // Line-shared metrics — same value for every box on a line.
    LineH  int
    LineA  int
}
```

Pre-relayout the layout fields are stale; post-relayout the
I-LAYOUT invariants (§7) hold. No layout consumer may read
X/Y/LineH/LineA or `f.lines` between a mutation and the
relayout that follows it.

### 2.2 Per-line summary (new)

```go
type lineSummary struct {
    FirstBox  int  // index into f.box of the first box on this line
    FirstRune int  // rune offset (sum of Nrune over f.box[:FirstBox])
    TopY      int  // line's top Y (== f.box[FirstBox].Y)
    LineH     int  // == f.box[FirstBox].LineH
    LineA     int  // == f.box[FirstBox].LineA
}

type frameimpl struct {
    // existing fields ...
    box   []*frbox
    lines []lineSummary   // index i is the i-th visible line
}
```

`FirstRune` is the rune-coordinate identity of a line — stable
across box-list-index shifts caused by inserts/deletes within
earlier lines, and the key the §3.5 diff uses to match
pre-mutation and post-mutation lines.

The summary table is **fully derived** from `f.box` and rebuilt
by `relayoutFrom` in the same pass that fills box fields. It
exists for O(1) answers to line-shaped questions:

- *Which visual line contains rune p?* — binary search
  `f.lines` by `FirstRune` (monotone non-decreasing).
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
// owns position assignment and is also the single site that
// performs long-word splitbox calls (§3.3); it does not merge
// boxes or otherwise restructure content.
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

### 3.3 Long-word splitbox lives inside `relayoutFrom`

A box whose `Wid > rect.Dx() - pt.X + rect.Min.X` cannot be
placed at the current X cursor; if `Wid > rect.Dx()` it cannot
fit on *any* line and must be broken. The split happens
**inline in the phase-A scan**, not in `bxscan` or any
upstream pass. `relayoutFrom` is therefore the single writer
of geometry *and* the single site that calls `splitbox` for
long-word fallback.

Per-iteration handling at box `nb`:

1. If `b.Wid` fits at the current `pt.X`, advance normally.
2. If `b.Wid` does not fit at `pt.X` **and** `b.Wid <=
   rect.Dx()`, the box belongs on the next line (soft wrap).
   Close the current line and restart phase-A with `b` at
   `pt = (rect.Min.X, prevY + prevLineH)`.
3. If `b.Wid > rect.Dx()` — the long-word case — call
   `splitbox(b, k)`, where `k` is the largest rune-prefix of
   `b.Ptr` whose pixel width fits the remaining line space
   (or, if even one rune doesn't fit at `pt.X`, the largest
   prefix that fits starting from `rect.Min.X`, after closing
   the line first). The splice replaces `f.box[nb]` with two
   boxes: the leading piece at `nb` (now line-fits) and the
   trailing piece at `nb+1`. Continue phase-A with the leading
   piece.

**Multi-split propagation.** The trailing piece is the next
box the loop visits. If it is itself still `> rect.Dx()`, the
next iteration splits it again. A `3·rect.Dx()`-wide word
produces two splits and three resulting boxes; the loop
handles this naturally and is bounded by `b.Nrune` (each split
strictly shrinks the trailing piece by at least one rune).

**Iterator state under splice.** The index advanced by the
loop is `nb`. `splitbox` inserts at `nb+1`, so the leading
piece keeps the index `nb` we are currently writing geometry
for. `len(f.box)` grows by one per split; boxes beyond `nb+1`
shift right but are not yet visited. The `f.lines` table is
appended to per closed line, so splits before a line is closed
do not perturb already-emitted summaries.

**No pre-split in bxscan.** `bxscan` and the other mutators
produce boxes with whatever `Wid` the content has and hand the
list to `relayoutFrom`. They do not need `rect.Dx()` awareness.
This keeps the mutator path rect-agnostic and concentrates
all geometry-driven box-list mutation in one place. (See
§6.1.)

**Eager coalesce — inverse splitbox.** Before deciding wrap at
box `nb`, if `box[nb]` and `box[nb+1]` carry the same `Style`
and `box[nb].Wid + box[nb+1].Wid` fits at `pt.X`, splice them
into a single box at `nb` (concatenated `Ptr`, summed `Nrune`,
recomputed `Wid`) and re-evaluate. This is the symmetric
inverse of the long-word split above: a previous layout (or a
prior splitbox) may have left adjacent same-style fragments
that no longer need to be separate — deletion of mid-line
content, a rect-width grow, or a style change that uniformized
neighbors all produce this state. Folding the merge into the
phase-A scan means the post-layout box list has no layout-only
fragmentation, which keeps later passes faster and the line
table tighter. The merge is bounded: each iteration either
advances `nb` or shrinks `len(f.box)` by one, so the loop still
terminates.

### 3.4 Seeding for partial relayout

§3.1 normalizes `nb0` to a line start, so seeding only ever
runs at a line boundary.

**Position seed.**

- `nb0 == 0` → seed `pt = rect.Min`.
- `nb0 > 0` → read `box[nb0-1]`. After normalization, `nb0-1`
  is guaranteed to be the *last box of the previous line*
  (whether that line ended in a hard newline or a soft wrap),
  and that line was laid out either by a previous call or
  earlier in the same pass — so its fields are current. Seed
  `pt = (rect.Min.X, prev.Y + prev.LineH)`. The hard-newline
  vs soft-wrap distinction collapses to the same formula.

**Line-table truncation (entry).** Let `k` be the index in
`f.lines` such that `lines[k].FirstBox == nb0` (must exist
post-normalization, except in the no-op case below).
`relayoutFrom` truncates `f.lines = f.lines[:k]` before
entering the per-line loop, then appends one entry per closed
line.

**Line-count-shrinking case.** If the mutation reflows the
suffix into fewer visible lines than before, the truncation
above plus the append-as-we-go discipline is sufficient: stale
trailing entries from the previous layout were already dropped
at entry, and only what the new pass emits ends up in
`f.lines`. No separate "shrink" step is needed.

**`lastlinefull` under partial relayout.** Set per §2.3 from
the suffix's final `bottomY`. A partial pass still completes
the suffix and therefore still computes the correct
`bottomY`; the rule does not change.

**End-of-box edge (`nb0 == len(f.box)`).** No-op for layout
(nothing to lay out), but the pass still truncates
`f.lines = f.lines[:k]` if `k < len(f.lines)` — i.e., the
previous mutator removed trailing boxes and we are catching up.
`lastlinefull` is recomputed from the now-final last entry of
`f.lines`.

**Conservative first cut.** Until partial relayout is wired
through every mutator, callers pass `nb0 = 0` and rebuild
`f.lines` from scratch.

### 3.5 Paint deltas: blit hints via line-table diff

`relayoutFrom` writes geometry; it does **not** issue paint or
blit commands. The mutator path turns the new layout into a
minimum-cost sequence of blits and repaints by **diffing the
pre-relayout `f.lines` snapshot against the post-relayout
state**. The diff is the single mechanism for both (a) the
existing single-shift bulk blit used by `Insert` / `Delete`,
and (b) the wire-cheap optimization where unrelated trailing
lines are blit-translated instead of repainted.

**Snapshot shape.** Captured immediately before the mutator
calls `relayoutFrom`:

```go
type lineSnap struct {
    FirstRune int  // rune offset of lines[i].FirstBox at snap time
    TopY      int
    LineH     int
    Style     uint64  // optional digest of the line's box styles
}

// snapshotLines returns one entry per visible line at call time.
func (f *frameimpl) snapshotLines() []lineSnap
```

`FirstRune` is the canonical identity of a line (independent
of box-list index, which the mutation will perturb). `Style`
is an optional fast digest — implementations may start without
it and treat any line that changed `FirstRune` or `LineH` as
dirty.

**Three-way per-line classification.** For each *new* line
`new[i]`, locate the *old* line `old[j]` with the same
`FirstRune` (binary search by `FirstRune` in the snapshot,
which is sorted by construction):

- **Identical.** `old[j].TopY == new[i].TopY` **and**
  `old[j].LineH == new[i].LineH` **and** style-digest matches
  (or, conservatively, the line's box content is unchanged).
  → No paint, no blit.
- **Shifted.** Same content (FirstRune match + style/box
  unchanged) but `TopY` differs. → Blit; ΔY = `new[i].TopY -
  old[j].TopY`.
- **Dirty.** No matching `old[j]`, or content/height changed.
  → Full repaint of the line's rect.

Lines in `old` with no match in `new` (deleted lines) are
covered implicitly — their pixels are either overwritten by a
shifted line above or fall outside the new content extent.

**Run compression.** Adjacent **shifted** lines with the same
ΔY compose into one blit. This is the "push down on insert /
pull up on delete" wire-cheap path. The implementation is a
single linear walk over the diff:

```
for each contiguous run of shifted lines with equal ΔY:
    blit(srcRect = union of old TopY..TopY+LineH, dst = +ΔY)
for each dirty line:
    clear(new[i] rect); paint boxes of new[i]
```

Today's `(pt1.Y - pt0.Y)` bulk blit in `deleteimpl` /
`insertbyteimpl` is the degenerate single-run case of this.

**Helper signature.**

```go
type paintOp struct {
    Kind  paintOpKind   // opBlit | opPaint
    Src   image.Rectangle  // valid for opBlit
    Dst   image.Rectangle
}

// diffLines compares the snapshot to f.lines and returns the
// minimum-cost paint plan. Output is in screen order, so
// callers can issue ops top-to-bottom without further sorting.
func (f *frameimpl) diffLines(snap []lineSnap) []paintOp
```

**Where it's used.** §6.1 (Insert), §6.2 (Delete), §6.3
(SetStyleRange), and the §6.4 scroll/fill path all call
`snapshotLines` before `relayoutFrom` and `diffLines` after.
The pre-existing "snapshot `contentBottomY`, compare after"
in `SetStyleRange` collapses into this primitive.

**Conservative first cut.** A correctness-first implementation
may return `[]paintOp{opPaint(entireFrame)}` and still satisfy
all I-LAYOUT-* invariants. Optimization is layered in: first
the single-shift bulk blit (current behavior, restated through
the helper), then run-compressed multi-blit, then
identical-line skipping.

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

// LineRange(i) returns [firstBox, nextLineFirstBox) for line i.
// For the last line (i == len(f.lines)-1), the upper bound is
// len(f.box). Panics if i is out of range.
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
| `(*frameimpl).ptofcharptb` | DELETE | `Ptofchar` (§4.1) |
| `(*frameimpl).ptofcharnb` | DELETE | `Ptofchar` (§4.1) |
| `(*frameimpl).charofptimpl` | DELETE | `Charofpt` (§4.2) |
| `(*frameimpl).lineHForAdvance` | DELETE | `LineHAt` (§4.3) / box-field read |
| `(*frameimpl).lineHAtPt` | DELETE | line-table lookup (§4.4) |
| `_draw` pt-accumulator | DELETE accumulator only — `_draw` remains as the paint walk | post-relayout box-field read |
| `deleteimpl` `contentBottomY` snapshot | DELETE | `snapshotLines` + `diffLines` (§3.5) |

Call sites currently in `deleteimpl`'s inner loop and
`insertbyteimpl`'s pt-walk loop are restructured per §6.

Names of internal helpers used by `relayoutFrom` may be kept
(`updateLineMaxes`, etc.); the public-shaped functions in the
table above go away.

---

## 6. Mutator flow

### 6.1 `Insert` / `InsertByte` / `InsertWithStyle`

```
1. snap = f.snapshotLines()      // §3.5
2. Run bxscan on the new bytes to produce a slice of new boxes.
   - bxscan does NOT split for line-fit; over-long words are
     split inside relayoutFrom (§3.3).
   - bxscan does NOT set X/Y/LineH/LineA; those come from
     relayoutFrom.
3. Splice the new boxes into f.box at position p.
4. relayoutFrom(0)    // or relayoutFrom(nbStart) once partial works
5. ops = f.diffLines(snap)       // §3.5
6. Issue ops in order: blits first (top-to-bottom), then
   paints for dirty lines and the inserted range.
7. Update sp0/sp1 / Tick / nlines. lastlinefull is already
   set by relayoutFrom — read it.
```

Step 4 may be `relayoutFrom(0)` in the first cut; tightening to
a partial pass is a follow-up after correctness is established.
The diff in step 5 reduces to the single-shift bulk-blit case
(equivalent to `pt1.Y - pt0.Y` of today) for a plain insert into
a non-wrapped suffix; the same code path naturally widens to
multi-blit / skip-identical when the suffix reflows.

### 6.2 `Delete`

```
1. snap = f.snapshotLines()      // §3.5
2. Splice boxes [n0, n1) out of f.box (gaps closed; splitbox
   at the delete boundary for partial-box deletes — distinct
   from the long-word splitbox owned by relayoutFrom).
3. relayoutFrom(0)
4. ops = f.diffLines(snap)       // §3.5
5. Issue ops in order: blits first (top-to-bottom for upward
   shifts is fine — src/dst don't overlap when ΔY < 0), then
   clear+paint dirty lines.
6. Update sp0/sp1 / Tick / nlines. lastlinefull is set by
   relayoutFrom — read it.
```

Per-box blit-and-walk loop (current `deleteimpl` inner body)
goes away. The bulk upward blit of today's code is the
single-run `ΔY < 0` case of the §3.5 diff.

### 6.3 `SetStyleRange`

```
1. snap = f.snapshotLines()      // §3.5
2. Find boxes [nb0, nb1) covering [p0, p1).
3. Splitbox at the boundaries if needed (boundary split,
   not long-word).
4. Write Style on each box; recompute Wid via boxWid(b).
5. relayoutFrom(0)
6. ops = f.diffLines(snap)       // §3.5
7. Issue ops. In the common case where the style change does
   not reflow, every line is either identical (skip) or dirty
   only for [nb0, nb1) (paint the affected sub-rect). When
   the change does reflow, the diff naturally widens to cover
   the shifted suffix.
```

The earlier `contentBottomY` snapshot collapses into the
§3.5 helper: `diffLines` is the per-line-table diff.

### 6.4 Scroll / fill (`Text` side)

The bxscan-driven path used by `Text.fill` and `Text.scroll`
becomes:

```
1. snap = f.snapshotLines()      // parent frame, §3.5
2. nframe = new frameimpl on the staging rect.
3. nframe.bxscan(bytes) populates nframe.box.
4. nframe.relayoutFrom(0) — one pass, complete.
5. Read pt1 = lines[-1].TopY + lines[-1].LineH on nframe.
   No _draw walk, no second relayout.
6. Splice nframe.box into parent f.box.
7. f.relayoutFrom(spliceStart) — geometry recomputed for the
    parent in its own coords.
8. ops = f.diffLines(snap); issue per §6.1.
```

This is the architectural fix for the bxscan→_draw→bulk-blit
divergence described in `layout-once-invariant.md` §"Why mostly
readers...".

### 6.5 Frame rect resize

When the surrounding window resizes, the `Text` layer is the
owner of "how much content the frame needs"; the frame is told
its new rect and then refilled by `Text` via the §6.4 scroll/
fill path. The frame-local resize sequence is:

```
1. snap = f.snapshotLines()      // before rect changes
2. f.rect = newRect              // caller-supplied
3. f.relayoutFrom(0)             // wrap points may all shift
4. ops = f.diffLines(snap)
5. Issue ops. In practice most lines will be `dirty` because
   TopY (and possibly LineH) shifts everywhere; the diff still
   composes correctly, and identical-line skipping kicks in
   for the degenerate "same width, same content" case (e.g.,
   height-only resize with no wrap change).
6. After relayoutFrom, if lastlinefull is false and the new
   rect is taller, signal Text to refill the trailing gap.
   If lastlinefull is true and the new rect is shorter, no
   content fetch needed.
```

The frame does **not** decide how much to fetch — `Text`
inspects `lastlinefull` and the new rect height post-resize
and drives any additional `fill` calls through §6.4. Same
trust model as scroll/fill: the frame surfaces what it has,
the caller decides what to feed it next.

Width changes are the interesting case for the diff: every
wrap point can move, so a height-stable horizontal resize may
still emit O(visible-lines) `dirty` ops. That's fine — there's
no cheaper plan when content reflows.

---

## 7. Invariants

These add to `frame-layout-invariants.md`. Each is testable.

### I-LAYOUT-1: layout-once

The **layout fields** are `box[i].X`, `box[i].Y`, `box[i].LineH`,
`box[i].LineA`, and the entries of `f.lines`. Between any two
consecutive `relayoutFrom` calls, every non-`relayoutFrom`
function reads these fields without writing them. (The content
fields `Ptr / Nrune / Wid / Style / Bc / Minwid` have legitimate
writers in both `relayoutFrom` — via eager split/coalesce —
**and** the mutator path; they are out of scope for this
invariant.)

**Test:** instrument the layout fields with a write counter
inside `relayoutFrom` only; assert each mutator operation
increments the counter exactly once per affected box, and that
no non-`relayoutFrom` writer of layout fields exists (compile-time
check via unexported setters or a static-analysis grep).

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

### I-LAYOUT-6: no layout-only fragmentation

After any `relayoutFrom`, for every adjacent pair `box[i]`,
`box[i+1]` that satisfies *all* of:

- same `Style`,
- neither is a hard newline (`Bc != '\n'`),
- `box[i].Y == box[i+1].Y` (same line),
- `box[i].Wid + box[i+1].Wid <= rect.Dx() - (box[i].X - rect.Min.X)`,

the pair could be coalesced without changing the layout. The
invariant asserts the eager coalesce in §3.3 leaves none such
pairs in `f.box`.

**Test:** `validateboxmodel` scan under `-validateboxes`.
Useful both as a correctness gate for the coalesce
implementation and as a tightness gate on the line table (no
unnecessary entries in `f.lines` from layout-only-fragmented
content).

---

## 8. Migration order

Each row is one CODING-PROCESS pass; commits land independently.

1. **Per-line summary table + eager split/coalesce.** Build
   `f.lines` (with `FirstRune`) in `relayoutFrom`. Fold eager
   split (§3.3) and eager coalesce (§3.3) into the same pass.
   No external consumers yet. Tests assert I-LAYOUT-2 /
   I-LAYOUT-3 / I-LAYOUT-6.
2. **Move `lastlinefull` ownership into `relayoutFrom`.** Drop
   the explicit reset in `deleteimpl`; assert I-LAYOUT-4.
3. **Route `Charofpt` / position queries through the
   summary table.** Performance + correctness wash.
4. **Eliminate `_draw` as accumulator walker.** `bxscan`
   reads `pt1` from the staging frame's last box / line-table
   entry. Single biggest correctness gain — this is the
   suspected root of the scroll-overlap glitches.
5. **Introduce `snapshotLines` + `diffLines` helpers (§3.5).**
   No mutator wired to them yet; unit tests against constructed
   pre/post `f.lines` states cover identical / shifted / dirty
   classification and run compression. `deleteimpl` is the
   first consumer in row 6.
6. **Restructure `deleteimpl`.** Pre-mutation `snapshotLines`,
   single relayout, single `diffLines`, issue ops. Delete the
   inner per-box loop and the `contentBottomY` snapshot.
7. **Restructure `insertbyteimpl`.** Same pattern.
8. **Restructure `SetStyleRange`** through `diffLines`
   (collapses the old `contentBottomY` shift detection).
9. **Restructure the §6.4 scroll/fill path** to call
   `snapshotLines` on the parent frame before splicing
   nframe; issue `diffLines` ops after.
10. **Wire the §6.5 resize sequence.** Frame-side relayout +
    diff; Text-side refill driven by `lastlinefull`.
11. **Delete the legacy walkers** (`cklinewrap`, `cklinewrap0`,
    `advance`, `ptofcharptb`, `ptofcharnb`, `charofptimpl`,
    `lineHForAdvance`, `lineHAtPt`). Compile errors at the call
    sites are the migration checklist.
12. **Wire `-validatelayout` / `-validateboxes`** to assert the
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
  of the scenarios above; `FirstRune` monotonicity asserted.
- **Eager split + coalesce (I-LAYOUT-6).**
  - Split-then-coalesce round trip: load a long-word that
    triggers `splitbox`, grow `rect`, relayoutFrom; assert
    the boxes coalesce back into a single box equivalent to
    the input.
  - Coalesce-then-split round trip: load two adjacent same-
    style fragments, narrow `rect` so they must wrap, relayout;
    assert they remain separate (no over-eager merge across
    a wrap boundary).
  - No-op coalesce: same-style adjacent boxes separated by a
    hard newline are not merged.
- **`lastlinefull`.** Insert / Delete / SetStyleRange across
  the rect.Max.Y boundary asserts I-LAYOUT-4.
- **Mutators.** Pre/post pt0/pt1 readers + a snapshot of the
  box geometry — assert one-and-only-one `relayoutFrom`
  fires per operation (I-LAYOUT-1).
- **Paint parity.** I-LAYOUT-5 fixture for each mutator.
- **`diffLines` classification (§3.5).** Construct synthetic
  pre/post `f.lines` states covering each path:
  - identical-only → empty op list,
  - single-shift run → one blit (matches today's bulk-blit),
  - multi-shift with mixed ΔY → multiple blits, top-to-bottom,
    no src/dst rectangle overlap on either run,
  - dirty-tail after a reflow → blits stop at the dirty
    boundary, paints cover the rest,
  - all-dirty (width resize) → no blits, full repaint.
- **No-op mutation diff-is-empty.** Zero-byte Insert, empty-range
  Delete, identity-style SetStyleRange → `diffLines(snap)` is
  empty; sanity guard against spurious paint.
- **Resize.** Height-only grow with refill, height-only shrink,
  width grow with reflow, width shrink with reflow. Assert
  `lastlinefull` post-relayout matches the formula and the
  emitted ops are valid (I-LAYOUT-5 holds post-paint).

### 9.2 Manual

`docs/designs/features/test-md-layout.md` opened in edwood
with `md2spans`, then exercise §1–§13's scenarios with click,
scroll, select, and tick placement. Sign-off criterion: every
section behaves correctly across one full scroll up + one
full scroll down, with no visible overlap or stale glyphs.

### 9.3 Regression

`regression.sh` green at every commit on the migration order
above.
