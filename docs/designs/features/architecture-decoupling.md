# Architecture Decoupling — Two-Tier Design

## Purpose

Tighten the boundaries between edwood's rich-rendering subsystems so each
becomes an independently usable piece. The original analysis identified three
subsystems (`rich/`, the spans filesystem, internal markdown preview mode) and
proposed a three-tier refactor culminating in deletion of the in-tree markdown
parser. This design takes a shortcut: **delete the markdown preview mode
upfront**, which collapses the most architecturally complex tier and leaves
two mechanical passes.

## What changed from the three-tier draft

The original draft introduced a `Producer` interface to unify the two parallel
mouse-handler stacks (styled-mode and preview-mode). That interface only
earned its keep with two implementations:

- `spans.Producer` — identity click-mapping, used by md2spans / edcolor /
  dirthumb output.
- `markdown.Producer` — `SourceMap`-backed translation, edit-time map
  maintenance, used by the in-tree `Markdown` tag command.

Removing internal markdown preview removes the second implementation. With
only one implementation, the interface adds no value — it would abstract
against zero alternatives. The "Producer" thinking is preserved as future
work in case clean-mode markdown rendering becomes a real demand signal
again, but it's no longer on the critical path.

## Cost of dropping preview mode

Visible to users:

- The `Markdown` tag builtin disappears.
- `.md` files render through md2spans (the file-hook auto-launches it).
  Source markers stay visible: `# Heading` shows the `#`, `**bold**` shows
  the asterisks, `[label](url)` shows the link text *and* the URL syntax.
  Rendering is still styled (heading scale, bold/italic fonts, blockquote
  bars, tables, images, hrules) — only the source-hiding "clean mode" is
  gone.
- The slide deck (`docs/slides.md`) renders with markers visible. Worth
  inspecting once before committing to the path, since slides have been
  rendered in clean mode to date.

Path back if we want it: extend md2spans with a flag (or some other
expression at the spans-protocol level) for "consume these source runes
without rendering them." That feature reintroduces clean-mode rendering
*without* reintroducing an in-tree parser; it would be its own design
exercise at that point.

## Current state (2026-05-08)

Package-level dependencies form a clean DAG:

```
                draw/
                  ↑
                rich/  ←───── rich/mdrender/  ←───── (paint phases — STAY)
                  ↑                                    used by main pkg
            ┌─────┴─────────┬──────────────┐
       markdown/      spanparse.go      richtext.go
        (DELETE)      spanstore.go      (in main pkg)
                      region.go
                      (all in main pkg)
                            ↑
                       wind.go (orchestration; the entanglement lives here)
```

The entanglement in `wind.go` falls into two categories:
- **Preview-mode glue** (~1,200 LOC across ~25 functions): deleted in Tier 1.
- **Spans-protocol bridge** (~300 LOC: `applyParsedSpans`, `buildStyledContent`,
  `styleSubRun`, `boxStyleToRichStyle`, `applyImagePayload`, the six
  `apply*Region` functions): moved into a new `spans/` package in Tier 2.

After the two tiers, `wind.go` retains only what belongs in window orchestration:
mode flag (just `styledMode` + `styledSuppressed` now, no `previewMode`), 9P
endpoint glue, mouse dispatch, edit observers.

## Two-tier approach

### Tier 1 — Delete internal markdown preview mode

**Code removed**:
- `markdown/` package entirely (~3,800 impl + ~11,500 tests).
- `wind/preview.go` (~87 lines).
- ~1,200 LOC of preview-mode functions in `wind.go`: `previewcmd`,
  `IsPreviewMode`, `TogglePreviewMode`, `HandlePreviewMouse`,
  `HandlePreviewKey`, `HandlePreviewType`, `previewTypeFinish`,
  `ShowInPreview`, `scrollPreviewToMatch`, `previewHScrollLatch`, the
  `SetPreview*` / `Preview*` accessor family, `recordEdit`,
  `updatePreviewModel`, `UpdatePreview`, `PreviewSnarf`, `PreviewLookText`,
  `PreviewExecText`, `PreviewExpandWord`, `previewRenderInterval` const, and
  the `previewLastRender` / `previewRenderPending` / `previewDebounceTimer`
  fields on `Window`.
