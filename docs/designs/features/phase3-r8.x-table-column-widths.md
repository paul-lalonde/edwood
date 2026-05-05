# Phase 3 Round 8.x — Table Column-Width Alignment — Design

Round 8 v1 ships GFM tables that render in monospace.
Source-aligned columns visually align via character-
position; ragged source renders ragged. v1's `align=`
per-cell metadata is on the wire but has no visible
effect.

Round 8.x closes both gaps. Cells get padded to a per-
column width so columns align across rows regardless of
source layout, and `align=` controls whether the
padding goes to the left, right, or both sides.

## Critical constraint: rendered text === body text

md2spans's path deliberately maintains the property that
the rendered text is character-for-character the body's
source text. This lets:
- Cursor positions in the rich frame map directly to
  body offsets without a source-map.
- Look, Snarf, double-click word-expansion, and other
  acme operations work on the body without indirection.

The in-tree markdown package PADS cell text directly
into rich.Span text, which produces visually-aligned
tables but breaks the rendered/body invariant — that
package compensates with `markdown/sourcemap.go`. We
don't want to introduce source-mapping for md2spans.

**Constraint**: round 8.x must NOT change the rune count
or characters of any rich.Span. Alignment happens via
LAYOUT — extra horizontal space inserted via xPos
advance at cell boundaries — not via text padding.

## Approach

Two-pass at layout time. The renderer:

1. **Pass 1 (measure)**: walk the table region's runes,
   identify per-row cell boundaries (by `|` runes),
   and measure each cell's CONTENT WIDTH in pixels
   using the relevant font. Build a per-table
   `[]int{col1Width, col2Width, ...}` array (taking
   the max content width per column across all rows).

2. **Pass 2 (place)**: walk the table region a second
   time placing boxes. At each cell's start, set xPos
   to the cell's target column-start position. Within
   the cell, advance xPos by the box's natural width
   for content, and pad the trailing portion (or
   leading, for right-align) by advancing xPos to fill
   to the cell's full width.

Both passes operate on the `rich.Span` list AFTER
buildStyledContent. The bridge already populates
`Style.Table`, `Style.TableHeader`, and `Style.TableAlign`
on cell runes — that's enough metadata.

## Where in the code

The cleanest place is `rich/layout.go`'s main `layout`
function. We add:

- A pre-pass when entering a sequence of `Style.Table`
  boxes that computes the table's per-column widths.
- Per-box xPos adjustments using the precomputed widths
  and the box's `Style.TableAlign`.

Existing layout state:
- `xPos int`: current horizontal position.
- `currentLine`: in-progress line.

New layout state for a table:
- `inTable bool` and `tableColWidths []int`: active
  while iterating boxes inside a table region.
- `currentTableCol int` and `currentCellStartX int`:
  per-line cursor for cell-boundary detection.

## Column boundary detection in pass 1

Within the table region, cells are separated by `|`
runes that have `Style.Table=true` (i.e., produced by
the table region). The `|` runes at the START and END
of each row count as boundaries too.

Algorithm:
- Walk boxes in order.
- A box's text containing `|` marks a cell boundary.
  The text BEFORE the `|` is the cell's content; AFTER
  is the next cell.
- Multiple `|` runes per box are possible (e.g. an
  empty cell `||`); split the box logically.

Edge case: `|` runes inside cell content (escaped or
plain) are NOT cell boundaries. v1 doesn't handle
escapes; raw `|` inside a cell breaks the parser
already (documented v1 limitation, round 8 design).

## Alignment math

For a cell with target width `W`, content width `C`,
and align `A`:

- `align=left`: leading pad = 0; trailing pad = `W - C`.
- `align=right`: leading pad = `W - C`; trailing pad = 0.
- `align=center`: leading pad = `(W - C) / 2`; trailing
  pad = `W - C - leading`.

xPos advances by `leading` BEFORE rendering content,
then `C` for content, then `trailing` to reach the
cell's right edge.

The minimum cell width: `max(content_widths) over all
cells in the column`. The separator row's `---` cells
contribute width too — typically narrow but not zero.

## Header row

`Style.TableHeader=true` triggers bold rendering. Bold
runes have a slightly different metric than non-bold
in many fonts; the column-width measurement must use
the actual font used for each cell. Pass 1 selects the
font via the existing `getFontForStyle` helper.

## Tables inside blockquotes

The blockquote indent (`BlockquoteDepth × Width`) is
applied to the LINE, not to the table block per se.
The table-column layout starts after the blockquote
indent. Pass 1's measurement is per-cell content width;
pass 2 positions cells relative to the line's effective
left edge (= blockquote indent applied normally).

The round-7 listitemShifted rule and the table region's
flag set don't conflict with this — they govern other
indent contributions that run before the cell layout.

## Wire format

Unchanged. Round 8.x is a layout-side change; the
producer (md2spans) emits the same directives as
round 8.0. The round-8 design's deferred alignment is
realized in the renderer.

## Tests

### Layout
- 2-column table with non-uniform source: short cell
  `| a |` and long cell `| longer |`. After 8.x,
  rendered xPos for the second cell's start matches
  across rows.
- Mixed alignment: `align=left,center,right` produces
  visible padding on the appropriate side.
- Empty cells: padded to column width (uniform across
  rows).
- Header row's bold metric used for measurement.
- Table inside blockquote: column alignment works on
  top of blockquote indent.

### Layout test pattern
Existing `rich/layout_test.go` builds `rich.Content`
directly and feeds to `layout()`. The new tests use
the same pattern: synthetic table content with
`Style.Table` + `Style.TableAlign`, verify Box
positions after layout.

## Files touched

- `rich/layout.go` — table pass + alignment logic.
- `rich/layout_test.go` — column-width + alignment
  tests.
- `docs/designs/features/phase3-r8.x-table-column-widths.md`
  (this file).

No `cmd/md2spans/` changes (producer is already
correct). No `wind.go` changes (bridge already passes
the metadata via Style.TableAlign).

## Risks

1. **Two-pass complexity in `layout`.** Existing
   `layout()` is a single-pass function. Adding a
   measurement sub-pass for tables means the function
   knows about table structure for the first time.
   Mitigation: extract the table sub-pass into a
   helper (`layoutTable(...)` or similar) so the main
   flow stays linear and the table case is a single
   block.

2. **Box-text granularity.** A single rich.Span box
   may contain multiple cell separators (`|` runes).
   The pre-pass needs to split content WITHIN a box
   to find cell boundaries. Implementation detail —
   walks the box's text rune-by-rune.

3. **Font metric variance.** Bold (header) vs. plain
   bodyfont have slightly different widths. Pass 1
   measures using each cell's actual font. Pass 2's
   xPos advance must match.

4. **Performance.** Two passes over the table doubles
   the layout cost FOR THE TABLE PORTION only. Tables
   are typically small (dozens of rows × handful of
   columns); the overhead is bounded.

5. **Cursor positioning at padding gaps.** The runes
   between cells are unchanged; `Charofpt` (the rich
   frame's position-to-rune lookup) needs to handle
   the padding gap correctly — clicking in the white
   space between two cells should map to one of the
   adjacent cell boundaries. Existing `Charofpt`
   should handle this since boxes don't claim the gap
   pixels; the click falls into the nearest box.

## Status

Design drafted. Awaiting review before any code.
