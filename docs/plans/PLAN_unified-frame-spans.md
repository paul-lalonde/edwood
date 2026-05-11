# Plan — Unified Frame + Spans

Working checklist for the design at
`docs/designs/features/unified-frame-spans.md`. Each numbered row is
one CODING-PROCESS pass on a specific deliverable. Treat each row as
the entire scope of one sitting: do not skip the test stage, do not
stage-jump on implementation, and do not skip the commit.

Row legend (per project CLAUDE.md):
- `[ ] Design`  — confirm the relevant slice of the design doc
- `[ ] Tests`   — write tests against the requirements
- `[ ] Iterate` — implement red → green → review
- `[ ] Commit`  — commit with the message specified in the row

---

## Phase 0 — Setup

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| 0.1 | [x] §12 Phase 0 | [x] `go test -race ./...` green at HEAD | [x] `regression.sh`, working log, this plan | [ ] `chore: phase 0 — regression runner + working log + plan` | Awaiting user approval to commit. |

Exit criterion: `./regression.sh` green; working log and plan
present; `cleanroom` sits on `upstream/master` HEAD.

---

## Phase 1 — Frame data types

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| 1.1 | [ ] §5.3 (`StyleRun`, `Style`, `ReplacedKind`) | [ ] Zero-value behavior, `IsZero()` correctness, field-by-field defaults | [ ] Add types in `frame/`; no interface changes | [ ] `frame: introduce StyleRun, Style, ReplacedKind` | Resist adding fields the frame does not consume during layout/render (Risk row 5). |

Exit criterion: types compile; `IsZero()` discriminates zero from
non-zero correctly; no caller changes.

---

## Phase 2 — Frame styled methods (additive)

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| 2.1 | [ ] §5.1, §5.4 `InsertWithStyle` semantics | [ ] nil-styles ≡ upstream `Insert`; all-default `StyleRun` slice ≡ fast path; multi-run apply; mismatched lens panic | [ ] Add `InsertWithStyle` to `Frame` and `SelectScrollUpdater`; default path delegates to upstream `Insert` | [ ] `frame: add InsertWithStyle (additive)` | Plain Insert remains; `InsertByte` kept as upstream. |
| 2.2 | [ ] §5.4 `SetStyleRange` | [ ] Stub: callable, no observable effect yet (no per-rune storage) | [ ] Add method as stub returning without state | [ ] `frame: add SetStyleRange stub` | Full behavior comes in Phase 6 once per-rune storage exists. |
| 2.3 | [ ] §5.4 `SetOriginYOffset` / `GetOriginYOffset` | [ ] Get returns 0; Set is no-op | [ ] Add stubs | [ ] `frame: add SetOriginYOffset/Get stubs` | Real behavior in Phase 6. |

Exit criterion: `frame` compiles, upstream tests still green, new
methods callable but inert beyond the `Insert` delegation in 2.1.

---

## Phase 3 — Spans package

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| 3.1 | [ ] §6.1, §6.3 (`Store`, `GetStyleRuns` shape) | [ ] Empty store, single region, multi-region, full-coverage of `[p0,p1)`, Len-sum invariant | [ ] In-memory store with sorted regions, binary search | [ ] `spans: introduce Store with GetStyleRuns` | |
| 3.2 | [ ] §6.2 `Inserted` / `Deleted` observer rules | [ ] Trailing-edge extension, leading-edge no-extension, deletion clipping/merging/erasing, post-delete shift | [ ] Implement observer attach + index maintenance | [ ] `spans: maintain index across buffer mutations` | The trailing-edge rule (§6.2 rationale) is the subtle bit. |
| 3.3 | [ ] §6.1 `Observe` API | [ ] Callback fires on `SetRegion` / `ClearRegion`; not fired by buffer-driven shifts | [ ] Add observer slice + dispatch | [ ] `spans: add style-change Observe callback` | Buffer-driven shifts are bookkeeping only; observer fires only for style-only mutations. |

Exit criterion: standalone `spans` package with full test coverage;
no integration with Text yet.

---

