# Phase 3 Round 7.x — Nested Lists — Plan

Extends round 7 v1 with leading-whitespace-driven
nesting. Multi-line continuation, lazy continuation, and
loose/tight distinctions remain deferred to round 7.y.

**Base design**: [`docs/designs/features/phase3-r7.x-nested-lists.md`](../designs/features/phase3-r7.x-nested-lists.md).

**Branch**: `phase3-r7.x-nested-lists`.

**Outcome**: Markdown like `- a\n  - b` produces a
top-level item containing a nested item, with the inner
item indented by ListIndent = 2 × ListIndentWidth in the
rendered view. Mixed marker types across nesting work.
Nested lists inside blockquotes work.

**Files touched**:
- `cmd/md2spans/parser.go` — leading-whitespace detection,
  list-stack state machine, paragraphRange extension,
  contentBytePos update.
- `cmd/md2spans/parser_test.go` — nested-list tests.
- `docs/designs/spans-protocol.md` — nested listitem
  example.
- `cmd/md2spans/README.md` — Lists row caveat update.

No `wind.go` / `rich/` changes expected — round 7's
ancestor-counting bridge and layout rule should compose
correctly.

---

## Phase 3.7.x.0: Plan + design

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | phase3-r7.x-nested-lists.md drafted | [base doc] | Decisions: (a) v1.1 covers leading-whitespace nesting only; (b) 2 spaces / 1 tab per level matches in-tree; (c) ~~outer item region COVERS its sub-list~~ — REVISED during row 2 to: each item is a sibling region; depth lives in source whitespace, not the wire (see row 2 commit message for the rationale: nested regions double-indent under markup-stays-visible); (d) wire format unchanged from r7. |
| [x] Tests | n/a (planning) | — | — |
| [x] Iterate | This plan + design | — | — |
| [x] Commit | — | — | `Add Phase 3 round 7.x design and plan: nested lists` |

## Phase 3.7.x.1: Detect indent + extend paragraphRange

The scanner-side groundwork. Add leading-whitespace
detection to `isListLine`; extend `paragraphRange` with
`ListDepth`. Don't yet rearrange the emitted regions —
this row makes the data available; row 2 wires it into
the emit.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | `isListLine` accepts an offset within the line and counts leading whitespace as 2-spaces-or-tab per level. Returns indent level alongside marker info. paragraphRange gets `ListDepth int`. `contentBytePos` accounts for leading whitespace. | base doc § "Indent counting" / "contentBytePos" | — |
| [x] Tests | TestIsListLineDepth (15 cases) + TestIsListLineContentByteSkipsLeadingWhitespace. | `cmd/md2spans/parser_test.go` | — |
| [x] Iterate | Updated isListLine signature; extended paragraphRange; updated contentBytePos. | `cmd/md2spans/parser.go` | — |
| [x] Commit | — | — | `md2spans: detect list indent depth; extend paragraphRange` |

## Phase 3.7.x.2: List-stack state machine + nested emission

The structural change. scanParagraphs maintains a list
stack tracking active list levels. Items at deeper depth
are nested INSIDE outer items; outer item regions cover
their sub-lists (Option A from the design doc).

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | First cut used a depth-stack to emit nested begin/end pairs (outer covers sub-list). Analysis on review showed nesting double-indents under markup-stays-visible; revised to SIBLING regions where each item's region covers its own line, source whitespace provides the visual depth. | base doc § "List-stack state machine" + row 2 commit body | — |
| [x] Tests | TestParseNestedListEmitsSiblingsNotNested (precise offsets), TestParseNestedListThreeLevels, TestParseNestedListMixedMarkers, TestParseNestedListItemDepthCarriedThrough, TestParseNestedListBlankLineTerminatesRun. | `cmd/md2spans/parser_test.go` | — |
| [x] Iterate | First commit added parseListRun with nested-stack logic (ca56fe7's predecessor). Second commit reverted to per-item parseListItemParagraph emission after the double-indent analysis. | `cmd/md2spans/parser.go` | — |
| [x] Commit | — | — | `md2spans: emit nested list items as sibling regions, not nested` |

## Phase 3.7.x.3: Layout interactions for deep nesting

Verify round 7's `listitemShifted` layout rule composes
correctly for `ListIndent > 1`. If smoke-tests show
artifacts (e.g., depth-2 nested item inside a depth-1
blockquote), patch the rule.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | n/a (validation) | base doc § "Layout interaction" / "Risk 2" | — |
| [x] Tests | Added TestLayoutNestedListViaSourceWhitespace pinning columns 20 / 40 / 60 for depth 1 / 2 / 3 with sibling-region emission. | `rich/layout_test.go` | — |
| [x] Iterate | No code change needed — sibling-region emission gives ListIndent=1 universally; layout's existing first-box-indent rule plus source whitespace produces the right visual. | — | — |
| [x] Commit | — | — | `rich: pin nested-list layout — source whitespace + ListIndent=1 produce N×Width` |

## Phase 3.7.x.4: Spec + README

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | n/a (doc) | — | — |
| [x] Tests | n/a (doc) | — | — |
| [x] Iterate | spans-protocol.md: rewrote Listitem-depth section to reflect the sibling-region decision; round 7 roadmap entry mentions 7.x; added a nested-list wire example. md2spans README's Lists row updated. | — | — |
| [x] Commit | — | — | `docs: spans protocol describes round 7.x sibling-region list nesting` |

## Phase 3.7.x.5: Smoke test + merge prep

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | n/a (validation) | — | — |
| [x] Tests | All packages green | `go test ./...` | Green. |
| [ ] Iterate | Build binaries; smoke-test in real edwood with: 2-level nested list (top-level); 3-level nested list; mixed markers; nested list inside blockquote; mixed `- ` and `1. ` across siblings. | — | Binaries rebuilt; awaiting user smoke. |
| [ ] Commit | — | — | n/a unless smoke surfaces a fix. |

---

## After this round

7.x establishes nested lists. 7.y can add multi-line
continuation (lazy + indented). Round 8 (tables) is the
final big region kind in the markdown-externalization
plan.

## Risks

(See base design doc.) Main concerns: outer-item region
containment requires lookahead; layout rule for
ListIndent > 1 may need tweaking; tab/space mixing edge
cases.

## Status

All rows complete. Awaiting smoke confirmation before
merging to master.
