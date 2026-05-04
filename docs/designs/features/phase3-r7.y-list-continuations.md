# Phase 3 Round 7.y — List Continuation Lines — Design

Round 7 v1 + 7.x cover single-line list items, with
nesting expressed via leading whitespace. Round 7.y
finishes lists by adding INDENTED CONTINUATION LINES —
the CommonMark feature where a non-marker line indented
to the active item's content column belongs to the
preceding item.

## Goal

Markdown like

```
- first item starts here
  and continues on the next line
- second item
```

renders as TWO list items, where the first item's
content INCLUDES the continuation line. The wire format
emits ONE listitem region whose range covers both lines
(not two separate regions).

## Scope

### v1.2 covers (this round)

- INDENTED continuation: a line indented to the active
  item's content column (or deeper) belongs to the item,
  unless it's itself a list marker line (which would
  start a new item or sub-item).
- Continuation lines extend the active item's
  paragraphRange `ByteEnd`. The listitem region's range
  spans the multi-line content.
- Blank line still terminates the list run (per round 7
  v1).

### v1.2 explicitly defers (round 7.z or beyond)

- LAZY continuation (CommonMark allows a non-indented,
  non-blank line after a list item to continue it; v1.2
  rejects this — the line will start a new paragraph).
- Multi-paragraph items (separated by blank lines but
  re-indented; CommonMark loose lists). v1.2 still
  treats blank line as list-terminator.
- Indented code blocks inside list items.

## Detection rules

A line is a CONTINUATION of the active list item if:

1. There IS an active list item (the previous non-blank
   line was a list marker line, or another continuation
   of the same item).
2. The line is NOT a list marker line itself (`isListLine`
   returns false).
3. The line is NOT blank.
4. The line's leading whitespace count is >= the active
   item's CONTENT COLUMN (the byte position right after
   the marker + space).

Content column is computed at scan time when the marker
line is detected:

- For unordered: marker char + space = 2 columns.
- For ordered: digits + ('.' or ')') + space = 2 + len(digits)
  columns.

So `- foo` has content column 2; `1. foo` has 3; `99. foo`
has 4.

A continuation line may have MORE leading whitespace than
the content column — that's still a continuation (the
extra whitespace is just rendered indent).

## Wire format

Unchanged. A multi-line item is still a single `begin
region listitem` / `end region` pair around the multi-
line content.

## md2spans changes

`scanParagraphs` adds state:

- `activeListItem *paragraphRange` — pointer to the
  item being built (nil when no list run is active).
- `activeContentCol int` — content column for the
  active item.

`flushLine` cases:

1. Active list, current line is a list marker:
   - Finalize the active item (push to out).
   - Start a new active item.
2. Active list, current line is blank:
   - Finalize the active item.
   - Clear active state.
   - Continue normal blank-line handling.
3. Active list, current line is indented continuation:
   - Extend `activeListItem.ByteEnd` to the current
     line's end.
4. Active list, current line is non-indented non-list:
   - Finalize the active item.
   - Clear active state.
   - Continue normal handling.
5. No active list, current line is a list marker:
   - Start active item.
6. No active list, normal handling.

`scanParagraphs` post-loop: finalize any active list
item.

## parseListItemParagraph changes

The function already takes a `paragraphRange` with
`ByteStart` / `ByteEnd` that may now span multiple
lines. The inline tokenizer (`parseInlineSpans`) already
walks all the runes between `contentBytePos(src, p)` and
`p.ByteEnd`, including any newlines. No structural
changes — the existing implementation just works for
multi-line items, with one caveat: the listitem region's
range needs to extend to the multi-line end.

The `itemRuneEnd` calculation:

```go
itemRuneEnd := p.RuneStart + utf8.RuneCountInString(src[p.ByteStart:p.ByteEnd])
```

continues to work — `src[p.ByteStart:p.ByteEnd]` now
includes the continuation lines.

## Layout interaction

Each line of a multi-line item independently goes through
the layout's first-box-determines-indent rule. The
continuation line's first rune is leading whitespace
(rendered text). Whether the continuation gets ListItem
flag depends on the listitem region's range:

- The listitem region covers the multi-line content. So
  the first rune of the continuation line IS in the
  listitem region.
- The bridge sets `ListItem=true, ListIndent=1` on that
  rune.
- Layout: line indent = 20.
- Plus source whitespace (2+ chars at content column).
- Continuation visually indents to about column 40.

This is consistent with the round-7.x sibling-region
model: each item's region covers all its content lines,
ListIndent stays at 1, source whitespace + line indent
combine to produce visual indent.

## Tests

### Parser
- `- a\n  cont` → one listitem region spanning both
  lines.
- `- a\n  cont\n- b` → two items; first covers both
  lines, second covers `- b`.
- `- a\n  cont1\n  cont2` → one item covering all three
  lines.
- `- a\n  cont\n\nplain` → one item; blank line
  terminates the run.
- `- a\n  cont\nplain` → one item ending after `cont`
  (the `plain` line is not indented, no longer a
  continuation; starts a new paragraph).
- `- a\nlazycont` → one item ending at `- a` (lazy
  continuation NOT supported in v1.2).
- `- a\n    deeply indented` → one item; the extra
  indent is just rendered whitespace (still a
  continuation since indent >= content column).
- `1. a\n   cont` → ordered item with content column 3
  → continuation requires 3+ leading spaces.
- `1. a\n  not enough indent` → not a continuation (2
  spaces < content col 3); the `not enough indent` line
  is treated as something else.

### Layout
- Multi-line item: each line gets ListItem flag → line
  indent applied uniformly. Source whitespace adds to
  visual position.

## Files touched

- `cmd/md2spans/parser.go` — scanParagraphs state for
  active list item; continuation detection;
  `paragraphRange` extension if needed.
- `cmd/md2spans/parser_test.go` — continuation tests.
- `docs/designs/spans-protocol.md` — multi-line listitem
  example.
- `cmd/md2spans/README.md` — Lists row caveat update.

No `wind.go` / `rich/` changes expected.

## Risks

1. **Indent-counting consistency.** Continuation
   detection uses byte-column counting (post-tab
   expansion isn't done; tab = 1 column for our purposes).
   Same simplification as 7.x's depth detection. CommonMark
   has more complex tab rules; users who hit this can be
   told the limitation.

2. **Sub-list vs continuation ambiguity.** `- a\n  - b`
   — is `  - b` a continuation of `a` or a sub-list?
   CommonMark says it's a sub-list (any line starting
   with a list marker, even indented, is a list line).
   v1.2 follows: isListLine check fires BEFORE
   continuation check.

3. **Nested-list continuation.** A continuation line at
   indent 4 inside a depth-2 outer list could be ambiguous
   (continuation of which level?). v1.2 keeps it simple:
   continuation always belongs to the MOST RECENT (deepest)
   active item.

## Status

Design drafted. Awaiting review before any code.
