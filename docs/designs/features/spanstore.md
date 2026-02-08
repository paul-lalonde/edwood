# SpanStore Design

The SpanStore is a gap-buffered sequence of styled text runs. It tracks which
style attributes apply to which rune ranges in a window's body buffer. The
store has no dependencies on the window, 9P filesystem, or rendering engine.

**Package**: `main` (in `spanstore.go`)

---

## Struct Definitions

```go
// StyleAttrs holds concrete styling for a span of text.
// Zero value means "default" (no explicit styling).
type StyleAttrs struct {
    Fg     color.Color // nil = default foreground
    Bg     color.Color // nil = default background
    Bold   bool
    Italic bool
    Hidden bool        // reserved for future use
}

// StyleRun is a contiguous range of runes sharing the same style.
type StyleRun struct {
    Len   int        // number of runes (must be >= 0)
    Style StyleAttrs
}

// SpanStore manages styled runs for a window's body text using a gap buffer.
type SpanStore struct {
    runs     []StyleRun // storage array with gap
    gap0     int        // start of gap (first unused index)
    gap1     int        // end of gap (first used index after gap)
    totalLen int        // cached sum of all run lengths
}
```

**Invariants**:
- `0 <= gap0 <= gap1 <= len(runs)`
- `totalLen == sum(runs[0:gap0].Len) + sum(runs[gap1:].Len)`
- No adjacent runs have equal `StyleAttrs` (runs are always merged)
- No run has `Len == 0` (zero-length runs are removed), except as transient
  state during an operation

---

## Public Method Signatures

```go
// NewSpanStore creates an empty SpanStore (TotalLen = 0, no runs).
func NewSpanStore() *SpanStore

// Insert adjusts runs when text is inserted into the body buffer at rune
// position pos. The inserted runes inherit the style of the run they fall
// within. If pos is at a run boundary, the preceding run is extended.
// If the store is empty, a default-style run is created.
func (s *SpanStore) Insert(pos, length int)

// Delete adjusts runs when runes in [pos, pos+length) are deleted from
// the body buffer. Runs fully inside the range are removed. Runs partially
// overlapping are shrunk. Adjacent runs with identical styles are merged
// after deletion.
func (s *SpanStore) Delete(pos, length int)

// RegionUpdate replaces style information in the rune range
// [offset, offset+sum(newRuns.Len)) with newRuns. Existing runs within
// the range are removed; runs partially overlapping at the edges are split.
// Adjacent runs with identical styles at the boundaries are merged.
// The total rune length of the store does not change (the new runs must
// cover the same number of runes as the old range).
func (s *SpanStore) RegionUpdate(offset int, newRuns []StyleRun)

// ForEachRun calls fn for each run in order, from the start of the buffer
// to the end. The function must not modify the store.
func (s *SpanStore) ForEachRun(fn func(StyleRun))

// Runs returns all runs as a new slice. Convenience wrapper over ForEachRun.
func (s *SpanStore) Runs() []StyleRun

// TotalLen returns the total number of runes covered by all runs.
// This should equal the body buffer length when the store is in use.
func (s *SpanStore) TotalLen() int

// NumRuns returns the number of active runs (excluding the gap).
func (s *SpanStore) NumRuns() int

// Clear removes all runs and resets TotalLen to 0.
func (s *SpanStore) Clear()
```

---

## Gap Buffer Mechanics

### Layout

The `runs` slice contains active elements and a gap:

```
 [run0] [run1] ... [run(gap0-1)] [  gap  ] [run(gap1)] ... [run(N-1)]
  \___________ before gap ___________/          \_______ after gap ______/
               gap0 elements                     len(runs)-gap1 elements
```

Logical run count: `gap0 + (len(runs) - gap1)`

Logical index `i` maps to physical index:
- `i` if `i < gap0`
- `i + (gap1 - gap0)` if `i >= gap0`

### moveGapTo(logicalIdx int)

Repositions the gap so that `gap0 == logicalIdx`:

- If `logicalIdx < gap0`: copy `runs[logicalIdx:gap0]` rightward to end at
  `gap1`, then adjust `gap1 -= (gap0 - logicalIdx)`, `gap0 = logicalIdx`.
- If `logicalIdx > gap0`: copy `runs[gap1 : gap1+(logicalIdx-gap0)]` leftward
  to start at `gap0`, then adjust both by `+(logicalIdx - gap0)`.
- If `logicalIdx == gap0`: no-op.

Cost: O(k) where k = |logicalIdx - gap0| (number of runs moved).

### growGap(needed int)

If the gap has fewer than `needed` slots, allocate a new slice with additional
capacity, copy the before-gap and after-gap portions to leave a larger gap in
the middle.

