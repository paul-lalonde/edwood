# Span Adjustment on Buffer Edits

This document describes how span positions in the `SpanStore` are adjusted
when the body text is modified (typing, cut/paste, Edit commands, 9P writes).
It covers the exact hook points in `text.go`, the deferred re-render strategy,
and the interaction with undo/redo.

**Package**: `main` (modifications to `text.go`)

---

## Overview

When a window is in styled mode, every insert or delete to the body buffer
must adjust the span store so that styles remain aligned with the text they
describe. The `SpanStore.Insert` and `SpanStore.Delete` methods (already
implemented in Phase 1) handle the run-level adjustments. This phase wires
those methods into the `Text.Inserted` and `Text.Deleted` observer callbacks.

---

## Hook Points in text.go

### Text.Inserted (text.go:430)

The `Inserted` callback fires for every observer of a `File` after an
insertion. The existing code already has a styled-mode analog to follow:
the preview mode early-return block at lines 452-456.

**Current preview mode pattern:**

```go
// In preview mode, don't update the text frame directly.
// Record the edit for incremental update; the caller is responsible
// for calling UpdatePreview() when the editing operation is complete.
if t.what == Body && t.w != nil && t.w.IsPreviewMode() {
    t.logInsert(oq0, b, nr)
    t.w.recordEdit(markdown.EditRecord{Pos: q0, OldLen: 0, NewLen: nr})
    return
}
```

**New styled mode block** — add immediately after the preview mode block
(before the `if q0 < t.org` line at ~line 458):

```go
// In styled mode, adjust span positions for the insertion.
// The text frame is updated normally (unlike preview mode), so we
// do NOT return early — we fall through to the existing frame update code.
// Re-render is deferred: the caller (e.g., Text.Type, paste, Edit cmd)
// triggers re-render after the complete operation finishes.
if t.what == Body && t.w != nil && t.w.IsStyledMode() && t.w.spanStore != nil {
    t.w.spanStore.Insert(q0, nr)
}
```

**Key difference from preview mode:** Styled mode does NOT early-return.
The plain `frame.Frame` is still used for cursor positioning and scrollbar
updates. The `rich.Frame` overlay re-renders separately. So the frame
update code (lines 458-472) executes normally after the span adjustment.

### Text.Deleted (text.go:584)

Same pattern. The preview mode block is at lines 602-609.

**Current preview mode pattern:**

```go
// In preview mode, don't update the text frame directly.
// Record the edit for incremental update; the caller is responsible
// for calling UpdatePreview() when the editing operation is complete.
if t.what == Body && t.w != nil && t.w.IsPreviewMode() {
    t.logInsertDelete(q0, q1)
    t.w.recordEdit(markdown.EditRecord{Pos: q0, OldLen: q1 - q0, NewLen: 0})
    return
}
```

**New styled mode block** — add immediately after the preview mode block
(before the `if q1 <= t.org` line at ~line 611):

```go
// In styled mode, adjust span positions for the deletion.
// Falls through to normal frame update (no early return).
if t.what == Body && t.w != nil && t.w.IsStyledMode() && t.w.spanStore != nil {
    t.w.spanStore.Delete(q0, q1-q0)
}
```

**Note on Delete signature:** `SpanStore.Delete(pos, length int)` takes
a position and a length, while the `Deleted` callback receives `(oq0, oq1)`
as start/end offsets. The length is `q1 - q0`.

---

## Re-render Strategy (Deferred)

Following PLAN.md's recommendation (Open Question #2), styled mode uses
a **deferred re-render** pattern. The span adjustment in `Inserted`/`Deleted`
only updates the `SpanStore`; it does NOT trigger a re-render immediately.

### Why Deferred?

Multi-character operations (paste, Edit commands, cut) generate multiple
`Inserted`/`Deleted` callbacks. Re-rendering after each individual callback
would be wasteful and could cause visual flicker. Instead, the re-render
happens once after the entire operation completes.

### Who Calls Re-render?

The re-render is called from the same places that currently call
`UpdatePreview()` for preview mode, plus the span write handler:

1. **xfidspanswrite** (xfid.go:626-633) — already calls
   `buildStyledContent` + `richBody.SetContent` + `richBody.Render` after
   a span region update. No change needed here.

2. **After typing** — `Text.Type` calls `file.InsertAt` which triggers
   `Inserted`. The span store is adjusted. After the character is inserted,
   the frame update code runs and `ScrDraw` is called. For styled mode,
   the `rich.Frame` needs a re-render at this point.

3. **After cut/paste** — `cut`/`paste` functions call `Delete`/`Insert`
   on the `Text`, which go through the observers. The span store adjusts.
   After the operation completes, `ScrDraw` is called.

4. **After Edit commands** — Edit commands use `Text.Insert`/`Text.Delete`
   which trigger observers. Multiple edits may occur. After the full Edit
   command completes, the display is refreshed.

### UpdateStyledView Method

Add a new method to `Window` that rebuilds and re-renders the styled content:

