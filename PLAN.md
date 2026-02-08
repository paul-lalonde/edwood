# Spans File: External Styling via the 9P Filesystem

Expose a per-window `spans` file in the 9P filesystem that external tools write to
set style attributes (foreground color, background color, bold, italic) on ranges of
body text. Rendering uses the existing `rich.Frame` engine. Span positions adjust
automatically on buffer edits.

**Key design decisions:**
- SpanStore uses a gap buffer of `StyleRun` values for O(1) amortized edits at cursor
- Styled mode reuses the existing `RichText`/`rich.Frame` infrastructure (mutually exclusive with preview mode)
- Source map is identity (rendered position == source position) since no content transformation
- Region update protocol: writes replace all spans in the covered range
- One span set per window; last writer wins

**Base design doc**: `docs/designs/features/spans-file.md`

---

## Phase 1: SpanStore Data Structure

The SpanStore is the core data structure with no external dependencies, making it ideal
for isolated TDD. Everything in later phases depends on SpanStore being correct. This is
also the highest-risk component (gap buffer edge cases), so building and testing it first
follows a risk-first strategy.

### 1.1 SpanStore Core

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Distill SpanStore design: types (`StyleAttrs`, `StyleRun`), gap buffer operations (`Insert`, `Delete`, `RegionUpdate`, `ForEachRun`, `TotalLen`, `Clear`), edge cases, cost analysis | `docs/designs/features/spans-file.md` | Output: `docs/designs/features/spanstore.md`. Must include: struct definitions, all public method signatures, gap buffer mechanics, edge cases for split/merge runs, test case matrix. |
| [x] Tests | Write tests for SpanStore in `spanstore_test.go` | `docs/designs/features/spanstore.md` | Cover: (1) empty store, (2) single run insert/delete, (3) insert at run boundary vs mid-run, (4) delete spanning multiple runs, (5) region update replacing subset of runs, (6) region update at start/end of buffer, (7) ForEachRun iteration, (8) Clear, (9) zero-length spans, (10) TotalLen consistency after operations. Document assumptions as comments. |
| [x] Iterate | Red/green/review until tests pass | `docs/designs/features/spanstore.md` | New file: `spanstore.go`. Package `main`. Gap buffer with `[]StyleRun` + gap0/gap1 indices. Flag any operation worse than O(n) on number of runs. |
| [x] Commit | Commit SpanStore | — | Message: `Add SpanStore gap buffer for styled text runs` |

---

## Phase 2: 9P Spans File Registration and Write Handler

Depends on Phase 1. Registers the `spans` file in the 9P filesystem directory table and
implements the write handler that parses span definitions and stores them in SpanStore.
This phase does NOT wire up rendering — the store is updated but the display is unchanged.
This lets us test the protocol parsing and validation in isolation.

### 2.1 9P File Registration and Span Protocol Parser

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Distill 9P spans protocol: file properties (`QWspans`, permissions `0200`), write format (offset/length/colors/flags), region update semantics, `clear` command, validation rules, error cases, lock type `'E'` | `docs/designs/features/spans-file.md` | Output: `docs/designs/features/spans-protocol.md`. Must include: `QWspans` constant placement (before `QMAX`), `dirtabw` entry, parse function signature (`parseSpanDefs(data []byte, bufLen int) ([]StyleRun, int, error)` or similar), xfidopen/close/write handler logic, lock type addition, xfidread default case consideration. |
| [x] Tests | Write tests for span definition parsing and xfid dispatch in `spanparse_test.go` and/or extend `xfid_test.go` | `docs/designs/features/spans-protocol.md` | Cover: (1) single span parse, (2) multi-span contiguous parse, (3) optional bg-color and flags, (4) `#rrggbb` and `-` color parsing, (5) `bold`/`italic`/`hidden` flag parsing, (6) `clear` command, (7) validation errors: overlapping spans, gaps, out-of-range offsets, bad format, (8) write to window with no body text is no-op. |
| [x] Iterate | Red/green/review until tests pass | `docs/designs/features/spans-protocol.md` | Modify: `dat.go` (add `QWspans` before `QMAX`), `fsys.go` (add `{"spans", plan9.QTFILE, QWspans, 0200}` to `dirtabw`), `xfid.go` (add open/close/write cases, add `QWspans` to `'E'` lock condition, implement `xfidspanswrite`). New helper: span parsing function. |
| [x] Commit | Commit 9P spans file | — | Message: `Register spans file in 9P filesystem with write handler` |

