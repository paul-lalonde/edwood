# Phase 3 Round 6.5 — Prep for Lists (Round 7)

## Why

Round 6's architecture review identified three structural
limits that round 7 (lists) would hit head-on. Doing them
inside round 7 would tangle a refactor with a feature; round
6.5 lands the refactors first so round 7 is purely additive.

The three items, each with the round-6 evidence:

1. **`Span` 3-mode discriminator deferral is overdue.** Today
   a `Span` has three implicit modes — styled, box (round 4),
   and region directive (round 5/6) — discriminated by which
   fields are non-zero. Round 7 will add a 4th mode (per-item
   indent + bullet/number marker) and probably a 5th. The
   round-5 arch review flagged this; round 6 didn't fix it
   ("wait for the 4th mode"). Round 7 IS the 4th mode.

2. **`snapToLineStart` is blockquote-specific.** The recursive
   md2spans path uses a snap-to-line-start step for nested
   blockquote begin directives, gated by
   `if s.RegionBegin == "blockquote"`. Round 7's listitem
   regions need the same anchoring (a list item begins at
   column 0 on its source line, not at the marker rune).
   Without generalization, round 7 copies the branch.

3. **`applyEnclosingRegions` assumes additive composition.**
   The current switch has cases that either set a flag
   (`code`) or increment a counter (`blockquote`) — both
   commute with ancestor-walk order. Listitem CAN'T compose
   that way: a sub-region's `marker=•` and `number=3` come
   from the deepest-listitem-of-kind, not from a counter
   over all ancestors. The current single switch admits the
   listitem case, but barely; restructuring into per-kind
   apply functions makes the per-kind composition strategy
   explicit.

Round 6.5 is a pure refactor — no behavior change, no new
wire-format additions, no new wire-format kinds. Tests at
every layer continue to pass byte-exactly except where
they're updated for new field names.

**Branch**: `phase3-r6.5-prep-for-lists`.

**Outcome**: Round 7's list design + plan can be drafted
against a Span / bridge / parser API that doesn't have
known-stale shapes. The three changes are small enough to
review in one pass each.

---

## 1. `Span.Kind` discriminator

### Today

`Span` (in `cmd/md2spans/parser.go` and the parallel
`StyleAttrs` / parsed-span shapes in edwood) carries every
field for every mode and discriminates implicitly:

| Mode             | Discriminator                                |
|------------------|----------------------------------------------|
| Styled           | not box, not region                          |
| Box              | `IsBox == true`                              |
| Region begin     | `RegionBegin != ""`                          |
| Region end       | `RegionEnd == true`                          |

Mutually-exclusive in practice but enforced only by callers.
Tests rely on this implicit shape. `FormatSpans` already
splits on it (`if s.RegionBegin != "" || s.RegionEnd { ... }`).

### Target

Add an explicit `Kind` field with a small `int` enum:

```go
type SpanKind int

const (
    SpanStyled SpanKind = iota
    SpanBox
    SpanRegionBegin
    SpanRegionEnd
)

type Span struct {
    Kind SpanKind
    // ... other fields unchanged
}
```

Constructors are NOT added. The existing field-driven
construction still works; the migration is mechanical (set
`Kind` at every emit site). `FormatSpans` and any other
discriminator switches by `Kind` instead of field-presence.

### Why a kind enum, not a typed sum

Go's typed sum (interface + per-variant struct) costs
verbosity at every call site (type switch on read, allocate
on write) and gains nothing the enum doesn't: per-mode field
isolation isn't actually wanted — many fields are SHARED
across modes (`Offset`, `Length`, region flags can layer onto
boxes in future, etc.). The enum is the minimal thing that
makes the modes explicit without committing to a different
data shape.

### Wire format

Unchanged. The enum is an in-memory discriminator only.

### Migration

- Add `SpanKind` and constants. Add the field. Default
  zero-value is `SpanStyled`, matching the existing
  field-empty default.
- Update every emit site that produces a non-Styled span:
  - `parseBlockquoteRange` (region begin/end) → set `Kind`.
  - `parseCodeBlockParagraph` (begin/end) → set `Kind`.
  - `tryImage` (box) → set `Kind = SpanBox`.
- Update `FormatSpans` to switch on `Kind`.
- Update `isDefaultFill` to gate on `Kind == SpanStyled`.
- Tests assert `Kind` on the relevant spans.

### Tests to add

- Round-trip a Styled span: `Kind == SpanStyled`.
- Round-trip a Box span: `Kind == SpanBox`, `IsBox == true`
  (both set; consistent).
- Round-trip a RegionBegin / RegionEnd span: kind matches.
- `FormatSpans` produces byte-equal output before and after
  (fixture-pinned).

---

## 2. Generalize `snapToLineStart`

### Today

`parseBlockquoteRange` in `cmd/md2spans/parser.go`:

```go
for _, s := range Parse(stripped) {
    s.Offset = mapping[s.Offset]
    if s.RegionBegin == "blockquote" {
        s.Offset = snapToLineStart(s.Offset, lineStarts)
    }
    out = append(out, s)
}
```

The conditional is kind-specific. Round 7 will need
`s.RegionBegin == "listitem"` for the same reason: a list
item starts at column 0, not at the bullet rune.

### Target

Replace the kind check with a registry:

