# Styled Rendering Design

This document describes how span-styled text is rendered using the existing
`rich.Frame` engine. It covers the window state model, `buildStyledContent()`
implementation, `styleAttrsToRichStyle` mapping, mode initialization/exit,
and the re-render flow after span writes.

**Package**: `main` (in `wind.go`, with modifications to `xfid.go`)

---

## Window State Model

A window is in one of three mutually exclusive rendering states:

| State | `previewMode` | `styledMode` | `richBody` | Rendering |
|-------|---------------|--------------|------------|-----------|
| Plain | `false` | `false` | `nil` (or stale) | `frame.Frame` via `w.body.fr` |
| Styled | `false` | `true` | initialized | `rich.Frame` via `w.richBody` |
| Preview | `true` | `false` | initialized | `rich.Frame` via `w.richBody` |

**Invariant**: `previewMode` and `styledMode` are never both `true`.

The `richBody` field is shared between styled and preview modes. When
switching between modes, the previous `richBody` is discarded and a new
one is initialized.

---

## New Window Fields

Add to `Window` struct in `wind.go`:

```go
styledMode bool // true when showing span-styled text via rich.Frame
```

The `spanStore *SpanStore` field already exists (added in Phase 2).

---

## Public Methods

```go
// IsStyledMode returns true if the window is in styled rendering mode.
func (w *Window) IsStyledMode() bool {
    return w.styledMode
}
```

---

## initStyledMode

Called when the first span write arrives on a plain-mode window. Follows the
`previewcmd` pattern in `exec.go:1233-1333`:

```go
func (w *Window) initStyledMode() {
    if w.styledMode || w.previewMode {
        return
    }

    display := w.display
    if display == nil {
        display = global.row.display
    }
    if display == nil {
        return
    }

    font := fontget(global.tagfont, display)
    boldFont := tryLoadFontVariant(display, global.tagfont, "bold")
    italicFont := tryLoadFontVariant(display, global.tagfont, "italic")
    boldItalicFont := tryLoadFontVariant(display, global.tagfont, "bolditalic")

    rt := NewRichText()

    rtOpts := []RichTextOption{
        WithRichTextBackground(global.textcolors[frame.ColBack]),
        WithRichTextColor(global.textcolors[frame.ColText]),
        WithRichTextSelectionColor(global.textcolors[frame.ColHigh]),
        WithScrollbarColors(
            global.textcolors[frame.ColBord],
            global.textcolors[frame.ColBack]),
    }
    if boldFont != nil {
        rtOpts = append(rtOpts, WithRichTextBoldFont(boldFont))
    }
    if italicFont != nil {
        rtOpts = append(rtOpts, WithRichTextItalicFont(italicFont))
    }
    if boldItalicFont != nil {
        rtOpts = append(rtOpts, WithRichTextBoldItalicFont(boldItalicFont))
    }

    rt.Init(display, font, rtOpts...)

    w.richBody = rt
    w.styledMode = true
}
```

**Key differences from `previewcmd`**:
- No markdown parsing, source map, or link map
- No image cache or base path
- No scaled fonts (headings) or code font — styled mode uses Fg/Bg/Bold/Italic only
- No file name check (any window can receive spans)
- Uses `global.textcolors[frame.ColBack]` and `global.textcolors[frame.ColText]` for
  background/foreground so the styled window matches the default acme color scheme
  (yellowish background) rather than hardcoded white/black

---

## exitStyledMode

Called when spans are cleared or when toggling to plain mode:

```go
func (w *Window) exitStyledMode() {
    if !w.styledMode {
        return
    }

    // Sync scroll position from rich frame back to the plain body
    // so the plain frame shows the same region of text.
    if w.richBody != nil {
        w.body.org = w.richBody.Origin()
    }

    w.styledMode = false
    w.richBody = nil

    // Force a full redraw of the body in plain mode.
    if w.display != nil {
        w.body.Resize(w.body.all, true, false)
        w.body.ScrDraw(w.body.fr.GetFrameFillStatus().Nchars)
        w.display.Flush()
    }
}
```

This follows `SetPreviewMode(false)` at `wind.go:892-899`. The scroll position
sync ensures that toggling back to plain mode shows the same region of text
the user was viewing in styled mode.

---

## buildStyledContent

Builds `rich.Content` from the body text and span store:

```go
func (w *Window) buildStyledContent() rich.Content {
    if w.spanStore == nil || w.spanStore.TotalLen() == 0 {
        // Fallback: return body text with default style.
        return rich.Plain(w.body.file.String())
    }

    var content []rich.Span
    offset := 0
    w.spanStore.ForEachRun(func(run StyleRun) {
        if run.Len == 0 {
            return
        }
        // Read the rune slice from the body buffer.
        buf := make([]rune, run.Len)
        w.body.file.Read(offset, buf)

        span := rich.Span{
            Text:  string(buf),
            Style: styleAttrsToRichStyle(run.Style),
        }
        content = append(content, span)
        offset += run.Len
    })
    return rich.Content(content)
}
```

