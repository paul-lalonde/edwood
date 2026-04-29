# Unified Vertical Scrollbar — Implementation Plan

Refactor the divergent vertical scrollbar implementations in `scrl.go`
(text mode) and `richtext.go` (rich-text mode) into a single shared
widget. Each mode supplies a small `ScrollModel` adapter; the widget
owns drawing, mouse handling, and the click-and-hold latch loop.

**Base design doc**: [docs/designs/features/unified-scrollbar.md](../designs/features/unified-scrollbar.md)

**Key design decisions** (see base doc for rationale):
- Single `Scrollbar` widget in `main` (Option A package layout).
- `ScrollModel` interface: `Geometry`, `DragTopToPixel`,
  `DragPixelToTop`, `JumpToFraction` — all pixel-space at the widget
  boundary.
- Frame colors only: `global.textcolors[frame.ColBord]` and
  `[frame.ColBack]`. `WithScrollbarColors` is removed.
- `MinThumbHeightPx = 10` constant (replaces `2` in `scrl.go` and the
  bare `10` in `richtext.go`).
- Acme drag semantics preserved exactly: B1 = drag-top-down-to-here;
  B3 = drag-line-here-up-to-top; B2 = jump-to-fraction. 200ms/80ms
  debounce; cursor warps to scrollbar centerline.
- Rich mode operates in true document pixels via `SetOriginYOffset`,
  fixing the tall-image scroll regression (no more snap-jumps over
  600px images).

**Branch strategy**: this is too large for `fix/slide-navigation-scroll`.
Land that branch first, then start a new branch
`refactor/unified-scrollbar` from `master`.

**Pre-flight blocker**: `wind.go.orig` and `wind.go.rej` are present in
the working tree. Resolve before starting Phase 3 (which edits
`wind.go`).

**Files touched**:
- `scrl.go` — replaced by `scrollbar.go` (new) + adapter in `text.go`.
- `text.go` — `Text.Scroll` deleted; new `textScrollModel`; the
  `acme.go:480` call site updated.
- `richtext.go` — scrollbar code at `richtext.go:273-507` deleted;
  `WithScrollbarColors` and `scrollBg`/`scrollThumb` fields removed;
  new `richScrollModel`; helpers `pixelYForOrigin` /
  `lineAndOffsetAtPixelY` retained.
- `wind.go` — `previewVScrollLatch` and `previewScrSleep` deleted;
  call sites at lines 1036, 2750 updated to use the widget.
  `previewHScrollLatch` (lines 1053, 1563-1620) is **untouched**.
- `acme.go` — line 480 dispatch updated to call the new widget.

---

## Phase 1: Foundation — `Scrollbar` widget and `ScrollModel` interface

This phase introduces the new types in isolation. Nothing is wired
into either mode yet. The widget can be tested with a fake
`ScrollModel` before any production code depends on it. **Outcome:**
shared widget compiles, has unit tests, but has zero runtime callers —
both modes still use their existing scrollbars unchanged.

### 1.1 `ScrollModel` interface and `MinThumbHeightPx` constant

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Distill `ScrollModel` interface and `MinThumbHeightPx` from base doc | `docs/designs/features/unified-scrollbar.md` | Output is the doc itself. |
| [x] Tests | n/a (interface only; tests come with concrete widget in 1.2) | — | — |
| [x] Iterate | Add `scrollbar.go` with the `ScrollModel` interface and `MinThumbHeightPx` constant. No `Scrollbar` struct yet. | `docs/designs/features/unified-scrollbar.md` § "ScrollModel interface" | Done in `c7f55a2`. |
| [x] Commit | Commit interface + constant | — | `c7f55a2` Add ScrollModel interface and MinThumbHeightPx constant |

### 1.2 `Scrollbar` widget — drawing