---

## Phase 3: Styled Rendering

Depends on Phases 1+2. Wires up auto-switch from plain text (`frame.Frame`) to styled
rendering (`rich.Frame` via `RichText`) when spans arrive. Implements
`buildStyledContent()` to produce `rich.Content` from body text + spans. This phase
modifies the span write path to trigger rendering after updating the store.

The existing preview mode pattern in `wind.go` is the template: `w.previewMode`/
`w.richBody`/`w.IsPreviewMode()`. Styled mode adds `w.styledMode` and reuses `w.richBody`
since the three states (Plain/Styled/Preview) are mutually exclusive.

### 3.1 Styled Rendering Mode and Content Building

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Distill styled rendering: window state model (Plain/Styled/Preview), `buildStyledContent()` signature and logic, `styleAttrsToRichStyle` mapping, auto-switch trigger, `RichText` initialization for styled mode (follow `previewcmd` pattern), identity source map, re-render after span write, interaction with preview mode (error on span write to preview window) | `docs/designs/features/spans-file.md` | Output: `docs/designs/features/spans-rendering.md`. Must include: new Window fields (`styledMode bool`, `spanStore *SpanStore`), `buildStyledContent` implementation, `initStyledMode`/`exitStyledMode` helpers, how `w.richBody` is shared between styled and preview, `styleAttrsToRichStyle` mapping (StyleAttrs.Fg/Bg/Bold/Italic to rich.Style fields), re-render flow on span update. Reference `richtext.go` for RichText API and `exec.go:previewcmd` for initialization pattern. |
| [x] Tests | Write tests for `buildStyledContent`, `styleAttrsToRichStyle`, mode switching in `wind_styled_test.go` | `docs/designs/features/spans-rendering.md` | Cover: (1) buildStyledContent with single run, (2) multiple runs with mixed styles, (3) default style (nil colors) maps to rich.DefaultStyle, (4) auto-switch from plain to styled on first span write, (5) span write to preview-mode window returns error, (6) clear reverts to plain mode, (7) styled mode flag tracking. Use existing `MockFrame` and `edwoodtest` patterns from `wind_test.go`. |
| [x] Iterate | Red/green/review until tests pass | `docs/designs/features/spans-rendering.md` | Modify: `wind.go` (add `styledMode`, `spanStore` fields, `buildStyledContent`, `styleAttrsToRichStyle`, `initStyledMode`, `exitStyledMode`, `IsStyledMode`). Update `xfid.go` span write handler to call rendering after store update. |
| [x] Commit | Commit styled rendering | — | Message: `Add styled rendering mode with rich.Frame for span-styled text` |

---

## Phase 4: Span Adjustment on Buffer Edits

Depends on Phases 1+3. Hooks `SpanStore.Insert`/`Delete` into the `Text.Inserted`/
`Text.Deleted` observer callbacks so styles track with text edits. When a window is in
styled mode, each edit adjusts spans and triggers a re-render, similar to how preview mode
records edits and calls `UpdatePreview`.

