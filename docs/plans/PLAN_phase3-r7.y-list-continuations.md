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
| [x] Commit | — | — | `Add Phase 3 round 7.y design and plan: list continuation lines` |

## Phase 3.7.y.1: Scanner state for active list item + continuation detection

The substantial change. scanParagraphs gains active-list-
item tracking and continuation handling.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | scanParagraphs adds `activeListIdx` + `activeContentCol`; clearActiveList helper called on blank/heading/HRule/fence/blockquote/non-indented-non-list lines. List marker sets activeListIdx to len(out)-1. Continuation extends out[activeListIdx].ByteEnd. | base doc § "md2spans changes" / "Detection rules" | — |
| [x] Tests | 10 new tests covering simple continuation, multi-line, ordered with deeper-indent requirement, lazy-not-supported, sibling vs continuation, blank-terminates, non-indented-terminates, sub-list-not-continuation. | `cmd/md2spans/parser_test.go` | — |
| [x] Iterate | scanParagraphs state + continuation logic. | `cmd/md2spans/parser.go` | — |
| [x] Commit | — | — | `md2spans: extend list items across indented continuation lines` |

## Phase 3.7.y.2: Spec + README

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | n/a (doc) | — | — |
| [x] Tests | n/a (doc) | — | — |
| [x] Iterate | spans-protocol.md round 7 roadmap entry updated; new multi-line listitem example added. md2spans README Lists row notes continuation-line support. | — | — |
| [x] Commit | — | — | `docs: spans protocol multi-line listitem example; md2spans handles continuation lines` |

## Phase 3.7.y.3: Smoke + merge prep

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | n/a (validation) | — | — |
| [x] Tests | All packages green | `go test ./...` | Green. |
| [x] Iterate | Smoke confirmed via test.md (covers continuation, ordered, nested, blockquote, lazy-not-supported). No fixes needed. | — | — |
| [x] Commit | — | — | n/a (no smoke fixes). |

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

All rows complete. Smoke confirmed. Ready to merge to
master.