Pixel-identical drawing output to the legacy `scrl.go:67-99` so the
diff in PR2 is verifiable visually.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Confirm thumb math equals `scrpos()` | `scrl.go:27-56`, `docs/designs/features/unified-scrollbar.md` § "Drawing" | Math reproduces `scrpos` semantics. `>>10` overflow guard preserved; `MinThumbHeightPx` (10) replaces the legacy `2` clamp. |
| [x] Tests | Write tests for `Scrollbar.Draw` thumb computation | `docs/designs/features/unified-scrollbar.md` § "Drawing" | `scrollbar_test.go` covers: full track when view ≥ doc, zero-doc, origin at top/mid/bottom, MinThumbHeightPx clamp (extends-down + pin-to-bottom), large-doc overflow guard, track offset from zero, dirty cache (first draws, repeated no-ops, model change repaints, SetRect invalidates). |
| [x] Iterate | Implement `Scrollbar` struct, `SetRect`, `Draw`, internal `scrtmp` allocation | `scrl.go:58-99`, `docs/designs/features/unified-scrollbar.md` § "Drawing" | Done in `b63ff39`. `computeThumbRect` + `clampThumbHeight` extracted as pure functions. |
| [x] Commit | Commit Scrollbar drawing | — | `b63ff39` Add Scrollbar widget drawing |

### 1.3 `Scrollbar` widget — mouse latch

Lifted bit-for-bit from `Text.Scroll` (`scrl.go:101-166`) and
`previewScrSleep` (`wind.go:1481`).

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Confirm latch loop matches `scrl.go:108-165` | `scrl.go:101-166`, `wind.go:1481-1561`, `docs/designs/features/unified-scrollbar.md` § "Mouse latch" | Loop body factored into `clampMouseY`, `warpToCenter`, `dispatch`, `waitForNextTick`, `drainMouseEvents`, `scrollbarSleep`. Ordering of Flush/MoveTo/Read/model-update preserved bit-for-bit. |
| [x] Tests | Write tests for `Scrollbar.HandleClick` button dispatch | `docs/designs/features/unified-scrollbar.md` § "Mouse latch" | `scrollbar_test.go` covers `dispatch` (B1→DragTopToPixel, B3→DragPixelToTop, B2→JumpToFraction with correct fraction, B2 zero-track-height guard, unknown-button no-op). The full latch loop (auto-repeat timing, drain) is **deferred to manual verification** in §2.3 — `Mousectl.Read` embeds a real-display `Flush()` and faking it requires invasive abstraction work. The timing test from the original plan is flagged as deferred. |
| [x] Iterate | Implement `HandleClick` | `scrl.go:101-166`, `wind.go:1481-1561` | Done in `68ad57a`. `initialDebounce` / `repeatDebounce` are package-level `time.Duration` vars (renamed in Phase 1.4) to leave a future test seam. |
| [x] Commit | Commit Scrollbar mouse latch | — | `68ad57a` Add Scrollbar widget mouse latch |

### 1.4 Address Phase 1 reviewer findings

Architect + code-reviewer pass against Phase 1 surfaced one bug
(scratch image not reallocated on screen growth) plus a batch of
correctness, idiomatic-Go, and test-sharpness improvements. All
non-deferred findings landed before opening the PR.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Triage findings → fix-now vs. defer | reviewer outputs | Fix-now: `s.tmp` lifetime, `time.Duration` typing, `waitForNextTick` signature, `lastSR` rename, `SetRect` short-circuit, doc clarifications, design doc honesty fixes, test sharpness. Deferred: H-scrollbar axis parameterization (Phase 4 cleanup), full latch loop testing (Phase 2.3 manual), lifecycle ownership audit (Phase 2/3). |
| [x] Tests | Strengthen tests in lockstep with each fix | — | Added: `s.tmp` resize self-heal (with same-size no-op); `Draw` empty-rect; `clampMouseY` boundary cases; `SetRect` same-rect idempotency; non-trivial `clickY` values for B1/B3/B2 dispatch (37, 73, 17, 88, 99 — chosen so any off-by-track-height error produces an unambiguous wrong number); exact draw-op count assertion (`expectedFirstDrawOps = 4`) instead of `> 0`. |
| [x] Iterate | Apply fixes | — | Done across `5971ece`, `84a090f`, `dfc2325`, `200e59f`, `6bbb6e3`, `3eecc67`. |
| [x] Commit | One commit per concern | — | See above SHAs. |

---

## Phase 2: Migrate text mode

