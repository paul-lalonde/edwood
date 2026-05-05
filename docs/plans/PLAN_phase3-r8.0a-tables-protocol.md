# Phase 3 Round 8.0a — Tables: Protocol Vocabulary + Bridge — Plan

First of three sub-rounds for round 8 (tables). Sub-round
8.0a lands the CONSUMER side: the protocol parser accepts
the three new region kinds, and the bridge has apply
functions that map them to `rich.Style` fields. No
producer yet — md2spans doesn't emit tables until 8.0b.

After this sub-round merges, edwood can RECEIVE table
directives from any future producer; rendering of those
directives uses the existing `rich.Style.Table` /
`TableHeader` / `TableAlign` machinery.

**Base design**: [`docs/designs/features/phase3-r8-tables.md`](../designs/features/phase3-r8-tables.md).

**Branch**: `phase3-r8.0a-tables-protocol`.

**Files touched**:
- `spanparse.go`, `spanparse_test.go` — extend kind set.
- `wind.go` — three new apply functions + dispatch.
- `wind_styled_test.go` — bridge tests.

No `cmd/md2spans/` changes; no `rich/` changes.

---

## Phase 3.8.0a.0: Plan + design

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Three new kinds (`table`, `tablerow`, `tablecell`); `header=true` on tablerow; `align=L|R|C` on tablecell. | base doc § "Wire format" | — |
| [x] Tests | n/a (planning) | — | — |
| [x] Iterate | This plan | — | — |
| [ ] Commit | — | — | `Add Phase 3 round 8.0a plan: tables protocol + bridge` |

## Phase 3.8.0a.1: Spans parser accepts the three new kinds

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Extend `validRegionKinds` in `spanparse.go` with `table`, `tablerow`, `tablecell`. | base doc § "Wire format" | One-line vocabulary change per round 6/7 pattern. |
| [ ] Tests | TestParseSpanMessageRegionTable, TestParseSpanMessageRegionTableRowHeader, TestParseSpanMessageRegionTableCellAlignment, TestParseSpanMessageRegionTableNested (3-deep nesting). Update TestParseSpanMessageRegionUnknownKind to remove `table` from the unknown list. | `spanparse_test.go` | — |
| [ ] Iterate | Three lines added to validRegionKinds. | `spanparse.go` | — |
| [ ] Commit | — | — | `spans: accept table / tablerow / tablecell as region kinds` |

## Phase 3.8.0a.2: Bridge — apply functions for the three new kinds

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Add `applyTableRegion` (sets `Table=true, Block=true`), `applyTableRowRegion` (reads `header=true` param), `applyTableCellRegion` (reads `align=` param, maps to `Style.TableAlign`). Three new cases in `applyEnclosingRegions`. | base doc § "Bridge changes" | — |
| [ ] Tests | TestBuildStyledContent_RunInsideTableRegion, _InsideTableHeaderRow, _CellAlignment{Left,Right,Center}, _TableInsideBlockquote (composition). | `wind_styled_test.go` | — |
| [ ] Iterate | Three apply functions + dispatch cases + parseAlignment helper (similar to round 7's parseListNumber). | `wind.go` | — |
| [ ] Commit | — | — | `wind: applyEnclosingRegions handles table / tablerow / tablecell (bridge)` |

## Phase 3.8.0a.3: Merge prep

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | n/a (validation) | — | — |
| [ ] Tests | All packages green | `go test ./...` | — |
| [ ] Iterate | No smoke at this sub-round — there's no producer yet. End-to-end smoke happens in 8.0b. | — | — |
| [ ] Commit | — | — | n/a (no smoke fixes possible without producer). |

---

## After this sub-round

8.0a's deliverable: edwood ACCEPTS table directives. No
md2spans emission; no rendering of real tables. Round
8.0b adds the producer; round 8.0c handles smoke + final
docs + merge of the full v1.

## Risks

Three nested kinds is the largest bridge dispatch
expansion in any round. Tests pin each case; cross-kind
composition (table inside blockquote) tested explicitly.

## Status

Plan drafted. Awaiting review before any code.
