# Architecture Decoupling — Plan

Two-tier refactor to separate edwood's rich-rendering subsystems into
reusable pieces. **Each tier's plan will be reviewed and revised
after the previous tier ships.** Tier 1 is fully detailed; Tier 2 has
skeletal rows that need fleshing out once Tier 1's deletion has
exposed the true shape of the spans/ extraction.

**Base design**: [`docs/designs/features/architecture-decoupling.md`](../designs/features/architecture-decoupling.md).

**Branches** (one per tier):
- Tier 1: `decouple/delete-markdown-preview`
- Tier 2: `decouple/spans-package` (branch off master after Tier 1
  merges)

---

## Tier 1 — Delete internal markdown preview mode

**Outcome**: in-tree markdown parser and preview mode removed; ~5k
impl + ~18k tests deleted; `wind.go` shrinks by ~1,200 lines; mouse
handling collapses to a single styled-mode handler.

**Mode after Tier 1**: only Plain ↔ Styled remain. `.md` files render
through md2spans (file-hook auto-launch). Source markers stay visible
in styled mode (this is the visible user-facing change).

**Files removed** (in their entirety):
- `markdown/` package — every `.go` file, every `*_test.go`.
- `wind/preview.go`.

**Files modified**:
- `wind.go`: remove preview-mode functions and fields (see design doc
  for the explicit list of ~25 functions and 4+ fields).
- `exec.go`: remove `previewcmd`, `previewExecute`,
  `pickPlainViewportAnchor`, the `Markdown` builtin row.
- `text.go`: remove the few preview-mode references.
- `richtext.go`: remove the small preview-side hooks.
- `acme.go`: remove `HandlePreviewMouse` dispatch branch.
- `wind_test.go` etc.: delete `TestPreview*` / `TestMarkdown*` /
  `TestSourceMap*` / `TestLinkMap*` / `TestStitch*` test functions.
- `richtext_test.go`, `wind_async_image_test.go`,
  `wind_selection_test.go`, `wind_incremental_test.go`,
  `wind/window_test.go`: delete preview-mode test cases.
- Remove `markdown` import statements from all the above.

**Files retained but reviewed**:
- `rich/mdrender/` — paint phases stay; they serve md2spans-driven
  styled rendering. Confirm during Tier 1.0 design pass that nothing
  here was preview-only.

### Tier 1.0: Plan + design pass

| Stage | Description | Read | Notes |
|---|---|---|---|
| [ ] Design | Confirm `rich/mdrender/` is preview-mode-free; map every preview-side symbol in `wind.go` / `exec.go` / `text.go` to a deletion target; identify the smallest set of intermediate compile-passing states for incremental commits. | base doc § "Tier 1" + `wind.go` / `exec.go` / `text.go` | Decisions to land: (a) is `rich/mdrender/` retained as-is, refactored, or moved? — likely retained; (b) are the slide-deck rendering changes acceptable? — confirm by smoke test. |
| [ ] Tests | n/a (planning) | — | — |
| [ ] Iterate | This plan + revised design. | — | This file. |
| [ ] Commit | — | — | `docs: revise architecture-decoupling design and plan to two-tier shape` |

### Tier 1.1: Smoke test slide rendering through md2spans

| Stage | Description | Read | Notes |
|---|---|---|---|
| [ ] Design | Open `slides.md` with internal preview off (so md2spans renders it). Inspect each slide; note any rendering regressions deemed unacceptable; if any, redesign before deletion. | `slides.md` | This is a go/no-go gate. If md2spans rendering of the slides is broken in a critical way, fix in md2spans first OR reverse the decision to delete preview mode. |
| [ ] Tests | n/a (manual smoke). | — | — |
| [ ] Iterate | If md2spans gaps are found, file or fix them; otherwise proceed. | — | Decision recorded in plan with date. |
| [ ] Commit | — | — | (only if md2spans changes happen) — TBD |

### Tier 1.2: Delete `Markdown` tag command + `previewcmd` wrappers

| Stage | Description | Read | Notes |
|---|---|---|---|
| [ ] Design | The smallest deletion that still compiles. Remove the `Markdown` builtin row in `exec.go`'s table; delete `previewcmd`, `previewExecute`, `pickPlainViewportAnchor`. Stop here for one commit, even though `wind.go` still has preview-mode infrastructure. | `exec.go:79–80, 1204+, 1399+` | Compiles because `wind.go` still defines `IsPreviewMode` etc. — they just have no callers from `exec.go` after this row. |
| [ ] Tests | Delete `exec_test.go` test cases that exercise the `Markdown` command. Run full test suite. | `exec_test.go` | — |
| [ ] Iterate | Delete the code; run tests. | — | — |
| [ ] Commit | — | — | `Remove Markdown tag builtin and previewcmd entry points` |

### Tier 1.3: Delete preview-mode tests

