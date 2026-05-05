# Phase 3 Round 8.0c — Tables: Spec, README, and Smoke — Plan

Third and final sub-round for round 8 v1. After 8.0a
(consumer) and 8.0b (producer), 8.0c lands the
documentation updates and user-driven smoke testing
that confirms the v1 visual is acceptable.

This is the sub-round where the user typically uncovers
issues that need fixing. Round 7 had four smoke-fix
commits; round 6 had two; previous rounds had varying
counts. The plan accommodates iteration.

**Base design**: [`docs/designs/features/phase3-r8-tables.md`](../designs/features/phase3-r8-tables.md).

**Depends on**: 8.0a + 8.0b merged.

**Branch**: `phase3-r8.0c-tables-spec-and-smoke`.

**Files touched**:
- `docs/designs/spans-protocol.md` — three new kinds
  documented; alignment + header params; nested-table
  example.
- `cmd/md2spans/README.md` — Tables row flips to ✓
  (with v1 caveats).
- Whatever the smoke surfaces.

---

## Phase 3.8.0c.0: Plan + design

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Inherits from base design doc. | base doc | — |
| [x] Tests | n/a (planning) | — | — |
| [x] Iterate | This plan | — | — |
| [ ] Commit | — | — | `Add Phase 3 round 8.0c plan: tables spec + smoke` |

## Phase 3.8.0c.1: Spec + README

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | n/a (doc) | — | — |
| [ ] Tests | n/a (doc) | — | — |
| [ ] Iterate | spans-protocol.md adds the three new kinds; documents `align=L|R|C` and `header=true` params; nested-table example. md2spans README's Tables row flips to ✓ with caveats (no column-width alignment in v1; round 8.x will revisit). Phase 3 roadmap entry: round 8 v1 ✓ landed. | — | — |
| [ ] Commit | — | — | `docs: spans protocol gains table region kinds; md2spans handles GFM tables` |

## Phase 3.8.0c.2: Smoke + iteration

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | n/a (validation) | — | — |
| [ ] Tests | All packages green | `go test ./...` | — |
| [ ] Iterate | Build binaries; produce a `tables-test.md` sample covering: simple 2×2 table; aligned columns (left/right/center); empty cells; emphasis inside cells; table inside blockquote; non-table line starting with `\|` (verify it stays plain text). User opens it in edwood + runs md2spans. | — | User-driven. |
| [ ] Commit | — | — | n/a unless smoke surfaces a fix. |

## Phase 3.8.0c.3: Smoke fixes (open-ended)

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Iterate | If smoke surfaces issues, fix them in this row. Each fix gets its own commit per the round-7 smoke-fix pattern. Commits typically touch parser, bridge, layout, or wire format depending on the bug. | — | — |
| [ ] Commit | — | — | (varies) |

---

## After this sub-round

Round 8 v1 (tables) is shipped. md2spans now covers all
in-tree markdown features EXCEPT column-width alignment.

**Round 8.x**: column-width-aware alignment. Either via
a frame-dimension 9P endpoint + producer-side width
computation, or via layout-side two-pass measurement.
Design TBD — likely a separate round once we see how
v1's monospace approach fares in real use.

After 8.x, **Phase 4** (migration default flip + in-tree
deletion) unlocks.

## Status

Plan drafted. Awaiting 8.0a + 8.0b merge before any code
here.
