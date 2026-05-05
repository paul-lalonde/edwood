# Phase 3 Round 8.x — Table Column-Width Alignment — Plan

Layout-side two-pass measurement to align table columns.
Cells are padded via xPos advances, NOT by modifying
text — preserves the rendered/body invariant md2spans
relies on for cursor positioning.

**Base design**: [`docs/designs/features/phase3-r8.x-table-column-widths.md`](../designs/features/phase3-r8.x-table-column-widths.md).

**Branch**: `phase3-r8.x-table-column-widths`.

**Files touched**:
- `rich/layout.go` — table pre-pass + alignment xPos
  logic.
- `rich/layout_test.go` — column-width + alignment tests.

No `cmd/md2spans/` or `wind.go` changes — round 8.0's
producer + bridge are already correct.

---

## Phase 3.8.x.0: Plan + design

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | phase3-r8.x design drafted | [base doc] | Decisions: (a) layout-side two-pass; (b) NO text padding (rendered text === body text invariant); (c) xPos advances for alignment; (d) wire format unchanged from r8.0. |
| [x] Tests | n/a (planning) | — | — |
| [x] Iterate | This plan | — | — |
| [ ] Commit | — | — | `Add Phase 3 round 8.x design and plan: table column-width alignment` |

## Phase 3.8.x.1: Pre-pass — measure per-column widths

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | When the layout enters a sequence of `Style.Table=true` boxes, do a sub-pass: walk all boxes in the table region, identify cell boundaries (`|` runes), measure each cell's content pixel-width via the appropriate font (`getFontForStyle`), record per-column max width. Returns a `[]int` of column widths used by pass 2. | base doc § "Approach" / "Column boundary detection" | First case where layout has a sub-pass. |
| [ ] Tests | TestLayoutTable_MeasurePerColumnWidths: synthetic table with rows of different cell widths → measured widths = max per column. | `rich/layout_test.go` | Test internals via a helper export OR via observable layout output. |
| [ ] Iterate | Extract `measureTableColumns(boxes []Box, startIdx int, font fontResolver) ([]int, int)` returning column widths + the index where the table ends. | `rich/layout.go` | — |
| [ ] Commit | — | — | `rich: measure per-column widths in a table pre-pass` |

## Phase 3.8.x.2: Pass 2 — apply alignment via xPos

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | When `layout` reaches a table boundary, call `measureTableColumns`, then iterate the table's boxes again with cell-aware xPos placement. At each cell start: xPos = lineStart + (Σ prior column widths) + leading_pad (per align). After the cell's content: xPos = lineStart + (Σ prior column widths) + currentColWidth + trailing_pad. | base doc § "Alignment math" | — |
| [ ] Tests | TestLayoutTable_AlignmentLeft / _Right / _Center: each verifies the cell content's X position relative to the column's start. TestLayoutTable_EmptyCellsPadToColumnWidth. TestLayoutTable_HeaderBoldUsedForMeasurement (subtle: bold widths can differ). TestLayoutTable_InsideBlockquote (composition with blockquote indent). | `rich/layout_test.go` | — |
| [ ] Iterate | Inline the table-aware layout block in `layout`. The first-box-of-cell logic determines leading pad; the last-box-of-cell determines trailing pad. | `rich/layout.go` | — |
| [ ] Commit | — | — | `rich: pad table cells via xPos advances; honor TableAlign` |

## Phase 3.8.x.3: Smoke + merge prep

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | n/a (validation) | — | — |
| [ ] Tests | All packages green | `go test ./...` | — |
| [ ] Iterate | Build binaries; smoke-test test.md (which already covers the v1 caveats from round 8.0c). Check: mixed alignment now visibly aligns; empty cells render at column width (not narrower); table inside blockquote still aligns; no regressions on other tables. | — | User-driven. |
| [ ] Commit | — | — | n/a unless smoke surfaces a fix. |

---

## After this round

Round 8.x ships the column-alignment feature. md2spans
is feature-complete vs. the in-tree path — Phase 4 (flip
preview default + delete in-tree `markdown/`) unlocks.

## Risks

(See base design doc.) Layout's first multi-pass; box-
text granularity (multiple `|` per box); font metric
variance for bold headers; cursor positioning over
padding gaps.

## Status

Plan + design drafted. Awaiting review before any code.
