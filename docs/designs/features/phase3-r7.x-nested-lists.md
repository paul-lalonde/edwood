# Phase 3 Round 7.x — Nested Lists — Design

Round 7 v1 covered column-0 single-line list items.
Round 7.x extends that with NESTED LISTS via leading
whitespace — the most common gap users hit when writing
CommonMark.

Multi-line continuation lines, loose-vs-tight, and lazy
continuation remain deferred to a later sub-round (7.y).

## Goal

Markdown like

```
- outer
  - inner
  - another inner
- back to outer
```

renders with `inner` and `another inner` indented one
ListIndentWidth deeper than `outer` — matching the
visual the in-tree markdown path produces.

## Scope

### v1.1 covers (this round)

- Leading-whitespace-driven nesting:
  - 2 spaces or 1 tab per indent level (matches the in-tree
    path; `markdown/parse.go:1535`).
  - Indent N → listitem region nested N levels deep.
- Mixed marker types across nesting (e.g., `-` outer with
  `*` or `1.` inner) — each item carries its own
  `marker=` / `number=` payload independently.
- Nested lists inside blockquotes (compose with round 6 +
  round 7).

### v1.1 explicitly defers to round 7.y or beyond

- Multi-line items via indented continuation:
  ```
  - first line
    second line still part of the item
  ```
  v1.1 still treats each line as its own item.
- Lazy continuation (CommonMark allows
  `- foo\nbar` — bar continues the foo item).
- Loose vs. tight list distinction (blank lines between
  items still terminate the list in v1.1).
- Indented code blocks via 4-space indent.

## Wire format

Unchanged from round 7. A nested list emits a sequence of
nested `begin region listitem` / `end region` pairs:

```
begin region listitem marker=-
... outer item content ...
begin region listitem marker=-
... inner item content ...
end region
end region
```

The bridge already counts listitem ancestors and
increments `ListIndent` accordingly (round 7 design
§ "applyEnclosingRegions change"). No `wind.go` changes.

## md2spans — nesting detection

### Indent counting

A line's indent level is computed from leading whitespace
BEFORE the marker:

- Each tab counts as 1 level.
- Each pair of spaces counts as 1 level.
- Odd trailing space (e.g. 1 space, 3 spaces) is rounded
  DOWN to the previous level (matches the in-tree path's
  `tabCount + spaceCount/2` semantics).

A line is a list line if, after stripping leading
whitespace, it matches the existing round-7 detection
(`- `, `* `, `+ `, `N. `, `N) `).

### List-stack state machine

`scanParagraphs` maintains a `listStack` — a slice of
indent levels active on the current "list run". When a
list line arrives:

1. Compute its indent level `L`.
2. Pop entries from `listStack` that are deeper than `L`
   (close those nested levels with `end region` directives).
3. If `listStack`'s top is at level `L`: this is a sibling
   item. Don't push.
4. If `listStack`'s top is at level `< L`: this is a new
   nested level. Push.
5. Emit the item's `paragraphRange` carrying its DEPTH
   (= `len(listStack)`).

Non-list and blank lines clear the stack (close all open
listitem regions). v1.1 keeps the round-7 v1 rule:
blank-line terminates the list run.

### `paragraphRange` extension

```go
type paragraphRange struct {
    // ... existing
    IsListItem           bool
    ListMarker           byte
    ListNumber           int
    ListContentRuneStart int
    ListDepth            int   // 1 for top-level, 2 for nested, etc. (round 7.x)
}
```

`ListDepth` is informational here — the actual nesting
of the emitted regions is driven by the scanner's
list-stack via begin/end pairs, not by this field. It's
included for testability (one assertion per item).

### Emit pattern

Round 7 v1's `parseListItemParagraph` emits one begin/end
pair around the item line. Round 7.x doesn't change that
function — the NESTING is expressed by emitting MULTIPLE
begin/end pairs across CONSECUTIVE lines, with the
scanner inserting parent `begin region`s before deeper
items and parent `end region`s after the run ends or when
unwinding to a shallower level.

This means scanParagraphs's emit phase needs awareness of
the list-stack state, OR we restructure such that
`scanParagraphs` returns the list-stack transitions and
`Parse` drives the begin/end emission.

**Decision**: keep the paragraphRange-shape interface
between scanParagraphs and Parse. Add two new
paragraphRange shapes:

- `IsListLevelOpen` — emitted when entering a new nesting
  level. Carries `ListDepth`. parseListLevelOpen emits a
  `begin region listitem` directive at the appropriate
  offset (the current item's marker position) WITHOUT
  emitting an `end region`.
- `IsListLevelClose` — emitted when exiting a nesting
  level. Carries the offset where the close should land.

Hmm — this is getting complex. **Alternate decision**:
simpler approach is to keep the scanner+Parse interface,
and have parseListItemParagraph emit a single item with
its depth, while scanParagraphs additionally emits
"open"/"close" sentinel paragraphs that produce the
matching outer-level begin/end pairs.

After thinking about this: the simplest model is to make
the scanner emit, for each list item:

- ZERO OR MORE "open level" sentinel ranges (when nesting
  deeper).
- The item's paragraphRange (one per item).
- ZERO OR MORE "close level" sentinel ranges at the end
  of the list run.

The Parse switch dispatches:
- `IsListLevelOpen` → emit a single `begin region
  listitem` (no end yet).
- normal `IsListItem` → emit `begin/content/end` for THIS
  level, then close THIS LEVEL's level-open at the end.

Wait, that doesn't cleanly nest either.

Let me revise. **Final decision**:

Each nested list ITEM gets ONE listitem region. Nesting
is achieved by REGISTERING outer-level "open" regions
that span OVER multiple inner items. The scanner emits a
HIERARCHY of nested item ranges, and a new
`parseListItemParagraphNested` emits the corresponding
nested begin/end pattern.

Actually, the cleanest implementation: the scanner emits
the items in order, each carrying `ListDepth`. A new
`Parse` post-processing pass converts this flat list into
properly-nested begin/end pairs by tracking depth
transitions.

I'll prototype with that approach in implementation.

## md2spans — content offset within a nested item

Round 7 v1 computes `ListContentRuneStart` as
`p.RuneStart + 2` (unordered) or after digits + delimiter
+ space (ordered). For nested items with leading
whitespace, the content starts AFTER the whitespace AND
the marker. Update `contentBytePos` to account for the
leading-whitespace prefix.

The listitem region itself begins at the line's first
non-whitespace position (the marker), not at column 0.
Leading whitespace runes are NOT in the listitem region —
they're plain text in the body. Same as v1's
markup-stays-visible stance.

### Layout interaction

A nested list item with `ListIndent=2` produces, per the
existing round-7 layout rule:

- For top-level (no blockquote): the line's first box has
  ListItem=true ListIndent=2 → line indent = 2 × 20 = 40.
  Visual: 40px gap before `-`.
- For inside-blockquote: combined effect of blockquote
  depth + listitem region entry shift. Round 7's smoke-fix
  shift ensures the deeper listitem advances xPos by
  ListIndent×Width.

For nested lists, the layout's per-line first-box rule
should still produce visual nesting. v1's
`listitemShifted` flag fires once per line at the FIRST
listitem entry; for an item at depth 2 inside a
blockquote, the shift is `ListIndent × Width = 2 × 20 =
40`. Combined with the blockquote indent, the line's `-`
is at `(BlockquoteDepth + ListIndent) × Width + something`.

**Risk**: the round-7 smoke-fix layout rule may need
revisiting for depth-2 nested-list-inside-blockquote
correctness. v1.1 will exercise this in tests and fix at
that time.

## md2spans — leading-whitespace rendering

Source has leading whitespace before the marker. Per
v1's markup-stays-visible stance, the whitespace is
visible in the body. Layout: the line's first box is the
first whitespace rune (no ListItem flag — listitem region
begins at the marker). Layout's first-box rule: no flags
→ no indent. The whitespace renders at column 0, then
marker, then content.

Wait — this means a depth-2 nested item is rendered
as `  - item` at column 0..2 + marker + content. The
visual list indent comes from the SOURCE WHITESPACE, not
the layout indent. That's actually consistent with how
the user types markdown: they put leading spaces and
expect them to render.

But with the listitem region's bridge applying ListIndent,
the LAYOUT also indents the line. Double indent?

Let me think through this carefully:

Source `  - item` (depth 2):
- runes 0, 1: leading spaces. NOT in listitem region.
  Style: empty.
- rune 2: `-`. In listitem region. Style: ListItem=true,
  ListIndent=2 (counts 2 ancestors? or 1?).
  - Wait, the OUTER listitem region (at depth 1) covers the
    OUTER item, NOT this inner line. The inner item's
    listitem region is at depth 2.
  - But the bridge counts ancestors. If only the inner
    listitem is in the chain (no outer listitem covers
    this line), then ListIndent = 1.
  - For the ancestor walk to give ListIndent = 2, we need
    the OUTER listitem region to ALSO contain the inner
    item's runes.

This depends on how nesting is represented in the regions.

Two options:

**Option A — Tree of CONTAINMENT regions**: outer item
region covers the whole outer item INCLUDING its sub-
list. The sub-list's items are nested inside the outer's
range. The bridge's ancestor walk finds 2 listitem
ancestors → ListIndent = 2.

**Option B — Sibling regions**: each item is its own
region; items at deeper depth are SIBLINGS at deeper
positions. The bridge's ancestor walk finds 1 listitem
ancestor → ListIndent = 1.

Option A composes correctly with the bridge's existing
ancestor-counting logic (matches round 6 blockquote).
Option B requires the bridge to use a different rule.

**Decision: Option A.** Outer item region covers from the
outer item's marker to the END of its content (including
any sub-list items). Inner item region nests inside.

This means scanParagraphs needs to KNOW where each outer
item ends — which requires looking ahead to find where
the deeper-indented sub-list ends.

### Item-end determination

An outer-level item ENDS when:
- The next line is at the same outer level (sibling).
- The next line is at a shallower level.
- The list run ends (blank line, non-list line).