Growth strategy: double the slice length or add `needed`, whichever is larger.
Minimum initial capacity: 32 runs.

### findRunAt(pos int) (logicalIdx, offsetInRun int)

Scans runs in logical order, accumulating lengths, to find which run contains
rune position `pos`. Returns the logical index and the offset within that run
(0 means `pos` is at the run's start boundary).

When `pos == TotalLen`, returns `(NumRuns(), 0)` — one past the last run,
which callers use for end-of-buffer operations.

When `pos` falls exactly at a run boundary (offsetInRun == 0, logicalIdx > 0),
the caller decides whether to use the preceding or current run depending on
the operation:
- **Insert**: use the preceding run (extend it)
- **RegionUpdate**: use the current run (the boundary is the split point)
- **Delete**: use the current run (start of deletion range)

Cost: O(n) where n = number of runs. This is acceptable because the subsequent
gap move will also be O(n) in the worst case, and sequential edits benefit
from the gap already being nearby.

---

## Operation Details

### Insert(pos, length int)

```
1. If store is empty (NumRuns == 0):
   - Insert a single run: StyleRun{Len: length, Style: StyleAttrs{}}
   - totalLen = length
   - return

2. If pos == 0:
   - Extend run at logical index 0: runs[0].Len += length
   - totalLen += length
   - return

3. Find (runIdx, offsetInRun) = findRunAt(pos)

4. If pos == TotalLen (append):
   - Extend the last run by length
   - totalLen += length
   - return

5. If offsetInRun == 0 (at run boundary):
   - Extend the preceding run: runs[runIdx-1].Len += length
   - totalLen += length
   - return

6. Otherwise (mid-run):
   - Extend runs[runIdx].Len += length
   - totalLen += length
   - return
```

No splitting is needed for Insert — the new text simply extends the containing
(or preceding) run. No merging is needed because no styles change.

### Delete(pos, length int)

```
1. Clamp: if pos + length > totalLen, length = totalLen - pos
2. If length == 0, return
3. Find (startIdx, startOff) = findRunAt(pos)
4. Find (endIdx, endOff) = findRunAt(pos + length)
5. Move gap to startIdx

6. Process the overlapping runs:

   Case A: startIdx == endIdx (deletion within a single run)
     - runs[startIdx].Len -= length
     - If runs[startIdx].Len == 0, remove it

   Case B: deletion spans multiple runs
     a. Shrink the first run: runs[startIdx].Len = startOff
        If startOff == 0, mark for removal
     b. Remove all fully-contained runs (startIdx+1 .. endIdx-1) by
        absorbing them into the gap
     c. Shrink the last run: runs[endIdx].Len -= endOff
        If endOff == runs[endIdx].Len (fully consumed), mark for removal
     d. Remove zero-length runs
     e. Merge adjacent runs with identical styles at the deletion boundary

7. totalLen -= length
```

**Merge after delete**: After removing middle runs, the run before the
deletion and the run after may now be adjacent and have the same style.
Check and merge if so.

### RegionUpdate(offset int, newRuns []StyleRun)

```
1. Compute newTotalLen = sum(newRuns[i].Len)
2. Define region [offset, offset + newTotalLen)

3. Find (startIdx, startOff) = findRunAt(offset)
4. Find (endIdx, endOff) = findRunAt(offset + newTotalLen)

5. Split at start boundary if needed:
   If startOff > 0:
     - Split runs[startIdx] into:
       runs[startIdx] = {Len: startOff, Style: same}  (keeps before region)
       insert {Len: runs[startIdx].Len - startOff, Style: same} after
     - startIdx++ (now points to the first run fully inside the region)

6. Split at end boundary if needed:
   If endOff > 0 and endIdx < NumRuns:
     - Split runs[endIdx] into:
       {Len: endOff, Style: same}  (inside region, will be replaced)
       {Len: runs[endIdx].Len - endOff, Style: same}  (after region, kept)
     - endIdx stays (the first half is inside the region)
     - endIdx++ to include it in the replacement range

   If endOff == 0 (region ends at run boundary):
     - No split needed; endIdx is already the first run after the region

7. Move gap to startIdx
8. Absorb runs [startIdx, endIdx) into the gap (remove old runs)
9. Grow gap if needed for newRuns
10. Insert newRuns into the gap at startIdx

11. Merge at boundaries:
    - If the first new run has the same style as the preceding run, merge
    - If the last new run has the same style as the following run, merge

12. totalLen remains unchanged (old region length == new region length)
```

**Empty newRuns**: If newRuns is empty and the region length is 0, this is a
no-op. If newRuns is empty but region length > 0, this would violate the
invariant that region length must be preserved; the caller must not do this.

### ForEachRun(fn func(StyleRun))

```
for i := 0; i < gap0; i++ {
    fn(runs[i])
}
for i := gap1; i < len(runs); i++ {
    fn(runs[i])
}
```

Cost: O(n) where n = number of runs.

### TotalLen() int

Returns `s.totalLen`. Cost: O(1).

### NumRuns() int

Returns `s.gap0 + (len(s.runs) - s.gap1)`. Cost: O(1).

### Clear()

Sets `gap0 = 0`, `gap1 = len(runs)`, `totalLen = 0`. The underlying array is
retained for reuse. Cost: O(1).

---

## Edge Cases for Split and Merge

### Split Cases

Splits occur during RegionUpdate when the region boundary falls mid-run:

| Case | Before | RegionUpdate | After |
|------|--------|--------------|-------|
| Split at start | `[{20,A}]` | offset=5, newRuns=[{10,B}] | `[{5,A},{10,B},{5,A}]` |
| Split at end | `[{20,A}]` | offset=0, newRuns=[{15,B}] | `[{15,B},{5,A}]` |
| Split at both | `[{20,A}]` | offset=5, newRuns=[{10,B}] | `[{5,A},{10,B},{5,A}]` |
| No split (aligned) | `[{10,A},{10,B}]` | offset=10, newRuns=[{10,C}] | `[{10,A},{10,C}]` |

### Merge Cases

Merges occur when an operation creates adjacent runs with identical styles:

| Case | Operation | Before | After (unmerged) | After (merged) |
|------|-----------|--------|------------------|----------------|
| Delete middle run | Delete runes of B | `[{5,A},{3,B},{7,A}]` | `[{5,A},{7,A}]` | `[{12,A}]` |
| RegionUpdate same style | Update [5,8) with A | `[{5,A},{3,B},{7,A}]` | `[{5,A},{3,A},{7,A}]` | `[{15,A}]` |
| RegionUpdate boundary | Update [0,5) with A | `[{5,B},{5,A}]` | `[{5,A},{5,A}]` | `[{10,A}]` |
| Insert doesn't merge | Insert in A | `[{5,A},{5,B}]` | `[{8,A},{5,B}]` | (no merge needed) |

### Delete Edge Cases

| Case | Store | Delete | Result |
|------|-------|--------|--------|
| Delete within one run | `[{10,A}]` | Delete(3, 4) | `[{6,A}]` |
| Delete entire single run | `[{10,A}]` | Delete(0, 10) | `[]` (empty) |
| Delete from start | `[{5,A},{5,B}]` | Delete(0, 3) | `[{2,A},{5,B}]` |
| Delete from end | `[{5,A},{5,B}]` | Delete(7, 3) | `[{5,A},{2,B}]` |
| Delete exactly one run | `[{5,A},{5,B},{5,C}]` | Delete(5, 5) | `[{5,A},{5,C}]` |
| Delete spans two runs | `[{5,A},{5,B}]` | Delete(3, 4) | `[{3,A},{3,B}]` |
| Delete spans two, merge | `[{5,A},{5,B},{5,A}]` | Delete(5, 5) | `[{10,A}]` |
| Delete entire store | `[{5,A},{5,B}]` | Delete(0, 10) | `[]` |
| Delete to exact end | `[{5,A},{5,B}]` | Delete(5, 5) | `[{5,A}]` |

### Insert Edge Cases

| Case | Store | Insert | Result |
|------|-------|--------|--------|
| Insert into empty store | `[]` | Insert(0, 5) | `[{5,default}]` |
| Insert at start | `[{5,A}]` | Insert(0, 3) | `[{8,A}]` |
| Insert at end | `[{5,A}]` | Insert(5, 3) | `[{8,A}]` |
| Insert mid-run | `[{10,A}]` | Insert(5, 3) | `[{13,A}]` |
| Insert at boundary (extends preceding) | `[{5,A},{5,B}]` | Insert(5, 3) | `[{8,A},{5,B}]` |
| Insert at start of first run | `[{5,A},{5,B}]` | Insert(0, 3) | `[{8,A},{5,B}]` |

### RegionUpdate Edge Cases

| Case | Store | RegionUpdate | Result |
|------|-------|-------------|--------|
| Replace entire store | `[{10,A}]` | (0, [{10,B}]) | `[{10,B}]` |
| Replace at start | `[{10,A}]` | (0, [{5,B}]) | `[{5,B},{5,A}]` |
| Replace at end | `[{10,A}]` | (5, [{5,B}]) | `[{5,A},{5,B}]` |
| Replace middle | `[{10,A}]` | (3, [{4,B}]) | `[{3,A},{4,B},{3,A}]` |
| Replace spanning runs | `[{5,A},{5,B}]` | (3, [{4,C}]) | `[{3,A},{4,C},{3,B}]` |
| Replace with merge left | `[{5,A},{5,B}]` | (5, [{5,A}]) | `[{10,A}]` |
| Replace with merge right | `[{5,A},{5,B}]` | (0, [{5,B}]) | `[{10,B}]` |
| Replace with multiple runs | `[{10,A}]` | (0, [{3,B},{4,C},{3,D}]) | `[{3,B},{4,C},{3,D}]` |
| Aligned with run boundaries | `[{5,A},{5,B},{5,C}]` | (5, [{5,D}]) | `[{5,A},{5,D},{5,C}]` |

---

## Cost Analysis

| Operation | Cost | Notes |
|-----------|------|-------|
| Insert at/near gap | O(1) amortized | Gap already nearby for sequential typing |
| Insert at distant position | O(k) | k = runs between old and new gap position |
| Delete at/near gap | O(1) amortized | Plus O(1) merge check |
| Delete spanning m runs | O(k + m) | k for gap move, m for run removal |
| RegionUpdate with m new runs | O(k + m) | k for gap move, m for insertion |
| ForEachRun | O(n) | n = total number of runs |
| TotalLen | O(1) | Cached field |
| NumRuns | O(1) | Computed from gap indices |
| Clear | O(1) | Resets indices, retains backing array |
| findRunAt | O(n) | Linear scan of run lengths |

For typical use (sequential typing near cursor), Insert and Delete are O(1)
amortized. The gap stays near the editing position, and findRunAt starts from
the gap's neighborhood in practice.

---

## StyleAttrs Equality

Two `StyleAttrs` values are equal when all fields match. For `Fg` and `Bg`
(which are `color.Color` interface values), equality is checked with a helper
that handles nil:

```go
func colorEqual(a, b color.Color) bool {
    if a == nil && b == nil {
        return true
    }
    if a == nil || b == nil {
        return false
    }
    r1, g1, b1, a1 := a.RGBA()
    r2, g2, b2, a2 := b.RGBA()
    return r1 == r2 && g1 == g2 && b1 == b2 && a1 == a2
}

func (a StyleAttrs) Equal(b StyleAttrs) bool {
    return colorEqual(a.Fg, b.Fg) &&
        colorEqual(a.Bg, b.Bg) &&
        a.Bold == b.Bold &&
        a.Italic == b.Italic &&
        a.Hidden == b.Hidden
}
```

This is used by merge logic to detect adjacent runs with identical styles.

---

## Test Case Matrix

Tests are organized by operation and cover the edge cases listed above.

### Empty Store

| # | Test | Expected |
|---|------|----------|
| 1 | `NewSpanStore()` has TotalLen 0 | TotalLen == 0, NumRuns == 0 |
| 2 | ForEachRun on empty store | fn never called |
| 3 | Runs on empty store | returns nil/empty slice |
| 4 | Clear on empty store | no panic, still empty |

### Insert

| # | Test | Setup | Operation | Expected |
|---|------|-------|-----------|----------|
| 5 | Insert into empty | empty | Insert(0, 5) | [{5,default}], TotalLen=5 |
| 6 | Insert at start | [{5,A}] | Insert(0, 3) | [{8,A}], TotalLen=8 |
| 7 | Insert at end | [{5,A}] | Insert(5, 3) | [{8,A}], TotalLen=8 |
| 8 | Insert mid-run | [{10,A}] | Insert(5, 3) | [{13,A}], TotalLen=13 |
| 9 | Insert at run boundary | [{5,A},{5,B}] | Insert(5, 3) | [{8,A},{5,B}], TotalLen=13 |
| 10 | Insert at start, multi-run | [{5,A},{5,B}] | Insert(0, 3) | [{8,A},{5,B}], TotalLen=13 |
| 11 | Insert in second run | [{5,A},{5,B}] | Insert(7, 2) | [{5,A},{7,B}], TotalLen=12 |

### Delete

| # | Test | Setup | Operation | Expected |
|---|------|-------|-----------|----------|
| 12 | Delete within one run | [{10,A}] | Delete(3, 4) | [{6,A}], TotalLen=6 |
| 13 | Delete entire single run | [{10,A}] | Delete(0, 10) | [], TotalLen=0 |
| 14 | Delete from start | [{5,A},{5,B}] | Delete(0, 3) | [{2,A},{5,B}], TotalLen=7 |
| 15 | Delete from end | [{5,A},{5,B}] | Delete(7, 3) | [{5,A},{2,B}], TotalLen=7 |
| 16 | Delete exact run | [{5,A},{5,B},{5,C}] | Delete(5, 5) | [{5,A},{5,C}], TotalLen=10 |
| 17 | Delete spanning two runs | [{5,A},{5,B}] | Delete(3, 4) | [{3,A},{3,B}], TotalLen=6 |
| 18 | Delete middle, merge | [{5,A},{5,B},{5,A}] | Delete(5, 5) | [{10,A}], TotalLen=10 |
| 19 | Delete entire store | [{5,A},{5,B}] | Delete(0, 10) | [], TotalLen=0 |
| 20 | Delete shrinks run to zero | [{3,A},{2,B},{5,C}] | Delete(3, 2) | [{3,A},{5,C}], TotalLen=8 |
| 21 | Delete spanning multiple, partial edges | [{5,A},{5,B},{5,C},{5,D}] | Delete(3, 14) | [{3,A},{3,D}], TotalLen=6 |

### RegionUpdate

| # | Test | Setup | Operation | Expected |
|---|------|-------|-----------|----------|
| 22 | Replace entire store | [{10,A}] | RU(0, [{10,B}]) | [{10,B}], TotalLen=10 |
| 23 | Replace at start | [{10,A}] | RU(0, [{5,B}]) | [{5,B},{5,A}], TotalLen=10 |
| 24 | Replace at end | [{10,A}] | RU(5, [{5,B}]) | [{5,A},{5,B}], TotalLen=10 |
| 25 | Replace middle | [{10,A}] | RU(3, [{4,B}]) | [{3,A},{4,B},{3,A}], TotalLen=10 |
| 26 | Replace spanning runs | [{5,A},{5,B}] | RU(3, [{4,C}]) | [{3,A},{4,C},{3,B}], TotalLen=10 |
| 27 | Replace with merge left | [{5,A},{5,B}] | RU(5, [{5,A}]) | [{10,A}], TotalLen=10 |
| 28 | Replace with merge right | [{5,A},{5,B}] | RU(0, [{5,B}]) | [{10,B}], TotalLen=10 |
| 29 | Replace with multi runs | [{10,A}] | RU(0, [{3,B},{4,C},{3,D}]) | [{3,B},{4,C},{3,D}], TotalLen=10 |
| 30 | Aligned boundaries | [{5,A},{5,B},{5,C}] | RU(5, [{5,D}]) | [{5,A},{5,D},{5,C}], TotalLen=15 |
| 31 | Replace at start, merge right | [{5,A},{5,B}] | RU(0, [{5,B}]) | [{10,B}], TotalLen=10 |

### ForEachRun

| # | Test | Setup | Expected |
|---|------|-------|----------|
| 32 | Single run | [{10,A}] | fn called once with {10,A} |
| 33 | Multiple runs | [{5,A},{3,B},{7,C}] | fn called 3 times in order |
| 34 | After gap move | build then Insert at distant pos | iteration still correct |

### Clear

| # | Test | Setup | Expected |
|---|------|-------|----------|
| 35 | Clear non-empty | [{5,A},{5,B}] | TotalLen=0, NumRuns=0 |
| 36 | Clear then insert | [{5,A}], Clear, Insert(0,3) | [{3,default}], TotalLen=3 |

### Zero-Length Spans

| # | Test | Setup | Expected |
|---|------|-------|----------|
| 37 | Zero-length in RegionUpdate | [{10,A}] | RU(5, [{0,B},{10-5,A}]) should drop zero-length or keep as marker (per policy: drop for now, zero-length runs violate invariant) |

### TotalLen Consistency

| # | Test | Description | Expected |
|---|------|-------------|----------|
| 38 | After Insert sequence | Insert 5, Insert 3, Insert 2 | TotalLen == 10 |
| 39 | After Delete sequence | [{10,A}], Delete(0,3), Delete(0,2) | TotalLen == 5 |
| 40 | After RegionUpdate | [{10,A}], RU(0,[{5,B},{5,C}]) | TotalLen == 10 |
| 41 | After mixed operations | Insert, RegionUpdate, Delete, Insert | TotalLen consistent with operations |

### StyleAttrs Equality

| # | Test | Expected |
|---|------|----------|
| 42 | Both nil colors | Equal |
| 43 | One nil, one non-nil | Not equal |
| 44 | Same RGBA values | Equal |
| 45 | Different RGBA values | Not equal |
| 46 | Same bool flags | Equal |
| 47 | Different bool flags | Not equal |
