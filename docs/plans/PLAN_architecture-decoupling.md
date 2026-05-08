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
| [x] Design | See "Tier 2.0 design output" below. Audit clean — zero subpackage spans-related types; all bridge-function callers confined to wind.go; spanparse.go / spanstore.go / region.go have zero edwood-package imports. | tier-1 retrospective | Cleaner than feared. |
| [x] Tests | n/a (planning). | — | — |
| [x] Iterate | Locked decisions in the section below. | — | — |
| [ ] Commit | — | — | `docs: tier-2 design output — clean extraction confirmed` |

#### Tier 2.0 design output (2026-05-08)

**Subpackage audit.** Zero spans-related references in `wind/`,
`command/`, or `internal/`. Tier 1.5 already deleted the only spans-
adjacent code in subpackages (`wind/preview.go`,
`command/preview_cmds*`).

**Call-graph audit.** Every candidate bridge function in `wind.go` is
called only from `wind.go`:
`buildStyledContent`, `styleSubRun`, `styleAttrsToRichStyle`,
`boxStyleToRichStyle`, `applyImagePayload`, `applyEnclosingRegions`,
`applyCodeRegion`, `applyBlockquoteRegion`, `applyListitemRegion`,
`applyTableRegion`, `applyTableRowRegion`, `applyTableCellRegion`,
`ancestorsOuterFirst`, `tryAddRegion`. **No cross-package callers**;
no Tier-1-style "misnamed cross-mode helper" surprises expected.

**Imports of the three leaf files.** `spanparse.go` / `spanstore.go` /
`region.go` import only stdlib (`fmt`, `image/color`, `strconv`,
`strings`). Zero edwood-package imports. Move-as-is.

**Public API.** Settled decisions:

| Public name | Replaces | Rationale |
|---|---|---|
| `spans.Store` | `SpanStore` | Drop the redundant prefix; package qualifies it. |
| `spans.RegionStore` | `RegionStore` | Same. |
| `spans.Region` | `Region` | Same. |
| `spans.StyleRun` | `StyleRun` | Same. |
| `spans.StyleAttrs` | `StyleAttrs` | Same. |
| `spans.NewStore()` | `NewSpanStore()` | — |
| `spans.NewRegionStore()` | `NewRegionStore()` | — |
| `spans.ParseMessage(data, bufLen)` | `parseSpanMessage` | Capitalize for export. |
| `spans.Render(body []rune, *Store, *RegionStore) rich.Content` | `(*Window).buildStyledContent` | Signature: `[]rune` body, not `BodyReader` — body is small, copy-once. |

Helpers stay package-private: `parseSpanLine`, `parseBoxLine`,
`parsePlacementFlag`, `parseScaleFlag`, `parseFamilyFlag`, `parseColor`,
`fillGaps`, etc.

**`cmd/*` tools.** md2spans, edcolor, dirthumb keep their local Span
types — defer the optional migration to a follow-up change. Tier 2 is
already large; not bundling.

**Move order** (each row is one commit per CODING-PROCESS):

1. **2.1**: empty `spans/` package + this audit recorded. (Optional
   commit; can fold into 2.2 if minimal.)
2. **2.2**: move `spanparse.go` + tests.
3. **2.3**: move `spanstore.go` + `region.go` + tests.
4. **2.4**: extract `Render` from `wind.go` plus the apply* family;
   thin window wrapper around `spans.Render`.
5. **2.5**: cleanup — orphan helpers, stale comments, doc updates.

Estimated impact: ~1,400 LOC moved into `spans/`; ~300 LOC of bridge
code leaves `wind.go`; main-package public surface keeps the same
behavior.

### Tier 2.1: Create `spans/` package shell + subpackage audit

| Stage | Description | Read | Notes |
|---|---|---|---|
| [x] Design | Audit clean. doc.go added. | — | — |
| [x] Tests | n/a. | — | — |
| [x] Iterate | `spans/doc.go` describes scope; subpackage audit recorded clean in 2.0. | — | — |
| [x] Commit | — | `ec3d97f` | `spans: add empty package` |

### Tier 2.2 + 2.3: Move parse + store + region (combined)

The leaf files were inseparable in practice — `spanparse.go`
references types defined in `spanstore.go`/`region.go`, so a
stagewise split would have required temporary cross-package
references that the next commit immediately undoes. Combined
into one commit per the row-level CODING-PROCESS Stage 4 rule
"Wrong design — update plan first."

| Stage | Description | Read | Notes |
|---|---|---|---|
| [x] Design | Combined-move. `parseSpanMessage` exposed as `spans.Parse` with internal format-detection dispatch (covers prefixed + legacy). `SpanStore` → `spans.Store` (drop redundant prefix). | — | Decided during execution; plan revised. |
| [x] Tests | Existing tests carried over. | — | — |
| [x] Iterate | `git mv` of all six files, package decl, public-API renames, qualifying refs in main pkg. | — | — |
| [x] Commit | — | `a71ec67` | `spans: move parse + store + region to spans package` |

### Tier 2.4: Extract render bridge from `wind.go`

| Stage | Description | Read | Notes |
|---|---|---|---|
| [x] Design | `spans.Render(body []rune, *Store, *RegionStore) rich.Content` chosen over a BodyReader interface — body is small, copy-once is simpler. `spans.TryAddRegion` exposed for `applyParsedSpans`. Apply-* family + style translators stay private. | — | — |
| [x] Tests | 26 unit tests for the helpers moved alongside their code into `spans/render_test.go`. Integration tests for the wider styled-mode flow stayed in `wind_styled_test.go`. | — | — |
| [x] Iterate | Bridge functions moved; wind.go's `buildStyledContent` reduces to a one-line call to `spans.Render`. wind.go shrinks 1942 → 1559 LOC. | — | — |
| [x] Commit | — | `06d15cb` | `spans: move render bridge from wind.go to spans package` |

### Tier 2.5: Final cleanup + documentation

| Stage | Description | Read | Notes |
|---|---|---|---|
| [x] Design | Stale comments in wind.go updated (two refs to `styleAttrsToRichStyle` rewritten to point at `spans.Render`). Orphan `isNearEnd` deleted (preview-mode tail-follow helper, unreferenced after Tier 1.5). `docs/designs/spans-protocol.md` paths refreshed. cmd/* migration deferred. | — | — |
| [x] Tests | Full `go test ./...` green; `spans/` package tests pass. | — | — |
| [x] Iterate | Stale-comment fixes + `isNearEnd` delete + spans-protocol.md path refresh. | — | — |
| [ ] Commit | — | — | `spans: tier 2 final cleanup — stale comments + orphan helper + doc refresh` |
