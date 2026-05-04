# Phase 3 Round 7 — Lists (Per-Item Regions) — Design

Third region kind. Adds `listitem` to v1's region kind set.
First kind whose region carries PER-INSTANCE PAYLOAD (the
bullet character or item number) — exercises the
nearest-of-kind composition pattern that round 6.5's
per-kind apply functions were prepared for.

## Goal

edwood renders Markdown bullet lists (`- foo`, `* foo`,
`+ foo`) and ordered lists (`1. foo`, `2. foo`) when
md2spans is the renderer, with the existing in-tree
visual: indented item, bullet/number visible at the start.

Round 7 v1 keeps scope tight. Nested lists (via leading
whitespace) and multi-line items (continuation lines) are
deferred to a follow-up sub-round (7.x). v1 covers what's
needed to flip the Lists row in the README to ✓-with-
caveats.

## Scope

### v1 covers
- Bullet markers `-`, `*`, `+` at column 0 followed by a
  space, then content.
- Ordered markers `N.` or `N)` at column 0 followed by a
  space, then content. N is one or more decimal digits.
- One LINE per item — no continuation lines.
- A run of consecutive list lines forms ONE list visually,
  but each line is its own region (per the round-6 "region
  per item" plan).
- Lists may appear inside blockquotes (round 6 +
  round 7 compose, like code-in-blockquote did).

### v1 explicitly defers (round 7.x)
- Leading-whitespace-driven NESTING (`  - inner`).
- Multi-line items (continuation lines indented under the
  marker's content column).
- Loose vs. tight list distinction.
- Lazy continuation (CommonMark allows `- foo\nbar` to
  treat `bar` as part of the foo item).

## Wire format

New region kind on the existing `begin region` /
`end region` directive shape:

```
begin region listitem marker=-
... content ...
end region
```

```
begin region listitem number=1
... content ...
end region
```

### Params

- `marker=X` — unordered list. X is one of `-`, `*`, `+`.
  Reflects the source bullet character (markup stays
  visible: the bullet is also a visible source rune in the
  body).
- `number=N` — ordered list. N is the item's decimal
  number, ≥ 1. Implies ordered. The renderer can use
  this for syntax-aware features later; v1 just keeps it
  metadata.

Exactly one of `marker` or `number` must be present.
Producers that emit neither produce a wire-format error
(not silently default).

`indent=` is NOT on the wire. Like blockquote depth, the
bridge computes nesting depth from ancestor count. v1
won't emit nested listitem regions, so depth is always 1
(one ancestor); but the mechanism is the same shape as
round 6's blockquote depth and ready for round 7.x's
nesting.

## Region scope (decision: full coverage)

A `listitem` region COVERS the entire item's source line,
including the bullet/number marker. Rationale:
- **Consistent with markup-stays-visible** (rounds 1-6).
  The marker renders as ordinary text.
- **Single bullet position.** The bullet is at the
  region's start. The renderer's existing layout reads
  `Style.ListItem` from the line's first box and applies
  `ListIndent * ListIndentWidth` to the LINE's leading
  edge.
- **Simpler bridge.** Per-rune lookup finds the same
  listitem region for marker AND content runes; no
  special-casing. Same approach as blockquote.

The region spans from the line's column-0 (the marker) to
the rune AFTER the line's terminating `\n` (or to EOF if
no `\n`). The terminating `\n` IS part of the region —
matches round 6 blockquote.

For consecutive list items (each line its own region),
the regions abut: one ends at the next's start.

## `applyEnclosingRegions` change (bridge)

Round 6.5 split `applyEnclosingRegions` into per-kind
apply functions. Round 7 adds a new one:

```go
case "listitem":
    applyListitemRegion(s, r)
```

```go
func applyListitemRegion(s *rich.Style, r *Region) {
    s.ListItem = true
    s.ListIndent++
    // Per-region payload extraction. Outermost-first walk
    // means the innermost listitem's marker/number wins
    // — natural nearest-of-kind via overwrite. v1 only
    // emits one level of listitem, so the chain has at
    // most one listitem ancestor; but the pattern is
    // ready for round 7.x's nesting.
    if marker, ok := r.Params["marker"]; ok && marker != "" {
        s.ListOrdered = false
        // v1: don't store marker char. The marker rune is
        // visible as body text already (markup stays
        // visible). Future rounds may add a
        // Style.ListMarker field if syntax-aware
        // rendering needs it.
    } else if number, ok := r.Params["number"]; ok {
        s.ListOrdered = true
        s.ListNumber = parseListNumber(number)
    }
}
```

`parseListNumber` is a small helper that converts the
`number=` string to an int, defaulting to 0 on parse
error (the protocol consumer should never see invalid
producer output, but be defensive).

### Composition rule classification

| Kind       | Rule                | What composes                                          |
|------------|---------------------|--------------------------------------------------------|
| code       | idempotent          | `Block`, `Code`, `Bg` set; multiple ancestors no-op    |
| blockquote | additive            | `BlockquoteDepth` increments per ancestor              |
| listitem   | additive + payload  | `ListIndent` increments; `ListNumber`/`ListOrdered` overwrite per-call (innermost wins) |

Round 7's listitem fits the existing per-kind function
pattern without further refactoring of
`applyEnclosingRegions`. Round 6.5's split made room.

## md2spans: list detection

`scanParagraphs` adds a "list line" check alongside the
existing blockquote check. A line is a list line if its
column 0 starts with one of:
- `- ` (dash + space)
- `* ` (asterisk + space)
- `+ ` (plus + space)
- `N. ` (digits + period + space)
- `N) ` (digits + closing paren + space)

A run of consecutive list lines (no blank line between)
forms ONE list group. Each line in the group becomes its
own `paragraphRange{IsListItem: true, ListMarker: <char>,
ListNumber: <N or 0>}`.

A blank line ends the list. A non-list non-blank line
ends the list. Behavior matches round 6's blockquote
group detection.

### `paragraphRange` extension

```go
type paragraphRange struct {
    // ... existing fields
    IsListItem bool
    ListMarker byte // '-', '*', '+', or 0 if ordered
    ListNumber int  // 1+ for ordered, 0 for unordered
}
```

### `parseListItemParagraph`

Emits the spans for one list item line:

```go
func parseListItemParagraph(src string, p paragraphRange) []Span {
    begin := Span{
        Kind:        SpanRegionBegin,
        Offset:      p.RuneStart,
        RegionBegin: "listitem",
    }
    if p.ListMarker != 0 {
        begin.RegionParams = map[string]string{
            "marker": string(p.ListMarker),
        }
    } else {
        begin.RegionParams = map[string]string{
            "number": strconv.Itoa(p.ListNumber),
        }
    }
    itemRuneEnd := p.RuneStart + utf8.RuneCountInString(src[p.ByteStart:p.ByteEnd])
    // Recursively parse the item's CONTENT (after the
    // marker + space) for inline emphasis / links.
    // v1: no further block constructs inside an item.
    // The marker + space themselves are NOT covered by an
    // inline span — they render as default text via
    // emit-time gap-fill, matching markup-stays-visible.
    contentStart := /* position after marker + space */
    content := parseInlineSpans(/* runes after marker */)
    end := Span{
        Kind:      SpanRegionEnd,
        Offset:    itemRuneEnd,
        RegionEnd: true,
    }
    return append(append([]Span{begin}, content...), end)
}
```

## Lists inside blockquotes

A blockquote group containing `>> - foo` lines: the outer
strip removes the first `>`, leaving `> - foo`. The inner
strip removes `> ` leaving `- foo` — recognized as a list
line.

scanParagraphs in the recursive call detects the list,
emits a listitem paragraphRange. parseListItemParagraph
emits the begin/end pair in doubly-stripped coords. Round
6.5's remap (rune-at for begin, boundary-before for end)
maps offsets back to original.

The listitem region in original covers the `>>` markers
PLUS the marker char + content. Wait — same problem as
round 6.5 round 5 had with code-in-blockquote: the `>>`
markers between consecutive list lines fall INSIDE the
listitem regions if a single region is emitted per item.

But each list LINE is its own region (no inter-line span).
The region for line N covers ONLY that line's runes
(including the line's `>>` markers — but those ARE the
markers of THIS item's line, not between items). The next
item's region starts at its own line's start.

So: between item 1 and item 2 (in original coords), the
boundary is at item 1's `\n` end / item 2's line start.
The `>>` markers of item 2 are inside item 2's region,
which is correct (they belong to that item's line).

No per-line splitting needed for lists (unlike code, which
v1 splits because a single code BODY spans multiple lines).
Each list ITEM is one line.

When v1 grows multi-line items (round 7.x), we'll revisit
— same structural concern as round 6.5's code body, same
fix (per-line emission within an item, OR explicit
boundary-aware region per content line).

## Tests

### `spanparse_test.go`
- `begin region listitem marker=-` parses to
  `Region{Kind: "listitem", Params: {"marker": "-"}}`.
- `begin region listitem number=3` parses similarly.
- Nested listitem (synthetic for v1): two
  begin/end pairs produce two regions.

### `wind_styled_test.go`
- Single listitem region → `ListItem=true,
  ListIndent=1, ListOrdered=false` for unordered.
- Single listitem with `number=3` → `ListItem=true,
  ListIndent=1, ListOrdered=true, ListNumber=3`.
- Listitem inside blockquote → both kinds compose.

### `cmd/md2spans/parser_test.go`
- `- foo` → one listitem region with `marker=-`.
- `* foo`, `+ foo` similarly.
- `1. foo` → one listitem region with `number=1`.
- `1) foo` → one listitem region with `number=1`.
- Multi-line list `- a\n- b\n- c` → three regions.
- List terminated by blank line.
- List terminated by non-list line.
- `- foo\nbar` → only `- foo` is a list item; `bar`
  starts a new plain paragraph (no continuation in v1).
- List inside blockquote: `> - foo` → blockquote
  region containing one listitem region.
- Non-list lines starting with `-` (e.g., `-foo` no
  space, `--- ` HRule precedence): negative cases.

### Visual smoke
- Single bullet list, single ordered list.
- Mixed ordered/unordered consecutively.
- List inside blockquote.

## Files touched

- `spanparse.go` — extend `validRegionKinds`.
- `wind.go:applyEnclosingRegions` — add `case "listitem"`
  + new `applyListitemRegion` per-kind function.
- `cmd/md2spans/parser.go` — `scanParagraphs` list
  detection; `parseListItemParagraph` emit.
- `cmd/md2spans/parser_test.go` — list-emission tests.
- `wind_styled_test.go` — listitem bridge tests.
- `spanparse_test.go` — listitem kind acceptance.
- `docs/designs/spans-protocol.md` — extend kind set;
  add listitem param documentation.
- `cmd/md2spans/README.md` — flip Lists to ✓ with
  caveats (column-0 only, single-line, no nesting in v1).

No `rich/` changes — existing `Style.ListItem`,
`Style.ListIndent`, `Style.ListOrdered`, `Style.ListNumber`
fields drive the layout.

## Risks

1. **HRule precedence.** A line like `--- ` matches both
   the existing HRule detection and could be misread as a
   list line. HRule must take precedence (matches CommonMark
   and the in-tree path). The existing scanParagraphs's
   detection order (HRule → list) handles this — the list
   check fires only if HRule doesn't.

2. **Asterisk ambiguity.** `* foo` is a list line. `*foo*`
   is emphasis. The list detection requires a SPACE after
   the marker; `*foo*` has no space, so it's not a list.
   The detection must be column-0-with-following-space,
   not column-0-asterisk.

3. **`-` vs HRule.** `---` (3+ dashes, only dashes) is
   HRule. `- foo` is a list. The HRule detection requires
   3+ same-char and only the marker char + whitespace.
   `- foo` doesn't match HRule (the `f` breaks it).

4. **md2spans line iteration.** scanParagraphs already
   has lots of line state. Adding list detection mustn't
   break the blockquote/code/HRule/heading state machine.
   v1: list detection fires AFTER the existing checks
   (HRule, heading, fence, blockquote). If none of those
   match, check list.

5. **Bridge nearest-of-kind correctness.** v1 has at most
   one listitem ancestor per rune (no nesting). Round
   7.x will introduce nesting; the overwrite-per-call
   composition needs verification against actual nested
   inputs. Defer the verification to 7.x.

## Status

Design drafted. Awaiting review before any code.
