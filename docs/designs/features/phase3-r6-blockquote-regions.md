# Phase 3 Round 6 — Blockquote (Nested Regions) — Design

## Purpose

Sixth round of Phase 3 of the markdown-externalization plan
([markdown-externalization.md](markdown-externalization.md)).
Adds the second region kind — `blockquote` — and exercises
**nested** regions for the first time. Validates the round-5
region machinery's claims about kind-vocabulary extension and
ancestor-walk composition.

Round 5 established the region machinery (begin/end
directives, sidecar tree storage, bridge expansion); round 6
adds the second kind without changing the wire format's shape.
The arch review of round 5 specifically flagged blockquote-
with-depth-counter as the case that would stress the bridge's
"OR-composition" approach. Round 6 implements the COUNT-by-
ancestor strategy that resolves that concern.

## Scope

- New region kind: `blockquote`. v1 has no required params;
  depth is computed from ancestor count in the bridge.
- md2spans recognizes Markdown blockquote syntax (`> ` and
  nested `>> `, `>>> `, ...).
- Renderer: zero changes. Existing `Style.Blockquote +
  Style.BlockquoteDepth` already drives indent
  (`rich/layout.go:614`) and bar painting
  (`rich/mdrender/blockquote.go`).
- Bridge: `applyEnclosingRegions` gains a `blockquote` case
  that increments `BlockquoteDepth` per ancestor (counting,
  not OR-ing). Round 5's outermost-first walk order makes
  this composes correctly when stacked.
- Bridge: `buildStyledContent` SPLITS each spanStore run at
  region boundaries (new `RegionStore.BoundariesIn` helper)
  before calling `applyEnclosingRegions`. This reverses
  round 5's "producer-responsibility" stance: previously
  producers had to emit run boundaries aligned with region
  boundaries, and md2spans's `code` regions satisfied this
  naturally because the inside style differed (`family=code`
  prevents spanStore coalescing). Round 6's `blockquote`
  regions cover default-styled runs, which DO coalesce
  across the boundary — so the bridge has to split. The
  reversal keeps producers simpler and prevents an entire
  class of "I forgot to split my run at the boundary" bugs
  in future region-emitting features.

## Design principles inherited from rounds 1-5

Round 6 inherits the protocol-shape principles from round 5
without modification:
- **Protocol expresses intent, not pixel placement.**
  `begin region blockquote` says "this is a blockquote";
  the renderer's existing layout decides where to indent
  and where to draw the bar.
- **Namespaced kind values.** `blockquote` joins `code` in
  v1's recognized kind set. Round 7 (`listitem`) and round 8
  (`table`) extend further.
- **Depth is computed, not declared.** The protocol does not
  carry a `depth=N` param on `begin region blockquote`. The
  consumer counts blockquote ancestors during the bridge
  walk. Producers that emit two nested `begin region
  blockquote` automatically produce depth=2 on the inner
  region's runes.

## Wire-format change

No new directives — round 6 only extends the kind vocabulary:

```
begin region blockquote
... content ...
end region
```

Nested example:
```
begin region blockquote
s 0 9 -                              ; "> level 1\n"
begin region blockquote
s 9 11 -                             ; ">> level 2\n"
end region
s 20 9 -                             ; "> level 1\n"
end region
```

`validRegionKinds` (in `spanparse.go`) extends from
`{"code"}` to `{"code", "blockquote"}`. No other parser
changes; the existing balanced-begin/end-per-Twrite rule
covers nesting.

## Region scope (decision: full coverage)

A blockquote region COVERS the entire blockquote source,
including the `>` markers at the start of each line.
Rationale:
- **Consistent with markup-stays-visible** (rounds 1-5).
  The `>` markers render as ordinary default-styled text.
- **Cleaner indent semantics.** With the markers inside the
  region, the indent shifts the whole line right; the bar
  appears in the gutter to the LEFT of the markers. Without
  this, the markers would render at column 0 and the body
  would shift, producing visually inconsistent left edges.
- **Simpler bridge.** Per-rune lookup
  (`regionStore.EnclosingAt`) finds the same blockquote
  region for the marker rune AND the content rune; no
  special-casing.

For nested `>>` blockquotes, BOTH `>` markers are inside
their respective regions. The deepest region is the inner
blockquote; the bridge counts both ancestors as it walks
outermost→deepest, producing `BlockquoteDepth=2` on the
inner region's runes.

## `applyEnclosingRegions` change (bridge)

The round-5 helper currently has one case:

```go
case "code":
    s.Block = true
    s.Code = true
    s.Bg = rich.InlineCodeBg
```

Round 6 adds:

```go
case "blockquote":
    s.Blockquote = true
    s.BlockquoteDepth++
```

