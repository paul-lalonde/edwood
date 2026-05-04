# Phase 3 Round 6.5 — Prep for Lists — Plan

Pure refactor round. Three structural changes that round 6's
arch review identified as "needed before round 7". No new
wire-format additions, no new region kinds.

**Base design**: [`docs/designs/features/phase3-r6.5-prep-for-lists.md`](../designs/features/phase3-r6.5-prep-for-lists.md).

**Branch**: `phase3-r6.5-prep-for-lists`.

**Outcome**: Round 7 (lists) can be drafted against a Span /
bridge / parser API that doesn't have known-stale shapes.

**Files touched**:
- `cmd/md2spans/parser.go` — `SpanKind` enum + field on
  `Span`; emit sites set Kind; generalized
  `kindsAnchorAtLineStart` registry replaces hardcoded
  blockquote check.
- `cmd/md2spans/emit.go` — `FormatSpans` switches on Kind
  instead of field-presence; `isDefaultFill` gates on
  `Kind == SpanStyled`.
- `cmd/md2spans/parser_test.go` / `emit_test.go` — assert
  Kind on relevant spans; pin "code begin not snapped" as a
  negative invariant.
- `wind.go` — `applyEnclosingRegions` split into per-kind
  apply functions; `ancestorsOuterFirst` helper extracted.
- `wind_styled_test.go` — same cases, byte-equal expected
  output (sanity check that the refactor is behavior-
  preserving).

No protocol-spec change; the Kind field is in-memory only.

---

## Phase 3.6.5.0: Plan + design

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Drafted | `docs/designs/features/phase3-r6.5-prep-for-lists.md` | Awaiting review. |
| [x] Tests | n/a (planning) | — | — |
| [x] Iterate | This plan + design | — | This file. |
| [x] Commit | — | — | `Add Phase 3 round 6.5 design and plan: prep for lists` |

## Phase 3.6.5.1: `Span.Kind` discriminator

The structural change with the most call-site touches.
Mechanical migration; tests catch any miss.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | `SpanKind` int enum with constants `SpanStyled`, `SpanBox`, `SpanRegionBegin`, `SpanRegionEnd`. Field added to `Span`. Zero value is `SpanStyled`, matching today's "no special fields" default. | base doc § "1. Span.Kind discriminator" | Mutually-exclusive with the IsBox / RegionBegin / RegionEnd discriminators that remain (kept for the wire format mapping). |
| [ ] Tests | New: `TestSpan_KindRoundTripStyled`, `TestSpan_KindRoundTripBox`, `TestSpan_KindRoundTripRegionBegin`, `TestSpan_KindRoundTripRegionEnd`. Existing test fixtures get `Kind: SpanXxx` added on construction. `FormatSpans` byte-output is unchanged (pinned via existing format tests). | `cmd/md2spans/parser_test.go`, `cmd/md2spans/emit_test.go` | — |
| [ ] Iterate | Add `SpanKind` and constants. Add `Kind` field. Update every emit site (parseBlockquoteRange, parseCodeBlockParagraph, tryImage, region-end sites). Update `FormatSpans`'s `for _, s := range input` split to switch on Kind. Update `isDefaultFill`. | `cmd/md2spans/parser.go`, `cmd/md2spans/emit.go` | — |
| [ ] Commit | — | — | `md2spans: add Span.Kind discriminator (refactor; no wire change)` |

## Phase 3.6.5.2: Generalize line-anchor helper

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Introduce `kindsAnchorAtLineStart map[string]bool` containing `"blockquote": true`. Replace the `if s.RegionBegin == "blockquote"` branch in `parseBlockquoteRange` with `if kindsAnchorAtLineStart[s.RegionBegin]`. | base doc § "2. Generalize snapToLineStart" | One-line registry; one-line conditional change. |
| [ ] Tests | Existing `TestParseBlockquoteNestedInnerBeginAtLineStart` still passes. New: `TestParseCodeBlockBeginNotSnapped` (negative invariant — a `code` region begin offset inside a blockquote is NOT snapped to line start; it remains at the body-start position after the fence's `\n`). | `cmd/md2spans/parser_test.go` | — |
| [ ] Iterate | Add the registry; update the conditional. | `cmd/md2spans/parser.go` | — |
| [ ] Commit | — | — | `md2spans: generalize line-start anchoring via kind registry` |

## Phase 3.6.5.3: `applyEnclosingRegions` per-kind apply functions

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Extract per-kind logic into `applyCodeRegion(s, r)` and `applyBlockquoteRegion(s, r)`. Extract chain-build into `ancestorsOuterFirst(deepest) []*Region`. The main switch dispatches by kind. Behavior is byte-equal to round 6. | base doc § "3. applyEnclosingRegions composition" | Refactor only. |
| [ ] Tests | Existing `wind_styled_test.go` tests pass byte-equal: single code, single blockquote, nested blockquote, code-inside-blockquote, three-level nested blockquote. | `wind_styled_test.go` | — |
| [ ] Iterate | Three small helper functions; the central function shrinks to the dispatch switch. | `wind.go` | — |
| [ ] Commit | — | — | `wind: split applyEnclosingRegions into per-kind apply functions` |

## Phase 3.6.5.4: Smoke test + merge prep

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | n/a (validation) | — | — |
| [ ] Tests | All packages green | `go test ./...` | — |
| [ ] Iterate | Build binaries; smoke-test in real edwood with the round 6 markdown sample (single + nested blockquotes + code inside blockquote). Verify visual parity is preserved (no regression from the refactor). | — | User-driven. |
| [ ] Commit | — | — | n/a (no code change unless smoke surfaces something). |

---

## After this round

Round 7 (lists) can:
- Add `case SpanRegionBegin` for listitem to FormatSpans
  without touching field-presence checks.
- Add `"listitem": true` to `kindsAnchorAtLineStart`.
- Add `applyListitemRegion(s, r)` beside the existing two,
  with whatever composition rule (nearest-of-kind, payload
  extraction) listitem actually needs.

## Risks

(See base design doc.) The Span.Kind migration is the
biggest churn; the other two are ~one-line each. All three
are pure refactors with byte-equal expected behavior.

## Status

Plan + design drafted. Awaiting review before any code.
