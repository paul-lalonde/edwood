# Markdown Externalization — Phase 1 Plan

Move markdown-specific PAINT logic out of `rich.Frame` into a new
in-tree `rich/mdrender` package, wire `RichText` preview mode through
the wrapper, and finalize geometry ownership. The wrapper is
transitional: as Phase 3 rounds land, each markdown feature in the
wrapper migrates to a spans-protocol primitive and the wrapper
shrinks until Phase 4 deletes it entirely.

**Base design doc**: [docs/designs/features/markdown-externalization.md](../designs/features/markdown-externalization.md)

**Branch**: `markdown-externalization`.

**Phase 1 explicitly does NOT do** (deferred to later phases):

- Remove markdown-specific fields from `rich.Style` (deferred:
  fields stay until each one's Phase 3 round migrates its consumer
  to a protocol primitive).
- Restructure `rich/layout.go` or `rich/box.go` (those are
  layout-affecting and need careful per-feature design).
- Touch the spans protocol (`StyleAttrs`).
- Touch the `markdown/` package's interface.
- Build `md2spans` (Phase 2).

**Phase 1 outcome**: `rich.Frame.drawTextTo` no longer contains
paint phases that interpret markdown semantics. Those phases move
to `rich/mdrender.Renderer` which wraps the frame. Markdown preview
still works; the path through the code is one indirection longer
but the rendering is identical.

**High-level files touched**:
- `rich/frame.go` — paint phases for blockquote borders and
  horizontal rules move out. `Init` drops `rect` parameter.
- `rich/mdrender/` (NEW) — wrapper package owning the moved phases.
- `richtext.go` — preview mode constructs and uses the wrapper.
  Geometry: `RichText` is the sole `SetRect` caller.
- Tests under `rich/` and `rich/mdrender/` — shift to follow the
  paint phases.

**Slides deprecated** (April 2026 revision): the slide-rendering
feature (`Style.SlideBreak`, `findSlideRegions`,
`adjustLayoutForSlides`, slide-fill paint, slide-related Frame
methods) is being deprecated as a separate concern from this
work. Phase 1 will NOT move slide-break handling into the wrapper;
slide-break is dropped from the lean-frame contract; the
slide-related Phase 3 round is removed. The slide code stays in
`rich/` for now (dormant); its removal is a follow-up work item
with its own design conversation. As a consequence, what was Phase
1.4 (slide-break move) is dropped, and what was Phase 1.5 (route
preview through wrapper) was already done in lockstep with Phase
1.2 per the option-(a) decision — so the only Phase 1 row left
after 1.3 is the geometry-ownership cleanup.

---

## Phase 1.0: Plan and scaffolding

The plan doc itself plus a sanity-check pass over what's actually
in scope.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Distill Phase 1 scope from base design doc | `docs/designs/features/markdown-externalization.md` § "Phase 1 detail" | Phase 1 is paint-phase moves only. Layout-affecting Style fields stay. Style field removal deferred to Phase 3 per-round migrations. |
| [x] Tests | n/a (planning row) | — | — |
| [x] Iterate | Write this plan doc | — | This file. |
| [ ] Commit | Commit plan doc | — | `Add Phase 1 plan: markdown externalization` |

## Phase 1.1: `rich/mdrender` package skeleton

Empty wrapper that holds a `*rich.Frame` reference and
pass-through-delegates `Redraw`. Establishes the package boundary
and the `Renderer` type. Zero behavior change — wrapping a frame
and calling `Renderer.Redraw` produces output identical to direct
`frame.Redraw`. Validates the import direction (`mdrender` imports
`rich`, never the reverse) and the test layout.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Confirm wrapper API: `New(rich.Frame) *Renderer`, `Redraw()`, `SetMarkdownContent(rich.Content)` for now | `docs/designs/features/markdown-externalization.md` § "Phase 1 detail" | API is intentionally narrow at 1.1; grows in 1.2-1.4. |
| [ ] Tests | Write tests: wrapper compiles, `Redraw` of wrapped frame produces identical output to direct `frame.Redraw` for non-markdown content | — | Can compare draw ops via existing `edwoodtest.GettableDrawOps`. |
| [ ] Iterate | Add `rich/mdrender/renderer.go` with the skeleton. No paint phases moved yet. | — | — |
| [ ] Commit | Commit skeleton | — | `Add rich/mdrender wrapper package skeleton` |

## Phase 1.2: Move blockquote borders to wrapper