```go
// UpdateStyledView rebuilds and re-renders the styled content.
// Called after editing operations that modify the body buffer while
// in styled mode.
func (w *Window) UpdateStyledView() {
    if !w.styledMode || w.richBody == nil || w.spanStore == nil {
        return
    }
    content := w.buildStyledContent()
    w.richBody.SetContent(content)
    w.richBody.Render(w.body.all)
    if w.display != nil {
        w.display.Flush()
    }
}
```

### Where to Call UpdateStyledView

Unlike preview mode, styled mode does not need to record edits for
incremental re-parsing. The `SpanStore` adjustments happen inline in the
observers. The re-render just needs to happen after the observers finish.

The trigger points are:

1. **Text.Inserted / Text.Deleted** — at the END of each callback (after
   `ScrDraw`), call `w.UpdateStyledView()`. This is safe because:
   - Each observer callback processes a complete insert/delete
   - The frame update has already happened
   - For multi-character operations, yes we re-render per-callback, but
     this is the same granularity as preview mode's `recordEdit` calls

   However, this means multiple re-renders for paste operations. To match
   preview mode's deferred pattern more closely, we could instead have the
   callers invoke `UpdateStyledView()` explicitly. But this would require
   modifying many call sites (`paste`, `cut`, Edit commands, etc.).

   **Decision: Call UpdateStyledView at the end of Inserted/Deleted.**
   This is the simplest approach and matches the granularity of existing
   frame updates. If profiling shows this is a bottleneck for large paste
   operations, the deferred pattern can be adopted later.

   Actually, on reflection, the re-render at the end of each observer
   callback is fine because:
   - The `frame.Frame` already does per-callback updates (InsertByte, Delete, fill, ScrDraw)
   - The `rich.Frame` re-render is comparably cheap (no markdown parsing)
   - The identity source map means no complex position remapping

---

## Exact Code Changes in text.go

### In Inserted (after preview mode block, before `if q0 < t.org`):

```go
// In styled mode, adjust span positions for the insertion and re-render.
if t.what == Body && t.w != nil && t.w.IsStyledMode() && t.w.spanStore != nil {
    t.w.spanStore.Insert(q0, nr)
}
```

And at the END of `Inserted` (after the `ScrDraw` call), add:

```go
if t.what == Body && t.w != nil && t.w.IsStyledMode() {
    t.w.UpdateStyledView()
}
```

### In Deleted (after preview mode block, before `if q1 <= t.org`):

```go
// In styled mode, adjust span positions for the deletion.
if t.what == Body && t.w != nil && t.w.IsStyledMode() && t.w.spanStore != nil {
    t.w.spanStore.Delete(q0, q1-q0)
}
```

And at the END of `Deleted` (after the `ScrDraw` call), add:

```go
if t.what == Body && t.w != nil && t.w.IsStyledMode() {
    t.w.UpdateStyledView()
}
```

---

## Interaction with Undo/Redo

### How Undo Works

`Window.Undo` (wind.go:442-467) calls `body.file.Undo(isundo)`, which
replays insert/delete operations on the file buffer. These replayed
operations fire the `Inserted`/`Deleted` observer callbacks naturally.

This means span adjustments happen automatically through the same observer
hooks — no special undo handling is needed. The span store will be adjusted
as if the user were typing the inverse operations.

### Fidelity Limitation

As noted in PLAN.md Open Question #1: undo restores text but spans may not
perfectly match their original pre-edit state. For example:
- User has spans [0,5 red] [5,5 blue]
- User inserts "x" at position 5 → spans become [0,6 red] [6,5 blue]
- User undoes → delete at position 5 → spans become [0,5 red] [5,5 blue]

In this simple case, the round-trip is exact. But edge cases involving
run splits and merges may not perfectly reconstruct the original run
boundaries. The external tool would need to re-tokenize after undo.

**No special code needed for undo** — the observers fire naturally.

### Special Case: Window.Undo UpdateStyledView

`Window.Undo` (wind.go:463-466) currently has:

```go
if w.IsPreviewMode() {
    w.UpdatePreview()
}
```

Add the styled mode analog:

```go
if w.IsStyledMode() {
    w.UpdateStyledView()
}
```

This ensures a final re-render after the undo operation completes, which
covers the case where the undo replays multiple operations.

---

## What Happens During Cut/Paste

### Cut (delete range)

1. `cut` calls `t.Delete(q0, q1, true)`
2. `t.Delete` calls `t.file.DeleteAt(q0, q1)`
3. File notifies all observers → `t.Deleted(q0, q1)` fires
4. `Deleted` adjusts span store: `w.spanStore.Delete(q0, q1-q0)`
5. `Deleted` updates frame, calls `ScrDraw`
6. `Deleted` calls `w.UpdateStyledView()`

### Paste (insert at point)

1. `paste` calls `t.Insert(q0, runes, true)`
2. `t.Insert` calls `t.file.InsertAt(q0, runes)`
3. File notifies all observers → `t.Inserted(q0, bytes, nr)` fires
4. `Inserted` adjusts span store: `w.spanStore.Insert(q0, nr)`
5. `Inserted` updates frame, calls `ScrDraw`
6. `Inserted` calls `w.UpdateStyledView()`

