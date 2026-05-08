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
| [x] Design | Audited `wind.go`, `text.go`, `globals.go`, `acme.go` for residual preview-mode debris. Updated Tier 2 rows below based on what the now-cleaner `wind.go` reveals about the spans bridge. | this file, `wind.go`, `exec.go` | See "Tier 1 retrospective" below. |
| [x] Tests | n/a (planning) | — | — |
| [x] Iterate | Final cleanup commit + revise this file's Tier 2 section. | — | — |
| [ ] Commit | — | — | `docs: tier-1 retrospective and tier-2 plan revision` |

---

## Tier 1 retrospective (2026-05-08)

Tier 1 shipped in eight commits, removing ~29,800 net LOC (804 added,
30,615 deleted) across 37 files. Lessons that informed the Tier 2
revision below:

### Lesson 1: misnamed cross-mode helpers

**Three helpers prefixed `preview*` actually served styled mode too**.
Each was identified during deletion when the build broke at a
HandleStyledMouse caller:

| Original name | Renamed to | Why |
|---|---|---|
| `previewExecute` | `richExecute` | B2-execute dispatcher used by both modes |
| `previewHScrollLatch` | `hscrollLatch` | Block-region scrollbar latching, mode-agnostic |
| `scrollPreviewToMatch` | `scrollRichToMatch` | Generic scroll-to-rendered-position |
| `previewClickRT/Pos/Msec` (3 fields) | `richClickRT/Pos/Msec` | Double-click tracking on rich body |

**Implication for Tier 2.** The spans-bridge functions in `wind.go`
(`buildStyledContent`, `styleSubRun`, `styleAttrsToRichStyle`,
`boxStyleToRichStyle`, `applyImagePayload`, the six `apply*Region`
functions) appear well-named for their role, but Tier 2's audit row
should still verify each function's caller graph before extracting —
a function named for a single concern may be reached from another.

### Lesson 2: vestigial subpackages I missed in the Tier 1.0 enumeration

Two whole-file deletions surfaced only when the build broke:
- `wind/preview.go` (`PreviewState` struct + `NewPreviewState()`,
  composed into `WindowBase`. No external caller.)
- `command/preview_cmds.go` + `command/preview_cmds_test.go` (an
  abstract dispatcher for the `Markdown` command. The `command/`
  package is never imported by anything in the repo.)

Both were planned-but-not-wired infrastructure. Tier 1.0's enumeration
walked the main package only.

**Implication for Tier 2.** Add an explicit subpackage audit to
Tier 2.0. Specifically check `wind/`, `command/`, and `internal/`
for any spans-related types that would need to follow the extraction
or be deleted as vestigial.

### Lesson 3: test deletion regex needed two passes

`^TestPreview...` missed `TestHandlePreview...`, `TestWindowPreviewMode`,
`TestStyledAndPreviewMutuallyExclusive`, etc. The `^Test[A-Za-z]*Preview...`
pattern caught everything on the second pass.

**Implication for Tier 2.** Tier 2 has no bulk test-deletion phase; the
existing tests for `spanparse`, `spanstore`, `region` move with their
files. The pattern lesson doesn't carry forward, but the meta-lesson
(verify with a wider net before declaring sweep complete) does.

### Lesson 4: dead helpers cluster around dead features

Removing `transformForPaste` exposed `stripMarkers`, `stripHeadingPrefix`,
`wrapWith` as orphan helpers (preview-only callers, nothing else).
`updateSelectionContext` exposed `analyzeSelectionContent`,
`classifyStyle`, `clampToBuffer`, `isWordChar`. `SelectionContext` /
`snarfContext` were preview-only types nothing else referenced.

**Implication for Tier 2.** After moving the spans bridge functions
to `spans/`, audit `wind.go` for orphan helpers that only the moved
functions called. Delete or move them too.

### Lesson 5: stale comments outlive their referents

Doc-comments referenced deleted symbols (`previewcmd`,
`HandlePreviewMouse`, etc.) for ~12 places. None broke compilation;
they just reduced the doc to misinformation. A `grep -i preview`
sweep at the end of Tier 1 caught them.

**Implication for Tier 2.** End-of-tier sweep should include a
reverse search: do any remaining comments mention symbols that
moved to `spans/`?

### Lesson 6: doc-comments occasionally lie about scope

`imageCache` was documented as "for preview mode" but was shared
with styled mode through `addImageRichTextOptions`. The comment
predated the styled-mode work and never got updated. The ground
truth was the call graph, not the comment.

**Implication for Tier 2.** When deciding whether a function or
field is in-scope for `spans/` extraction, verify by call graph,
not by godoc. (This was already implicit in the plan; it's now
explicit.)

### Quantitative Tier 1 outcome (vs Tier 1.0 estimate)

| Item | Tier 1.0 estimate | Actual |
|---|---:|---:|
| Impl LOC removed | ~5,360 | ~6,000 |
| Test LOC removed | ~19,150 | ~24,600 |
| Total LOC removed | ~24,500 | ~30,600 |
| Files touched | (not counted) | 37 |
| `wind.go` size after | (not estimated) | 1,720 (was ~3,400) |

