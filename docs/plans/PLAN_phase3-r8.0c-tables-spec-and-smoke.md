# Phase 3 Round 8.0c — Tables: Spec, README, and Smoke — Plan

Third and final sub-round for round 8 v1. After 8.0a
(consumer) and 8.0b (producer), 8.0c lands the
documentation updates and user-driven smoke testing
that confirms the v1 visual is acceptable.

This is the sub-round where the user typically uncovers
issues that need fixing. Round 7 had four smoke-fix
commits; round 6 had two; previous rounds had varying
counts. The plan accommodates iteration.

**Base design**: [`docs/designs/features/phase3-r8-tables.md`](../designs/features/phase3-r8-tables.md).

**Depends on**: 8.0a + 8.0b merged.

**Branch**: `phase3-r8.0c-tables-spec-and-smoke`.

**Files touched**:
- `docs/designs/spans-protocol.md` — three new kinds
  documented; alignment + header params; nested-table
  example.
- `cmd/md2spans/README.md` — Tables row flips to ✓
  (with v1 caveats).
- Whatever the smoke surfaces.

---

## Phase 3.8.0c.0: Plan + design

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Inherits from base design doc. | base doc | — |
| [x] Tests | n/a (planning) | — | — |
| [x] Iterate | This plan | — | — |
| [x] Commit | — | — | `Split round 8 plan into three sub-rounds (8.0a / 8.0b / 8.0c)` |

## Phase 3.8.0c.1: Spec + README

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | n/a (doc) | — | — |
| [x] Tests | n/a (doc) | — | — |
| [x] Iterate | spans-protocol.md adds the three new kinds + `header=true` and `align=L|R|C` param documentation + nested-table example. Roadmap entry round 8 ✓ landed. md2spans README Tables row updated. | — | — |
| [x] Commit | — | — | `docs: spans protocol gains table region kinds; md2spans handles GFM tables` |

## Phase 3.8.0c.2: Smoke + iteration

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | n/a (validation) | — | — |
| [x] Tests | All packages green | `go test ./...` | Green. |
| [x] Iterate | Smoke confirmed via test.md. Surfaced two real bugs (both fixed): table-in-blockquote misalignment (lines 2+ jumped to gutter indent while line 1 stayed at blockquote indent) and lack-of-monospace on `\|` markers between cells. Mixed alignment + empty-cell column-width misalignment confirmed as the v1 caveat — both need column-width-aware padding (round 8.x). | — | — |
| [x] Commit | — | — | `md2spans+wind: tables render in monospace; snap inside-blockquote table to line start` |

## Phase 3.8.0c.3: Smoke fixes (open-ended)

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Iterate | Two smoke fixes shipped: (a) Code=true added to applyTableRegion so the entire table block (including `\|` markers between cells) renders in the monospace font. (b) `table` added to kindsAnchorAtLineStart so a table inside a blockquote has all rows aligned at the gutter indent (without snap, line 1's `>` was outside the table region and rendered at blockquote-only indent while subsequent rows jumped to gutter — visible vertical jog). | — | — |
| [x] Commit | — | — | `md2spans+wind: tables render in monospace; snap inside-blockquote table to line start` |

---

## After this sub-round

Round 8 v1 (tables) is shipped. md2spans now covers all
in-tree markdown features EXCEPT column-width alignment.

**Round 8.x** (next): column-width-aware alignment. Smoke
confirmed two specific symptoms users care about:
  - Mixed-alignment column doesn't visibly align
    (left/right/center metadata in wire but no padding
    to align WITHIN).
  - Empty cells with fewer source spaces render
    narrower than non-empty cells in the same column.
Both fixes need cells padded to a common per-column
width. Approach (TBD): either a frame-dimension 9P
endpoint + producer-side width computation, or layout-
side two-pass measurement.

After 8.x, **Phase 4** (migration default flip + in-tree
deletion) unlocks.

## Status

All rows complete. Smoke confirmed via test.md; surfaced
two bugs that were fixed in the smoke iteration. Two
remaining issues (mixed alignment + empty-cell widths)
are the documented v1 caveats — both need column-width
padding which is round 8.x's deferred feature. Ready to
merge to master.
