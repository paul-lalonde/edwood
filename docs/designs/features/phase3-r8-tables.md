# Phase 3 Round 8 — Tables — Design

The final region kind in the markdown-externalization
plan. Adds GFM-style pipe-delimited tables to md2spans.

This round is the BIGGEST in scope of the per-feature
rounds; it has structural challenges (cell sub-regions,
column widths) that previous rounds didn't. To stay
shippable, round 8 v1 takes a deliberately MINIMAL
approach to visual rendering — column alignment is
deferred to round 8.x.

## Scope

### v1 covers (this round)
- GFM table detection: header row + separator row +
  body rows, all pipe-delimited.
- Three nested region kinds added to the v1 vocabulary:
  - `table` — the full block.
  - `tablerow` — one row.
  - `tablecell` — one cell. Optional `align=L|R|C` param
    derived from the separator row.
- Header rows carry an extra `header=true` param on
  their `tablerow` (or maybe a separate `tableheader`
  kind — TBD below).
- Cell content uses `family=code` (monospace) so that
  source-aligned tables visually align by character
  position. Source-unaligned tables still legible (just
  monospace text).
- Tables inside blockquotes (compose with round 6).

### v1 explicitly defers (round 8.x)
- **Column-width-aware alignment.** v1 doesn't compute
  column widths or pad cells. If the user's source
  has aligned columns, the monospace rendering keeps
  them aligned; if not, the table looks ragged. Round
  8.x will add either (a) a frame-dimension 9P endpoint
  + producer-side width computation, or (b) layout-side
  two-pass measurement.
- **Box-drawing chars.** The in-tree path replaces `|`
  with `┌─┬─┐` etc. in the rendered output. v1 keeps
  the `|` markup visible (markup-stays-visible stance).
- **Mid-cell line breaks** (`<br>` HTML, escape, etc.).
- **Cell content extending beyond simple inline
  formatting.** No nested lists / blockquotes / code
  inside cells. v1 cells contain inline-only content.

## Wire format

Three new region kinds. v1 valid set extends from
`{code, blockquote, listitem}` to
`{code, blockquote, listitem, table, tablerow, tablecell}`.

Nested structure:

```
begin region table
begin region tablerow header=true
begin region tablecell align=left
... cell content ...
end region
begin region tablecell align=center
... cell content ...
end region
end region
begin region tablerow
begin region tablecell align=left
... cell content ...
end region
... more cells ...
end region
... more rows ...
end region
```

Three levels of nesting. RegionStore handles arbitrary
nesting from round 5; no change needed.

### Header row indication

Two options:

**Option A — `header=true` param on `tablerow`**: the
producer marks the header row distinctly. Bridge sets
TableHeader on cell runes whose tablerow has
`header=true`.

**Option B — separate `tableheader` kind**: the producer
emits `begin region tableheader` for the header row, no
param. Bridge has a separate apply function.

**Decision: Option A**. One fewer kind in the
vocabulary; the param convention is already established
for `marker=`/`number=` on listitem.

### Alignment

`align=` param on `tablecell`. Values: `left`, `right`,
`center`. Derived from the separator row's `:` markers:
- `:---` → left
- `---:` → right
- `:---:` → center
- `---` → left (default)

Bridge maps to `Style.TableAlign` (the existing rich.Alignment field).

## md2spans changes

### Detection

A line is a table-row line if it starts with `|`. A
table-separator line is a row whose every cell consists
of dashes (with optional leading/trailing colons for
alignment).

A table BLOCK is: header row (table-row) + separator
row + zero or more body rows. v1 requires exactly that
shape; lines that don't match are not tables.

scanParagraphs (as it does for blockquote groups) needs
to look ahead one line when it sees a `|` line: if the
next line is a separator, it's a table; otherwise the
`|` line is plain text.

This is the FIRST scanner case that needs LOOKAHEAD.

### `paragraphRange` extension

```go
type paragraphRange struct {
    // ... existing
    IsTable      bool
    TableLines   []tableLineRange
    TableAligns  []rich.Alignment
}

type tableLineRange struct {
    ByteStart, ByteEnd int
    RuneStart          int
    IsHeader           bool
    IsSeparator        bool
    Cells              []tableCellRange
}

type tableCellRange struct {
    ByteStart, ByteEnd int  // [start, end) of cell content (between `|` chars)
    RuneStart          int
}
```

Or: keep `paragraphRange` simple (one for the whole
table block) and parse cell positions in
`parseTableParagraph`. Let me go with the simpler
approach.

### `parseTableParagraph`

Walks the table block byte by byte, identifying:
- The opening `|` of each row.
- The `|` separators between cells.
- The trailing `|` (optional).
- The cell content between separators.