`paintPhaseBlockquoteBorders` is the cleanest "decorate on top of
already-painted text" case — left-edge bars per line. The phase
runs after `paintPhaseBoxBackgrounds` in `drawTextTo`, draws
narrow vertical bars based on `Style.BlockquoteDepth`, and is
purely additive (doesn't modify line layout). Easiest first move.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Confirm phase is purely decorative; verify it has no layout side-effects | `rich/frame.go:paintPhaseBlockquoteBorders` callees | If it touches anything beyond `target.Draw`, expand scope. |
| [ ] Tests | Write tests in `rich/mdrender/` for blockquote-bar rendering on wrapped frame; pin existing `rich/` tests for visual equivalence | `docs/designs/features/blockquotes.md` (existing) | Tests assert wrapper draws bars at correct (x, y, w, h) for given content. |
| [ ] Iterate | Move `paintPhaseBlockquoteBorders` and `drawBlockquoteBorders` from `rich/frame.go` into `rich/mdrender/`. Wrapper's `Redraw` calls `frame.Redraw()` then `r.paintBlockquoteBorders()`. Wrapper reads `Style.BlockquoteDepth` from frame's content (frame keeps a getter). | `rich/frame.go` blockquote-related code | The Style.Blockquote field stays on rich.Style (deferred per Phase 1 scope). |
| [ ] Commit | Commit blockquote move | — | `Move blockquote-border painting to rich/mdrender wrapper` |

## Phase 1.3: Move horizontal rules to wrapper

Same shape as 1.2 — `paintPhaseHorizontalRules` is decorative.
Reserves vertical space via the layout (`HRule` boxes have
height) but the *drawing* of the rule is post-paint.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Confirm rule rendering doesn't depend on frame internals beyond what's already exported | `rich/frame.go:paintPhaseHorizontalRules`, `drawHorizontalRuleTo` | Rule height comes from layout (Box.Height); paint just fills a rect. |
| [ ] Tests | Tests for wrapper-driven HRule rendering | — | Mirror the existing rich-side HRule test asserting position and color. |
| [ ] Iterate | Move `paintPhaseHorizontalRules` and `drawHorizontalRuleTo` to wrapper. Frame's `drawTextTo` orchestrator drops the call. | `rich/frame.go` HRule paths | — |
| [ ] Commit | — | — | `Move horizontal-rule painting to rich/mdrender wrapper` |

## Phase 1.4 — DROPPED: Move slide-break handling to wrapper

Slides are being deprecated as a separate concern; this row is
removed from the plan. The slide-rendering code (`Style.SlideBreak`,
`findSlideRegions`, `adjustLayoutForSlides`, slide-fill paint,
`HasSlideBreakBetween`, `SnapOriginToSlideStart`) stays in `rich/`
for now (dormant). Its actual removal is a follow-up work item
with its own design conversation.

## Phase 1.5 — SUBSUMED: Route RichText preview mode through the wrapper

Done as part of Phase 1.2 in lockstep with the blockquote-paint
move (option-(a) decision in `rich/mdrender/blockquote.design.md`).
`RichText.Init` constructs the wrapper unconditionally;
`RichText.Redraw` and `Render` route through it. No separate
row needed; remaining concern (styled mode bypassing the wrapper
for cleanliness) is optional polish, deferred.

## Phase 1.4: Finalize geometry ownership (P1-6 = D)

(Renumbered from old 1.6.) `RichText` is the sole `SetRect`
caller. Drop the `rect` parameter from `rich.Frame.Init`. The
frame's rectangle is set exclusively via `SetRect` from the
wrapper / `RichText`. Removes the dual-ownership ambiguity the
architect flagged.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Audit all `rich.Frame.Init` callers (production + test) | `grep "frame\\.Init\\|Init(image\\." --include="*.go"` | Most call sites pass the rect; they all switch to `Init()` + `SetRect(rect)`. |
| [ ] Tests | Update existing rich tests to use the new Init signature | — | Mechanical; should be a find-and-replace. |
| [ ] Iterate | Drop `rect` from `Init`; update callers; add a doc note that `SetRect` is the only geometry setter | `rich/frame.go:Init`, `rich/options.go` if needed | — |
| [ ] Commit | — | — | `Drop rect from rich.Frame.Init; SetRect is the sole geometry setter` |

---

## After Phase 1

Phase 1 leaves the system in this state:

- `rich.Frame` no longer paints blockquote bars or horizontal
  rules. Those move to `rich/mdrender`.
- `RichText` uses `rich/mdrender` unconditionally; the wrapper's
  extra paint pass is a no-op for styled mode (no blockquote /
  HRule content there). Styled-mode bypass for cleanliness is
  optional polish, deferred.
- `rich.Style` still carries markdown-specific fields. Removing
  them is per-round work in Phase 3.
- Slide-rendering code remains dormant in `rich/`; its removal
  is a separate work item with its own design conversation.
- `markdown/` package interface unchanged.
- Spans protocol (`StyleAttrs`) unchanged.

**Phase 2** then begins: build `md2spans` v1 against the *current*
spans protocol. That work has its own design doc and plan.

---

## Risks for Phase 1

1. **Test fixture coverage gaps.** Existing rich-side tests cover
   blockquote and HRule rendering. If wrapper-driven rendering
   subtly differs (e.g., draw-call ordering), tests should catch
   it — but only if we run them on the FULL preview path, not
   just the wrapper in isolation. Each row's test stage must
   include the integrated path.
2. **Backward-compat of Init signature.** The renumbered Phase 1.4 drops a
   parameter. Any code outside this branch that calls `Init(rect, ...)`
   breaks. The surface is contained to `rich/` callers + a few in
   `richtext.go`; small impact.

---

## Status

Rows 1.0 through 1.3 complete. Phase 1.4 (slide-break move)
dropped due to slides deprecation. Phase 1.5 (preview routing)
subsumed into 1.2. The renumbered Phase 1.4 (geometry ownership)
remains as the only outstanding row before Phase 1 closes.
