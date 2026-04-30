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
- `rich/frame.go` — paint phases for blockquote borders, horizontal
  rules, slide-break fills move out. `Init` drops `rect` parameter.
- `rich/mdrender/` (NEW) — wrapper package owning the moved phases.
- `richtext.go` — preview mode constructs and uses the wrapper.
  Geometry: `RichText` is the sole `SetRect` caller.
- Tests under `rich/` and `rich/mdrender/` — shift to follow the
  paint phases.

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

## Phase 1.4: Move slide-break handling to wrapper

`findSlideRegions` + `adjustLayoutForSlides` + the slide-fill
draw logic. Slightly trickier than 1.2-1.3 because slide-region
detection runs during *layout*, not paint — `adjustLayoutForSlides`
mutates line Y values to expand slide regions. Need to either
(a) keep slide layout adjustments in frame but driven by a
wrapper-supplied callback / region list, or (b) move the
layout adjustment too and have the wrapper own that piece of
the pipeline. (a) is smaller. Decide at Design.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Pick (a) wrapper provides slide-region offsets to frame as data, or (b) wrapper owns post-layout adjustment. Read both paths' costs. | `rich/frame.go:findSlideRegions`, `adjustLayoutForSlides`, `paintPhaseGutterRepaint` | Likely (a): smaller, keeps frame's layout pipeline intact. Frame exposes `SetSlideRegions([]SlideRegion)`; wrapper sets it before `Redraw`. |
| [ ] Tests | Tests for slide-break expansion behavior driven via wrapper | `docs/designs/features/manual-markdown-mode.md` slide sections | Existing slide tests in `rich/` need to migrate or stay if frame retains the SetSlideRegions API. |
| [ ] Iterate | Move slide-break detection into wrapper. Frame retains a `SetSlideRegions` setter. `adjustLayoutForSlides` either stays (if option a) or moves (b). | — | — |
| [ ] Commit | — | — | `Move slide-break detection to rich/mdrender wrapper` |

## Phase 1.5: Route RichText preview mode through the wrapper

`RichText` currently constructs a `rich.Frame` directly. After
this row, preview mode constructs a `rich.Frame` AND a
`mdrender.Renderer` wrapping it; preview-mode `Redraw` calls
the wrapper. Styled mode keeps using the lean `rich.Frame`
directly (styled mode never used the moved paint phases anyway).
This row is the load-bearing wiring change — after it, the
internal markdown path goes through the wrapper end-to-end.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Confirm styled-mode path doesn't call any of the moved paint phases | `wind.go:initStyledMode`, `wind.go:HandleStyledMouse`, `richtext.go:Redraw` | Styled mode uses Fg/Bg/Bold/Italic only; no blockquote/HRule/slide. Safe to bypass wrapper. |
| [ ] Tests | End-to-end test: preview mode renders blockquote/HRule/slide content correctly via wrapper | — | Use existing markdown test fixtures. |
| [ ] Iterate | `RichText.Init` constructs `mdrender.Renderer` for preview mode. `RichText.Redraw` delegates to wrapper or frame depending on mode. | `richtext.go`, `wind.go:initPreviewMode` | Mode flag may live on RichText or be inferred from caller; design at this row. |
| [ ] Commit | — | — | `Route preview mode through rich/mdrender wrapper` |

## Phase 1.6: Finalize geometry ownership (P1-6 = D)

After 1.5, `RichText` is the sole `SetRect` caller. Drop the
`rect` parameter from `rich.Frame.Init`. The frame's rectangle
is set exclusively via `SetRect` from the wrapper / `RichText`.
Removes the dual-ownership ambiguity the architect flagged.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Audit all `rich.Frame.Init` callers (production + test) | `grep "frame\\.Init\\|Init(image\\." --include="*.go"` | Most call sites pass the rect; they all switch to `Init()` + `SetRect(rect)`. |
| [ ] Tests | Update existing rich tests to use the new Init signature | — | Mechanical; should be a find-and-replace. |
| [ ] Iterate | Drop `rect` from `Init`; update callers; add a doc note that `SetRect` is the only geometry setter | `rich/frame.go:Init`, `rich/options.go` if needed | — |
| [ ] Commit | — | — | `Drop rect from rich.Frame.Init; SetRect is the sole geometry setter` |

---

## After Phase 1

Phase 1 leaves the system in this state:

- `rich.Frame` no longer paints blockquote bars, horizontal rules,
  or slide-fill regions. Those move to `rich/mdrender`.
- `RichText` (preview mode) uses `rich/mdrender`. Styled mode
  bypasses to `rich.Frame` directly.
- `rich.Style` still carries markdown-specific fields. Removing
  them is per-round work in Phase 3.
- `markdown/` package interface unchanged.
- Spans protocol (`StyleAttrs`) unchanged.

**Phase 2** then begins: build `md2spans` v1 against the *current*
spans protocol. That work has its own design doc and plan.

---

## Risks for Phase 1

1. **Test fixture coverage gaps.** Existing rich-side tests cover
   blockquote/HRule/slide rendering. If wrapper-driven rendering
   subtly differs (e.g., draw-call ordering), tests should catch
   it — but only if we run them on the FULL preview path, not
   just the wrapper in isolation. Each row's test stage must
   include the integrated path.
2. **Slide layout adjustment is layout-touching.** Phase 1.4 will
   need a clean abstraction so the wrapper drives slide-region
   expansion without owning the whole layout pipeline. Wrong cut
   here forces a bigger change in 1.5.
3. **Mode flag plumbing.** Phase 1.5 needs `RichText` to know
   whether it's in preview vs. styled mode. The current code has
   this knowledge in `Window` but not directly in `RichText`. The
   cleanest answer is probably an explicit flag on `RichText.Init`
   options; design at row 1.5.
4. **Backward-compat of Init signature.** Phase 1.6 drops a
   parameter. Any code outside this branch that calls `Init(rect, ...)`
   breaks. The surface is contained to `rich/` callers + a few in
   `richtext.go`; small impact.

---

## Status

Phase 1 not yet started. This plan is the next thing to be agreed.
