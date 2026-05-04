# Phase 3 Round 7.y — List Continuation Lines — Plan

Finishes lists by adding indented-continuation support.
Lazy continuation and multi-paragraph items remain
deferred (round 7.z if ever needed; possibly never).

**Base design**: [`docs/designs/features/phase3-r7.y-list-continuations.md`](../designs/features/phase3-r7.y-list-continuations.md).

**Branch**: `phase3-r7.y-list-continuations`.

**Outcome**: Markdown like `- foo\n  bar` produces ONE
listitem region spanning both lines; `bar` is part of
the foo item, not a fresh paragraph.

**Files touched**:
- `cmd/md2spans/parser.go` — scanParagraphs active-item
  state; continuation detection.
- `cmd/md2spans/parser_test.go` — continuation tests.
- `docs/designs/spans-protocol.md` — multi-line listitem
  example.
- `cmd/md2spans/README.md` — Lists row caveat update.

No `wind.go` / `rich/` changes expected.

---

## Phase 3.7.y.0: Plan + design

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | phase3-r7.y-list-continuations.md drafted | [base doc] | Decisions: (a) v1.2 covers indented continuation only; (b) content-column-based detection (2 cols for `-`, 2+digits for ordered); (c) blank line still terminates the run; (d) lazy continuation deferred. |
| [x] Tests | n/a (planning) | — | — |
| [x] Iterate | This plan + design | — | — |
| [ ] Commit | — | — | `Add Phase 3 round 7.y design and plan: list continuation lines` |

## Phase 3.7.y.1: Scanner state for active list item + continuation detection

The substantial change. scanParagraphs gains active-list-
item tracking and continuation handling.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | scanParagraphs adds `activeListItem` (pointer/index into out) and `activeContentCol`. flushLine cases: (a) active list + list-marker line → finalize, start new; (b) active list + blank → finalize, clear; (c) active list + indented non-list → extend ByteEnd; (d) active list + non-indented non-list → finalize, clear, normal handling; (e) no active list + list marker → start. Post-loop: finalize. | base doc § "md2spans changes" / "Detection rules" | — |
| [ ] Tests | TestParseListContinuationSimple (`- a\n  cont` → 1 region spanning 2 lines), TestParseListContinuationMultiline (3+ continuation lines), TestParseListContinuationTerminatedByBlank, TestParseListContinuationTerminatedByNonIndented, TestParseListContinuationOrderedNeedsDeeperIndent (`1. a\n  not enough` doesn't continue), TestParseListLazyContinuationNotSupported (`- a\nbar` doesn't continue), TestParseListContinuationFollowedByNewItem. | `cmd/md2spans/parser_test.go` | — |
| [ ] Iterate | scanParagraphs state + continuation logic; isListLine reused. | `cmd/md2spans/parser.go` | — |
| [ ] Commit | — | — | `md2spans: extend list items across indented continuation lines` |

## Phase 3.7.y.2: Spec + README

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | n/a (doc) | — | — |
| [ ] Tests | n/a (doc) | — | — |
| [ ] Iterate | spans-protocol.md adds a multi-line listitem example. md2spans README's Lists row notes continuation-line support; lazy continuation explicitly excluded. Round 7.y roadmap entry. | — | — |
| [ ] Commit | — | — | `docs: spans protocol multi-line listitem example; md2spans handles continuation lines` |

## Phase 3.7.y.3: Smoke + merge prep

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | n/a (validation) | — | — |
| [ ] Tests | All packages green | `go test ./...` | — |
| [ ] Iterate | Build binaries; smoke-test in real edwood with: simple multi-line item; ordered multi-line; mixed continuation + sibling; continuation inside blockquote; nested item with continuation. | — | User-driven. |
| [ ] Commit | — | — | n/a unless smoke surfaces a fix. |

---

## After this round

7.y closes out lists. Round 8 (tables) is the final
region kind in the markdown-externalization plan. After
8 lands, md2spans is feature-complete vs. the in-tree
path → Phase 4 (migration & in-tree deletion) unlocks.

## Risks

(See base design doc.) Indent-counting tab semantics;
sub-list vs continuation disambiguation (isListLine
fires first — already correct); nested-list continuation
ambiguity (resolved by "most recent active item").

## Status

Plan + design drafted. Awaiting review before any code.
