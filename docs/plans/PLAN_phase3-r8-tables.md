# Phase 3 Round 8 — Tables — Plan

The final region kind in the markdown-externalization
plan. v1 ships pipe-delimited GFM-style tables with
THREE new region kinds (`table`, `tablerow`, `tablecell`)
and `align=` param on cells.

**Column-width-aware alignment is DEFERRED to round
8.x.** v1 uses `family=code` overlay so that source-
aligned columns render aligned via monospace
character-positioning; non-aligned source produces
ragged but legible tables.

**Base design**: [`docs/designs/features/phase3-r8-tables.md`](../designs/features/phase3-r8-tables.md).

**Branch**: `phase3-r8-tables`.

**Files touched** (full list in design doc):
- `spanparse.go`, `wind.go`, `cmd/md2spans/parser.go` +
  tests.
- `docs/designs/spans-protocol.md`, `cmd/md2spans/README.md`.

No `rich/` changes — existing `Style.Table`,
`Style.TableHeader`, `Style.TableAlign` already drive the
layout. Column-width-aware alignment in 8.x will revisit.

---

## Phase 3.8.0: Plan + design

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | phase3-r8-tables.md drafted | [base doc] | Decisions: (a) v1 = three new kinds (table/tablerow/tablecell); (b) `header=true` param on tablerow rather than separate `tableheader` kind; (c) `align=L|R|C` on tablecell; (d) cell content uses `family=code` overlay; (e) NO column-width alignment in v1. |
| [x] Tests | n/a (planning) | — | — |
| [x] Iterate | This plan + design | — | — |
| [ ] Commit | — | — | `Add Phase 3 round 8 design and plan: tables` |

## Phase 3.8.1: Parser — accept the three new region kinds

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Extend `validRegionKinds` in `spanparse.go` with `table`, `tablerow`, `tablecell`. One-line vocabulary change per round 6/7 pattern. | base doc § "Wire format" | — |
| [ ] Tests | Add three TestParseSpanMessageRegion{Table,TableRow,TableCell} cases plus a nested-test pinning the 3-deep nesting. Update TestParseSpanMessageRegionUnknownKind to remove `table` from the unknown list. | `spanparse_test.go` | — |
| [ ] Iterate | Three lines added to validRegionKinds. | `spanparse.go` | — |
| [ ] Commit | — | — | `spans: accept table / tablerow / tablecell as region kinds` |

## Phase 3.8.2: Bridge — apply functions for the three new kinds

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Add `applyTableRegion` (sets `Table=true, Block=true`), `applyTableRowRegion` (sets `TableHeader=true` if `header=true` param present), `applyTableCellRegion` (sets `TableAlign` from `align=` param). Three new cases in `applyEnclosingRegions` switch. | base doc § "Bridge changes" | — |
| [ ] Tests | TestBuildStyledContent_RunInsideTableRegion, _InsideTableHeaderRow, _CellAlignment{Left,Right,Center}, _TableInsideBlockquote (composition). | `wind_styled_test.go` | — |
| [ ] Iterate | Three apply functions + dispatch cases + parseAlignment helper. | `wind.go` | — |
| [ ] Commit | — | — | `wind: applyEnclosingRegions handles table / tablerow / tablecell` |

## Phase 3.8.3: md2spans — table detection + emission

The largest single row in the round. Combines detection
(scanner with lookahead) and emission (nested begin/ends
+ cell content with family=code overlay).

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | scanParagraphs accumulates the table block when it sees a `\|` line followed by a separator line. Emits a single `paragraphRange{IsTable: true, ...}` for the block; parseTableParagraph walks cells, emits the nested begin/end tree, attaches `align=` from the separator row, marks the first row with `header=true`. Cell content goes through parseInlineSpans with family=code overlay applied to each emitted span. | base doc § "md2spans changes" / "parseTableParagraph" | — |
| [ ] Tests | Multi-shape: simple 2×2 table; alignment `|---|:--:|---:|` produces `align=left,center,right`; empty cells; cell with bold emphasis (bold span has family=code); not-a-table negative cases (no separator); table inside blockquote. | `cmd/md2spans/parser_test.go` | — |
| [ ] Iterate | isTableRow + isTableSeparator helpers; scanParagraphs lookahead; parseTableParagraph; alignment param parsing; family=code overlay on cell content spans. | `cmd/md2spans/parser.go` | — |
| [ ] Commit | — | — | `md2spans: detect and emit GFM tables` |

## Phase 3.8.4: Spec + README

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | n/a (doc) | — | — |
| [ ] Tests | n/a (doc) | — | — |
| [ ] Iterate | spans-protocol.md adds the three new kinds; documents `align=` and `header=` params; nested-table example. md2spans README's Tables row flips to ✓ with caveats (no column-width alignment in v1; round 8.x). Phase 3 roadmap: round 8 v1 ✓ landed. | — | — |
| [ ] Commit | — | — | `docs: spans protocol gains table region kinds; md2spans handles GFM tables` |

## Phase 3.8.5: Smoke + merge prep

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | n/a (validation) | — | — |
| [ ] Tests | All packages green | `go test ./...` | — |
| [ ] Iterate | Build binaries; smoke-test in real edwood with: simple 2×2 table; aligned columns; mixed alignment; empty cells; emphasis inside cells; table inside blockquote; non-table line starting with `\|` (verify it stays plain text). | — | User-driven. |
| [ ] Commit | — | — | n/a unless smoke surfaces a fix. |

---

## After this round

Round 8 v1 ships with the protocol and basic rendering.
Visual columns are aligned ONLY when the user's source
has them aligned (monospace cell-rendering preserves
character-position alignment).

**Round 8.x** addresses column-width-aware rendering:
either via a frame-dimension 9P endpoint + producer-
side width computation, or via layout-side two-pass
measurement. That round will likely change the layout's
table handling significantly.

After round 8.x, md2spans is feature-complete vs. the
in-tree path. Phase 4 (migration default flip + in-tree
deletion) unlocks.

## Risks

(See base design doc.) Lookahead in scanner is new; three
nested kinds is the most complex bridge yet; visual
without alignment will look ugly until 8.x. Mitigations
documented per item.

## Status

Plan + design drafted. Awaiting review before any code.
