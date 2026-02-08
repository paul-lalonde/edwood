# Styled Mode Mouse Handling

This document describes how mouse events are routed through the `rich.Frame`
when a window is in styled rendering mode. Without this, scrolling and clicks
operate on the plain `frame.Frame` while the display shows `rich.Frame` content,
causing only the first screen of styled text to be visible.

**Package**: `main` (modifications to `wind.go` and `acme.go`)

---

## Problem

When styled mode is active, the window renders through `rich.Frame` via
`w.richBody`. However, mouse events (scrolling, clicks, selection) still go
through the plain `frame.Frame` path in `acme.go` and `text.go`. This means:

- Scroll wheel changes the plain frame's view offset but `rich.Frame` stays
  at its initial scroll position — only the first screen is visible.
- Scrollbar clicks operate on the wrong frame.
- Click-to-position and drag-to-select hit `frame.Frame` coordinates, not
  `rich.Frame` coordinates.

Preview mode solved this with `HandlePreviewMouse` (wind.go:939). Styled mode
needs the same treatment, simplified by the identity mapping: `rich.Frame`
rune positions map 1:1 to body buffer positions.

---

## Design

### Event Routing in acme.go

Add a styled-mode check alongside the preview-mode check at acme.go:442:

```go
// Handle styled mode: route body mouse events to HandleStyledMouse
if w != nil && t.what == Body && w.IsStyledMode() {
    w.Lock('M')
    w.HandleStyledMouse(&m, global.mousectl)
    w.Unlock()
    return
}
```

This intercepts all body mouse events when in styled mode, preventing them
from reaching the plain `frame.Frame` scroll/click handlers.

### HandleStyledMouse Method

Added to `wind.go` after the existing styled-mode methods. Modeled on
`HandlePreviewMouse` with these simplifications:

- **No source map**: Rune positions in `rich.Frame` directly correspond to
  body buffer positions (identity mapping). Where preview mode calls
  `w.previewSourceMap.ToRendered` / `.ToSource`, styled mode reads/writes
  `w.body.q0` / `w.body.q1` directly.
- **No link map**: B3 does not check for hyperlinks.
- **No image map**: B3 does not check for images.
- **No horizontal scroll**: Block-region horizontal scrolling is a markdown
  preview feature.

### Event Handling

| Event | Handling |
|-------|----------|
| Scroll wheel (buttons 4/5) | `rt.ScrollWheel(up)` + `w.Draw()` |
| Scrollbar click (B1/B2/B3) | Reuse `previewVScrollLatch(rt, mc, button, scrRect)` |
| B1 click/drag | Selection via `rt.Frame().Charofpt`; set `w.body.q0`/`w.body.q1` directly |
| B1 double-click | Word expansion via `rt.Frame().ExpandAtPos` |
| B1+B2 chord | Cut via `cut(&w.body, ...)` + `w.UpdateStyledView()` |
| B1+B3 chord | Paste via `paste(&w.body, ...)` + `w.UpdateStyledView()` |
| B1+B2+B3 chord | Snarf via `cut(&w.body, ..., false)` |
| B2 click/sweep | Execute: expand word, call `previewExecute(&w.body, text)` |
| B3 click/sweep | Look: expand word, `search(&w.body, text)`, scroll to match |

### Selection Sync

In styled mode, the `rich.Frame` positions ARE body positions:

```go
// Sync from rich.Frame to body:
p0, p1 := rt.Selection()
w.body.q0 = p0
w.body.q1 = p1

// Sync from body to rich.Frame:
rt.SetSelection(w.body.q0, w.body.q1)
```

### Chord Handling

After cut/paste chord operations modify the body buffer:

1. `w.UpdateStyledView()` rebuilds content from updated spans
2. `rt.SetSelection(w.body.q0, w.body.q1)` sets selection directly

This replaces preview mode's `w.UpdatePreview()` + source map remapping.

---

## Reused Functions

| Function | Location | Used for |
|----------|----------|----------|
| `previewVScrollLatch` | wind.go:1348 | Scrollbar click latching |
| `previewScrSleep` | wind.go:1334 | Debounce during scroll latch |
| `previewExecute` | exec.go:290 | B2 command dispatch |
| `search` | look.go:272 | B3 look/search |
| `cut` | exec.go:356 | B1+B2 chord cut |
| `paste` | exec.go:439 | B1+B3 chord paste |
| `clearmouse` | util.go:15 | Clear mouse state after chord |
| `w.Draw` | wind.go:919 | Re-render (already handles styled mode) |

---

## Summary of File Changes

| File | Change |
|------|--------|
| `wind.go` | Add `HandleStyledMouse` method |
| `acme.go` | Add styled-mode mouse event routing alongside preview-mode check |

---

## Reference Implementation: gocolor

The `cmd/gocolor` tool is a working example of an external tool that uses
the spans file. It syntax-colors Go source files by reading the body,
lexing with `go/scanner`, and writing span definitions to the spans file.

### gocolor Event Loop

Gocolor stays resident after initial coloring, watching for edit events:

1. `acme.Open(id, nil)` connects to the existing window for event reading.
2. `win.OpenEvent()` explicitly opens the event file (which sets
   `w.filemenu = false` in edwood).
3. `win.Ctl("menu")` re-enables filemenu so Undo/Redo/Put remain in the
   tag. **The `menu` ctl must be written AFTER the event file is opened**,
   not before — `EventChan()` lazily opens the event file, so writing
   `menu` before would be undone.
4. `client.MountService("acme")` provides a separate 9P connection for
   spans writing (needs manual chunking at line boundaries).
5. `eventLoop` uses `select` on `win.EventChan()` and a 300ms debounce
   timer. I/D events schedule re-coloring; x/X/l/L events are written
   back to acme for normal handling.
6. Exits when the event channel closes (window deleted).

---

## Test Plan

1. **Scroll wheel**: Open .go file, run gocolor, scroll wheel down/up.
   Styled text should scroll through the full file.

2. **Scrollbar clicks**: B1 in scrollbar scrolls down, B3 scrolls up,
   B2 jumps to proportional position.

3. **B1 click**: Click in styled text positions cursor. Verify body.q0/q1
   update to match click position.

4. **B1 drag**: Drag to select text. Selection highlight appears in
   rich.Frame.

5. **B1 double-click**: Double-click selects word.

6. **B1+B2 chord**: Select text, chord B2 to cut. Text is removed,
   styled rendering updates.

7. **B1+B3 chord**: Cut text, chord B3 to paste. Text is inserted,
   styled rendering updates.

8. **B2 execute**: Middle-click on a command word (e.g., "gocolor")
   in styled text. Command executes.

9. **B3 look**: Right-click on a word. Search finds next occurrence
   and scrolls to it.

10. **Window move no afterimage**: Move a styled window by dragging its
    tag scrollbar. The old position should be cleanly erased — no
    afterimage of styled content at the previous location. This verifies
    that `SetSelect` uses `Render(body.all)` (current rect) rather than
    `Frame().Redraw()` (stale rect).

11. **Undo/Redo in tag with gocolor running**: Run gocolor on a window.
    Verify "Undo" and "Redo" appear in the tag after edits. This verifies
    the `OpenEvent()` then `Ctl("menu")` ordering.
