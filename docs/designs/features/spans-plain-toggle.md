# Plain Toggle Command Design

This document describes the `Plain` command that toggles between styled and
plain text rendering for windows with span data. It is implemented as both a
ctl file keyword (for programmatic use by external tools) and a tag command
(for user interaction via middle-click).

**Package**: `main` (modifications to `xfid.go` and `exec.go`)

---

## Behavior

The `Plain` command toggles a window's rendering mode between styled and plain:

| Current State | Spans Exist? | Action |
|---------------|-------------|--------|
| Styled mode | yes | Switch to plain mode. Spans are preserved but not rendered. |
| Plain mode | yes | Switch to styled mode. Re-render with existing spans. |
| Plain mode | no | No-op. |
| Preview mode | — | No-op. (Preview mode is unrelated to span styling.) |

**Key invariant**: Toggling to plain mode preserves the span store. The spans
are still tracked and adjusted on edits (the `Text.Inserted`/`Text.Deleted`
hooks check `w.spanStore != nil`, not `w.styledMode`). Toggling back to
styled mode re-renders from the preserved spans.

---

## ctl File Keyword (`xfidctlwrite`)

Add a `"Plain"` case to the switch statement in `xfidctlwrite` at
`xfid.go:658`. This follows the pattern of simple state-toggle keywords like
`"clean"`, `"dirty"`, `"nomenu"`, `"menu"`.

```go
case "Plain":
    plaincmd(&w.body, nil, nil, false, false, "")
```

This delegates to the same `plaincmd` function used by the tag command,
keeping the toggle logic in one place. The function signature matches the
`Exectab.fn` type so it can be used in both contexts.

---

## Tag Command (`globalexectab`)

Add an entry to `globalexectab` in `exec.go` (alphabetically between
`"Paste"` and `"Put"`):

```go
{"Plain", plaincmd, false, true /*unused*/, true /*unused*/},
```

- `mark`: `false` — toggling rendering mode is not an undoable text edit.
- `flag1`, `flag2`: unused (following the `Markdeep` pattern).

---

## `plaincmd` Function

Implement in `exec.go`, near `previewcmd` since it follows a similar pattern:

```go
// plaincmd toggles between styled and plain text rendering for windows
// with span data. No-op if the window has no spans or is in preview mode.
func plaincmd(et *Text, _ *Text, _ *Text, _, _ bool, _ string) {
    if et == nil || et.w == nil {
        return
    }
    w := et.w

    // No-op in preview mode.
    if w.IsPreviewMode() {
        return
    }

    // No-op if no spans exist.
    if w.spanStore == nil || w.spanStore.TotalLen() == 0 {
        return
    }

    // Toggle: styled -> plain
    if w.IsStyledMode() {
        w.exitStyledMode()
        return
    }

    // Toggle: plain -> styled (spans exist, re-enter styled mode)
    w.initStyledMode()
    if w.styledMode && w.richBody != nil {
        content := w.buildStyledContent()
        w.richBody.SetContent(content)
        w.richBody.SetOrigin(w.body.org)
        w.richBody.SetSelection(w.body.q0, w.body.q1)
        w.richBody.Render(w.body.all)
        if w.display != nil {
            w.display.Flush()
        }
    }
}
```

### Toggle Logic Details

**Styled to plain** (`w.IsStyledMode()` is true):
- Calls `w.exitStyledMode()`, which sets `w.styledMode = false`,
  nils `w.richBody`, and redraws the body in plain mode.
- `w.spanStore` is NOT cleared — spans are preserved for toggling back.
- Span adjustment via `Text.Inserted`/`Text.Deleted` continues to work
  because those hooks check `w.spanStore != nil` (not `w.styledMode`).

**Plain to styled** (spans exist but `w.styledMode` is false):
- Calls `w.initStyledMode()`, which creates a new `RichText` renderer
  and sets `w.styledMode = true`.
- After initialization, builds styled content from the existing span store
  and renders it. This is the same flow as a span write arriving.
- Syncs `SetOrigin(w.body.org)` and `SetSelection(w.body.q0, w.body.q1)`
  before rendering so the styled view shows the same scroll position and
  cursor as the plain view. Without this, toggling to styled mode would
  reset the view to the top of the file.
- If `initStyledMode` fails (e.g., no display), the window stays in plain
  mode — the function checks `w.styledMode` before rendering.

---

## Span Adjustment in Plain Mode

When `Plain` toggles a styled window to plain mode, span adjustment must
continue. The hooks in `text.go` (`Text.Inserted` and `Text.Deleted`) call
`w.spanStore.Insert`/`Delete` when `w.spanStore != nil`. They call
`w.UpdateStyledView()` which checks `w.styledMode` and returns early if
false. This is the correct behavior: spans track edits, but no re-render
happens until styled mode is re-entered.

---

## Interaction with `clear`

If the span file receives a `clear` command while in plain mode (toggled off
via `Plain`), the span store is cleared and the window remains in plain mode.
A subsequent `Plain` command will be a no-op (no spans exist).

If `clear` arrives while in styled mode, `exitStyledMode()` is called as
before, switching to plain and clearing spans.

---

## Summary of File Changes

| File | Change |
|------|--------|
| `exec.go` | Add `{"Plain", plaincmd, false, true, true}` to `globalexectab`; implement `plaincmd` function |
| `xfid.go` | Add `case "Plain":` to `xfidctlwrite` switch, delegating to `plaincmd` |

---

## Test Plan (for Tests stage)

Tests go in `exec_test.go` or a new `plain_test.go`:

1. **Plain toggles styled to plain**: Window in styled mode with spans.
   Call `plaincmd`. Verify `w.IsStyledMode() == false` and
   `w.spanStore.TotalLen() > 0` (spans preserved).

2. **Plain toggles plain to styled when spans exist**: Window in plain mode
   with populated span store. Call `plaincmd`. Verify
   `w.IsStyledMode() == true`.

3. **Plain is no-op when no spans**: Window in plain mode with nil/empty
   span store. Call `plaincmd`. Verify `w.IsStyledMode() == false`.

4. **Spans preserved after toggle to plain**: Write spans, toggle to plain,
   verify span store still has data. Edit text, verify spans adjust.

5. **Re-render correct after toggle back to styled**: Write spans, toggle
   to plain, toggle back to styled. Verify styled content is built from
   preserved spans.

6. **Plain during preview mode is no-op**: Window in preview mode. Call
   `plaincmd`. Verify `w.IsPreviewMode() == true` (unchanged).

7. **Plain preserves scroll position (styled to plain)**: Window in styled
   mode with rich frame scrolled to non-zero origin. Toggle to plain.
   Verify `w.body.org` matches the rich frame's origin before toggle.

8. **Plain preserves scroll position (plain to styled)**: Window in plain
   mode with `w.body.org` at a non-zero position. Toggle to styled.
   Verify `richBody.Origin()` matches `w.body.org`.

9. **Plain preserves dot (plain to styled)**: Window in plain mode with
   `w.body.q0 = 50, w.body.q1 = 55`. Toggle to styled. Verify
   `richBody.Selection()` returns `(50, 55)`.