### 4.1 Edit Observer Hooks

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Distill span adjustment: hook points in `Text.Inserted`/`Text.Deleted`, styled mode early-return pattern (following preview mode's pattern), deferred re-render strategy (who calls re-render after the edit chain completes), interaction with undo/redo (undo replays as insert/delete so observers fire naturally) | `docs/designs/features/spans-file.md` | Output: `docs/designs/features/spans-edit-tracking.md`. Must include: exact code locations in `text.go` for hooks, the early-return pattern for styled mode (analogous to preview mode's `if t.w.IsPreviewMode()` block), `SpanStore.Insert(pos, length)` and `SpanStore.Delete(pos, length)` call sites, re-render trigger point (analogous to `t.w.UpdatePreview()`), what happens during cut/paste (multiple observer calls). |
| [x] Tests | Write tests for span tracking through edit sequences in `spanstore_test.go` (unit) and `text_styled_test.go` (integration) | `docs/designs/features/spans-edit-tracking.md` | Cover: (1) insert within a styled run extends it, (2) insert at run boundary extends preceding run, (3) delete within a run shrinks it, (4) delete spanning multiple runs removes middle runs and shrinks edges, (5) delete an entire run removes it, (6) sequential typing in styled mode, (7) cut operation (delete range), (8) paste operation (insert at point), (9) undo reverses span adjustments. |
| [x] Iterate | Red/green/review until tests pass | `docs/designs/features/spans-edit-tracking.md` | Modify: `text.go` (add styled mode check in `Inserted`/`Deleted`, call `SpanStore.Insert`/`Delete`, trigger re-render). Pattern: add block after the existing `if t.w.IsPreviewMode()` check. |
| [x] Commit | Commit span edit tracking | — | Message: `Hook span adjustment into text edit observers` |

---

## Phase 5: Plain Toggle Command

Depends on Phase 3. Adds `Plain` as both a ctl file keyword (for programmatic use by
tools) and a tag command (for user interaction via middle-click). This is straightforward
state toggling with minimal risk.

### 5.1 Plain Toggle

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Distill Plain command: ctl file keyword in `xfidctlwrite`, tag command in `globalexectab`, toggle behavior (styled ↔ plain), no-op when no spans, preserve spans when toggling off (render plain but keep store), re-render when toggling back on | `docs/designs/features/spans-file.md` | Output: `docs/designs/features/spans-plain-toggle.md`. Must include: `xfidctlwrite` case block, `globalexectab` entry, `plaincmd` function signature, toggle logic referencing `w.IsStyledMode()` and `w.spanStore`. |
| [x] Tests | Write tests for Plain command in `xfid_test.go` and/or `exec_test.go` | `docs/designs/features/spans-plain-toggle.md` | Cover: (1) Plain toggles styled → plain, (2) Plain toggles plain → styled when spans exist, (3) Plain is no-op when no spans, (4) spans preserved after toggle to plain, (5) re-render correct after toggle back to styled, (6) Plain during preview mode is no-op (or error). |
| [x] Iterate | Red/green/review until tests pass | `docs/designs/features/spans-plain-toggle.md` | Modify: `xfid.go` (add `"Plain"` case to `xfidctlwrite`), `exec.go` (add `{"Plain", plaincmd, false, true, true}` to `globalexectab`), implement `plaincmd` function. |
| [x] Commit | Commit Plain toggle | — | Message: `Add Plain command for toggling styled/plain text rendering` |

---

## Open Questions

1. **Undo/redo and span fidelity**: When text is undone, the `Inserted`/`Deleted` observers
   fire naturally (undo replays as inserts/deletes), so spans adjust via the same mechanism.
   However, this means undo restores text but spans may not perfectly match their original
   pre-edit state (e.g., if an insert split a run, undo deletes the inserted text but the
   run split/merge may not produce identical run boundaries). The external tool would need
   to re-tokenize after undo. **Is this acceptable, or should span state itself be part of
   the undo history?**

2. **Re-render strategy in styled mode**: Preview mode uses a deferred pattern — `Inserted`/
   `Deleted` record edits, and the caller explicitly calls `UpdatePreview()` after the
   operation completes. Should styled mode follow the same deferred pattern (with a
   `UpdateStyledView()` method), or can it re-render inline since styled content building
   is simpler than markdown parsing? The deferred pattern is safer for multi-character
   operations (paste, Edit commands). **Recommend: follow the deferred pattern.**

3. **Plain as tag command vs ctl-only**: The design doc says to modify `exec.go`, suggesting
   `Plain` should be a middle-clickable tag command (like `Markdeep`). Should it also be a
   ctl file keyword for programmatic use? **Recommend: both, for consistency with the
   tool-driven philosophy. Plan includes both.**

4. **xfidread for spans**: The file is `0200` (write-only), so the 9P permission check should
   reject reads before they reach `xfidread`. However, `xfidread` has a `default` case that
   returns `"unknown qid"`. Should we add an explicit `case QWspans:` that returns a
   permission error, or rely on the 9P layer? **Recommend: add explicit case for clarity.**