This phase replaces text mode's scrollbar with the shared widget,
verifying no behavioral or visual regression. After this phase,
`scrl.go` is deleted but rich mode is still on its own scrollbar.

### 2.1 `textScrollModel` adapter

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Confirm adapter formulas equal current `Text.Scroll` | `scrl.go:127-148`, `docs/designs/features/unified-scrollbar.md` § "Text-mode adapter" | Bit-for-bit equivalent to scrl.go branches: BackNL+SetOrigin for B1, Charofpt-based for B3, BackNL(p0,2) snap for B2. |
| [x] Tests | Write tests for `textScrollModel` | `docs/designs/features/unified-scrollbar.md` § "Text-mode adapter" | `text_scroll_test.go` covers all required cases plus mid-doc + below-q1 + nil-frame edge handling. |
| [x] Iterate | Add `textScrollModel` type | — | Done in `4887ecc`. |
| [x] Commit | Commit text-mode adapter | — | `4887ecc` Add textScrollModel adapter for shared Scrollbar widget |

### 2.2 Replace `Text.ScrDraw` and `Text.Scroll` with widget delegation

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Confirm preview-mode gating preserved | `scrl.go:73-79`, `docs/designs/features/unified-scrollbar.md` "Risks" #3 | Body-only and IsPreviewMode guards remain at the call site (Text.ScrDraw); widget unaware of either. |
| [x] Tests | Existing scrollbar tests should continue to pass | — | Full suite green. |
| [x] Iterate | Rewrite `Text.ScrDraw` and `Text.Scroll` as delegators | — | Done in `7d49af1`. Bug found in Phase 2.3: SetRect's idempotent short-circuit broke post-Redraw clobber recovery; fixed in `42657fd`. |
| [x] Commit | Commit text-mode delegation | — | `7d49af1` Delegate Text scrollbar to shared Scrollbar widget |

### 2.3 Manual visual verification

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Test | Boot edwood, open a long file, exercise the scrollbar | — | Verified iteratively across multiple branches of the diff: directory-listing initial paint (`42657fd`), B1 click direction (`f0d4997`), gap-aware click→line mapping for rich mode (`5543f81`), latch debounce for cursor jitter (`b03ef9c`). |
| [x] Commit | n/a — verification only | — | Bugs found and fixed in-stream rather than discovered later. |

### 2.4 Delete `scrl.go`

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Iterate | Delete `scrl.go`; relocate `Text.ScrDraw` / `Text.Scroll` delegators to `text.go`; remove `ScrlResize` call from `acme.go`; update `FrameScroll`'s `ScrSleep(100)` to use `drainScrollEvents`. | — | Done in `2c4daff`. |
| [x] Tests | Full test suite | — | All green. |
| [x] Commit | Commit deletion | — | `2c4daff` Delete scrl.go (subsumed by Scrollbar widget) |

---

## Phase 3: Migrate rich-text mode

This phase replaces rich mode's scrollbar with the shared widget,
including the pixel-line alignment fix for tall images. **Outcome:**
both modes use the same widget; visuals and interactions are
identical; tall images scroll smoothly.

### 3.1 Pre-flight: resolve `wind.go.orig` / `wind.go.rej`

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Iterate | Investigate and resolve the unresolved merge artifacts | — | Done way back in this branch's setup; .rej was a stale fix superseded by `dbafabb` on master. |
| [x] Commit | Commit resolution if changes were needed | — | n/a — files just deleted as stale. |

### 3.2 `richScrollModel` adapter (with sub-line offset)

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Confirm adapter delegates correctly | — | Adapter delegates to scrollClickAt, which already encapsulates the gap-aware math from Phase 2 (LinePixelYs, lineAtDocY/RoundUp, asymmetric snap). Sub-line offset (originYOffset) handled by scrollClickAt's existing path. |
| [x] Tests | Write tests for `richScrollModel` | — | `richtext_scroll_model_test.go` covers Geometry (file-top, mid-doc, nil-frame), DragPixelToTop/DragTopToPixel direction, JumpToFraction, nil-safe behavior, and the B1/B3 inverse-pair invariant exercised through the adapter. |
| [x] Iterate | Add `richScrollModel` type | — | Done in `2a8952c`. |
| [x] Commit | Commit rich-mode adapter | — | `2a8952c` Add richScrollModel adapter for shared Scrollbar widget |

