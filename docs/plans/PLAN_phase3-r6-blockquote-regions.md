# Phase 3 Round 6 — Blockquote (Nested Regions) — Plan

Second region kind. Adds `blockquote` to the protocol's
v1 region kind set; exercises NESTED regions for the first
time. Validates round-5 region-machinery claims about
kind extension and ancestor-walk composition. The arch
review of round 5 specifically flagged blockquote-with-
depth-counter as the case that would stress the bridge —
round 6 implements the COUNT-by-ancestor strategy.

**Base design**: [`docs/designs/features/phase3-r6-blockquote-regions.md`](../designs/features/phase3-r6-blockquote-regions.md).

**Branch**: `phase3-r6-blockquote-regions`.

**Outcome**: edwood renders Markdown blockquotes (`> `, `>>`,
nested) when md2spans is the renderer, with the existing
in-tree visual: gutter indent + vertical bar per nesting
level. Code blocks inside blockquotes render correctly
(both region kinds compose).

**Files touched**:
- `spanparse.go` — extend `validRegionKinds` with
  `blockquote`.
- `wind.go:applyEnclosingRegions` — add the `blockquote`
  case (Blockquote=true, BlockquoteDepth++).
- `cmd/md2spans/parser.go` — `scanParagraphs` detects `>`
  lines; group adjacent blockquote lines; track depth;
  paragraphRange or sidecar struct gains
  blockquote-grouping fields. Recursive emit:
  parseBlockquoteRange invokes parseParagraph etc. on
  contained sub-paragraphs.
- `docs/designs/spans-protocol.md` — extend
  validRegionKinds documentation; add nested-blockquote
  example.
- `cmd/md2spans/README.md` — flip Blockquote to ✓.
- Tests at every layer.

No rich.Frame / rich/mdrender changes — existing
`Style.Blockquote && BlockquoteDepth` rendering pipeline
(layout indent + paint bars) works unchanged.

---

## Phase 3.6.0: Plan + design

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | phase3-r6-blockquote-regions.md drafted | [base doc] | Decisions: (a) blockquote covers whole source incl. `>` markers; (b) depth computed from ancestors; (c) recursive md2spans parser; (d) no renderer changes; (e) existing Style.Blockquote/BlockquoteDepth fields drive layout + bar via the existing pipeline. |
| [x] Tests | n/a (planning) | — | — |
| [x] Iterate | This plan + design | — | This file. |
| [ ] Commit | — | — | `Add Phase 3 round 6 design and plan: nested blockquote regions` |

## Phase 3.6.1: Parser — accept `blockquote` as a region kind

Smallest change first. The parser already handles
begin/end region directives (round 5); round 6 just adds
`blockquote` to the recognized set.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Extend `validRegionKinds` map to include `blockquote: true`. No other parser changes — existing balanced-begin/end-per-Twrite rule covers nesting unchanged. | base doc § "Wire-format change" | One-line change. |
| [ ] Tests | `begin region blockquote` parses to Region{Kind: "blockquote"}; nested blockquote begin/end pair produces two regions in the flat list | `spanparse_test.go` | — |
| [ ] Iterate | Add the kind to the validRegionKinds map | `spanparse.go` | — |
| [ ] Commit | — | — | `spans: accept blockquote as a region kind` |

## Phase 3.6.2: Bridge — `applyEnclosingRegions` counts blockquote depth

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Add `case "blockquote"` in applyEnclosingRegions: sets `s.Blockquote = true`, increments `s.BlockquoteDepth`. The increment composes with round 5's outermost-first walk: outer ancestor bumps to 1, inner bumps to 2, etc. | base doc § "applyEnclosingRegions change" | The first non-idempotent kind. |
| [ ] Tests | Single blockquote → depth=1; two nested → depth=2; three nested → depth=3; code inside blockquote → both flag sets composed; outer code containing inner blockquote (synthetic; would never happen in markdown) → both apply. | `wind_styled_test.go` | — |
| [ ] Iterate | One case statement | `wind.go:applyEnclosingRegions` | — |
| [ ] Commit | — | — | `wind: applyEnclosingRegions counts blockquote depth from ancestors` |

