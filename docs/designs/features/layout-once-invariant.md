# Layout-Once Invariant — Audit & Plan

**Goal:** establish that `relayoutFrom` is the **single** function
that computes `box.X` / `box.Y` / `box.LineH` / `box.LineA`, and that
every other consumer reads those fields without re-deriving them.
Eliminate the duplicate accumulators that diverge under variable
line heights.

This document is the diagnosis + plan; the implementation is a follow-up.

---

## The invariant

> After `relayoutFrom(0)` returns, **every visible box** has
> `box.X` / `box.Y` / `box.LineH` / `box.LineA` that exactly match
> where its glyphs are painted and where `Ptofchar(p)` /
> `Charofpt(pt)` agree the rune lives. No other function in the
> frame package may compute these values independently.

Corollary: **all walks are pure readers.** Specifically:

- `Ptofchar(p)`     → returns `(box[k].X + glyph-offset-in-box, box[k].Y)`.
- `Charofpt(pt)`    → finds `box[k]` whose rect contains `pt`.
- `drawtext / repaintBoxRange` → iterate `f.box`, paint at `(b.X, b.Y)`.
- `drawsel0` → iterate `f.box`, paint at `(b.X, b.Y)` with line metrics from `b.LineH/LineA`.
- `tick` → uses `box.LineH` for its rect height.

---

## Current code-path inventory

Functions that touch layout. ✅ = pure reader; ⚠ = walks/recomputes;
🔴 = duplicates `relayoutFrom`'s work.

| Function | Status | Notes |
|---|---|---|
| `relayoutFrom` | source-of-truth | Single forward pass. Updates `X/Y/LineH/LineA` on every box. |
| `paintBox` | ✅ reader | Reads `b.LineH/LineA` for bg-rect/baseline; takes caller's `pt`. |
| `repaintBoxRange` | ✅ reader | Calls `paintBox(b, image.Pt(b.X, b.Y), ...)`. |
| `drawtext` (on nframe) | ⚠ offset | Uses `(b.X, b.Y + offY)` with first-line-X offset. Assumes child relayout has run. |
| `drawsel0` | ✅ reader | After recent R7 fix, reads `b.X/Y/LineH/LineA`. |
| `ptOfCharReader` | ✅ reader | The R3-introduced direct reader. |
| `charOfPtReader` | ✅ reader | Same. |
| `LineHAt`, `lineHAtPt`, `lineHForAdvance` | ✅ readers | Scan `f.box` for line height. |
| `cklinewrap` | 🔴 recomputes | Advances `pt.Y` via `lineHForAdvance` (= a box lookup). Used in legacy walks. |
| `cklinewrap0` | 🔴 recomputes | Same. Used in `_draw`, `deleteimpl` inner loop. |
| `advance` | 🔴 recomputes | Newline branch advances by `b.LineH`. Same family. |
| `_draw` (on nframe) | 🔴 recomputes | Walks via `cklinewrap0 + manual newline advance`. After R7, calls `frame.relayoutFrom` first then duplicates that walk anyway. |
| `ptofcharptb` | 🔴 recomputes | Legacy walk via `cklinewrap + advance`. Same answer as `ptOfCharReader` post-relayout — but it's a separate code path. |
| `charofptimpl` | 🔴 recomputes | Same family. |
| `deleteimpl` inner loop | 🔴 mixes mutation + blit + walk | Walks `cklinewrap0`/`advance` while doing per-box blits and box-list mutation. |
| `insertbyteimpl` bulk blit (q0/q1 math) | 🔴 reads `lineHAtPt` for the shift size | The shift amount IS `pt1 - pt0` so the height lookup is redundant. |
| `insertbyteimpl` ripple loop | 🔴 per-box blit + walk via `pts[]` | Uses `pts` slice populated by an earlier `cklinewrap/advance` walk. |
| `clean` | ⚠ uses `cklinewrap` | Box-merge logic; advance pt to decide merge guards. |

The 🔴 entries are where variable-height bugs come from. Even though
each individual walk now uses `lineHForAdvance` / `b.LineH` (so they
*could* produce the same answer as `relayoutFrom`), they're computed
on *mid-mutation* box state, and they don't always agree with the
post-mutation relayout output.

---

## Why "mostly readers + a few walks" still has bugs

