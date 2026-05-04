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
| [ ] Commit | — | — | `Add Phase 3 round 7 design and plan: list per-item regions` |

## Phase 3.7.1: Parser — accept `listitem` as a region kind

Smallest change first. The protocol parser accepts the new
kind; tests pin it before any md2spans producer emits it.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Extend `validRegionKinds` in `spanparse.go` to include `listitem: true`. | base doc § "Wire format" | One-line change. |
| [ ] Tests | Add `TestParseSpan_RegionListitemMarker` and `TestParseSpan_RegionListitemNumber` in `spanparse_test.go`: parse the directive lines, assert `Region{Kind: "listitem", Params: {"marker": "-"}}` and `{Params: {"number": "3"}}`. | `spanparse_test.go` | — |
| [ ] Iterate | Add the kind. | `spanparse.go` | — |
| [ ] Commit | — | — | `spans: accept listitem as a region kind` |

## Phase 3.7.2: Bridge — `applyEnclosingRegions` handles listitem

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Add `case "listitem": applyListitemRegion(s, r)` to the dispatch. New `applyListitemRegion` sets `s.ListItem=true`, increments `s.ListIndent`, extracts `marker=` or `number=` from `r.Params` (number= sets `ListOrdered=true` and `ListNumber=N`). | base doc § "applyEnclosingRegions change" | — |
| [ ] Tests | New tests in `wind_styled_test.go`: single listitem with `marker=-` → `ListItem=true, ListIndent=1, ListOrdered=false`. Single with `number=3` → `ListItem=true, ListIndent=1, ListOrdered=true, ListNumber=3`. Listitem inside blockquote → both flag sets compose. | `wind_styled_test.go` | — |
| [ ] Iterate | New apply function + dispatch case. | `wind.go` | — |
| [ ] Commit | — | — | `wind: applyEnclosingRegions handles listitem (bridge)` |

## Phase 3.7.3: md2spans — list-line detection

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | scanParagraphs detects a list line: column-0 starts with one of `- `, `* `, `+ `, `N. `, `N) `. A run of consecutive list lines forms a list group; each line is its own `paragraphRange{IsListItem: true, ListMarker: c, ListNumber: n}`. Existing HRule / heading / fence / blockquote checks take precedence (HRule `---` ≠ bullet `- foo`). | base doc § "md2spans: list detection" | The hardest md2spans change so far is blockquote (recursive); list is per-line which is simpler. |
| [ ] Tests | Pass-through tests at this row only assert `paragraphRange` shape: each list line produces one IsListItem range. Negative: `-foo` (no space), `*x*` (emphasis, not list), `--- ` (HRule). Single ordered (`1. foo`), single unordered (`- foo`), mix consecutively. | `cmd/md2spans/parser_test.go` (probe via Parse output for now) | — |
| [ ] Iterate | Add detection helpers (`isListLine`, etc.); extend paragraphRange struct; extend scanParagraphs's flushLine. | `cmd/md2spans/parser.go` | — |
| [ ] Commit | — | — | `md2spans: detect column-0 list items` |

## Phase 3.7.4: md2spans — emit listitem region directives

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | `parseListItemParagraph` emits `begin region listitem marker=X` (or `number=N`) at the line's start, parseInlineSpans on the line's content for emphasis/links, `end region` at line's `\n`+1. Parse() switch dispatches `IsListItem` paragraphs to it. | base doc § "parseListItemParagraph" | — |
| [ ] Tests | Single `- foo` → 2 spans (begin, end). Single `1. foo` → 2 spans with number. Multi-line `- a\n- b\n- c` → 6 spans (3 pairs). Mix `- a\n1. b` → 4 spans. List inside blockquote `> - foo` → blockquote begin + listitem begin + listitem end + blockquote end. List with emphasis content `- *x*` → begin + italic span + end. List terminated by blank line. List terminated by non-list line. | `cmd/md2spans/parser_test.go` | — |
| [ ] Iterate | parseListItemParagraph function; scanParagraphs emits paragraphRanges; Parse dispatch case. | `cmd/md2spans/parser.go` | — |
| [ ] Commit | — | — | `md2spans: emit listitem region directives` |

## Phase 3.7.5: Spec + README

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | n/a (doc) | — | — |
| [ ] Tests | n/a (doc) | — | — |
| [ ] Iterate | spans-protocol.md adds `listitem` to v1-recognized kinds; documents `marker=` / `number=` params; examples (single bullet, single ordered, list-in-blockquote). md2spans README v1 scope flips Lists to ✓ with caveats (column-0, single-line, no nesting). Phase 3 roadmap entry for round 7 flips to ✓ landed. | — | — |
| [ ] Commit | — | — | `docs: spans protocol gains listitem region kind; md2spans handles single-line lists` |

## Phase 3.7.6: Smoke test + merge prep

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | n/a (validation) | — | — |
| [ ] Tests | All packages green | `go test ./...` | — |
| [ ] Iterate | Build binaries; smoke-test in real edwood with a markdown containing a bullet list, an ordered list, mixed, and a list inside a blockquote. Verify visual indent + bullet position match the in-tree path's rendering. | — | User-driven. |
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

Plan + design drafted. Awaiting review before any code.