## Phase 3.6.3: md2spans — blockquote line detection

The first md2spans-side row. Detect `>`-prefixed lines and
group consecutive ones; track nesting depth.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | scanParagraphs recognizes lines starting with `>` (with optional space after); consecutive `>` lines form one blockquote group; depth counted from the longest leading `>` run (or `> > >` spaced form). Output shape for groups: TBD at this row's design step (sidecar slice or extension of paragraphRange). | base doc § "md2spans: blockquote detection" | Most complex md2spans change so far. |
| [ ] Tests | Single `>` line; multiple lines; nested `>>` mid-group; depth changes within a group; negative cases (mid-line `>`, no leading space variant). For now: no recursion into contents — those land in row 3.6.4. | `cmd/md2spans/parser_test.go` | — |
| [ ] Iterate | Add detection + grouping; output the grouping data; defer the contained-content emit to row 3.6.4 | `cmd/md2spans/parser.go` | — |
| [ ] Commit | — | — | `md2spans: detect and group Markdown blockquote lines` |

## Phase 3.6.4: md2spans — recursive emit for blockquote content

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | parseBlockquoteRange emits `begin region blockquote` at group start, recurses through contained sub-paragraphs (which may be plain, headings, fenced code, HRules, or NESTED blockquotes), emits `end region` at group end. Depth changes within a group emit additional begin/end pairs at the appropriate offsets. | base doc § "parseBlockquoteRange" | This is the FIRST recursive parser path in md2spans — flag in code. |
| [ ] Tests | Single-line `> a`; multi-line group; `>>` nested; blockquote containing heading; blockquote containing fenced code (cross-kind nesting); blockquote followed by paragraph | `cmd/md2spans/parser_test.go` | — |
| [ ] Iterate | parseBlockquoteRange function; Parse() switch dispatches based on group kind | `cmd/md2spans/parser.go` | — |
| [ ] Commit | — | — | `md2spans: emit nested begin/end region directives for blockquotes` |

## Phase 3.6.5: Spec + README

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | n/a (doc) | — | — |
| [ ] Tests | n/a (doc) | — | — |
| [ ] Iterate | spans-protocol.md adds `blockquote` to v1-recognized kinds; documents depth-from-nesting; nested example; cross-kind (code-inside-blockquote) example. md2spans README v1 scope flips Blockquote to ✓; notes lazy-continuation deferred. | — | — |
| [ ] Commit | — | — | `docs: spans protocol gains blockquote region kind; md2spans handles nested blockquotes` |

## Phase 3.6.6: Smoke test + merge prep

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | n/a (validation) | — | — |
| [ ] Tests | All packages green | `go test ./...` | — |
| [ ] Iterate | Build binaries; smoke-test in real edwood with a markdown containing single-level blockquotes, nested `>>` blockquotes, and a blockquote containing a fenced code block; verify visual parity with in-tree path. | — | User-driven. |
| [ ] Commit | — | — | n/a (no code change unless smoke surfaces something) |

---

## After this round

Round 6 establishes the second region kind and validates
nesting + cross-kind composition. Round 7 (lists) extends
the kind set further and adds the per-item region pattern
(each list item is its own region). Round 8 (tables) adds
the most complex region kind with cell sub-regions and
frame-dimension introspection.

The arch-review-flagged `Span` 3-mode discriminator concern
remains deferred. Round 7 will likely add a 4th mode (the
per-item indent + bullet/number marker structures don't
fit cleanly into any of {styled, box, region directive}),
which is the natural moment to refactor into a discriminated
type.

## Risks

(See base design doc.) The recursive md2spans parser is the
main concern; the depth-counting bridge is the secondary
one (validates round 5's walk-order fix).

## Status

Plan + design drafted. Awaiting review before any code.
