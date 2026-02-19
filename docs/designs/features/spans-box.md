# Box Elements in the Spans Protocol

This document describes box elements: inline replaced elements in the spans
protocol that reserve a fixed pixel region, cover buffer runes, and carry an
opaque payload for the layout engine.

**Package**: `main`, `rich`

---

## Problem

The spans protocol originally supported only styled text runs (`s` lines).
There was no way to embed non-text visual elements (images, charts, widgets)
via the 9P spans interface. Tools like edcolor could color text but couldn't
insert visual content.

---

## Design

A **box** is an inline replaced element defined by the `b` prefix in the
spans protocol:

```
b offset length width height [fg-color] [bg-color] [flags...] [payload...]
```

### Key properties

- **Rune coverage**: A box covers `length` runes in the body buffer. These
  runes exist in the buffer (for addressing, selection, search) but are not
  rendered as text. Instead, the box's pixel region is displayed.

- **Pixel dimensions**: `width` and `height` define the box size in pixels.
  The layout engine respects these dimensions directly.

- **Payload**: An opaque string passed to the rendering pipeline. Currently
  recognized prefix: `image:` — the rest is treated as an image path/URL.

- **Styling**: Boxes support the same fg/bg colors and bold/italic/hidden
  flags as text spans.

### Box invalidation

Edits that touch a box's rune region **invalidate** the box: the box run is
converted back to a default text run (no styling, no box). The underlying
buffer runes become visible as plain text.

This is implemented in `SpanStore.Insert()` and `SpanStore.Delete()` via
`invalidateBoxIfNeeded()`. The rationale is that editing within a box's
region means the buffer content has changed, so the box's visual
representation is stale. The tool must re-send the box after observing the
edit event.

### Merge prevention

Because `StyleAttrs.Equal()` compares all box fields (`IsBox`, `BoxWidth`,
`BoxHeight`, `BoxPayload`), `mergeAdjacent()` will never merge a box with
a text run or with a dissimilar box. Each box remains a distinct run in the
span store.

---

## Rendering Pipeline

```
Box StyleAttrs (IsBox=true)
    --> boxStyleToRichStyle()     [wind.go]
        --> rich.Style{Image: true, ImageWidth, ImageHeight, ImageURL, ImageAlt}
            --> rich.Frame image rendering
                --> imageBoxDimensions() respects explicit ImageHeight
```

1. `buildStyledContent()` in `wind.go` checks each `StyleRun` for `IsBox`.
   Box runs use `boxStyleToRichStyle()` instead of `styleAttrsToRichStyle()`.

2. `boxStyleToRichStyle()` maps box fields to `rich.Style`:
   - Sets `Image = true`, `ImageWidth`, `ImageHeight`
   - Parses payload: `image:` prefix sets `ImageURL`
   - Sets `ImageAlt` to the underlying buffer text (accessibility/placeholder)

3. `imageBoxDimensions()` in `rich/layout.go` uses `Style.ImageHeight` when
   set, giving the spans protocol full control over box dimensions.

4. `initStyledMode()` creates an `ImageCache` so box-sourced images are
   loaded and cached.

---

## Payload Format

The payload is the rest of the line after consuming all recognized fields
(offset, length, width, height, colors, flags). Tokens are joined with
spaces, so payloads with spaces work naturally:

```
b 10 5 200 150 image:/path with spaces/img.png
```

Currently recognized payload prefixes:

| Prefix | Behavior |
|--------|----------|
| `image:` | Rest is treated as image path/URL, set as `ImageURL` |
| (other) | Stored in `BoxPayload` but not interpreted by the renderer |

Future payload prefixes can be added without protocol changes.

---

## Data Structures

### StyleAttrs (spanstore.go)

```go
type StyleAttrs struct {
    // ... existing fields ...
    IsBox      bool
    BoxWidth   int    // pixels
    BoxHeight  int    // pixels
    BoxPayload string // opaque payload
}
```

### rich.Style (rich/style.go)

```go
type Style struct {
    // ... existing fields ...
    ImageHeight int // Explicit height in pixels (0 = proportional)
}
```

---

## Examples

Image box covering 5 runes at offset 10:
```
b 10 5 200 150 image:/tmp/diagram.png
```

Mixed spans and boxes:
```
s 0 10 #0000cc
s 10 5 - bold
b 15 8 200 150 image:/tmp/diagram.png
s 23 12 #008000 italic
```

Box with styling:
```
b 0 3 100 50 #ff0000 #000000 bold image:/icon.png
```

---

## Test Coverage

- `spanstore_test.go`: `TestInsertInvalidatesBox`, `TestDeleteInvalidatesPartialBox`,
  `TestDeleteRemovesFullBox`, `TestMergeAdjacentNeverMergesBoxWithText`
- `spanparse_test.go`: `TestParseSpanMessage_Box*`, mixed content tests
- `wind_styled_test.go`: `TestBuildStyledContentBoxRun`,
  `TestBuildStyledContentMixedSpansAndBoxes`, `TestBoxStyleToRichStyle*`