So the outer item's range is FROM its own marker line TO
just before the next sibling or list-end.

### Concrete algorithm

scanParagraphs's list handling:

```
listStack = []  // each element: {level, openOffset}

for each line:
    if blank or non-list:
        close all entries in listStack
        ...
        continue
    
    L, marker, number, contentByte = isListLine(...)
    if not list line:
        close all entries in listStack
        ...
        continue
    
    // Pop deeper levels (close them).
    while listStack non-empty and listStack.top.level > L:
        emit close paragraph for listStack.top
        listStack.pop()
    
    // Pop SAME level if any (close current item to start
    // a new sibling at same level).
    if listStack non-empty and listStack.top.level == L:
        emit close paragraph for listStack.top
        listStack.pop()
    
    // Push current level.
    emit open paragraph for L (begin region listitem)
    listStack.push({level: L, openOffset: lineStart})
    // Emit content lookup happens inline.

at EOF:
    close all in listStack.
```

The "open paragraph" and "close paragraph" are
paragraphRange shapes that emit ONLY the begin or only
the end directive.

Hmm, this is getting complex. Let me simplify further:
emit each item as its own paragraphRange (with depth and
marker info). Add a POST-PROCESSING pass that walks the
paragraphRange list and rearranges to produce nested
begin/ends.

I'll prototype it in code.

## Tests

### Parser
- `- a\n  - b` → one outer + one inner; offset of inner
  begin reflects the line-start of `  - b` (after the
  leading whitespace, at the marker position; OR at the
  line start — TBD).
- 3-deep nest: `- a\n  - b\n    - c` → three regions
  nested.
- Mixed markers: `- a\n  1. b` → outer marker `-`,
  inner marker number `1`.
- Pop on shallower: `- a\n  - b\n- c` → outer / inner /
  sibling; inner closed before sibling.
- Blank line clears stack.
- Non-list line clears stack.
- Tab counts as 1 level (`- a\n\t- b`).
- Odd-spaces don't open (`- a\n - b` — 1 space — does it
  count as level 0 + 0.5 → rounded to 0, treated as
  continuation? Per CommonMark, this is a continuation,
  not a sub-list. v1.1 v1: probably treat as continuation
  → which we don't support → stays at outer level.
  Actually let me think... the existing rule
  `tabCount + spaceCount/2` would give 0 for 1 space.
  So 1 space + `- ` is treated as level 0 (sibling).
  That matches in-tree behavior).

### Bridge
- Three-level-nested listitem region in the region store
  → bridge's applyEnclosingRegions sees 3 ancestors →
  `ListIndent = 3`.

### Layout
- Item at depth 2 (top-level): line indent = 2 × Width.
- Item at depth 2 inside blockquote depth 1: combined
  indent works (existing layout rule from round 7 smoke
  fix).

## Files touched

- `cmd/md2spans/parser.go` — leading-whitespace detection,
  list-stack state machine, paragraphRange extension,
  parseListItemParagraph adjustments for content offset
  with leading whitespace.
- `cmd/md2spans/parser_test.go` — nested-list tests.
- `docs/designs/features/phase3-r7.x-nested-lists.md` (this
  file).
- `cmd/md2spans/README.md` — update Lists row to mention
  nesting.
- `docs/designs/spans-protocol.md` — examples of nested
  listitem regions.

No `wind.go` / `rich/` changes expected — round 7's
ancestor-counting bridge and layout rule should compose
correctly. (If smoke surfaces issues, we patch in 7.x.)

## Risks

1. **Region containment semantics.** Deciding outer item
   region COVERS its sub-list (Option A above) is the
   correct call but requires lookahead in the scanner
   — first round to require it. Wrong implementation
   could produce wrong region boundaries → misaligned
   regions → render artifacts.

2. **Layout interactions.** Round 7's smoke-fix layout
   rule (`listitemShifted` once-per-line, restricted to
   ListItem && Blockquote) was designed for v1's
   single-line-list assumption. v1.1's nested lists at
   depth 2+ inside blockquotes may need the rule
   revisited (e.g., shifting by `ListIndent × Width`
   where ListIndent > 1).

3. **Marker rune index.** With leading whitespace,
   `ListContentRuneStart` is `lineStart + leadingWhitespace
   + markerLen + 1` (the trailing space). v1's
   `contentBytePos` only accounts for marker length;
   needs extension.

4. **Tab vs space mixing.** CommonMark says tab is
   "expanded" to align to the next 4-column tab stop. v1.1
   simplifies: tab = 1 level; space pair = 1 level.
   Mixing might produce surprising depth values; users
   who hit this edge case can be told the limitation.

5. **Lazy continuation NOT supported.** A user typing
   `- foo\nbar` (with `bar` at column 0, no whitespace
   continuation) will have `bar` parsed as a fresh
   paragraph, not as continuation of the foo item. This
   is the round-7-v1 behavior preserved. Users may not
   notice but watch for confusion.

## Status

Design drafted. Awaiting review before any code.