## Phase 4 — Text wiring (no producers)

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| 4.1 | [ ] §7.1, §7.2, §8.1 | [ ] Window construction registers spans observer before Text observer; assertion catches reversed order | [ ] Add `Text.spans` field; thread store through Window construction | [ ] `text: thread spans store through Window construction` | Tags get `nil` (§8.4). |
| 4.2 | [ ] §7.3 `Inserted`, §7.4 `Deleted` | [ ] Insert with `spans==nil` matches upstream; with empty store also matches upstream (fast path); with non-empty store, styles propagate to frame | [ ] Modify `Text.Inserted` to call `InsertWithStyle` when applicable | [ ] `text: style-aware Inserted` | `Deleted` requires no change (frame's per-rune array shrinks alongside). |
| 4.3 | [ ] §7.5 `fill` / `setorigin` | [ ] Visible runes carry their styles after scroll; tall-element y-offset wired (still no-op until Phase 6) | [ ] Wire `GetStyleRuns` into fill; thread y-offset through `setorigin` | [ ] `text: style-aware fill and setorigin` | |
| 4.4 | [ ] §7.6 `attachSpans` | [ ] Observer clips to visible range; `SetStyleRange` called with frame-relative args | [ ] Implement helper; register on store | [ ] `text: attachSpans helper` | Verifies clipping math without relying on frame side-effects. |

Exit criterion: every body's behavior with no producer attached is
identical to upstream (regression baseline). Visible regression
suite passes.

---

## Phase 5 — 9P spans file

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| 5.1 | [ ] §6.4 directive format | [ ] Parser correctness on s/c/b directives; key=value style encoding; round-trip via Snapshot | [ ] Implement parser + serializer in `spans` package | [ ] `spans: directive parser/serializer` | |
| 5.2 | [ ] §8.3 `QWspans` qid | [ ] xfid open/read/write/close; multiple concurrent readers; "last writer wins" | [ ] Add qid in `xfid.go`; hook to per-window store | [ ] `xfid: add QWspans qid` | |
| 5.3 | [ ] Integration | [ ] Hand-written test producer writes directives over 9P; spans changes propagate to Text and onto the frame | [ ] Wire end-to-end | [ ] `text: integrate spans file with producer flow` | |

Exit criterion: a script can write directives to `/mnt/wsys/<id>/spans`
and see visible styling change.

---

## Phase 6 — Replaced elements

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| 6.1 | [ ] §5.3 `Style.Replaced*` fields, §5.4 replaced-rune rendering | [ ] Width/height honored; line height bumped; single-rune line break; click-to-charofpt inside element | [ ] Add per-rune style storage in frame; render path for `Replaced=true` | [ ] `frame: render Replaced runes` | Per-rune style storage is the data-layout decision left to the implementer (§15 item 1). |
| 6.2 | [ ] §5.4 `SetStyleRange` real behavior | [ ] Re-style updates storage and repaints; line-height recompute when Replaced flips | [ ] Replace Phase 2.2 stub with real implementation | [ ] `frame: SetStyleRange recomputes layout and repaints` | |
| 6.3 | [ ] §5.4 `SetOriginYOffset` real behavior | [ ] Non-zero yPx clips top of tall element; clamped to 0 for non-tall; reset to 0 on `Delete(0, *)` | [ ] Replace Phase 2.3 stubs | [ ] `frame: SetOriginYOffset clips tall elements` | |
| 6.4 | [ ] §7.5 `computeTallElementYOffset`, `tallY` state | [ ] `setorigin` emits correct y-offset for tall-element scrolls | [ ] Add helper + state to Text | [ ] `text: tall-element y-offset state` | |

Exit criterion: a styled body with an inline image taller than the
viewport scrolls correctly through the image's interior.

---

## Phase 7 — Image cache

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| 7.1 | [ ] §15 item 4 cache scope decision | [ ] LRU eviction, cache hit/miss behavior, decode correctness | [ ] Add LRU cache; consult from frame Replaced render path | [ ] `frame: image cache for replaced elements` | Default to global scope unless profiling argues otherwise. |

Exit criterion: scrolling past then back to an image hits the cache
instead of redecoding.

---

## Phase 8 — Producer rewrites

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| 8.1 | [ ] §11 md2spans | [ ] Golden-output tests on sample markdown (qualify under §13.1) | [ ] Clean-room re-impl as 9P client of spans file | [ ] `cmd/md2spans: clean-room rewrite` | |
| 8.2 | [ ] §11 edcolor | [ ] Per-language golden tests | [ ] Clean-room re-impl | [ ] `cmd/edcolor: clean-room rewrite` | |
| 8.3 | [ ] §11 dirthumb | [ ] Directory listing → thumbnail directives | [ ] Clean-room re-impl | [ ] `cmd/dirthumb: clean-room rewrite` | |

Exit criterion: producers work end-to-end against the new spans
file; visible rendering matches expected styled output.

---

## Phase 9 — Polish

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| 9.1 | [ ] §12 Phase 9 drag-scroll past frame edge | [ ] Drag past edge scrolls plain and styled identically | [ ] Move logic from per-mode path into Text | [ ] `text: unify drag-scroll past edge` | |
| 9.2 | [ ] §9.2 sub-element drag for very tall images | [ ] Reconsider only if real workflows demand | [ ] Deferred until called for | [ ] (no commit by default) | Explicitly *not* in v1 per §9.2. |
| 9.3 | [ ] §9.3 'S' event | [ ] Emitted under all four §9.3 conditions; suppressed when any fails | [ ] Wire emit in `Text.SetSelect` | [ ] `text: emit S event on selection change` | New event-file char; minimal vocabulary addition. |
| 9.4 | [ ] §10.2 horizontal scroll for wide replaced elements | [ ] Wheel routes to `HOffset` over wide elements; clamps to `[0, intrinsicW - frameW]` | [ ] Add `HScrollAt` and wheel routing in Text | [ ] `text: route wheel to wide replaced elements` | |

Exit criterion: design's full v1 surface present; performance
budgets (§13.3) met.

---

## Cross-phase invariants

Every commit on this branch must keep these green:

1. `./regression.sh` (mirrors CI).
2. Plain-text behavior identical to upstream — measured by upstream's
   own test suite continuing to pass without modification.
3. Observer order: `spans.Store` registers on the buffer *before*
   any `Text` (§4 numbered diagram, §8.1).
4. No mode flags on `Window` (§2 non-goal, §8.2). Body styling
   presence is a property of `t.spans != nil` and `!t.spans.Empty()`.
5. No parallel mouse-input loop (§2, §9). All body mouse input goes
   through `Text.Select`.

## Bug classification (Stage 4) reminder

When a test fails on this branch, classify before fixing:

- **Implementation accident** — code does not match the design; fix
  the code.
- **Undefined behavior** — design is silent on this case; pause,
  decide, update the design doc, then fix the code.
- **Wrong design** — design says X but reality demands Y; pause,
  discuss with the user, update the design doc, then fix the code.

The fix starts at the earliest affected stage, not at the code.