The actual is ~25% larger than estimated, primarily from the two
vestigial subpackages and the wider test-deletion regex catching
more files.

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
| [ ] Design | Confirm `spans/` package boundary + public API shape now that the markdown package is gone. Decide: (a) public type names (`Store` vs `SpanStore`); (b) `Render` function signature — `BodyReader` interface or `[]rune`? Lean toward `[]rune` since the body is small and copy-once is simpler than maintaining an interface; (c) whether `cmd/*` tools (md2spans, edcolor, dirthumb) migrate to import `spans/` now or follow up. **Recommend NOT migrating cmd/* this tier** — they have small local Span types that work, and a same-tier migration would balloon the diff. | tier-1 retrospective | The retrospective revealed that the spans-bridge functions in `wind.go` aren't entangled with preview-mode (preview was its own parser path). This makes Tier 2 a cleaner extraction than originally feared. |
| [ ] Tests | n/a (planning). | — | — |
| [ ] Iterate | Revise the row notes below if 2.0's design pass surfaces unexpected coupling. | — | — |
| [ ] Commit | — | — | `docs: tier-2 design pass — confirm spans/ public API` |

### Tier 2.1: Create `spans/` package shell + subpackage audit

| Stage | Description | Read | Notes |
|---|---|---|---|
| [ ] Design | Empty package with `doc.go`. **Per Tier 1 lesson 2:** also audit `wind/`, `command/`, `internal/` for spans-related types that might need to follow or be deleted as vestigial. Tier 1 found two vestigial subpackages this way; Tier 2 should not assume the audit is unnecessary. | `wind/`, `command/`, `internal/` | If any vestigial spans-related code is found, file as a row before 2.2 to delete it cleanly. |
| [ ] Tests | n/a. | — | — |
| [ ] Iterate | Add `spans/doc.go`; record any subpackage-audit findings here. | — | — |
| [ ] Commit | — | — | `spans: add empty package + subpackage audit` |

### Tier 2.2: Move parse.go + tests

| Stage | Description | Read | Notes |
|---|---|---|---|
| [ ] Design | Verify `spanparse.go` has zero edwood-specific imports beyond `rich/`. **Per Tier 1 lesson 6:** check by call graph, not by godoc — any helper that LOOKS spans-only but is called from elsewhere needs special handling (rename or split). | `spanparse.go` | The existing imports are limited based on Tier 1's earlier inspection. Worth a fresh check post-Tier-1. |
| [ ] Tests | Existing `spanparse_test.go` should continue passing after move. | `spanparse_test.go` | — |
| [ ] Iterate | `git mv spanparse.go spans/parse.go && git mv spanparse_test.go spans/parse_test.go`; rename package; update imports in main package. | — | Use `git mv` to preserve history. |
| [ ] Commit | — | — | `spans: move spanparse to spans package` |

### Tier 2.3: Move store + region

| Stage | Description | Read | Notes |
|---|---|---|---|
| [ ] Design | Same import + call-graph audit for `spanstore.go` and `region.go`. | `spanstore.go`, `region.go` | The `region.go` file has helpers like `tryAddRegion`, `ancestorsOuterFirst` that may or may not move — check during design. |
| [ ] Tests | Existing tests carry over. | `spanstore_test.go`, `region_test.go` | — |
| [ ] Iterate | Move files; rename package; update imports. | — | — |
| [ ] Commit | — | — | `spans: move store and region to spans package` |

### Tier 2.4: Extract render bridge from `wind.go`

| Stage | Description | Read | Notes |
|---|---|---|---|
| [ ] Design | Move `buildStyledContent`, `styleSubRun`, and the `apply*Region` family (`applyEnclosingRegions`, `applyCodeRegion`, `applyBlockquoteRegion`, `applyListitemRegion`, `applyTableRegion`, `applyTableRowRegion`, `applyTableCellRegion`, `ancestorsOuterFirst`), `styleAttrsToRichStyle`, `boxStyleToRichStyle`, `applyImagePayload`. **Per Tier 1.0 spec choice:** `Render([]rune body, *Store, *RegionStore) rich.Content`. | `wind.go` post-Tier-1 | These functions reach into `Window` for `body.file.Read`. The cleanest signature is to take a `[]rune` body slice (read once at call site) rather than an interface. |
| [ ] Tests | New `spans/render_test.go` covering the conversion paths. Existing tests at the Window level either move (if pure data-driven) or stay (if exercising the wider styled-mode flow). | — | Tier 1 didn't preserve test counts here; review existing wind_styled_test.go for tests that should move. |
| [ ] Iterate | Move functions; replace Window-side callers with `spans.Render(body, w.spanStore, w.regionStore)`. | — | — |
| [ ] Commit | — | — | `spans: move render bridge from wind.go to spans package` |

### Tier 2.5: Final cleanup + documentation

| Stage | Description | Read | Notes |
|---|---|---|---|
| [ ] Design | **Per Tier 1 lessons 4 & 5:** after extraction, audit `wind.go` for orphan helpers whose only callers were the moved functions, and grep stale doc-comments mentioning moved symbols (`buildStyledContent`, `styleSubRun`, etc.) for cleanup. Decide whether to optionally migrate `cmd/*` tools to import `spans/` types (recommended: defer to a separate change). | — | — |
| [ ] Tests | Full `go test ./...` green. | — | — |
| [ ] Iterate | Remove dead code; clean up stale comments; update `docs/designs/spans-protocol.md` references. | — | — |
| [ ] Commit | — | — | `spans: final cleanup after tier-2 extraction` |
