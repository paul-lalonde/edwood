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
| [x] Commit | — | — | `Add Phase 3 round 8.x design and plan: table column-width alignment` |

## Phase 3.8.x.1: Pre-pass — measure per-column widths

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | measureTableColumns walks the Table-styled boxes from startIdx, identifies cell boundaries by `\|` boxes, measures per-cell widths via boxWidth + per-style font resolver, returns per-column max width + the endIdx of the table. | base doc § "Approach" / "Column boundary detection" | — |
| [x] Tests | TestMeasureTableColumns_BasicTwoColumn / RaggedSourceWidthsTakeMax / StopsAtNonTableBox. | `rich/layout_test.go` | — |
| [x] Iterate | measureTableColumns + growMax helper. | `rich/layout.go` | — |
| [x] Commit | — | — | `rich: measure per-column widths in a table pre-pass` |

## Phase 3.8.x.2: Pass 2 — apply alignment via xPos

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | layoutTable helper places each table row with cell-aware xPos. Per-row pre-walk computes content widths + alignment; place-walk advances xPos to column edges at `\|` boundaries and applies leading pad based on alignment. | base doc § "Alignment math" | — |
| [x] Tests | TestLayoutTable_VerticalBarsAlignAcrossRows (column-width property) + TestLayoutTable_RightAlignmentLeadingPad. | `rich/layout_test.go` | — |
| [x] Iterate | layoutTable function + main layout loop hand-off + appendSpanBoxes splits on `\|` for Table spans (separator rows like `\|---\|---\|` need each `\|` in its own box). | `rich/layout.go` | — |
| [x] Commit | — | — | `rich: pad table cells via xPos advances; honor TableAlign` |

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