### Sequential Typing

Each keystroke in `Text.Type` calls `t.file.InsertAt(t.q0, rp[:nr])`,
which triggers `Inserted`. The span for the typing position extends by 1
rune each time. `UpdateStyledView` re-renders after each character.

---

## SpanStore Re-sync in xfidspanswrite

The current `xfidspanswrite` has a re-sync check (lines 609-615):

```go
} else if w.spanStore.TotalLen() != bufLen {
    // Re-sync: the store length should match the buffer.
    // This can happen if the store was created but the buffer
    // changed without edit tracking (Phase 4 will fix this).
    w.spanStore.Clear()
    w.spanStore.Insert(0, bufLen)
}
```

After Phase 4, the `SpanStore.TotalLen()` should always match `bufLen`
because every edit adjusts the store. The re-sync is kept as a safety net
but should not trigger in normal operation.

---

## Interaction with SetSelect (Cursor Tracking During Edits)

After each `Inserted`/`Deleted` callback, the existing code calls
`t.SetSelect(t.q0, t.q1)`. In styled mode, `SetSelect` updates the
`rich.Frame` selection and re-renders:

```go
if t.what == Body && t.w != nil && t.w.IsStyledMode() {
    t.w.richBody.SetSelection(q0, q1)
    t.w.richBody.Render(t.w.body.all)
    // Flush
    return
}
```

This ensures the cursor tracks the insertion point correctly during
sequential typing. Note that `UpdateStyledView` (called at the end of
`Inserted`/`Deleted`) does NOT set the selection — it runs inside the
observer callback where `body.q0` may still hold the pre-edit position.
The correct cursor position is set by `SetSelect`, which is called by
`Text.Type` after the insertion is complete.

---

## Summary of File Changes (Phase 4)

| File | Change |
|------|--------|
| `text.go` | Add styled mode span adjustment in `Inserted` (after preview block); add styled mode span adjustment in `Deleted` (after preview block); add `UpdateStyledView` calls at end of both; add styled mode `SetSelect` path for cursor tracking |
| `wind.go` | Add `UpdateStyledView()` method; add styled mode check in `Undo` alongside preview mode check |

---

## Test Plan (for Tests stage)

### Unit Tests (spanstore_test.go)

Test `SpanStore.Insert` and `SpanStore.Delete` directly:

1. **Insert within a styled run extends it**: Store has [5, red] [5, blue].
   Insert(3, 2). Result: [7, red] [5, blue]. TotalLen = 12.

2. **Insert at run boundary extends preceding run**: Store has [5, red]
   [5, blue]. Insert(5, 3). Result: [8, red] [5, blue]. TotalLen = 13.

3. **Delete within a run shrinks it**: Store has [10, red]. Delete(3, 4).
   Result: [6, red]. TotalLen = 6.

4. **Delete spanning multiple runs removes middle runs and shrinks edges**:
   Store has [3, red] [4, blue] [3, green]. Delete(2, 6). Result:
   [2, red] [2, green] (or merged if styles match). TotalLen = 4.

5. **Delete an entire run removes it**: Store has [5, red] [5, blue]
   [5, green]. Delete(5, 5). Result: [5, red] [5, green]. TotalLen = 10.

6. **Insert at position 0 extends first run**: Store has [5, red].
   Insert(0, 3). Result: [8, red]. TotalLen = 8.

7. **Insert at end extends last run**: Store has [5, red]. Insert(5, 3).
   Result: [8, red]. TotalLen = 8.

Note: Most of these are already tested in existing `spanstore_test.go`.
Verify coverage and add any missing cases.

### Integration Tests (text_styled_test.go)

Test span tracking through the `Text.Inserted`/`Text.Deleted` observer
callbacks:

1. **Sequential typing in styled mode**: Window with spans [5, red]
   [5, blue]. Simulate typing 3 chars at position 3. Verify span store
   becomes [8, red] [5, blue]. TotalLen = 13.

2. **Cut operation (delete range)**: Window with spans [5, red] [5, blue].
   Delete range [2, 7). Verify span store adjusts correctly.

3. **Paste operation (insert at point)**: Window with spans. Insert
   runes at a position. Verify spans extend.

4. **Undo reverses span adjustments**: Apply an insert, then undo.
   Verify span store returns to original state (approximately — run
   boundaries may differ but TotalLen matches).

5. **Multiple observer windows (clone)**: Two Text views on the same file.
   Insert in one view. Verify span store is adjusted once (only the body
   Text with `w.IsStyledMode()` does the adjustment, not clone texts
   that may not have styled mode).

6. **Cursor position after typing**: Window in styled mode with spans.
   Simulate typing a character at position 10. After the insert, call
   `SetSelect(11, 11)`. Verify `richBody.Selection()` returns `(11, 11)`.
   This ensures the cursor tracks correctly after edits (not stuck at
   the pre-edit position).

7. **UpdateStyledView does not set selection**: Verify that
   `UpdateStyledView()` does NOT call `richBody.SetSelection`. The
   selection should only be updated by `SetSelect` (called after the
   observer callback completes with the correct post-edit cursor position).
