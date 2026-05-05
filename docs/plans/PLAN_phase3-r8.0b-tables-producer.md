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
| [ ] Commit | — | — | `Add Phase 3 round 8.0b plan: tables producer` |

## Phase 3.8.0b.1: Detect table blocks (scanner with lookahead)

The first scanner case requiring lookahead: a `|` line
is a table-row line ONLY if the next line is a
separator. scanParagraphs needs to peek.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | scanParagraphs gains: when current line starts with `\|` AND looks like a table row, peek at the NEXT line; if it's a separator, accumulate the table block (header + separator + body rows until a non-table line); emit a single paragraphRange{IsTable: true, ...}. Implementation: switch from "process one line at a time" model to "scan ahead when a `\|` line appears." | base doc § "Detection" / "Risks: Lookahead" | First scanner case with lookahead. |
| [ ] Tests | At this row, tests verify shape via Parse output: a table source produces SOME paragraphRange identifiable as a table; non-table `\|`-lines stay as plain paragraphs. Detection-only tests (parseTableParagraph still emits placeholder spans). | `cmd/md2spans/parser_test.go` | Intermediate — full nesting tests come in row 2. |
| [ ] Iterate | isTableRowLine + isTableSeparatorLine helpers; lookahead in scanParagraphs (or pre-process lines). | `cmd/md2spans/parser.go` | — |
| [ ] Commit | — | — | `md2spans: detect GFM table blocks (header + separator + body)` |

## Phase 3.8.0b.2: parseTableParagraph emits the nested region tree

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | parseTableParagraph walks the table block, emits `begin region table`, then for each row `begin region tablerow [header=true]`, then for each cell `begin region tablecell [align=L|R|C]` + cell content (parseInlineSpans with family=code overlay) + `end region`. The separator row IS emitted as a row but its cells contain only `---` content (visual divider). | base doc § "parseTableParagraph" | — |
| [ ] Tests | Simple 2×2 table → 1 table region + 3 tablerow regions + 6 tablecell regions. Alignment `\|---\|:--:\|---:\|` produces `align=left,center,right`. Empty cells. Cell with bold (bold span has family=code overlay). Table inside blockquote. Not-a-table negative cases. | `cmd/md2spans/parser_test.go` | — |
| [ ] Iterate | parseTableParagraph + cell-walking + alignment-from-separator + family=code overlay merge for content spans. | `cmd/md2spans/parser.go` | — |
| [ ] Commit | — | — | `md2spans: emit nested table region directives with cell alignment` |

## Phase 3.8.0b.3: Merge prep

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | n/a (validation) | — | — |
| [ ] Tests | All packages green | `go test ./...` | — |
| [ ] Iterate | No user-driven smoke at this sub-round; end-to-end smoke is round 8.0c. Internal sanity: build binaries, run a quick mental test that a sample table parses without crashing. | — | — |
| [ ] Commit | — | — | n/a |

---

## After this sub-round

End-to-end tables work. Round 8.0c handles spec + README
updates + user smoke + merge of the full v1.

## Risks

Lookahead in scanner is new; other rounds' scanners are
single-pass. Triple nesting in emit is more complex than
prior rounds. Mitigations are unit tests at each layer.

## Status

Plan drafted. Awaiting 8.0a merge before any code here.