**Reading body text**: Uses `w.body.file.Read(offset, buf)` which reads
`len(buf)` runes starting at rune offset `offset` into the provided slice.
This is the same API used throughout the codebase (e.g., `wind.go:125`).

---

## styleAttrsToRichStyle

Maps `StyleAttrs` (from span protocol) to `rich.Style` (for rendering):

```go
func styleAttrsToRichStyle(sa StyleAttrs) rich.Style {
    s := rich.Style{
        Scale: 1.0, // Always normal scale for span-styled text
    }
    s.Fg = sa.Fg // nil means default (rich.Frame handles nil)
    s.Bg = sa.Bg // nil means default
    s.Bold = sa.Bold
    s.Italic = sa.Italic
    // sa.Hidden is reserved for future use; not mapped to rich.Style yet.
    return s
}
```

**Mapping**:

| StyleAttrs field | rich.Style field | Notes |
|------------------|------------------|-------|
| `Fg` (color.Color) | `Fg` (color.Color) | Direct assignment; nil = default |
| `Bg` (color.Color) | `Bg` (color.Color) | Direct assignment; nil = default |
| `Bold` (bool) | `Bold` (bool) | Direct assignment |
| `Italic` (bool) | `Italic` (bool) | Direct assignment |
| `Hidden` (bool) | — | Not mapped; reserved for future |

Fields NOT set in styled mode (they are preview/markdown features):
`Code`, `Link`, `Block`, `HRule`, `ParaBreak`, `ListItem`, `ListBullet`,
`ListIndent`, `ListOrdered`, `ListNumber`, `Table`, `TableHeader`,
`TableAlign`, `Blockquote`, `BlockquoteDepth`, `Image`, `ImageURL`,
`ImageAlt`, `ImageWidth`, `Scale` (always 1.0).

---

## Re-render Flow on Span Update

When a span write arrives in `xfidspanswrite`, after updating the store:

```
xfidspanswrite(x, w)
    ├── parseSpanDefs(data, bufLen)
    ├── w.spanStore.RegionUpdate(regionStart, runs)
    ├── if !w.styledMode && !w.previewMode:
    │       w.initStyledMode()       // auto-switch to styled mode
    ├── content = w.buildStyledContent()
    ├── w.richBody.SetContent(content)
    ├── w.richBody.Render(w.body.all)
    └── display.Flush()
```

The `xfidspanswrite` function is modified to replace the Phase 3 placeholder
comments with the actual rendering calls.

For the `clear` command path:

```
xfidspanswrite(x, w) [clear]
    ├── w.spanStore.Clear()
    └── w.exitStyledMode()    // switch back to plain mode
```

---

## Source Map (Identity)

Since styled mode renders the same text as the body buffer (no content
transformation), the source map is trivially an identity mapping: rendered
rune position N == source rune position N. No `markdown.SourceMap` is needed.

This means:
- No `previewSourceMap` is set in styled mode
- Selection positions in `richBody` directly correspond to `body.q0`/`body.q1`
- Cursor remapping is a no-op
- `syncSourceSelection()` is not needed (it checks `previewMode`)

---

## Interaction with SetSelect (Cursor Tracking)

In styled mode, `Text.SetSelect` must update the `rich.Frame` selection
instead of the plain frame's `DrawSel`. The implementation uses
`richBody.Render(body.all)` rather than `richBody.Frame().Redraw()`:

```go
if t.what == Body && t.w != nil && t.w.IsStyledMode() {
    if t.w.richBody != nil {
        t.w.richBody.SetSelection(q0, q1)
        t.w.richBody.Render(t.w.body.all)
        if t.w.display != nil {
            t.w.display.Flush()
        }
    }
    return
}
```

**Why `Render(body.all)` instead of `Frame().Redraw()`**: The rich frame's
internal rectangle (`f.rect`) is set by the previous `Render` call. During a
`Window.Resize`, the body rectangle has been updated but the rich frame's
internal rect is stale. `Frame().Redraw()` would draw at the OLD window
position and then `Flush()`, causing an afterimage. `Render(body.all)` uses
the current body rectangle, avoiding this.

Preview mode avoids this issue because `SetSelect` has no special case for
preview — it falls through to the plain frame's `DrawSel` which is invisible
behind the rich frame.

---

## Interaction with Resize

The `Window.Resize` method at `wind.go:353` passes `w.previewMode` as the
`noredraw` argument to `body.Resize`. Styled mode needs the same treatment:

```go
w.body.Resize(r1, keepextra, w.previewMode || w.styledMode)
```

The rendering block at `wind.go:359-363` needs to account for styled mode:

