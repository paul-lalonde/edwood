# Phase 3 Round 8.0b — Tables: md2spans Producer — Plan

Second of three sub-rounds for round 8. Sub-round 8.0b
lands the md2spans-side detection and emission of GFM
tables. Depends on 8.0a being merged (consumer-side
support for the new kinds).

After this sub-round merges, end-to-end tables work:
md2spans detects `| H1 | H2 |\n|---|---|\n| a | b |` in
the body and emits the nested begin/end region tree;
edwood's bridge maps it to `Style.Table` flags; the
existing layout treats the run as a block-level element.

Visual: monospace cell text. Source-aligned columns
look aligned (character-position), unaligned source
looks ragged. Column-width-aware alignment is round
8.0c's problem (or punted further).

**Base design**: [`docs/designs/features/phase3-r8-tables.md`](../designs/features/phase3-r8-tables.md).

**Depends on**: 8.0a merged.

**Branch**: `phase3-r8.0b-tables-producer`.

**Files touched**:
- `cmd/md2spans/parser.go` — table detection + emission.
- `cmd/md2spans/parser_test.go` — table tests.

No `wind.go` / `rich/` / spec / README changes here —
those land in 8.0c after smoke confirms.

---

## Phase 3.8.0b.0: Plan + design

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Inherits from base design doc. | base doc § "md2spans changes" / "parseTableParagraph" | — |
| [x] Tests | n/a (planning) | — | — |
| [x] Iterate | This plan | — | — |
| [x] Commit | — | — | `Split round 8 plan into three sub-rounds (8.0a / 8.0b / 8.0c)` |

## Phase 3.8.0b.1+2: Detection + emission (combined)

Detection without emission produces no testable output;
combining them gives one tested red→green cycle. Per
the round-7 precedent (where rows 3 + 4 of round 7
were similarly combined).

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | scanParagraphs gains inTable / tableStart* state + nextLineSep lookahead helper. Detection: a column-0 `\|`-line whose successor is a separator confirms the table; subsequent `\|`-lines extend; any non-`\|` line emits and falls through. parseTableParagraph emits the nested begin/end tree (table → tablerow [header=true on row 0] → tablecell [align=L\|R\|C from separator]); cell content uses family=code overlay via emitTableCellContent (parseInlineSpans + gap-fill family=code). | base doc § "Detection" + "parseTableParagraph" | — |
| [x] Tests | TestParseTableSimple (1 table + 3 tablerow + 6 tablecell), TestParseTableAlignment (`:---\|:--:\|---:` → left,center,right), TestParseTableNotATable / TestParseTableLeadingPipeAlone (negative cases), TestParseTableEmptyCells, TestParseTableInsideBlockquote. | `cmd/md2spans/parser_test.go` | — |
| [x] Iterate | isTableRowLine + isTableSeparatorLine + tableSeparatorCellAlign helpers; scanParagraphs state + lookahead; parseTableParagraph + emitTableCellContent. | `cmd/md2spans/parser.go` | — |
| [x] Commit | — | — | `md2spans: detect and emit GFM table blocks` |

## Phase 3.8.0b.3: Merge prep

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | n/a (validation) | — | — |
| [x] Tests | All packages green | `go test ./...` | Green. |
| [x] Iterate | No user-driven smoke at this sub-round; end-to-end smoke is round 8.0c. | — | — |
| [x] Commit | — | — | n/a |

---

## After this sub-round

End-to-end tables work. Round 8.0c handles spec + README
updates + user smoke + merge of the full v1.

## Risks

Lookahead in scanner is new; other rounds' scanners are
single-pass. Triple nesting in emit is more complex than
prior rounds. Mitigations are unit tests at each layer.

## Status

All rows complete. Ready to merge to master (no smoke
at this sub-round; smoke is 8.0c's purpose).