The increment composes correctly with the round-5
outermost-first walk: an outer blockquote bumps
`BlockquoteDepth` to 1, then a nested inner blockquote
visits AFTER and bumps to 2. A code block nested inside a
blockquote sets `Block + Code + Bg(InlineCodeBg)` LAST
(since code is the deepest), AND the blockquote ancestor
contributed `Blockquote + BlockquoteDepth=1`. Both effects
visible.

Tests should pin:
- Single blockquote → `Blockquote=true, BlockquoteDepth=1`.
- Two nested → `BlockquoteDepth=2`.
- Three nested → `BlockquoteDepth=3`.
- Code inside blockquote → both flag sets composed.

## md2spans: blockquote detection

CommonMark blockquote syntax: a line beginning with `>`
(with optional space after) is a blockquote line. Multiple
consecutive blockquote lines form one blockquote. The first
non-blockquote, non-blank line ends the blockquote. Nesting
is by leading `>` count: `>> ` is depth 2, `>>> ` is depth 3,
etc.

### Detection rules (md2spans v1 round 6)

A line is a blockquote line if its leading non-whitespace
content starts with `>`. The depth is the count of `>`
characters encountered in succession at the start, with
optional intervening single spaces (CommonMark allows
`> > nested` as depth 2 spaced).

For v1 of round 6:
- Recognize `>` runs at the very start of the line (no
  leading whitespace).
- Recognize spaced form (`> > `) as depth-2.
- Optional: a single space after each `>` is conventional
  but not required.

Negative cases (NOT blockquotes):
- Lines starting with `>` after non-whitespace text
  (e.g., `text > arrow`).
- HTML-style `>` characters mid-line.

### scanParagraphs extension

The round-5 `scanParagraphs` function gains blockquote
state tracking. New `paragraphRange` fields:

```go
type paragraphRange struct {
    // ... existing ...
    IsBlockquote     bool
    BlockquoteDepth  int  // 1, 2, 3 ...
    // CodeBodyRuneStart / CodeBodyRuneEnd used by code blocks;
    // for blockquote, the body covers the WHOLE paragraph
    // (markers included), so we use ByteStart/ByteEnd /
    // RuneStart as before.
}
```

Or simpler: a separate `blockquoteRanges []blockquoteRange`
emitted alongside paragraphs (same shape as paragraphs but
representing the blockquote group, with depth and
contained-paragraph indices). Final shape settled at row
3.6.6 design.

The blockquote group spans MULTIPLE paragraph-like
sub-units: a nested code block, headings inside a quote,
multiple paragraphs of quoted text. The grouping is
hierarchical — rounds 6 and 7 both produce hierarchical
output (nested blockquotes in 6, lists with items in 7), so
the data shape needs to support nesting from this round on.

### parseBlockquoteRange

A new emit path. For each blockquote group, md2spans:
1. Emits `begin region blockquote` at the group's RuneStart.
2. For each contained line/paragraph, emits the styled
   spans from the existing parsers (parseParagraph,
   parseHeadingParagraph, parseCodeBlockParagraph,
   parseHRuleParagraph) — recursively, since a code block
   inside a blockquote produces another `begin region
   code` nested inside.
3. For nested blockquote depth changes: emit additional
   `begin region blockquote` / `end region` pairs at the
   appropriate offsets.
4. Emits `end region` at the group's RuneEnd.

Round 6's md2spans recurses into existing parsers to handle
content inside blockquotes — this is an architectural
inflection: the parser becomes structurally recursive on
regions.

### Span shape (no new fields)

Round 6 reuses the round-5 `Span.RegionBegin` /
`Span.RegionEnd` / `Span.RegionParams` sentinels. The kind
is just a different string value (`"blockquote"`). No new
fields added to `Span`. The arch-review-flagged 3-mode
discriminator concern doesn't grow here.

## Wire-format examples

### Single-line blockquote
Source: `> a quote\n`
```
begin region blockquote
s 0 10 -
end region
```

### Two-line blockquote
Source: `> line one\n> line two\n`
```
begin region blockquote
s 0 22 -
end region
```

### Nested blockquote
Source:
```
> outer
>> inner
> outer again
```

Wire (rune offsets):
```
begin region blockquote
s 0 8 -                      ; "> outer\n"
begin region blockquote
s 8 9 -                      ; ">> inner\n"
end region
s 17 13 -                    ; "> outer again"
end region
```

The deepest region for the inner `>>` line is the inner
blockquote; the bridge walks outer→inner and produces
`BlockquoteDepth=2` on those runes.

### Blockquote containing fenced code
Source:
```
> ```go
> fmt.Println()
> ```
```