For each row:
- Emit `begin region tablerow` (with `header=true` if
  it's the header).
- For each cell:
  - Emit `begin region tablecell` (with `align=` from
    the separator).
  - Inline content spans for the cell (parseInlineSpans
    over the cell content), with `family=code` overlay.
  - Emit `end region`.
- Emit `end region` (close tablerow).

Wrap with `begin region table` / `end region`.

The separator row itself: it gets emitted as a row, but
its cells contain only `---` content. Visual: monospace
dashes between rows. Functionally a divider.

## Bridge changes

`applyEnclosingRegions` switch gains three cases:

```go
case "table":
    applyTableRegion(s, r)
case "tablerow":
    applyTableRowRegion(s, r)
case "tablecell":
    applyTableCellRegion(s, r)
```

Where:

```go
func applyTableRegion(s *rich.Style, _ *Region) {
    s.Table = true
    s.Block = true // force block-level rendering
}

func applyTableRowRegion(s *rich.Style, r *Region) {
    if r.Params["header"] == "true" {
        s.TableHeader = true
    }
}

func applyTableCellRegion(s *rich.Style, r *Region) {
    switch r.Params["align"] {
    case "right":
        s.TableAlign = rich.AlignRight
    case "center":
        s.TableAlign = rich.AlignCenter
    default: // "left" or unset
        s.TableAlign = rich.AlignLeft
    }
}
```

Composition rule: idempotent for table; idempotent for
tablerow's header flag; per-cell payload for tablecell
(nearest-of-kind for align — but cells don't nest, so
this is moot).

## Layout interaction

The existing `rich/layout.go` already handles `Table:
true` as a block element (gutter indent, no wrap for
overflow). v1 of round 8 inherits that — tables look
like indented blocks of monospace text.

Column alignment: NONE. The `family=code` overlay makes
character-positions consistent across rows; if the
user's source has aligned columns, the visual is
aligned. If not, ragged.

Round 8.x will add column-width computation. v1 ships
with the simpler model.

## Tests

### Parser
- `| H1 | H2 |\n|---|---|\n| a | b |` → table with 2
  rows × 2 cells, header alignment all-left.
- Alignment: `|---|:--:|---:|` → left, center, right.
- Empty cells: `|  |  |\n|---|---|\n|  |  |`.
- Malformed (no separator): `| a | b |\n| c | d |` →
  not a table; emits as plain paragraphs.
- Table inside blockquote: `> | a | b |\n> |---|---|\n
  > | 1 | 2 |` → blockquote containing a table region.
- Cell with emphasis: `| **bold** |...` — emphasis
  parsed normally inside the cell.

### Bridge
- Table region → `Style.Table=true, Style.Block=true`.
- Tablerow with `header=true` → `Style.TableHeader=true`.
- Tablecell with `align=center` → `Style.TableAlign=
  rich.AlignCenter`.
- Composition with blockquote: blockquote depth+table
  flags both set.

## Files touched

- `spanparse.go` — extend `validRegionKinds` with
  `table`, `tablerow`, `tablecell`.
- `wind.go:applyEnclosingRegions` — three new apply
  functions + dispatch cases.
- `cmd/md2spans/parser.go` — table detection +
  parseTableParagraph.
- Test files at every layer.
- `docs/designs/spans-protocol.md` — three new kinds
  documented; alignment param.
- `cmd/md2spans/README.md` — Tables row flips to ✓
  (with v1 caveats).

## Risks

1. **Lookahead in scanParagraphs.** First case requiring
   it. The blockquote scanner does NOT look ahead — it
   just consumes lines starting with `>`. Tables need to
   peek at the NEXT line to see if it's a separator.
   Implementation: scanParagraphs accumulates lines and
   does the table check at scan time, OR processes lines
   into a list first then groups.

2. **Three nested kinds is a lot.** Bridge dispatch
   gains 3 cases at once. Composition between them is
   non-trivial — a tablecell rune is in table + tablerow +
   tablecell ancestors; bridge sees all three.

3. **`family=code` overlay vs. emphasis inside cells.**
   If the cell has `**bold**`, parseInlineSpans emits a
   bold span. That span needs to also have family=code
   (or the bold renders in non-monospace and ruins
   alignment). Need to merge family=code into all spans
   inside cells.

4. **No alignment in v1 will look bad.** Users will
   complain. Mitigation: design v1 README explicitly says
   alignment is round 8.x; tables look "monospaced but
   ragged" until then.

5. **Headers should look distinct.** TableHeader
   typically renders bold. Verify the layout / mdrender
   already does this for the in-tree path; it likely
   does. v1 inherits.

6. **Cell content with `|` characters** (escaped or
   raw). v1 doesn't handle escape sequences; raw `|`
   inside a cell breaks the parsing. Document as a
   known limitation.

## Status

Design drafted. Awaiting review before any code.

Round 8 is large enough that the user may want to scope
DOWN further (e.g., skip `tablerow header=true` for v1)
or commit to multiple sub-rounds (8.0, 8.x, 8.y).
Recommendation: ship 8.0 with this scope (no alignment
visualization) and validate the protocol shape; fold
column-width work into 8.x.