```go
if (w.previewMode || w.styledMode) && w.richBody != nil {
    w.richBody.Render(w.body.all)
} else {
    w.body.ScrDraw(w.body.fr.GetFrameFillStatus().Nchars)
}
```

Note that `body.Resize` → `Text.Redraw` → `SetSelect` fires during Resize.
The styled-mode `SetSelect` path (above) uses `Render(body.all)` which
correctly renders at the new window position. This is critical for avoiding
afterimages when windows are moved or resized.

---

## Interaction with Window.Draw

The `Window.Draw` method at `wind.go:914-929` currently checks
`previewMode`. Styled mode uses the same rendering path:

```go
func (w *Window) Draw() {
    if (w.previewMode || w.styledMode) && w.richBody != nil {
        w.richBody.Render(w.body.all)
    } else {
        // existing plain rendering
    }
}
```

---

## Interaction with Preview Mode

Writing spans to a preview-mode window returns an error (already implemented
in Phase 2's `xfidspanswrite`):

```go
if w.IsPreviewMode() {
    x.respond(&fc, fmt.Errorf("cannot write spans to preview mode window"))
    return
}
```

If `Markdeep` is invoked on a styled-mode window, it should exit styled
mode first (since preview takes over). This is handled naturally because
`previewcmd` sets `w.previewMode = true` and creates a new `w.richBody`,
which replaces the styled mode's `richBody`. However, to be explicit,
`previewcmd` should set `w.styledMode = false` when entering preview mode.
The span store is preserved so that exiting preview mode can restore
styled rendering if desired.

---

## Interaction with Window.Close

`Window.Close` at `wind.go:417-429` clears `previewMode` and `richBody`.
Add `styledMode` cleanup:

```go
func (w *Window) Close() {
    if w.ref.Dec() == 0 {
        w.previewMode = false
        w.styledMode = false     // new
        w.richBody = nil
        // ... rest unchanged
    }
}
```

---

## Summary of File Changes (Phase 3)

| File | Change |
|------|--------|
| `wind.go` | Add `styledMode bool` field; add `IsStyledMode()`, `initStyledMode()`, `exitStyledMode()`, `buildStyledContent()`, `styleAttrsToRichStyle()` methods/functions; update `Resize()`, `Draw()`, `Close()` to handle styled mode |
| `xfid.go` | Update `xfidspanswrite`: after region update, call `initStyledMode` + `buildStyledContent` + render; on `clear`, call `exitStyledMode` |

---

## Test Plan (for Tests stage)

Tests go in `wind_styled_test.go`:

1. **buildStyledContent with single run**: Set up a SpanStore with one run
   (red foreground) covering body text "hello". Call `buildStyledContent()`.
   Verify one `rich.Span` with text "hello" and Fg=red.

2. **buildStyledContent with multiple runs**: Body text "hello world". Two
   runs: [5, red] [6, blue]. Verify two spans with correct text splits and
   colors.

3. **styleAttrsToRichStyle default**: `StyleAttrs{}` (zero value) maps to
   `rich.Style{Scale: 1.0}` (which is `rich.DefaultStyle()`).

4. **styleAttrsToRichStyle with colors and flags**: `StyleAttrs{Fg: red,
   Bg: green, Bold: true, Italic: true}` maps to `rich.Style{Fg: red,
   Bg: green, Bold: true, Italic: true, Scale: 1.0}`.

5. **Auto-switch from plain to styled on first span write**: Window starts
   in plain mode. Write spans. Verify `w.styledMode == true` and
   `w.richBody != nil`.

6. **Span write to preview-mode window returns error**: Window in preview
   mode. Write spans. Verify error returned.

7. **clear reverts to plain mode**: Window in styled mode. Send `clear`.
   Verify `w.styledMode == false` and rendering reverts to plain frame.

8. **styledMode flag tracking**: Verify `IsStyledMode()` returns correct
   value after init/exit.

9. **initStyledMode uses global colors**: Verify `initStyledMode` passes
   `global.textcolors[frame.ColBack]` and `global.textcolors[frame.ColText]`
   to `WithRichTextBackground` and `WithRichTextColor` respectively (not
   hardcoded white/black).

10. **SetSelect in styled mode uses Render not Frame().Redraw()**: After
    `body.Resize` with a new rectangle, call `SetSelect(q0, q1)`. Verify
    the rich frame renders at the NEW body rectangle, not the old one.
    This prevents afterimage artifacts during window moves.

11. **exitStyledMode syncs scroll origin**: Enter styled mode, scroll to
    a non-zero origin. Call `exitStyledMode()`. Verify `w.body.org` matches
    the rich frame's origin so the plain frame shows the same text region.

Use existing test patterns from `wind_test.go` and `edwoodtest` package.
Since `initStyledMode` depends on display/font infrastructure, the unit
tests for `buildStyledContent` and `styleAttrsToRichStyle` should test
those functions in isolation (they don't require display). Integration tests
for mode switching may use headless windows.