- ~243 LOC in `exec.go`: `previewcmd`, `previewExecute`,
  `pickPlainViewportAnchor`, the `Markdown` builtin row.
- Preview-mode tests across `wind_test.go` (~6,650 LOC),
  `wind_async_image_test.go` (~510), `wind_selection_test.go` (~200),
  `wind/window_test.go` (~190), `richtext_test.go` (~95).

**Code retained**:
- `rich/mdrender/` paint phases — used by md2spans-driven styled rendering
  (regions, blockquote bars, hrules). These are NOT internal-parser-specific.
- `Plain` tag builtin — still meaningful for exiting styled mode (md2spans
  flow).
- All spans-protocol code — untouched in Tier 1.

**Mouse handling after Tier 1**: only `HandleStyledMouse` remains. The
mode-dispatch branch (`if w.previewMode then ... else ...`) collapses to
straight-line code.

**Outcome**: ~5k impl + ~18k tests removed. `wind.go` shrinks by ~1,200
lines. The styledMode↔previewMode dichotomy becomes single-mode-or-plain.

### Tier 2 — Extract `spans/` package

Move the protocol-pure code into a `spans/` package:

- Wire-format parser (`spanparse.go` → `spans/parse.go`).
- Stores (`spanstore.go` → `spans/store.go`, `region.go` → `spans/region.go`).
- Bridge functions extracted from `wind.go` into `spans/render.go`:
  `styleAttrsToRichStyle`, `boxStyleToRichStyle`, `applyImagePayload`,
  `applyEnclosingRegions`, `applyCodeRegion`, `applyBlockquoteRegion`,
  `applyListitemRegion`, `applyTableRegion`, `applyTableRowRegion`,
  `applyTableCellRegion`, the `ancestorsOuterFirst` helper, plus the
  `buildStyledContent` / `styleSubRun` chain.

**Public API of `spans/`**:
- `spans.Store`, `spans.RegionStore`, `spans.StyleRun`, `spans.Region`,
  `spans.StyleAttrs` types.
- `spans.ParseMessage(data string, bufLen int) (...)` — wire-format parse.
- `spans.Render(store *Store, regions *RegionStore, body BodyReader) rich.Content`
  — pure data transformation; produces input to `rich.Frame`.

**What stays in main package**:
- The 9P endpoint (`xfid.go:xfidspanswrite`, `QWspans` qid registration).
- `Window`'s `spanStore`/`regionStore` fields (now `*spans.Store` /
  `*spans.RegionStore`).
- `applyParsedSpans` thin wrapper that calls `spans.ParseMessage` and updates
  the window's stores.
- The styled-mode trigger logic (auto-enter on first span write,
  `clearSpansAndRegions` on `c` directive).

**Outcome**: ~1,400 LOC moved into `spans/`; `wind.go` shrinks by an
additional ~300 LOC of bridge code. spans-protocol clients (md2spans, edcolor,
dirthumb) optionally gain a Go-API import alongside the wire-format access
they already have.

**Tier 2's plan will be revised after Tier 1 ships.** With preview-mode
removed, hidden coupling between the spans bridge and preview-mode helpers
should become visible in the cleanup phase of Tier 1; the Tier 2 plan should
incorporate any lessons from that.

## Cross-cutting design decisions

### What stays in `wind.go` after both tiers

Window orchestration that's genuinely window-shaped: mode-flag transitions,
mouse dispatch, the 9P qid table, edit observers feeding spans/regions back to
producers. Estimate: ~1,500 LOC remains in wind.go (down from ~3,000 today,
counting preview removal + spans extraction).

### `rich/mdrender/` future