Wire:
```
begin region blockquote
s 0 7 -                      ; "> ```go\n"
begin region code lang=go
s 7 17 - family=code         ; "> fmt.Println()\n"
end region
s 24 5 -                     ; "> ```\n"
end region
```

The code region's body covers `> fmt.Println()\n` — including
the `>` marker, since the blockquote ALSO covers it. Both
regions claim those runes; the bridge walks blockquote
(setting depth=1) then code (setting Block, Code, Bg) — both
applied. The renderer indents AND draws code-block treatment
AND draws the blockquote bar in the gutter. Visual: a code
block inside a blockquote.

## Failure modes

| Case | Behavior |
|---|---|
| Unmatched `>` mid-line (`text > arrow`) | Not a blockquote; render as default text |
| Tab-indented blockquote (`\t> quote`) | v1: not recognized (CommonMark allows up to 3 leading spaces; v1 accepts none; document as limitation) |
| Spaced nesting (`> > a`) | Depth 2, both `>` markers visible |
| Blockquote containing heading | `> # h1` — emitted as nested heading inside blockquote region |
| Blockquote containing fenced code | Nested regions, both apply (see example above) |
| Blockquote at end of document with no trailing blank | Region closes at EOF (matches CommonMark) |
| Two consecutive blockquote groups separated by blank | Two SEPARATE top-level regions |

## Test plan

1. **`spanparse.go` tests**: `begin region blockquote` parses
   to Region{Kind: "blockquote"}; nested begin/end pair
   produces two regions in the flat list (outer first,
   inner second per parser ordering).
2. **`region.go` tests**: existing tests cover the storage
   layer; round 6 adds no new operations. Add a synthetic
   "blockquote-inside-blockquote" Add test (already partly
   covered by `TestRegionStore_AddDeeplyNested`).
3. **`applyEnclosingRegions` tests** (`wind_styled_test.go`):
   single blockquote → BlockquoteDepth=1; nested two →
   depth=2; nested three → depth=3; code inside blockquote
   → both flag sets present.
4. **`buildStyledContent` tests**: a region of kind
   blockquote over body runes produces spans with
   Style.Blockquote=true and the right depth.
5. **md2spans parser tests**: single-line `>` blockquote;
   multi-line group; nested `>>`; mixed content (heading
   inside quote, fenced code inside quote); negative
   cases (mid-line `>`, tab-indented).
6. **md2spans emit tests**: round-trip a single blockquote
   through Parse + FormatSpans; nested blockquote produces
   correctly-ordered begin/end pairs.
7. **End-to-end smoke**: open a markdown file with
   blockquotes (single, nested, with embedded code) and
   verify visual parity with the in-tree path (indent +
   bar at appropriate depth).

## Non-goals

- **Lazy continuation** (CommonMark allows blockquote
  paragraphs to continue without `>` on the next line):
  v1 of round 6 requires `>` on every line. Document as
  a v1 limitation.
- **Blockquote with leading whitespace before `>`**:
  CommonMark allows up to 3 leading spaces; v1 requires
  `>` at column 0.
- **Setext headings inside blockquote**: deferred (the
  in-tree path supports this; v1 of round 6 doesn't).
- **Emit-time depth=N param** on the wire: depth is
  computed from ancestor count, not declared. Producers
  that want explicit depth can emit it as `depth=N` in
  Params, but the bridge ignores it (the count is
  authoritative).

## Risks

1. **Recursive md2spans parser**: round 6 changes the
   md2spans parser's structure from flat (one paragraph
   handler per kind) to recursive (a blockquote
   contains paragraphs, which themselves may be code
   blocks or headings). Risk of state-management bugs
   in scanParagraphs. Mitigation: extensive test
   fixtures with nested content kinds.

2. **Bridge depth-counting verifies the round-5
   walk-order fix**: round 5's review fixed the
   ancestor-walk iteration order; round 6's
   `BlockquoteDepth++` per ancestor is the first
   non-idempotent kind to use it. If round 5's fix is
   wrong, round 6 tests will catch it. Acceptable
   coupling.

3. **`Span` 3-mode discriminator concern (deferred)**:
   round 5 review flagged that `Span` becomes
   unmanageable as a discriminator-without-discriminator.
   Round 6 doesn't add a new mode; reuses the existing
   RegionBegin/RegionEnd sentinels with a different
   kind value. The discriminator concern is real but
   can wait until round 7 lands the 4th mode (per-item
   list regions).

4. **Layout's ListIndentWidth coupling**: blockquote
   indent uses `BlockquoteDepth * ListIndentWidth`
   (`rich/layout.go:615`). Round 7's lists ALSO use
   `ListIndentWidth` for their indent. If a list item is
   nested inside a blockquote (round 7+), the indents
   compose. Document as a sequencing concern; round 7
   will verify.

5. **Code-inside-blockquote rendering parity with
   in-tree**: the in-tree markdown path renders nested
   code+blockquote with specific behavior (likely
   indented code + bg + bar). Round 6 must produce the
   same visual; bridge composing both kinds correctly is
   the test.

## Status

Design — drafted. Awaiting review.
