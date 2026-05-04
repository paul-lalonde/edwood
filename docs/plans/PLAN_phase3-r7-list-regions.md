# Phase 3 Round 7 — Lists (Per-Item Regions) — Plan

Third region kind. v1 covers column-0 single-line list
items only (bullet `-`, `*`, `+` and ordered `N.`, `N)`).
Nesting and multi-line continuations deferred to a
follow-up sub-round (7.x).

**Base design**: [`docs/designs/features/phase3-r7-list-regions.md`](../designs/features/phase3-r7-list-regions.md).

**Branch**: `phase3-r7-list-regions`.

**Outcome**: edwood renders Markdown bullet and ordered
lists when md2spans is the renderer. Each item is its own
region. Lists inside blockquotes work via the existing
recursive parse path.

**Files touched** (full list in the design doc):
- `spanparse.go`, `wind.go`, `cmd/md2spans/parser.go` +
  tests, protocol spec + README.

No `rich/` changes — existing `Style.ListItem`,
`Style.ListIndent`, `Style.ListOrdered`, `Style.ListNumber`
fields drive the layout.

---

## Phase 3.7.0: Plan + design

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | phase3-r7-list-regions.md drafted | [base doc] | Decisions: (a) v1 column-0 single-line items only; (b) `marker=X` or `number=N` on the wire — exactly one; (c) listitem region covers the whole item line; (d) bridge applyListitemRegion per-call overwrite gives nearest-of-kind for marker/number; (e) no rich/ changes. |
| [x] Tests | n/a (planning) | — | — |
| [x] Iterate | This plan + design | — | This file. |
| [x] Commit | — | — | `Add Phase 3 round 7 design and plan: list per-item regions` |

## Phase 3.7.1: Parser — accept `listitem` as a region kind

Smallest change first. The protocol parser accepts the new
kind; tests pin it before any md2spans producer emits it.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Extend `validRegionKinds` in `spanparse.go` to include `listitem: true`. | base doc § "Wire format" | One-line change. |
| [x] Tests | Added `TestParseSpanMessageRegionListitemMarker` and `TestParseSpanMessageRegionListitemNumber` in `spanparse_test.go`. Pre-existing TestParseSpanMessageRegionUnknownKind updated (listitem case removed since it's now valid). | `spanparse_test.go` | — |
| [x] Iterate | Added "listitem": true to validRegionKinds. | `spanparse.go` | — |
| [x] Commit | — | — | `spans: accept listitem as a region kind` |

## Phase 3.7.2: Bridge — `applyEnclosingRegions` handles listitem

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Add `case "listitem": applyListitemRegion(s, r)` to the dispatch. New `applyListitemRegion` sets `s.ListItem=true`, increments `s.ListIndent`, extracts `marker=` or `number=` from `r.Params` (number= sets `ListOrdered=true` and `ListNumber=N`). | base doc § "applyEnclosingRegions change" | — |
| [x] Tests | Added TestBuildStyledContent_RunInsideListitemRegion{Unordered,Ordered} and TestBuildStyledContent_ListitemInsideBlockquote (cross-kind composition). | `wind_styled_test.go` | — |
| [x] Iterate | applyListitemRegion + parseListNumber added. Switch dispatch updated. | `wind.go` | — |
| [x] Commit | — | — | `wind: applyEnclosingRegions handles listitem (bridge)` |

## Phase 3.7.3-4: md2spans — list-line detection + region emission

Combined into one commit. The plan's split into two rows
would have produced an intermediate state with no useful
tests (detection without emission emits no observable
regions); landing them together gives a single tested
red→green cycle.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | scanParagraphs detects column-0 `- `, `* `, `+ `, `N. `, `N) `. Each list line is its own `paragraphRange{IsListItem, ListMarker, ListNumber, ListContentRuneStart}`. Detection runs AFTER existing HRule check so `---` continues to parse as HRule. parseListItemParagraph emits begin/inline-spans/end. | base doc § "md2spans: list detection" + "parseListItemParagraph" | — |
| [x] Tests | TestParseListItem{UnorderedDash,UnorderedAsteriskAndPlus,Ordered,Multiple,MixedOrderedUnordered,NotEmphasis,NotHRule,RequiresSpace,TerminatedByBlankLine,InsideBlockquote,EmphasisInContent}. Pre-existing TestParseHRuleNotAList updated to assert "no HRule emitted" rather than "no spans" (round 7 now emits a listitem region for `- item`). | `cmd/md2spans/parser_test.go` | — |
| [x] Iterate | isListLine helper; paragraphRange extended with IsListItem/ListMarker/ListNumber/ListContentRuneStart; scanParagraphs.flushLine; parseListItemParagraph; contentBytePos; strconv import; Parse switch dispatch. | `cmd/md2spans/parser.go` | — |
| [x] Commit | — | — | `md2spans: detect and emit column-0 list items` |

## Phase 3.7.5: Spec + README

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | n/a (doc) | — | — |
| [x] Tests | n/a (doc) | — | — |
| [x] Iterate | spans-protocol.md: validRegionKinds extended; `marker=` / `number=` params documented; new "Listitem depth" subsection mirroring round 6's blockquote depth; examples added (single bullet, ordered, list-in-blockquote). md2spans README: Lists row flipped to ✓ with caveats. | — | — |
| [x] Commit | — | — | `docs: spans protocol gains listitem region kind; md2spans handles single-line lists` |

## Phase 3.7.6: Smoke test + merge prep

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | n/a (validation) | — | — |
| [x] Tests | All packages green | `go test ./...` | Green. |
| [ ] Iterate | Build binaries; smoke-test in real edwood with a markdown containing a bullet list, an ordered list, mixed, and a list inside a blockquote. Verify visual indent + bullet position match the in-tree path's rendering. | — | Binaries rebuilt at /Users/paul/dev/edwood/{md2spans,edwood}; awaiting user smoke. |
| [ ] Commit | — | — | n/a (no code change unless smoke surfaces something). |

---

## After this round

Round 7 v1 establishes the third region kind and the
`marker=` / `number=` per-region payload pattern. Round
7.x extends to nested lists and continuation lines. Round
8 (tables) is the most complex region kind, with cell
sub-regions and frame-dimension introspection.

The arch-review-flagged Span 3-mode discriminator concern
is RESOLVED by round 6.5's `SpanKind`; round 7 doesn't
need a 4th mode.

## Risks

(See base design doc.) Main concerns: HRule-vs-bullet
precedence, asterisk ambiguity (emphasis vs list),
md2spans state-machine complexity in scanParagraphs.

## Status

All rows complete. Awaiting smoke confirmation before
merging to master.