mdrender stays for now. After tier 2 it's the only "main package consumer of
rich/" beyond direct rich.Frame use. Whether to fold its paint phases into
rich/ proper, or keep mdrender as a separate decoration package, is a
follow-up question — addressed only if we feel like it after the two tiers
land.

### Tag commands

`Plain` stays. `Markdown` deletes in Tier 1. No new tag commands introduced.

## Non-goals

- **A `Producer` interface or any in-process renderer abstraction.** Preserved
  as future-work below in case clean-mode demand returns.
- **Public Go-module versioning of `rich/` or `spans/`.** They become
  "publishable" but staying in-tree is fine.
- **Unifying `rich/mdrender/` paint phases into `rich/`.** Separate concern.
- **Performance work.** Pure structural refactor.
- **Reintroducing clean-mode markdown rendering by some other path.** If/when
  it happens, it's an md2spans feature, not in-tree.

## Future work — if clean-mode demand returns

If users want clean-mode markdown rendering back (source markers hidden),
the path is:

1. Extend the spans protocol with a way to express "consume these source
   runes without rendering them." Likely a `hidden` flag on `s` directives,
   plus careful caret-mapping work (the same `SourceMap` shape that lived
   in-tree, now produced by md2spans).
2. Add the corresponding mode to md2spans.
3. Optionally introduce a `Producer` interface inside edwood at this point,
   with two implementations (today's identity-mapping and a hide-mode that
   needs source-map plumbing). Justified now because there *are* two
   implementations.

This was the original Tier 3 spike. Deferred until concrete demand signal,
not on the critical path of the architecture cleanup.

## Risks

| Risk | Tier | Mitigation |
|---|---|---|
| Slide deck rendering changes are unacceptable | 1 | Inspect `slides.md` rendered through md2spans before merging Tier 1. Keep clean-mode option open via the future-work path if disliked. |
| Hidden coupling between preview-mode tests and styled-mode tests | 1 | Run the full test suite after each deletion sub-step, not just at the end. |
| `spans/` extraction surfaces a function that secretly needed `markdown` types | 2 | Tier 2 starts with an audit row that checks all extracted functions' import requirements. Caught early, fixed by promoting types to `spans/`. |

## Addendum — Tier 1 deletion targets (enumerated)

Produced 2026-05-08 from a code sweep on commit `86d05c1`. Line numbers are
authoritative for that snapshot; they shift as deletions land. The
intermediate row commits below correspond to plan rows 1.2–1.6.

### Whole-file deletes
- `wind/preview.go` — 87 LOC.
- `markdown/` package — 13 `.go` files; ~3,805 impl + ~11,511 tests
  (per the count we did earlier).

### `Window` struct fields (in `wind.go`, ~line 74–93)
1. `previewMode bool`
2. `previewSourceMap *markdown.SourceMap`
3. `previewLinkMap *markdown.LinkMap`
4. `previewClickPos int`
5. `previewClickMsec uint32`
6. `previewClickRT *RichText`
7. `prevBlockIndex *markdown.BlockIndex`
8. `pendingEdits []markdown.EditRecord`
9. `previewLastRender time.Time`
10. `previewRenderPending bool`
11. `previewDebounceTimer *time.Timer`

`imageCache *rich.ImageCache` **stays** — used by both `initStyledMode` and
`previewcmd`. (The doc-comment claiming "preview mode" is stale; the cache
is shared with styled mode through `addImageRichTextOptions`.)

### `Window` methods to delete (in `wind.go`)
1. `IsPreviewMode` (line 932)
2. `SetPreviewMode` (line 939)
3. `TogglePreviewMode` (line 959)
4. `HandlePreviewMouse` (line 990)
5. `previewHScrollLatch` (line 1523) — verify whether styled-mode
   `HandleStyledMouse` calls it; if so, **rename instead of delete**.