| Stage | Description | Read | Notes |
|---|---|---|---|
| [ ] Design | Remove `TestPreview*` / `TestMarkdown*` / `TestSourceMap*` / `TestLinkMap*` / `TestStitch*` tests across `wind_test.go`, `wind_async_image_test.go`, `wind_selection_test.go`, `richtext_test.go`, `wind/window_test.go`. Tests still pass because the underlying production code still exists; we're just deleting their test coverage. | the test files | This row is deletion-only. Production code unchanged. |
| [ ] Tests | After deletion, full suite green. | — | — |
| [ ] Iterate | Delete tests. | — | — |
| [ ] Commit | — | — | `Remove preview-mode tests in preparation for preview-mode deletion` |

### Tier 1.4: Delete `HandlePreviewMouse` dispatch + preview-mode mouse path

| Stage | Description | Read | Notes |
|---|---|---|---|
| [ ] Design | Remove the dispatch branch in `acme.go` that routes to `HandlePreviewMouse` when `IsPreviewMode()`. Remove `HandlePreviewMouse`, `HandlePreviewKey`, `HandlePreviewType`, `previewTypeFinish`, `ShowInPreview`, `scrollPreviewToMatch`, `previewHScrollLatch` (or rename if it's used by styled-mode too). Verify mouse paths after deletion don't reference preview-only fields. | `acme.go`, `wind.go:HandlePreview*` | `previewHScrollLatch` is misleadingly named — it's used by both modes. Either rename it or keep as-is; design pass decides. |
| [ ] Tests | Run full suite; smoke test mouse handling on a styled-mode window. | — | — |
| [ ] Iterate | Delete mouse-side code; rename if needed. | — | — |
| [ ] Commit | — | — | `Remove HandlePreviewMouse and the styled/preview mouse-dispatch branch` |

### Tier 1.5: Delete preview-mode state on `Window` + accessors

| Stage | Description | Read | Notes |
|---|---|---|---|
| [ ] Design | Remove `previewMode` flag, `previewLastRender`, `previewRenderPending`, `previewDebounceTimer` fields from `Window`. Remove `IsPreviewMode`, `SetPreviewMode`, `TogglePreviewMode`, `SetPreviewSourceMap` / `PreviewSourceMap`, `SetPreviewLinkMap` / `PreviewLinkMap`, `PreviewLookLinkURL`, `recordEdit`, `updatePreviewModel`, `UpdatePreview`, `PreviewSnarf`, `PreviewLookText`, `PreviewExecText`, `PreviewExpandWord`. Remove `wind/preview.go` entirely. | `wind.go`, `wind/preview.go` | At this point `Window` no longer imports `markdown`. |
| [ ] Tests | Full suite green. | — | — |
| [ ] Iterate | Delete fields and methods; remove `wind/preview.go`. | — | — |
| [ ] Commit | — | — | `Remove preview-mode state and accessors from Window` |

### Tier 1.6: Delete `markdown/` package

| Stage | Description | Read | Notes |
|---|---|---|---|
| [ ] Design | Confirm zero remaining imports of `markdown` from main package or any subpackage. `git rm -r markdown/`. | repository-wide grep | If any import remains, the deletion in 1.5 was incomplete; back out and fix. |
| [ ] Tests | Full suite green. | — | — |
| [ ] Iterate | `git rm -r markdown/`. | — | — |
| [ ] Commit | — | — | `Remove markdown package` |

### Tier 1.7: Cleanup + Tier-2 plan revision

| Stage | Description | Read | Notes |
|---|---|---|---|
| [ ] Design | Audit `wind.go` and `exec.go` for any remaining preview-mode debris (comment references, unused imports, dead helpers). Update `Tier 2` rows below based on what the now-cleaner `wind.go` reveals about the spans bridge — particularly any helpers we thought we'd extract that turn out to be unnecessary, or vice-versa. **This is mandatory.** | this file, `wind.go`, `exec.go` | The Tier 2 plan was sketched against pre-deletion `wind.go`; some of its row content may be stale. |
| [ ] Tests | n/a (planning) | — | — |
| [ ] Iterate | Final cleanup commit + revise this file's Tier 2 section. | — | — |
| [ ] Commit | — | — | `docs: cleanup post-deletion debris and revise tier-2 plan` |

---

## Tier 2 — Extract `spans/` package (REVISION EXPECTED AFTER TIER 1)

**Outcome**: spans-protocol code (parser, stores, render bridge) lives
in its own Go package. `wind.go` shrinks by ~300 LOC. `cmd/md2spans`,
`cmd/edcolor`, `cmd/dirthumb` optionally gain a Go-API import.

**The rows below are SKELETAL.** They will be revised after Tier 1
ships and we can see the spans bridge clearly without preview-mode
noise around it.

**Files moved** (target paths in parens):
- `spanparse.go` → `spans/parse.go`
- `spanparse_test.go` → `spans/parse_test.go`
- `spanstore.go` → `spans/store.go`
- `spanstore_test.go` → `spans/store_test.go`
- `region.go` → `spans/region.go`
- `region_test.go` → `spans/region_test.go`
- Bridge functions extracted from `wind.go` into `spans/render.go`:
  `styleAttrsToRichStyle`, `boxStyleToRichStyle`, `applyImagePayload`,
  `applyEnclosingRegions`, `applyCodeRegion`, `applyBlockquoteRegion`,
  `applyListitemRegion`, `applyTableRegion`, `applyTableRowRegion`,
  `applyTableCellRegion`, `ancestorsOuterFirst`,
  `buildStyledContent`, `styleSubRun`. Tests move accordingly.

**Files modified**:
- `wind.go`: `applyParsedSpans` becomes a thin wrapper around
  `spans.ParseMessage`; `buildStyledContent` callers go through
  `spans.Render(...)`; `clearSpansAndRegions` calls `spans.Store.Clear`
  / `spans.RegionStore.Clear`.
- `xfid.go:xfidspanswrite`: unchanged shape, calls into `spans.`
  package.
- `cmd/md2spans/`, `cmd/edcolor/`, `cmd/dirthumb/`: optional — they
  may import `spans/` for shared types instead of redefining
  Span/SpanKind/etc. locally. **Decision in Tier 2.0 design pass.**

### Tier 2.0: Design pass + Tier 1 retrospective integration

| Stage | Description | Read | Notes |
|---|---|---|---|
| [ ] Design | Confirm `spans/` package boundary + public API shape after Tier 1 has shipped. Identify any helpers freshly visible as bridge code (or no longer needed) post-deletion. Decide whether `cmd/*` tools migrate to import `spans/` now or in a follow-up. | tier-1 retrospective | Decisions to land: (a) public type names (`Store` vs `SpanStore`); (b) `Render` function signature (does it take a `BodyReader` interface or a rune slice?); (c) whether `cmd/*` tools migrate now. |
| [ ] Tests | n/a (planning). | — | — |
| [ ] Iterate | Revise this Tier 2 section based on findings. | — | — |
| [ ] Commit | — | — | `docs: revise tier-2 plan after tier-1 retrospective` |

### Tier 2.1: Create empty `spans/` package shell

| Stage | Description | Read | Notes |
|---|---|---|---|
| [ ] Design | Empty package with `doc.go` that names the future contents. | base doc | One file; nothing else. |
| [ ] Tests | n/a — file is empty package decl + comment. | — | — |
| [ ] Iterate | Add `spans/doc.go`. | — | — |
| [ ] Commit | — | — | `spans: add empty package` |

### Tier 2.2: Move parse.go + tests

| Stage | Description | Read | Notes |
|---|---|---|---|
| [ ] Design | Verify `spanparse.go` has zero edwood-specific imports beyond `rich/`. | `spanparse.go` | If imports surface dependencies on main-package types, lift those to public `spans/` types in this row's design pass. |
| [ ] Tests | Existing `spanparse_test.go` should continue passing after move. | `spanparse_test.go` | — |
| [ ] Iterate | `git mv spanparse.go spans/parse.go && git mv spanparse_test.go spans/parse_test.go`; rename package; update imports in main package; build + test. | — | Use `git mv` to preserve history. |
| [ ] Commit | — | — | `spans: move spanparse to spans package` |

### Tier 2.3: Move store + region

| Stage | Description | Read | Notes |
|---|---|---|---|
| [ ] Design | Same import audit for store.go and region.go. | `spanstore.go`, `region.go` | Region tree may have helpers that touch types we didn't expect; check during design pass. |
| [ ] Tests | Existing tests carry over. | `spanstore_test.go`, `region_test.go` | — |
| [ ] Iterate | Move files; rename package; update imports. | — | — |
| [ ] Commit | — | — | `spans: move store and region to spans package` |

### Tier 2.4: Extract render bridge from `wind.go`

| Stage | Description | Read | Notes |
|---|---|---|---|
| [ ] Design | Move `buildStyledContent`, `styleSubRun`, and the apply* family. Decide whether `Render` takes the body as a rune-reader interface or a rune slice. | `wind.go` post-Tier-1 | This is the row most likely to expose hidden coupling — these functions reach into `Window` for `body.file.Read` etc. Need a small adapter for `BodyReader`. |
| [ ] Tests | New `spans/render_test.go` covering the conversion paths. | — | Tests may already exist as Window-level tests; copy or move. |
| [ ] Iterate | Move functions; replace Window-side callers with `spans.Render(w.spanStore, w.regionStore, &windowBody{w})` or similar. | — | — |
| [ ] Commit | — | — | `spans: move render bridge from wind.go to spans package` |

### Tier 2.5: Final cleanup + documentation

| Stage | Description | Read | Notes |
|---|---|---|---|
| [ ] Design | Audit `wind.go` post-move for remaining direct rich.* references that should also live in spans/. Optionally migrate `cmd/*` tools to import `spans/` types. | — | — |
| [ ] Tests | Full `go test ./...` green. | — | — |
| [ ] Iterate | Remove dead code; update `cmd/*` tools optionally; update `docs/designs/spans-protocol.md` references. | — | — |
| [ ] Commit | — | — | `spans: final cleanup after tier-2 extraction` |