Concrete trace of one failure mode (the user's scroll-and-overlap):

1. **`Insert` at top of frame** (backward scroll path).
2. `bxscan` builds `nframe` boxes for the inserted content.
3. `nframe.relayoutFrom(0)` — boxes get `LineH = scaled` for headings.
4. `nframe._draw(pt0)` — walks accumulator from `pt0`. **This walk
   uses cklinewrap0 + manual advance**, computing `pt1` independently.
   If the walk diverges from `relayoutFrom`'s layout by even one
   pixel, `pt1` is wrong.
5. `pt1` is used by the parent's bulk blit to compute the shift
   amount (`pt1 - pt0`). Wrong `pt1` → wrong shift.
6. Existing content blits to the wrong Y → visible as overlap.

The architectural fix: **`_draw` should not exist as an accumulator
walk**. After `nframe.relayoutFrom`, `pt1` is just
`nframe.box[len-1].Y + nframe.box[len-1].LineH` (or `rect.Min.X,
last.Y + last.LineH` if the last box is a newline). One pure read.
No walk. No possibility of divergence.

The same logic applies to the other 🔴 entries: each one duplicates
calculation that `relayoutFrom` already did. Each duplication is a
chance for divergence — especially on the mutation paths where
box-state changes are interleaved with blit operations.

---

## Proposed clean architecture

### 1. Eliminate `_draw` as a separate walker

`bxscan` should:
1. Build `nframe` boxes.
2. Call `nframe.relayoutFrom(0)`.
3. Compute `pt1` from `nframe.box[len-1]` directly (one read).
4. Apply `nframe.lastlinefull` from a single check
   (`box[len-1].Y + LineH >= rect.Max.Y`).
5. Truncate `nframe.box` if any box's `Y >= rect.Max.Y`
   (since `relayoutFrom` no longer truncates).

No `_draw` invocation. No accumulator walk.

### 2. Eliminate `ptofcharptb` / `charofptimpl`

Every external call site already routes through `Ptofchar` /
`Charofpt` which now use the reader. Internal call sites
(`deleteimpl` start, `drawselimpl` internals, etc.) can use the
reader once the mutation path is restructured so that box state is
consistent at the read time.

### 3. Restructure mutation paths

Each of `Insert` / `Delete` / `SetStyleRange` becomes:

```
1. Compute pre-mutation positions (pt0, pt1) via reader.
2. Compute the shift in pixels: shift = (pt1 - pt0).
3. Mutate the box list (split/merge/delete/insert boxes).
4. relayoutFrom(0)  ← single layout pass.
5. Blit pixels using the pre-mutation shift.
6. Paint new / changed boxes from the post-relayout box state.
```

The blit (step 5) is a **single Draw call** in pixel space; it
doesn't care about line structure. The paint (step 6) iterates
boxes and uses each one's `b.X, b.Y, b.LineH, b.LineA` directly.

For variable-height frames where the in-place shift can't restore
correctness (e.g. lines collapse or grow), step 5 is replaced by
"clear the affected region" and step 6 paints everything in that
region. Plain-text frames keep the parsimonious blit.

### 4. Eliminate the inner per-box blit loops in `deleteimpl` /
   `insertbyteimpl`

The per-box loops mix blit + mutation + walk in a way that's
intrinsically hard to keep right under variable heights. After the
restructure above, they become unnecessary — the single pre-relayout
shift + post-relayout paint subsumes them.

### 5. Keep `cklinewrap` / `cklinewrap0` / `advance`
   for `relayoutFrom`'s internal use only

These are the per-step primitives `relayoutFrom` uses to decide line
boundaries. They're fine *inside* `relayoutFrom`. They should not
be called from anywhere else.

### 6. `lastlinefull` is set by `relayoutFrom`, not by walks

After `relayoutFrom`, `f.lastlinefull = (last_box.Y + last_box.LineH
>= rect.Max.Y)`. Single rule. The current scatter (`_draw` sets it,
`deleteimpl` recomputes from `nlines`, `Init` resets it) is replaced
by one assignment in `relayoutFrom`.

---

## Plan of attack

In priority order:

1. **Add test markdown** (`docs/designs/features/test-md-layout.md`)
   — done in this commit. We can scroll through it manually to
   reproduce the variable-height bugs deterministically.

2. **Move `lastlinefull` ownership into `relayoutFrom`**. Small,
   contained. Removes the "stale lastlinefull" class of bug
   (already fixing one such bug in deleteimpl).

3. **Eliminate `_draw` as a walker in `bxscan`**. After
   `nframe.relayoutFrom`, read `pt1` from the last box directly.
   Removes the bxscan→_draw→bulk-blit divergence — the suspected
   root of the user's overlap glitches.

4. **Restructure `deleteimpl`**:
   - Save `pt0` / `pt1` via reader pre-mutation.
   - Compute shift.
   - Mutate box list (no per-box blits).
   - `relayoutFrom`.
   - Single bulk blit for the shift.
   - Clear+repaint affected region for variable-height case.

5. **Restructure `insertbyteimpl`** with the same pattern.

6. **Migrate remaining `ptofcharptb` / `charofptimpl` callers** to
   readers; mark the legacy functions deprecated.

7. **Optimize:** add the constant-height fast path where appropriate
   (single-blit shift instead of clear+repaint). Only after the
   correct-by-default path is shown to work on the test markdown.

---

## Test plan

Manual: open `docs/designs/features/test-md-layout.md` in edwood,
toggle `md2spans`, then exercise each section's scenario by:
- Click-scroll up and down one line at a time.
- Right-click scroll to jump to fractions.
- Click-select across line boundaries.
- Tick (caret) placement on each heading vs body.

Automated: extend `frame/` unit tests so each restructured function
has a test that asserts:
- Layout is computed exactly once (a counter incremented in
  `relayoutFrom`).
- Post-mutation `box.X / box.Y / box.LineH / box.LineA` match what
  paint actually emitted (the visible state matches the layout).