6. `ShowInPreview` (line 1592)
7. `scrollPreviewToMatch` (line 1631)
8. `SetPreviewSourceMap` (line 1694)
9. `PreviewSourceMap` (line 1699)
10. `SetPreviewLinkMap` (line 1705)
11. `PreviewLinkMap` (line 1710)
12. `PreviewLookLinkURL` (line 1718)
13. `recordEdit` (line 1726)
14. `updatePreviewModel` (line 1733)
15. `UpdatePreview` (line 1826)
16. `PreviewSnarf` (line 1860)
17. `PreviewLookText` (line 1890)
18. `PreviewExecText` (line 1918)
19. `PreviewExpandWord` (line 1926)
20. `HandlePreviewKey` (line 2003)
21. `HandlePreviewType` (line 2093)
22. `previewTypeFinish` (line 2204)
23. `previewRenderInterval` const (line 2197 area)

### Top-level functions to delete (in `exec.go`)
1. `previewExecute` (line 290)
2. `previewcmd` (line 1204)
3. `pickPlainViewportAnchor` (line 1419)
4. `pickRichViewportAnchor` (line 1435 area) — preview-only sibling.

Plus the `Markdown` row in `globalexectab` (line 80).

### Conditional preview branches in shared code
- `wind.go:365` — `Resize` `noredraw` flag uses `w.previewMode || w.styledMode`
  → simplifies to `w.styledMode`.
- `wind.go:371` — `Resize` post-resize redraw guard, same simplification.
- `wind.go:431–432` — `Free` resets `previewRenderPending` and `previewMode`;
  both lines deleted.
- `wind.go:797` — preview-only render path in `addressMode` or similar; whole
  branch deleted.
- `wind.go:971` — `RedrawIfNeeded` check, same simplification as 365/371.
- `text.go:487` — `Inserted`'s `recordEdit` call → deleted.
- `text.go:657` — `Deleted`'s `recordEdit` call → deleted.
- `text.go:1479` — `(t.w.styledMode || t.w.previewMode)` → simplifies to
  `t.w.styledMode`.

### `import` statements to remove
- `wind.go`: `"github.com/rjkroege/edwood/markdown"` (only after deleting all
  the methods that reference it).
- `text.go`: same.
- `exec.go`: same.
- `wind/preview.go`: file gone.
- All test files that import `markdown` for fixtures.

### Tests to delete (matching `^func Test(Preview|Markdown|SourceMap|LinkMap|Stitch|Incremental)`)
- 122 test functions across:
  - `wind_test.go` (bulk)
  - `wind_async_image_test.go` (`TestPreviewAsyncImage*`, ~7 tests)
  - `wind_selection_test.go` (`TestPreviewSnarf*`, `TestPreviewLookText*`, ~12 tests)
  - `wind_incremental_test.go` (`TestIncrementalPreview*`, ~10 tests)
  - `richtext_test.go` (~3 tests including `TestPreviewMouseAfterResize`)
  - `wind/window_test.go` (preview-side)
- Plus `markdown/*_test.go` — gone with the package.

Also any test helpers (`makePlainTestWindow`, builders that set up preview
state) only used by deleted tests.

### Per-row scope (cross-references plan)
| Plan row | Files touched | Approx LOC removed |
|---|---|---|
| 1.2 Markdown builtin + previewcmd | `exec.go` (only) | ~270 (3 funcs + 1 builtin row) |
| 1.3 Preview tests | the test files above | ~7,650 |
| 1.4 Preview mouse path | `acme.go`, `wind.go` (HandlePreviewMouse + 4 helpers) | ~620 |
| 1.5 Preview Window state | `wind.go`, `wind/preview.go`, `text.go` | ~990 + 87 |
| 1.6 markdown/ package | `markdown/` | ~3,805 + ~11,511 |
| **Total** | | **~25,000 LOC** |

Matches the rough estimate from the earlier counting exercise (~5k impl +
~18k tests = ~23k) within tolerance; the extra ~2k is the
non-`markdown/`-package preview-side test code in `wind_test.go` etc.
