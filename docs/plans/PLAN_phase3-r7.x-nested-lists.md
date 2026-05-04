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
| [x] Design | phase3-r7.x-nested-lists.md drafted | [base doc] | Decisions: (a) v1.1 covers leading-whitespace nesting only; (b) 2 spaces / 1 tab per level matches in-tree; (c) outer item region COVERS its sub-list (Option A — composes with bridge ancestor walk); (d) wire format unchanged from r7. |
| [x] Tests | n/a (planning) | — | — |
| [x] Iterate | This plan + design | — | — |
| [ ] Commit | — | — | `Add Phase 3 round 7.x design and plan: nested lists` |

## Phase 3.7.x.1: Detect indent + extend paragraphRange

The scanner-side groundwork. Add leading-whitespace
detection to `isListLine`; extend `paragraphRange` with
`ListDepth`. Don't yet rearrange the emitted regions —
this row makes the data available; row 2 wires it into
the emit.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | `isListLine` accepts an offset within the line and counts leading whitespace as 2-spaces-or-tab per level. Returns indent level alongside marker info. paragraphRange gets `ListDepth int`. `contentBytePos` accounts for leading whitespace. | base doc § "Indent counting" / "contentBytePos" | — |
| [ ] Tests | New tests pin: `isListLine` reports depth correctly for `- a` (depth 1), `  - b` (depth 2), `    - c` (depth 3), `\t- d` (depth 2), `   - e` (3 spaces — depth 2 by floor division). `_foo` returns no list. | `cmd/md2spans/parser_test.go` | Helper-level tests. |
| [ ] Iterate | Update isListLine signature; extend paragraphRange; update contentBytePos. parseListItemParagraph at depth-1 still works exactly as before (no behavior change yet). | `cmd/md2spans/parser.go` | — |
| [ ] Commit | — | — | `md2spans: detect list indent depth; extend paragraphRange` |

## Phase 3.7.x.2: List-stack state machine + nested emission

The structural change. scanParagraphs maintains a list
stack tracking active list levels. Items at deeper depth
are nested INSIDE outer items; outer item regions cover
their sub-lists (Option A from the design doc).

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | scanParagraphs adds a list-stack. On each list line, pop stack to current depth (or below for siblings); push new entry; emit paragraphRange with nesting info. Non-list / blank lines clear stack. Item regions emit begin at first list-marker line of that depth, end after the last sub-line at that depth or deeper. | base doc § "List-stack state machine" / "Concrete algorithm" | — |
| [ ] Tests | `- a\n  - b` → 2 listitem regions; outer covers both, inner nested. `- a\n  - b\n- c` → 2 outer siblings, 1 inner under first outer. `- a\n  - b\n    - c` → 3-level nesting. Mixed markers compose. Stack clears on blank line. | `cmd/md2spans/parser_test.go` | — |
| [ ] Iterate | scanParagraphs list-stack; nested begin/end emission via parseListItemParagraph or a post-processing pass. | `cmd/md2spans/parser.go` | — |
| [ ] Commit | — | — | `md2spans: emit nested listitem regions for indent-based nesting` |

## Phase 3.7.x.3: Layout interactions for deep nesting

Verify round 7's `listitemShifted` layout rule composes
correctly for `ListIndent > 1`. If smoke-tests show
artifacts (e.g., depth-2 nested item inside a depth-1
blockquote), patch the rule.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | n/a (validation) | base doc § "Layout interaction" / "Risk 2" | — |
| [ ] Tests | Add layout tests pinning xPos for `ListIndent=2` cases (top-level nested + nested-inside-blockquote). | `rich/layout_test.go` | — |
| [ ] Iterate | If tests reveal a layout bug, fix `listitemShifted` rule. Otherwise, no code change. | `rich/layout.go` | — |
| [ ] Commit | — | — | If patched: `rich: <fix description>`; else skip. |

## Phase 3.7.x.4: Spec + README

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | n/a (doc) | — | — |
| [ ] Tests | n/a (doc) | — | — |
| [ ] Iterate | spans-protocol.md adds a nested-list example. md2spans README's Lists row drops the "no nesting" caveat. Round 7.x roadmap entry. | — | — |
| [ ] Commit | — | — | `docs: spans protocol nested listitem example; md2spans handles nested lists` |

## Phase 3.7.x.5: Smoke test + merge prep

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | n/a (validation) | — | — |
| [ ] Tests | All packages green | `go test ./...` | — |
| [ ] Iterate | Build binaries; smoke-test in real edwood with: 2-level nested list (top-level); 3-level nested list; mixed markers; nested list inside blockquote; mixed `- ` and `1. ` across siblings. | — | User-driven. |
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

Plan + design drafted. Awaiting review before any code.