```go
// kindsAnchorAtLineStart lists region kinds whose begin
// directive must anchor at the source line's start (not
// the marker rune that opens the construct). Members:
// "blockquote" (round 6), "listitem" (round 7+).
var kindsAnchorAtLineStart = map[string]bool{
    "blockquote": true,
}

// Inside parseBlockquoteRange (and any future analog):
if kindsAnchorAtLineStart[s.RegionBegin] {
    s.Offset = snapToLineStart(s.Offset, lineStarts)
}
```

Round 7 adds `"listitem": true` to the registry — one line.

### Why a map, not an interface or method

The set is closed (we own all the kinds), small (≤ 4 in v1),
and accessed at one site. A map is the cheapest way to make
the per-kind decision explicit without introducing a kind
type.

### Tests to update

- The existing `TestParseBlockquoteNestedInnerBeginAtLineStart`
  still passes (blockquote is still in the set).
- New test: a `code` region's begin offset is NOT snapped
  (negative invariant — code's begin sits at the body
  start, after the fence's `\n`). This pins the blockquote
  vs. code distinction explicitly so a future "snap
  everything" change fails loudly.

---

## 3. `applyEnclosingRegions` composition

### Today

In `wind.go`:

```go
for i := len(chain) - 1; i >= 0; i-- {
    switch chain[i].Kind {
    case "code":
        s.Block = true
        s.Code = true
        s.Bg = rich.InlineCodeBg
    case "blockquote":
        s.Blockquote = true
        s.BlockquoteDepth++
    }
}
```

Both cases compose by walking outermost-first and applying
each ancestor's effect. Code is idempotent (set flags);
blockquote is additive (increment counter). Both are
ancestor-chain reductions over the chain.

Round 7's listitem doesn't fit. Listitem carries per-region
payload — a marker character and an item number — that
varies per region instance, not per ancestor count. The
correct per-rune effect is "the FIRST listitem ancestor
encountered (deepest, since we walk outermost-first then
last write wins) sets `s.ListMarker` and `s.ListNumber`".
That's a different composition strategy: nearest-of-kind,
not reduce-over-chain.

### Target

Split the switch into per-kind apply functions, each of
which knows its composition rule for the chain:

```go
// applyEnclosingRegions walks the ancestor chain and lets
// each kind contribute its per-rune effect. Each kind
// declares its own composition rule via its apply
// function:
//   - applyCodeRegion: idempotent (sets flags; multiple
//     code ancestors produce the same result).
//   - applyBlockquoteRegion: additive (increments depth;
//     called once per blockquote ancestor).
// Round 7 will add applyListitemRegion: nearest-of-kind
// (the deepest listitem ancestor's marker/number wins).
func applyEnclosingRegions(s *rich.Style, deepest *Region) {
    if deepest == nil {
        return
    }
    chain := ancestorsOuterFirst(deepest)
    for _, r := range chain {
        switch r.Kind {
        case "code":
            applyCodeRegion(s, r)
        case "blockquote":
            applyBlockquoteRegion(s, r)
        }
    }
}

func applyCodeRegion(s *rich.Style, _ *Region) {
    s.Block = true
    s.Code = true
    s.Bg = rich.InlineCodeBg
}

func applyBlockquoteRegion(s *rich.Style, _ *Region) {
    s.Blockquote = true
    s.BlockquoteDepth++
}

// ancestorsOuterFirst extracted because round 7 may want
// to walk the chain twice (once for additive kinds, once
// for nearest-of-kind kinds).
```

This is a STRUCTURAL refactor only — the resulting per-rune
flags are byte-equal to round 6. The win is that round 7's
`applyListitemRegion` slots in beside the others, with the
"nearest-of-kind" rule encapsulated in its function instead
of leaking into the central switch.

### Why not commit to a strategy registry now

A registry (`map[string]func(*rich.Style, *Region)`) is the
"obvious next step" but premature: round 7 might want to
walk the chain twice (once for additive, once for
nearest-of-kind kinds), or want kind-specific access to the
chain (not just the current region). Committing to a single
function signature now would lock in a decision we don't
have evidence for. Three named functions in a switch is
mechanically equivalent and easier to evolve.

### Tests

- Existing `wind_styled_test.go` cases for code, blockquote,
  nested blockquote, code-inside-blockquote → byte-equal
  output.
- No new tests in 6.5 — round 7 pins listitem behavior.

---

## Non-goals

- Splitting `cmd/md2spans/parser.go` (now 1166 LOC, 2.3× the
  500-LOC budget). Round 6's code review flagged this. It's
  worthwhile but not blocking round 7 — defer to a separate
  cleanup round.
- Adding a `Kind` enum on the wire format. The discriminator
  is in-memory only.
- Adding `listitem` to `validRegionKinds`. That's round 7.
- Changing the `Region` type or `RegionStore` API. Round 6's
  shape is correct for round 7.
- Refactoring `applyEnclosingRegions` to pre-classify
  ancestors by composition rule (additive vs. nearest-of-kind)
  before applying. Round 7's listitem will say whether that's
  needed.

## Risks

- **Span.Kind migration touches many call sites.** Mitigation:
  the field is additive (zero value matches existing
  styled-span default); migration can land row-by-row
  (parser sites, then formatter, then tests).
- **A test using a struct literal with positional fields
  would break silently if Kind is inserted before another
  field.** Mitigation: scan the test code; Go struct
  literals in this repo use named fields throughout, but
  verify.
- **`ancestorsOuterFirst` extraction changes the iteration
  pattern slightly.** Mitigation: keep the existing
  innermost-first chain build + reverse pattern; only the
  loop body becomes a function call.

## Status

Design drafted. Awaiting review before any code.