### 3.3 Delete the in-`richtext.go` scrollbar implementation

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Iterate | Delete `scrDraw` / `scrDrawAt` (truly unused after wire-in). | — | Done in `9e4b515`. `scrThumbRect`, `scrThumbRectAt`, `ScrollClick`, `scrollClickAt`, `scrollBg`/`scrollThumb` fields, and `WithScrollbarColors` are still referenced (scrollClickAt is the body of richScrollModel methods; ScrollClick is the body of the legacy fallback in previewVScrollLatch). Full deletion of those is a Phase 4 cleanup task. |
| [x] Tests | Full test suite + manual verification | — | All tests green; user verified visually that the rich-mode ghosting is gone after the wire-in (Phase 3.3+3.4 painted full track on every cache miss, eliminating the legacy ghost-thumb echoes). |
| [x] Commit | Commit removal | — | `9e4b515` Delete unused legacy scrDraw / scrDrawAt in richtext.go |

### 3.4 Replace `previewVScrollLatch` with widget delegation

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Iterate | At `wind.go:1036` and `wind.go:2780`, replace the `previewVScrollLatch(rt, mc, button, scrRect)` call with `rt.scrollbar.HandleClick(button)`. | — | Done in `d956526`. previewVScrollLatch + previewScrSleep retained as dead-code fallback; `previewHScrollLatch` still uses neither (uses inline drainScrollEvents now). Deletion of both is a Phase 4 cleanup task. |
| [x] Tests | Manual verification: rich-mode B1/B3/B2 all work; ghosting gone | — | User verified across multiple test sessions on multiple markdown files. |
| [x] Commit | Commit widget delegation in wind.go | — | `d956526` Wire rich-text mode to the unified Scrollbar widget |

---

## Phase 4: Cleanup

Tidies up after the major change. None of these block the others; can
be batched into a single PR or done individually.

### 4.1 Remove `nchars` parameter from `Text.ScrDraw`

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Iterate | Drop the unused `nchars` parameter (already TODO'd at `scrl.go:66`) | `scrl.go:66`, all `ScrDraw` call sites | Five call sites: `text.go:477,648,1300,1372,1758`. After Phase 2 the parameter is genuinely unused. |
| [ ] Tests | Compile + full test suite | — | — |
| [ ] Commit | Commit signature cleanup | — | Message: `Drop unused nchars parameter from Text.ScrDraw` |

### 4.2 Update working log

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Iterate | If a `docs/working-log.md` exists for this branch (per `/Users/paul/CLAUDE.md`), record what changed and why | — | Per CLAUDE.md, long-lived feature branches carry a working log. Add entry summarizing the unification, link to design doc and this plan. |
| [ ] Commit | Commit log update | — | Message: `Update working log: unified scrollbar refactor` |

### 4.3 Final visual diff

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Test | Side-by-side build of `master` vs `refactor/unified-scrollbar` | — | Open the same files in both. Confirm pixel-identical scrollbar rendering in text mode. Confirm rich mode now matches text mode exactly. Confirm tall-image regression is fixed. Capture screenshots for the PR description. |
| [ ] Commit | n/a — verification only | — | — |

---

## Open questions

1. **Phase boundaries vs PRs.** The plan above suggests four phases.
   For review, mapping is: Phase 1 = PR1, Phase 2 = PR2, Phase 3 =
   PR3, Phase 4 = PR4. Phases 1 and 2 each leave the tree compiling
   and behaviorally unchanged for the unmodified mode. Phase 3 is
   the big change. Confirm with the user that four PRs is the
   desired granularity vs. a single larger PR.
2. **`previewHScrollLatch` deletion timing.** If `previewScrSleep` is
   still referenced by the H-scrollbar after Phase 3, we keep one
   piece of duplication until the H-scrollbar follow-up. Acceptable
   short-term; flag in the working log.
3. **Test coverage of the latch loop.** The auto-repeat timing test
   (Phase 1.3) is inherently flaky. If we can't get a reliable
   timing test, accept manual verification as the gate and document
   the gap.
